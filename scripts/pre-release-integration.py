#!/usr/bin/env python3
"""Produce candidate-owned evidence for Core-only static integration contracts.

These checks do not qualify real browser journeys or private runtime consumers.
"""
import argparse
import importlib.util
import json
import os
import pathlib
import re
import subprocess


CHECKS = {
    'integration/static/public-contracts': [
        ['./contracts/...', './internal/machine', './internal/machinereadmodels']],
    'integration/static/configuration-policy': [
        ['./internal/orgconfig', './internal/policy'],
        ['./cmd/baha', '-run', '^(TestOrganization|TestHTTPEnvironmentIsAnExplicitOrganizationResolverInput)']],
    'integration/static/browser-terminal': [
        ['./internal/machinehttp'], ['./contracts', '-run', '^(TestGeneratedTerminal|TestTargetAccessWire)']],
}


def checked_tests(command, output):
    """Reject empty or skipped selections even when go test exits successfully."""
    with output.open('w') as log:
        completed = subprocess.run(command, stdout=log, stderr=subprocess.STDOUT,
                                   timeout=900, check=False)
    passed, skipped = [], []
    for line in output.read_text().splitlines():
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue  # Compiler diagnostics stay in the original retained log.
        if not isinstance(event, dict) or not event.get('Test'):
            continue
        if event.get('Action') == 'pass':
            passed.append(event['Package'] + '/' + event['Test'])
        elif event.get('Action') == 'skip':
            skipped.append(event['Package'] + '/' + event['Test'])
    if completed.returncode or not passed or skipped:
        raise ValueError(f'contract checks failed, empty or skipped; inspect {output.name}')
    return {'command': command, 'passed': passed, 'log': output.name}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--gate', required=True, choices=CHECKS)
    parser.add_argument('--candidate', required=True)
    parser.add_argument('--demo', required=True)
    parser.add_argument('--tag', required=True)
    parser.add_argument('--output', required=True, type=pathlib.Path)
    args = parser.parse_args()
    args.output.mkdir(parents=True, exist_ok=True)
    # A failed rerun must never leave an earlier successful receipt in place.
    manifest_path = args.output / 'manifest.json'
    manifest_path.unlink(missing_ok=True)
    if any(not re.fullmatch('[0-9a-f]{40}', sha) for sha in [args.candidate, args.demo]):
        raise ValueError('candidate and demo must be exact commit SHAs')
    actual = subprocess.check_output(['git', 'rev-parse', 'HEAD']).decode().strip()
    if actual != args.candidate:
        raise ValueError('checked-out Core differs from the evidence candidate')
    spec = importlib.util.spec_from_file_location('resume', pathlib.Path(__file__).with_name('pre-release-resume.py'))
    resume = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(resume)
    raw = subprocess.check_output(['git', 'show', args.candidate + ':' + resume.REQUIREMENTS_PATH])
    requirements = resume.load_requirements(raw, args.tag)
    requirement = next((gate for gate in requirements if gate['id'] == args.gate), None)
    if not requirement or requirement['dependencies'] != ['core']:
        raise ValueError('static checker cannot qualify consumer-dependent evidence')
    run, attempt = os.environ['GITHUB_RUN_ID'], os.environ['GITHUB_RUN_ATTEMPT']
    if not run.isdigit() or not attempt.isdigit() or min(int(run), int(attempt)) < 1:
        raise ValueError('positive GitHub origin run and attempt are required')
    checks = [checked_tests(['go', 'test', '-race', '-count=1', '-json', *selection],
                            args.output / f'check-{index}.jsonl')
              for index, selection in enumerate(CHECKS[args.gate])]
    _, runtime, gate = args.gate.split('/')
    manifest = {'schema': requirement['schema'], 'candidate_sha': args.candidate,
                'demo_ref': args.demo, 'workflow_run_id': run,
                'workflow_run_attempt': int(attempt), 'requirements_digest': resume.digest(requirements),
                'id': runtime + '/' + gate, 'runtime': runtime, 'gate': gate,
                'outcome': 'success', 'cleanup_outcome': 'skipped',
                'qualification_scope': 'core-source-contracts', 'checks': checks}
    resume.validate_manifest(manifest, args.gate, int(run), args.candidate, args.demo, int(attempt))
    manifest_path.write_text(json.dumps(manifest, indent=2) + '\n')


if __name__ == '__main__':
    try:
        main()
    except (ValueError, KeyError, OSError, subprocess.SubprocessError) as error:
        raise SystemExit(f'integration contract evidence rejected: {error}')

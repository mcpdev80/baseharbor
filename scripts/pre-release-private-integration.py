#!/usr/bin/env python3
"""Authenticate full private consumer qualification and publish only commitments."""
import argparse
import importlib.util
import json
import os
import pathlib
import re
import subprocess

from private_consumer_evidence import QUALIFICATIONS, commitment, private_verifier_from_environment


def produce(output, requirement, requirements, candidate, demo, run, attempt, verifier, resume):
    output.mkdir(parents=True, exist_ok=True)
    manifest_path = output / 'manifest.json'
    failure_path = output / 'failure.json'
    # An unsuccessful rerun may never retain an earlier approval-shaped receipt.
    manifest_path.unlink(missing_ok=True)
    failure_path.unlink(missing_ok=True)
    try:
        gate = requirement['id']
        roles = set(requirement['dependencies']) & {'console', 'connector'}
        if gate not in QUALIFICATIONS or not roles or verifier is None:
            raise ValueError('authenticated private consumer qualification is required')
        pins = verifier.pins.get(gate)
        if not isinstance(pins, dict) or set(pins) != roles:
            raise ValueError('private consumer source roles differ')
        if any(not re.fullmatch('[0-9a-f]{40}', sha) for sha in [candidate, demo]):
            raise ValueError('exact Core and demo pins are required')
        if not run.isdigit() or not attempt.isdigit() or min(int(run), int(attempt)) < 1:
            raise ValueError('positive source-bound origin run and attempt are required')
        _, runtime, name = gate.split('/')
        manifest = {'schema': requirement['schema'], 'candidate_sha': candidate,
                    'demo_ref': demo, 'workflow_run_id': run,
                    'workflow_run_attempt': int(attempt),
                    'requirements_digest': resume.digest(requirements),
                    'id': runtime + '/' + name, 'runtime': runtime, 'gate': name,
                    'outcome': 'success',
                    'cleanup_outcome': 'skipped' if runtime == 'static' else 'success',
                    'qualification_scope': 'authenticated-private-consumer-integration',
                    'consumer_commitments': {role: commitment(pins[role]) for role in sorted(roles)}}
        # Every private origin, exact source, attempt, trusted qualification job,
        # archive digest, production journey and cleanup is re-authenticated.
        # Partial/source-only receipts cannot generate a successful public gate.
        verifier.verify(manifest, requirement, candidate, demo)
        resume.validate_manifest(manifest, gate, int(run), candidate, demo, int(attempt))
        manifest_path.write_text(json.dumps(manifest, indent=2) + '\n')
        return manifest
    except Exception:
        failure_path.write_text(json.dumps({'result': 'failure',
            'cause': 'private_consumer_qualification_not_authenticated'}) + '\n')
        raise


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--gate', required=True, choices=QUALIFICATIONS)
    parser.add_argument('--candidate', required=True)
    parser.add_argument('--demo', required=True)
    parser.add_argument('--tag', required=True)
    parser.add_argument('--output', required=True, type=pathlib.Path)
    args = parser.parse_args()
    args.output.mkdir(parents=True, exist_ok=True)
    (args.output / 'manifest.json').unlink(missing_ok=True)
    (args.output / 'failure.json').write_text(json.dumps({'result': 'failure',
        'cause': 'private_consumer_qualification_not_authenticated'}) + '\n')
    if any(not re.fullmatch('[0-9a-f]{40}', sha) for sha in [args.candidate, args.demo]):
        raise ValueError('exact source pins required')
    actual = subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip()
    if actual != args.candidate:
        raise ValueError('checked-out source differs')
    spec = importlib.util.spec_from_file_location('resume', pathlib.Path(__file__).with_name('pre-release-resume.py'))
    resume = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(resume)
    raw = subprocess.check_output(['git', 'show', args.candidate + ':' + resume.REQUIREMENTS_PATH])
    requirements = resume.load_requirements(raw, args.tag)
    requirement = next((row for row in requirements if row['id'] == args.gate), None)
    if requirement is None:
        raise ValueError('candidate does not require this gate')
    verifier = private_verifier_from_environment(resume.read_archive)
    produce(args.output, requirement, requirements, args.candidate, args.demo,
            os.environ['GITHUB_RUN_ID'], os.environ['GITHUB_RUN_ATTEMPT'], verifier, resume)


if __name__ == '__main__':
    try:
        main()
    except (ValueError, KeyError, OSError, subprocess.SubprocessError):
        # Configuration/API diagnostics must not leak private source identities.
        raise SystemExit('Private integration rejected: full authenticated consumer qualification is required.') from None

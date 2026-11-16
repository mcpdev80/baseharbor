#!/usr/bin/env python3
"""Run normal Core contract and rootless bootstrap checks.

These checks do not qualify real browser journeys or private runtime consumers.
"""
import argparse
import json
import os
import pathlib
import re
import subprocess


CHECKS = {
    'integration/docker/core-bootstrap': [
        ['./cmd/baha', '-run', '^TestCoreOnlyBootstrapRuntimeAcceptance$', '-timeout', '28m'],
        ['./internal/development', '-run', '^TestGeneratedTemplatesNativeBuild$', '-timeout', '20m']],
    'integration/podman/core-bootstrap': [
        ['./cmd/baha', '-run', '^TestCoreOnlyBootstrapRuntimeAcceptance$', '-timeout', '28m'],
        ['./internal/development', '-run', '^TestGeneratedTemplatesNativeBuild$', '-timeout', '20m']],
    'integration/docker/coreless-lifecycle': [
        ['./cmd/baha', '-run', '^TestCorelessFreshWorkloadRuntimeLifecycle$', '-timeout', '15m'],
        ['./cmd/baha', '-run', '^TestCorelessLocalProdRuntimeLifecycle$', '-timeout', '15m'],
        ['./cmd/baha', '-run', '^TestSelectiveIdentityRuntimeAddRemoveReAdd$', '-timeout', '20m'],
        ['./cmd/baha', '-run', '^TestSelectiveBackendRuntimeAddRemoveReAdd$', '-timeout', '20m'],
        ['./cmd/baha', '-run', '^TestSelectiveApplicationBackendRuntimeAddRemoveReAdd$', '-timeout', '20m'],
        ['./cmd/baha', '-run', '^TestSelectiveHTTPSRuntimeAddRemoveReAdd$', '-timeout', '15m'],
        ['./cmd/baha', '-run', '^TestSelectiveSecretsRuntimeAddRemoveReAdd$', '-timeout', '20m'],
        ['./cmd/baha', '-run', '^TestManagedRegisteredOTLPNativeConsumerHandoff$', '-timeout', '15m'],
        ['./internal/runtime', '-run', '^TestRenovateVolumeRecoveryRuntimeAcceptance$', '-timeout', '15m'],
        ['./cmd/baha', '-run', '^TestApplicationCapabilityLifecycleExistingCore$', '-timeout', '25m']],
    'integration/podman/coreless-lifecycle': [
        ['./cmd/baha', '-run', '^TestCorelessFreshWorkloadRuntimeLifecycle$', '-timeout', '15m'],
        ['./cmd/baha', '-run', '^TestCorelessLocalProdRuntimeLifecycle$', '-timeout', '15m'],
        ['./cmd/baha', '-run', '^TestSelectiveIdentityRuntimeAddRemoveReAdd$', '-timeout', '20m'],
        ['./cmd/baha', '-run', '^TestSelectiveBackendRuntimeAddRemoveReAdd$', '-timeout', '20m'],
        ['./cmd/baha', '-run', '^TestSelectiveApplicationBackendRuntimeAddRemoveReAdd$', '-timeout', '20m'],
        ['./cmd/baha', '-run', '^TestSelectiveHTTPSRuntimeAddRemoveReAdd$', '-timeout', '15m'],
        ['./cmd/baha', '-run', '^TestSelectiveSecretsRuntimeAddRemoveReAdd$', '-timeout', '20m'],
        ['./cmd/baha', '-run', '^TestManagedRegisteredOTLPNativeConsumerHandoff$', '-timeout', '15m'],
        ['./internal/runtime', '-run', '^TestRenovateVolumeRecoveryRuntimeAcceptance$', '-timeout', '15m'],
        ['./cmd/baha', '-run', '^TestApplicationCapabilityLifecycleExistingCore$', '-timeout', '25m']],
    'integration/static/public-contracts': [
        ['./contracts/...', './internal/machine', './internal/machinereadmodels'],
        ['./cmd/baha', '-run', '^TestExternalProviderRemoval'],
        ['./internal/application', '-run', '^TestExternalRemoval']],
    'integration/static/configuration-policy': [
        ['./internal/orgconfig', './internal/policy'],
        ['./cmd/baha', '-run', '^(TestOrganization|TestHTTPEnvironmentIsAnExplicitOrganizationResolverInput)']],
    'integration/static/browser-terminal': [
        ['./internal/machinehttp'], ['./contracts', '-run', '^(TestGeneratedTerminal|TestTargetAccessWire)']],
}


def checked_tests(command, output, timeout=900):
    """Reject empty or skipped selections even when go test exits successfully."""
    with output.open('w') as log:
        completed = subprocess.run(command, stdout=log, stderr=subprocess.STDOUT,
                                   timeout=timeout, check=False)
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
        print(output.read_text(), end='')
        raise ValueError(f'contract checks failed, empty or skipped; inspect {output.name}')
    return {'command': command, 'passed': passed, 'log': output.name}


def configure_runtime_environment(gate):
    if not gate.endswith(('/core-bootstrap', '/coreless-lifecycle')):
        return False
    expected = gate.split('/')[1]
    if os.environ.get('BASEHARBOR_TEST_RUNTIME') != expected:
        raise ValueError('selected runtime differs from the prepared qualification host')
    if gate.endswith('/coreless-lifecycle'):
        os.environ['BASEHARBOR_CORELESS_ACCEPTANCE'] = '1'
        os.environ['BASEHARBOR_IDENTITY_LIFECYCLE_ACCEPTANCE'] = '1'
        os.environ['BASEHARBOR_BACKEND_LIFECYCLE_ACCEPTANCE'] = '1'
        os.environ['BASEHARBOR_HTTPS_LIFECYCLE_ACCEPTANCE'] = '1'
        os.environ['BASEHARBOR_SECRETS_LIFECYCLE_ACCEPTANCE'] = '1'
        os.environ['BASEHARBOR_OTLP_HANDOFF_ACCEPTANCE'] = '1'
        os.environ['BASEHARBOR_RENOVATE_RECOVERY_ACCEPTANCE'] = '1'
        return True
    os.environ['BASEHARBOR_CORE_BOOTSTRAP_ACCEPTANCE'] = '1'
    os.environ['BASEHARBOR_BUG_HUNT_LIFECYCLE_ACCEPTANCE'] = '1'
    os.environ['BASEHARBOR_TEMPLATE_BUILD_ACCEPTANCE'] = '1'
    return True


def verify_candidate_source(candidate, root=None):
    """A commit label alone cannot qualify locally altered test inputs."""
    prefix = ['git'] if root is None else ['git', '-C', str(root)]
    actual = subprocess.check_output([*prefix, 'rev-parse', 'HEAD']).decode().strip()
    if actual != candidate:
        raise ValueError('checked-out Core differs from the candidate')
    changed = subprocess.run([*prefix, 'diff', '--quiet', candidate, '--'], check=False)
    untracked = subprocess.check_output([*prefix, 'ls-files', '--others', '--exclude-standard', '-z'])
    if changed.returncode or untracked:
        raise ValueError('candidate has changed or untracked source inputs')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--gate', required=True, choices=CHECKS)
    parser.add_argument('--candidate', required=True)
    parser.add_argument('--demo', required=True)
    parser.add_argument('--tag', required=True)
    parser.add_argument('--output', required=True, type=pathlib.Path)
    args = parser.parse_args()
    args.output.mkdir(parents=True, exist_ok=True)
    if any(not re.fullmatch('[0-9a-f]{40}', sha) for sha in [args.candidate, args.demo]):
        raise ValueError('candidate and demo must be exact commit SHAs')
    verify_candidate_source(args.candidate)
    runtime_bootstrap = configure_runtime_environment(args.gate)
    checks = [checked_tests(['go', 'test', '-race', '-count=1', '-json', *selection],
                            args.output / f'check-{index}.jsonl', 1800 if runtime_bootstrap else 900)
              for index, selection in enumerate(CHECKS[args.gate])]
    verify_candidate_source(args.candidate)
    for check in checks:
        print('Passed ' + str(len(check['passed'])) + ' checks: ' + ' '.join(check['command']))


if __name__ == '__main__':
    try:
        main()
    except (ValueError, KeyError, OSError, subprocess.SubprocessError) as error:
        raise SystemExit(f'integration checks failed: {error}')

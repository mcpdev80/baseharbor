#!/usr/bin/env python3
"""Run normal Core contract and rootless bootstrap checks.

These checks do not qualify real browser journeys or private runtime consumers.
"""
import argparse
import json
import os
import pathlib
import re
import selectors
import subprocess
import sys
import time


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

CORELESS_TEST_ANCHORS = {
    'cmd/baha/app_backend_selective_runtime_test.go':
        'TestSelectiveBackendCleanupPreservesRuntimeBaseline',
}

# Each shard runs on its own CI runner and therefore its own isolated runtime
# daemon. Keep the test list explicit so changes cannot silently omit coverage.
CORELESS_SHARD_TESTS = {
    'foundation': (
        'TestCorelessFreshWorkloadRuntimeLifecycle',
        'TestCorelessLocalProdRuntimeLifecycle',
        'TestRenovateVolumeRecoveryRuntimeAcceptance',
    ),
    'identity-backend': (
        'TestSelectiveIdentityRuntimeAddRemoveReAdd',
        'TestSelectiveBackendRuntimeAddRemoveReAdd',
        'TestSelectiveBackendCleanupPreservesRuntimeBaseline',
    ),
    'application': (
        'TestSelectiveApplicationBackendRuntimeAddRemoveReAdd',
        'TestSelectiveHTTPSRuntimeAddRemoveReAdd',
        'TestManagedRegisteredOTLPNativeConsumerHandoff',
    ),
    'core-capability': (
        'TestApplicationCapabilityLifecycleExistingCore',
    ),
}

CORELESS_SHARD_NAMES = tuple(CORELESS_SHARD_TESTS)


def coreless_anchor_checks(gate, repository=None):
    """Require and run every mapped Go test anchor in this candidate."""
    if not gate.endswith('/coreless-lifecycle'):
        return []
    root = pathlib.Path('.') if repository is None else pathlib.Path(repository)
    checks = []
    for source, test_name in CORELESS_TEST_ANCHORS.items():
        path = root / source
        if not path.is_file():
            raise ValueError(f'mandatory Coreless test anchor source is missing: {source}')
        content = path.read_text()
        if not re.search(rf'^func {re.escape(test_name)}\(', content, re.MULTILINE):
            raise ValueError(f'mandatory Coreless test anchor is missing: {source}:{test_name}')
        checks.append(['./cmd/baha', '-run', '^' + test_name + '$', '-timeout', '20m'])
    return checks


def coreless_shard_checks(gate, shard, repository=None):
    """Return the exact Coreless test selections owned by one runner shard."""
    if not gate.endswith('/coreless-lifecycle'):
        raise ValueError('shards are only supported for Coreless lifecycle gates')
    if shard not in CORELESS_SHARD_TESTS:
        raise ValueError('unknown Coreless lifecycle shard')
    root = pathlib.Path('.') if repository is None else pathlib.Path(repository)
    selections = CHECKS[gate]
    by_test = {}
    for selection in selections:
        expression = selection[selection.index('-run') + 1]
        if not (expression.startswith('^') and expression.endswith('$')):
            raise ValueError('Coreless test catalog requires exact test names')
        by_test[expression[1:-1]] = selection
    for command in coreless_anchor_checks(gate, root):
        name = command[command.index('-run') + 1][1:-1]
        by_test[name] = command
    selected = []
    for test_name in CORELESS_SHARD_TESTS[shard]:
        command = by_test.get(test_name)
        if command is None:
            raise ValueError(f'Coreless shard {shard} references unregistered test {test_name}')
        selected.append(command)
    if not selected:
        raise ValueError(f'Coreless lifecycle shard {shard} is empty')
    return selected


def validate_coreless_shard_partition(gate, repository=None):
    """Fail when a registered Coreless test is omitted or assigned twice."""
    if not gate.endswith('/coreless-lifecycle'):
        return
    root = pathlib.Path('.') if repository is None else pathlib.Path(repository)
    expected = set()
    for selection in CHECKS[gate]:
        expression = selection[selection.index('-run') + 1]
        if not (expression.startswith('^') and expression.endswith('$')):
            raise ValueError('Coreless test catalog requires exact test names')
        expected.add(expression[1:-1])
    for command in coreless_anchor_checks(gate, root):
        expected.add(command[command.index('-run') + 1][1:-1])
    assigned = [name for names in CORELESS_SHARD_TESTS.values() for name in names]
    if len(assigned) != len(set(assigned)) or set(assigned) != expected:
        raise ValueError('Coreless lifecycle shards must partition every registered test exactly once')


def checked_tests(command, output, timeout=900):
    """Stream Go test output while retaining JSONL and reject empty/skipped selections."""
    with output.open('w') as log:
        process = subprocess.Popen(command, stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                                   bufsize=0)
        selector = selectors.DefaultSelector()
        selector.register(process.stdout, selectors.EVENT_READ)
        deadline = time.monotonic() + timeout
        try:
            while selector.get_map():
                remaining = deadline - time.monotonic()
                if remaining <= 0:
                    raise subprocess.TimeoutExpired(command, timeout)
                for key, _ in selector.select(remaining):
                    chunk = os.read(key.fileobj.fileno(), 8192)
                    if not chunk:
                        selector.unregister(key.fileobj)
                        continue
                    decoded = chunk.decode(errors='replace')
                    log.write(decoded)
                    log.flush()
                    sys.stdout.write(decoded)
                    sys.stdout.flush()
            returncode = process.wait()
        except BaseException:
            process.kill()
            process.wait()
            raise
        finally:
            selector.close()
            process.stdout.close()
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
    if returncode or not passed or skipped:
        raise ValueError(f'contract checks failed, empty or skipped; inspect {output.name}')
    return {'command': command, 'passed': passed, 'log': output.name}


def go_test_command(selection):
    """Keep every native acceptance invocation race-enabled and uncached."""
    return ['go', 'test', '-race', '-count=1', '-json', *selection]


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
        os.environ['BASEHARBOR_APP_STOP_ACCEPTANCE'] = '1'
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
    parser.add_argument('--shard', choices=CORELESS_SHARD_NAMES)
    args = parser.parse_args()
    args.output.mkdir(parents=True, exist_ok=True)
    if any(not re.fullmatch('[0-9a-f]{40}', sha) for sha in [args.candidate, args.demo]):
        raise ValueError('candidate and demo must be exact commit SHAs')
    verify_candidate_source(args.candidate)
    runtime_bootstrap = configure_runtime_environment(args.gate)
    if args.gate.endswith('/coreless-lifecycle'):
        if not args.shard:
            raise ValueError('Coreless lifecycle gates require an explicit --shard')
        validate_coreless_shard_partition(args.gate)
        selections = coreless_shard_checks(args.gate, args.shard)
    else:
        if args.shard:
            raise ValueError('--shard is only valid for Coreless lifecycle gates')
        selections = CHECKS[args.gate]
    checks = [checked_tests(go_test_command(selection),
                            args.output / f'check-{index}.jsonl', 1800 if runtime_bootstrap else 900)
              for index, selection in enumerate(selections)]
    verify_candidate_source(args.candidate)
    for check in checks:
        print('Passed ' + str(len(check['passed'])) + ' checks: ' + ' '.join(check['command']))


if __name__ == '__main__':
    try:
        main()
    except (ValueError, KeyError, OSError, subprocess.SubprocessError) as error:
        raise SystemExit(f'integration checks failed: {error}')

#!/usr/bin/env python3
"""Trusted #893 PR job matrix and final result adapter (no PR-controlled evidence)."""
import argparse
import json
import re
import subprocess
from pathlib import Path

WIZARD_TEST_FAMILIES = {
    'wizard-app-edit-load-tests': (r'^TestAppEditLoad',),
    'wizard-capability-selection-tests': (r'^TestAppCapabilitySelection',),
    'wizard-provider-choice-tests': (r'^TestAppProviderChoice',),
    'wizard-footprint-presenter-tests': (r'^TestAppFootprintPresenter',),
    'wizard-change-presentation-tests': (r'^TestAppChangePreview', r'^TestAppChangeResultPresenter'),
    'wizard-controller-tests': (r'^TestAppWizardController',),
    'wizard-app-new-tests': (r'^TestAppNewWizard',),
    'wizard-app-edit-tests': (r'^TestAppEditApply', r'^TestAppEditCommand'),
    'wizard-footprint-command-tests': (r'^TestAppFootprintShow',),
    'wizard-machine-adapter-tests': (r'^TestAppMachineAdapters',),
    # core-921 reviewed production registration regressions; each family distinct
    # and must actually RUN/PASS (never a combined OR that can hide a miss).
    'wizard-core921-tests': (
        r'^TestCore921ActualCommandTreeHelpAndReadOnlyEntry$',
        r'^TestCore921MCPRealDiscoveryAndRejectIncompleteEdit$',
        r'^TestCore921MCPEditSchemaIsSnakeCaseAndPhaseConditional$',
        r'^TestCore921MCPProposeReachesHandlerWithZeroWrites$',
        r'^TestCore921MCPEditHandlerLevelNegatives$',
    ),
    'wizard-core918-native-sql-safety-tests': (
        r'^TestNativeSQLInventoryRejectsUnverifiedMaterialRegardlessOfKnownPath',
        r'^TestNativeSQLInventoryKnownFilesRequireActualInspection',
        r'^TestNativeSQLGeneratedProjectionRequiresProtectedAncestorAndSource',
        r'^TestNativeSQLProductionMaterialGateRejectsOversizedAndBrokenPublicSources',
    ),
    # Each required registration family is distinct and must actually RUN/PASS.
    # The MCP family exercises real in-process SDK discovery and invocation,
    # rather than treating a subprocess/stdio smoke as registration coverage.
    'wizard-core754-mcp-tests': (
        r'^TestExpectedPreconditionsShareTypedCLIMCPErrors$',
        r'^TestMCPReadEntryExpectedPreconditions$',
    ),
    'wizard-registration-tests': (
        r'^TestAppRegistrationCI',
        r'^TestCommandTreeCI',
        r'^TestMachineCoverageCI',
        r'^TestMachineDocumentationTracksRegistryAndCommandTree',
        r'^TestHumanCommandIndexTracksExactCommandMetadata',
        r'^TestMCPRegistrationCI',
    ),
}

VALIDATIONS = {'documentation-audit', 'mkdocs-en-strict', 'mkdocs-de-strict',
               'pages-candidate-build', 'source-contracts', 'cli-guided-tests',
               'cli-gofmt', 'cli-vet', 'cli-build', 'capability-package-tests',
               'pr-planner-tests', 'ci-policy', 'ci-actionlint',
               'ci-executor-verifier-tests',
               'inspect-gofmt', 'inspect-package-tests', 'inspect-vet',
               'inspect-consumer-tests', 'inspect-consumer-build',
               'pins-data-validation'} | set(WIZARD_TEST_FAMILIES)


def execute_validation(check, repository):
    """Execute a trusted profile check against the candidate checkout.

    Go's -run exits zero when no tests match. Discover first and require at
    least one matching test before executing the family.
    """
    if check not in WIZARD_TEST_FAMILIES:
        raise ValueError('unsupported executable validation')
    patterns = WIZARD_TEST_FAMILIES[check]
    if check == 'wizard-core918-native-sql-safety-tests':
        patterns = tuple(pattern + chr(36) for pattern in patterns)
    repo = Path(repository)
    all_matches = []
    for pattern in patterns:
        listed = subprocess.run(
            ['go', 'test', './cmd/baha', '-list', pattern],
            cwd=repo, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        if listed.returncode:
            raise RuntimeError('test discovery failed for ' + pattern + ': ' + listed.stderr.strip())
        matches = [
            line.strip() for line in listed.stdout.splitlines()
            if re.fullmatch(r'Test[A-Za-z0-9_]+', line.strip())
            and re.search(pattern, line.strip())
        ]
        if not matches:
            raise RuntimeError('zero matching tests for ' + check + ' (' + pattern + ')')
        # Run each mandatory family independently so a combined OR expression
        # cannot replace missing tests or silently skip a required family.
        executed = subprocess.run(
            ['go', 'test', './cmd/baha', '-run', pattern, '-count=1', '-v'],
            cwd=repo, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
        if executed.returncode:
            raise RuntimeError('test execution failed for ' + check +
                               ' (' + pattern + '): ' + executed.stdout)
        for name in matches:
            if not re.search(r'(?m)^=== RUN   ' + re.escape(name) + r'(?:$|/)', executed.stdout):
                raise RuntimeError('required test did not execute: ' + name)
            if re.search(r'(?m)^--- SKIP: ' + re.escape(name) + r'(?:$|\s)', executed.stdout):
                raise RuntimeError('required test skipped: ' + name)
            if not re.search(r'(?m)^--- PASS: ' + re.escape(name) + r'(?:$|\s)', executed.stdout):
                raise RuntimeError('required test missing PASS: ' + name)
        all_matches.extend(matches)
    return all_matches

def matrix(plan):
    checks = plan.get('required_validation_checks')
    if not isinstance(checks, list):
        raise ValueError('missing validation checks')
    ids = [row['id'] for row in checks]
    if len(ids) != len(set(ids)) or any(i not in VALIDATIONS for i in ids):
        raise ValueError('unsupported validation check')
    # Matrix cannot be empty; an explicit inert marker is NOT a PASS.
    return {'include': [{'check': i} for i in ids] or [{'check': 'not-required'}]}


def report(plan, job_conclusion, matrix_result):
    ids = [row['id'] for row in plan['required_validation_checks']]
    # Fail closed on composite job outcome: one failed/omitted matrix lane
    # must never be projected as independent success.
    success = matrix_result == 'success' and job_conclusion == 'success'
    return {
        'schema': 'baseharbor.pr-check-results/v1',
        **{k: plan.get(k) for k in ('target_base_sha', 'head_sha',
                                   'merge_base_sha', 'integration_sha')},
        'results': [
            {'kind': 'validation', 'id': name,
             'conclusion': 'success' if success else 'blocked'}
            for name in ids
        ],
    }


# Immutable consumer repositories a pins-data file may reference. The Core and
# Demo SHAs are not published in the consumer-pins file itself; the public
# surface that can be fail-closed-validated without fetching private sources is
# the Connector/Console origin repository plus the joint Connector commit that
# every gate must share.
PINNED_PINS_REPOSITORIES = {'mcpdev80/baseharbor-console',
                            'mcpdev80/baseharbor-node-connector'}


def pins_data_exact(value):
    import re as _pins_re
    return isinstance(value, str) and _pins_re.fullmatch('[0-9a-f]{40}', value) is not None


def validate_pins_data(check, repository):
    """Validate a docs/releases/<tag>.consumer-pins.json metadata add.

    The consumer-repository identities recorded here are private, and the Core
    and Demo SHAs are not published in the file, so this validation reads only
    the public origin surface: the Connector/Console origin repository, a 40-hex
    commit, and the joint Connector commit that every gate must share (the same
    invariant the release resolver enforces). A malformed, missing, mismatched,
    structurally different or divergent-pins file must fail, so the planner
    cannot select a validation against unreviewed data. No private source is
    fetched or published.
    """
    source = Path(repository).resolve()
    pins_files = sorted(source.rglob('docs/releases/*.consumer-pins.json'))
    pins_files = [p for p in pins_files if p.suffix == '.json']
    if not pins_files:
        raise RuntimeError('no consumer-pins metadata found under docs/releases')
    validated = set()
    for pins_path in pins_files:
        if pins_path.name == 'index.md':
            continue
        pins = json.loads(pins_path.read_text())
        if (not isinstance(pins, dict) or
                pins.get('schema') != 'baseharbor.consumer-evidence-pins/v1' or
                not isinstance(pins.get('gates'), dict) or not pins['gates']):
            raise RuntimeError('consumer-pins file differs: ' + str(pins_path))
        connector_commits = set()
        console_repo = None
        console_commit = None
        for gate, pin in pins['gates'].items():
            if not isinstance(pin, dict) or 'connector' not in pin:
                raise RuntimeError('consumer-pins gate missing connector: ' + gate)
            connector = pin['connector']
            repo = connector.get('repository')
            commit = connector.get('commit')
            if not isinstance(connector, dict) or repo not in PINNED_PINS_REPOSITORIES or \
                    not pins_data_exact(commit):
                raise RuntimeError('consumer-pins connector origin differs: ' + gate)
            connector_commits.add(commit)
            if gate == 'integration/static/live-console':
                console = pin['console']
                console_repo = console.get('repository')
                console_commit = console.get('commit')
        # The joint Connector commit is identical across every gate, the same
        # invariant the release resolver seeds into the live-console origin.
        if len(connector_commits) != 1:
            raise RuntimeError('consumer-pins Connector commits diverge across gates')
        validated.add(pins_path.name)
    return sorted(validated)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('mode', choices=['matrix', 'results', 'execute'])
    parser.add_argument('--plan', type=Path)
    parser.add_argument('--check')
    parser.add_argument('--repository', default='.')
    parser.add_argument('--job-conclusion', default='blocked')
    parser.add_argument('--matrix-result', default='blocked')
    args = parser.parse_args()
    if args.mode == 'execute':
        if not args.check:
            parser.error('--check is required for execute')
        if args.check == 'pins-data-validation':
            matched = validate_pins_data(args.check, args.repository)
            print(json.dumps({'check': args.check, 'matched_tests': matched}, sort_keys=True))
            return
        matches = execute_validation(args.check, args.repository)
        print(json.dumps({'check': args.check, 'matched_tests': matches}, sort_keys=True))
        return
    if not args.plan:
        parser.error('--plan is required for matrix/results')
    plan = json.loads(args.plan.read_text())
    value = matrix(plan) if args.mode == 'matrix' else report(
        plan, args.job_conclusion, args.matrix_result)
    print(json.dumps(value, sort_keys=True, separators=(',', ':')))


if __name__ == '__main__':
    main()

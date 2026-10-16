#!/usr/bin/env python3
"""Trusted #893 PR job matrix and final result adapter (no PR-controlled evidence)."""
import argparse
import json
import re
import subprocess
from pathlib import Path

WIZARD_TEST_FAMILIES = {
    'wizard-app-edit-load-tests': r'^TestAppEditLoad',
    'wizard-capability-selection-tests': r'^TestAppCapabilitySelection',
    'wizard-provider-choice-tests': r'^TestAppProviderChoice',
    'wizard-footprint-presenter-tests': r'^TestAppFootprintPresenter',
    'wizard-change-presentation-tests': r'^(TestAppChangePreview|TestAppChangeResultPresenter)',
    'wizard-controller-tests': r'^TestAppWizardController',
    'wizard-app-new-tests': r'^TestAppNewWizard',
    'wizard-app-edit-tests': r'^(TestAppEditApply|TestAppEditCommand)',
    'wizard-footprint-command-tests': r'^TestAppFootprintShow',
    'wizard-machine-adapter-tests': r'^TestAppMachineAdapters',
    'wizard-registration-tests': r'^(TestAppRegistration|TestCommandTree|TestMachineCoverage|TestMCP)',
}

VALIDATIONS = {'documentation-audit', 'mkdocs-en-strict', 'mkdocs-de-strict',
               'pages-candidate-build', 'source-contracts', 'cli-guided-tests',
               'cli-gofmt', 'cli-vet', 'cli-build', 'capability-package-tests',
               'pr-planner-tests', 'ci-policy', 'ci-actionlint',
               'ci-executor-verifier-tests',
               'inspect-gofmt', 'inspect-package-tests', 'inspect-vet',
               'inspect-consumer-tests', 'inspect-consumer-build'} | set(WIZARD_TEST_FAMILIES)


def execute_validation(check, repository):
    """Execute a trusted profile check against the candidate checkout.

    Go's -run exits zero when no tests match. Discover first and require at
    least one matching test before executing the family.
    """
    if check not in WIZARD_TEST_FAMILIES:
        raise ValueError('unsupported executable validation')
    pattern = WIZARD_TEST_FAMILIES[check]
    repo = Path(repository)
    listed = subprocess.run(
        ['go', 'test', './cmd/baha', '-list', pattern],
        cwd=repo, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    if listed.returncode:
        raise RuntimeError('test discovery failed: ' + listed.stderr.strip())
    matches = [
        line.strip() for line in listed.stdout.splitlines()
        if line.strip().startswith('Test') and re.search(pattern, line.strip())
    ]
    if not matches:
        raise RuntimeError('zero matching tests for ' + check + ' (' + pattern + ')')
    executed = subprocess.run(
        ['go', 'test', './cmd/baha', '-run', pattern, '-count=1'],
        cwd=repo)
    if executed.returncode:
        raise RuntimeError('test execution failed for ' + check)
    return matches


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

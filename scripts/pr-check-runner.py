#!/usr/bin/env python3
"""Trusted #893 PR job matrix and final result adapter (no PR-controlled evidence)."""
import argparse
import json
from pathlib import Path

VALIDATIONS = {'documentation-audit', 'mkdocs-en-strict', 'mkdocs-de-strict',
               'pages-candidate-build', 'source-contracts', 'cli-guided-tests',
               'cli-vet', 'cli-build', 'capability-package-tests', 'pr-planner-tests', 'ci-policy',
               'ci-executor-verifier-tests'}


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
    parser.add_argument('mode', choices=['matrix', 'results'])
    parser.add_argument('--plan', type=Path, required=True)
    parser.add_argument('--job-conclusion', default='blocked')
    parser.add_argument('--matrix-result', default='blocked')
    args = parser.parse_args()
    plan = json.loads(args.plan.read_text())
    value = matrix(plan) if args.mode == 'matrix' else report(
        plan, args.job_conclusion, args.matrix_result)
    print(json.dumps(value, sort_keys=True, separators=(',', ':')))


if __name__ == '__main__':
    main()

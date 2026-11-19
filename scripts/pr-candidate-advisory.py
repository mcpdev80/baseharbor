#!/usr/bin/env python3
"""Run candidate-added validation checks without making them merge-required."""
import argparse
import json
import os
from pathlib import Path
import subprocess
import sys


def added_checks(base_plan, candidate_plan):
    """Return candidate validation rows absent or changed in the base plan."""
    if (not isinstance(base_plan, dict) or
            not isinstance(candidate_plan, dict) or
            candidate_plan.get('schema') != 'baseharbor.pr-gate-plan/v1'):
        raise ValueError('candidate and base plans must use the PR gate plan schema')
    rows = candidate_plan.get('required_validation_checks')
    base_rows = base_plan.get('required_validation_checks')
    if not isinstance(rows, list) or not isinstance(base_rows, list):
        raise ValueError('plan validation checks must be lists')
    if any(not isinstance(row, dict) or not isinstance(row.get('id'), str)
           for row in rows + base_rows):
        raise ValueError('plan validation checks must have string IDs')
    base = {row['id']: row for row in base_rows}
    return [row for row in rows
            if row['id'] not in base or base[row['id']].get('command') != row.get('command')]


def run(command, root, env):
    return subprocess.run(command, cwd=root, env=env, text=True,
                          stdout=subprocess.PIPE, stderr=subprocess.STDOUT)


def execute_checks(checks, root, environment):
    results = []
    for row in checks:
        command = row.get('command')
        if not isinstance(command, str) or not command.strip():
            result = subprocess.CompletedProcess([], 2,
                                                 'candidate check has no executable command')
        else:
            result = run(['bash', '-euo', 'pipefail', '-c', command], root, environment)
        results.append({'id': row['id'], 'command': command,
                        'returncode': result.returncode, 'output': result.stdout})
    return results


def report(path, lines):
    text = '\n'.join(lines) + '\n'
    print(text, end='')
    if path:
        with path.open('a') as summary:
            summary.write('## Candidate-added checks (advisory)\n\n')
            summary.write(text)


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-plan', type=Path, required=True)
    parser.add_argument('--diff', type=Path, required=True)
    parser.add_argument('--repository', type=Path, default=Path('.'))
    parser.add_argument('--summary', type=Path)
    args = parser.parse_args(argv)
    root = args.repository.resolve()
    try:
        base_plan = json.loads(args.base_plan.read_text())
        diff = json.loads(args.diff.read_text())
        if not isinstance(base_plan, dict) or not isinstance(diff, dict):
            raise ValueError('base plan and verified diff must be JSON objects')
        for field in ('target_base_sha', 'head_sha', 'integration_sha'):
            if base_plan.get(field) != diff.get(field):
                raise ValueError('candidate/base plan input mismatch for ' + field)
        environment = os.environ.copy()
        environment.update({
            'BASE_SHA': base_plan['target_base_sha'],
            'HEAD_SHA': base_plan['head_sha'],
            'INTEGRATION_SHA': base_plan['integration_sha'],
        })
        plan_command = [sys.executable, str(root / 'scripts/pr-gate-plan.py'),
                        '--diff', str(args.diff.resolve()),
                        '--requirements', str(root / 'scripts/release-requirements.json'),
                        '--mapping', str(root / 'scripts/pr-gate-map-v1.json')]
        planned = run(plan_command, root, environment)
        if planned.returncode:
            report(args.summary, [
                'Candidate planner: FAIL (the required base plan is unaffected).',
                'Candidate plan reproduction: `python3 scripts/pr-gate-plan.py --diff pr-diff.json`.',
                '```text', planned.stdout.rstrip(), '```',
            ])
            return 1
        candidate_plan = json.loads(planned.stdout)
        if not isinstance(candidate_plan, dict):
            raise ValueError('candidate planner did not return a JSON object')
        if candidate_plan.get('coverage_status') != 'resolved':
            raise ValueError('candidate planner returned unresolved coverage: ' +
                             str(candidate_plan.get('unresolved_coverage')))
        checks = added_checks(base_plan, candidate_plan)
        if any(candidate_plan.get(field) != base_plan.get(field)
               for field in ('target_base_sha', 'head_sha', 'integration_sha')):
            raise ValueError('candidate planner changed verified PR identities')
        candidate_gates = candidate_plan.get('selected_gates')
        base_gates = base_plan.get('selected_gates')
        if (not isinstance(candidate_gates, list) or
                not all(isinstance(gate, str) for gate in candidate_gates) or
                not isinstance(base_gates, list) or
                not all(isinstance(gate, str) for gate in base_gates)):
            raise ValueError('plan selected gates must be string lists')
        gates = sorted(set(candidate_gates) - set(base_gates))
        if not checks:
            lines = ['No candidate-added validation checks. The trusted base plan remains the only merge requirement.']
            if gates:
                lines.extend(['', 'Candidate-added native gates are advisory: ' + ', '.join(gates),
                              'Exercise them in candidate workflow jobs; do not add them to PR Selected Checks.'])
            report(args.summary, lines)
            return 0

        lines = ['The trusted base plan remains the only merge requirement.',
                 'Candidate-added validation checks run here as advisory checks.']
        results = execute_checks(checks, root, environment)
        for result in results:
            status = 'PASS' if result['returncode'] == 0 else 'FAIL'
            command = result['command'] or 'missing command'
            lines.append(f'- `{result["id"]}`: {status}; candidate command: `{command}`')
            if result['output'].strip():
                lines.extend(['', '```text', result['output'].rstrip(), '```'])
        failed = any(result['returncode'] != 0 for result in results)
        if gates:
            lines.extend(['', 'Candidate-added native gates are advisory: ' + ', '.join(gates),
                          'Exercise them in candidate workflow jobs; do not add them to PR Selected Checks.'])
        report(args.summary, lines)
        return 1 if failed else 0
    except (OSError, ValueError, json.JSONDecodeError) as error:
        report(args.summary, [
            'Candidate advisory setup/planning: FAIL (the required base plan is unaffected).',
            'Inspect the candidate plan input and reproduce with `scripts/ci-fast-check.sh`.',
            '```text', str(error), '```',
        ])
        return 1


if __name__ == '__main__':
    raise SystemExit(main())

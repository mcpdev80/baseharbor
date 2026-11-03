#!/usr/bin/env python3
"""Require normal job results from this workflow's needs context."""
import argparse
import json
import os
from pathlib import Path

BASE_JOBS = ('workflow_policy', 'pr_plan', 'pr_focused', 'pr_validations')


def verify(plan, needs, *, head=None, integration=None):
    if not isinstance(plan, dict) or plan.get('schema') != 'baseharbor.pr-gate-plan/v1':
        return ['missing or invalid selected-check plan']
    if not isinstance(needs, dict):
        return ['missing workflow dependency results']
    errors = []
    if plan.get('coverage_status') != 'resolved' or plan.get('unresolved_coverage') != []:
        errors.append('unresolved check coverage: ' + str(plan.get('unresolved_coverage')))
    if head is not None and plan.get('head_sha') != head:
        errors.append('plan belongs to another PR head')
    if integration is not None and plan.get('integration_sha') != integration:
        errors.append('plan belongs to another integration commit')
    gates = plan.get('selected_gates')
    checks = plan.get('required_validation_checks')
    if not isinstance(gates, list) or not all(isinstance(x, str) for x in gates):
        return errors + ['invalid selected gates']
    if not isinstance(checks, list) or not all(isinstance(x, dict) and isinstance(x.get('id'), str) for x in checks):
        return errors + ['invalid validation checks']
    if not gates and not checks:
        errors.append('no selected checks')
    required = set(BASE_JOBS)
    for gate in gates:
        parts = gate.split('/')
        if len(parts) == 3 and parts[0] == 'atomic' and parts[1] in ('docker', 'podman'):
            required.update(('pr_candidate', 'pr_runner_interface', 'pr_runtime_selected'))
        elif gate == 'integration/static/public-contracts':
            required.add('pr_public_contracts')
        else:
            errors.append('selected gate has no workflow executor: ' + gate)
    if 'pr_runtime_selected' in required or 'pr_public_contracts' in required:
        dependencies = plan.get('required_consumer_dependencies')
        if not isinstance(dependencies, list) or not all(isinstance(x, str) for x in dependencies) or not set(dependencies) <= {'core', 'demo', 'console', 'connector'}:
            errors.append('invalid consumer dependency selection')
        elif set(dependencies) & {'console', 'connector'}:
            required.add('pr_consumer_pins')
        else:
            required.add('pr_demo_pin')
    for name in sorted(required):
        job = needs.get(name)
        if not isinstance(job, dict) or job.get('result') != 'success':
            errors.append('required job did not succeed: ' + name)
    # An optional skipped job is normal. An executed failure is never ignored.
    for name, job in needs.items():
        if not isinstance(job, dict) or job.get('result') not in ('success', 'skipped'):
            errors.append('workflow job did not succeed: ' + name)
    return errors


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--plan', type=Path, required=True)
    args = parser.parse_args()
    try:
        errors = verify(json.loads(args.plan.read_text()), json.loads(os.environ['CI_NEEDS']),
                        head=os.environ['EXPECTED_HEAD'], integration=os.environ['EXPECTED_INTEGRATION'])
    except (OSError, ValueError, KeyError) as exc:
        raise SystemExit('PR Selected Checks: missing workflow inputs') from exc
    for error in errors:
        print('BLOCKED: ' + error)
    if errors:
        raise SystemExit(1)
    print('PR Selected Checks: PASS')


if __name__ == '__main__':
    main()

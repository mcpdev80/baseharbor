#!/usr/bin/env python3
"""Require normal job results from this workflow's needs context."""
import argparse
import json
import os
from pathlib import Path

BASE_JOBS = ('workflow_policy', 'source_format', 'pr_plan', 'pr_focused', 'pr_validations')
BASE_WORKFLOW_JOBS = (
    'workflow_policy', 'source_format', 'pr_plan', 'pr_focused', 'pr_validations',
    'pr_candidate', 'pr_consumer_pins', 'pr_demo_pin', 'pr_runner_interface',
    'pr_runtime_selected', 'pr_selected_integration', 'pr_public_contracts',
    'pr_owner_node_native', 'pr_native_impact', 'pr_native_selected', 'pr_unit_shards',
)
CORELESS_LIFECYCLE_GATES = {
    'integration/docker/coreless-lifecycle',
    'integration/podman/coreless-lifecycle',
}


def verify(base_plan, needs, *, head=None, integration=None):
    """Verify only the trusted base plan and its declared job results.

    Candidate-added checks stay outside the required dependency set. Their
    results can be shown as advisory workflow jobs without changing this plan.
    """
    if not isinstance(base_plan, dict) or base_plan.get('schema') != 'baseharbor.pr-gate-plan/v1':
        return ['missing or invalid selected-check plan']
    if not isinstance(needs, dict):
        return ['missing workflow dependency results']
    errors = []
    if base_plan.get('coverage_status') != 'resolved' or base_plan.get('unresolved_coverage') != []:
        errors.append('unresolved check coverage: ' + str(base_plan.get('unresolved_coverage')))
    if head is not None and base_plan.get('head_sha') != head:
        errors.append('plan belongs to another PR head')
    if integration is not None and base_plan.get('integration_sha') != integration:
        errors.append('plan belongs to another integration commit')
    gates = base_plan.get('selected_gates')
    checks = base_plan.get('required_validation_checks')
    if not isinstance(gates, list) or not all(isinstance(x, str) for x in gates):
        return errors + ['invalid selected gates']
    if not isinstance(checks, list) or not all(isinstance(x, dict) and isinstance(x.get('id'), str) for x in checks):
        return errors + ['invalid validation checks']
    if not gates and not checks:
        errors.append('no selected checks')
    # card 0428-208/#1746 fix: the PR-executed static gate set is single-sourced
    # from the stage boundary (ATOMIC_STATIC_PR_EXECUTED_GATES). Only gates the
    # boundary declares PR-executed map to pr_focused; deferred release-only
    # static gates map to NO PR job, and an unclassified atomic/static/* gate
    # still falls to the no-executor error. This keeps pr_focused out of the
    # required set for deferred-only plans (pr-stage-boundary: deferral never
    # adds a required job).
    import importlib.util as _ilu
    _sb_spec = _ilu.spec_from_file_location('pr_stage_boundary_for_final',
                                           Path(__file__).parent / 'pr-stage-boundary.py')
    _sb = _ilu.module_from_spec(_sb_spec)
    _sb_spec.loader.exec_module(_sb)
    _pr_executed_static = frozenset(_sb.ATOMIC_STATIC_PR_EXECUTED_GATES)

    required = set(BASE_JOBS)
    if set(gates) & CORELESS_LIFECYCLE_GATES:
        required.update(('pr_selected_integration', 'pr_demo_pin'))
    deferred = base_plan.get('deferred_integration_gates')
    if deferred is not None:
        if not isinstance(deferred, list) or not all(isinstance(gate, str) for gate in deferred):
            errors.append('invalid deferred integration gates')
            deferred = []
        allowed_deferred = {
            'integration/static/public-contracts',
            # Release-only atomic/static gates whose executable check runs only
            # at release/qualify (pre-release.yml:663 static_gates ->
            # pre-release-runtime-suite.yml:267, GATE=shell-ux runs
            # tests/shell-ux/run.sh, GATE=ci-fast-check runs scripts/ci-fast-check.sh),
            # never in a PR. No PR executor exists; the requirement is honoured by
            # deferral, not by a focused-python regression. See pr-stage-boundary.py.
            'atomic/static/shell-ux',
            'atomic/static/ci-fast-check',
            'atomic/static/mcp',
        }
        unknown_deferred = set(deferred) - allowed_deferred
        if unknown_deferred:
            errors.append('unsupported deferred gate: ' + ', '.join(sorted(unknown_deferred)))
    if any(check['id'] in ('go-unit-tests', 'cli-package-tests') for check in checks):
        required.add('pr_unit_shards')
    for gate in gates:
        parts = gate.split('/')
        if len(parts) == 3 and parts[0] == 'atomic' and parts[1] in ('docker', 'podman'):
            required.update(('pr_candidate', 'pr_runner_interface', 'pr_runtime_selected'))
        elif gate == 'integration/static/public-contracts':
            required.add('pr_public_contracts')
        elif gate in CORELESS_LIFECYCLE_GATES:
            pass
        elif gate in _pr_executed_static:
            # PR-executed atomic/static/* gates (boundary-declared; empty today,
            # the ci-fast-check executable proof runs in the release suite) map
            # to the existing PR · Focused Python job only.
            required.add('pr_focused')
        else:
            errors.append('selected gate has no workflow executor: ' + gate)
    if 'pr_runtime_selected' in required or 'pr_public_contracts' in required:
        dependencies = base_plan.get('required_consumer_dependencies')
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
    # Preserve the existing failure behavior for the base-defined job graph.
    # New candidate-only jobs are reported separately until they merge.
    for name in BASE_WORKFLOW_JOBS:
        if name not in needs:
            continue
        job = needs[name]
        if not isinstance(job, dict) or job.get('result') not in ('success', 'skipped'):
            errors.append('workflow job did not succeed: ' + name)
    return errors


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-plan', '--plan', dest='base_plan', type=Path, required=True,
                        help='plan produced by the trusted PR base policy')
    args = parser.parse_args()
    try:
        errors = verify(json.loads(args.base_plan.read_text()), json.loads(os.environ['CI_NEEDS']),
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

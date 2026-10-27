#!/usr/bin/env python3
"""L7 BELEG: dual integration simulation for ci-workflow-policy.sh mapping.

integ1 = 2-parent merge(basis, PR1); integ2 = 2-parent merge(basis, PR2).
basis = 2-parent merge(develop, l7/ustrack). Both integrations resolve the
single unresolved path 'scripts/ci-workflow-policy.sh' to coverage_status
== resolved with an empty unresolved_coverage list.
"""
import importlib.util
import json
import subprocess
from pathlib import Path

ROOT = Path('/home/marcel/dev/baseharbor-ecosystem')
BASIS = ROOT / 'baseharbor'
INTEG1 = ROOT / 'worktrees' / 'l7-integ1'
INTEG2 = ROOT / 'worktrees' / 'l7-integ2'
DEVELOP = 'develop'
RECEIPT_SURFACE = [
    'scripts/receipt_writer.py', 'scripts/receipt_verifier.py',
    'scripts/test_receipt_chain.py', 'scripts/test_receipt_writer.py',
    'scripts/test_receipt_verifier.py', 'scripts/test_ci_workflow_policy_receipts.py',
    'scripts/test_pr_gate_plan_receipt_paths.py', 'scripts/ci-workflow-policy.sh',
    'docs/development/minimal-ci-receipts.md',
    'docs/de/development/minimal-ci-receipts.md',
    '.github/workflows/receipt-approval.yml',
    '.github/workflows/receipt-post-run.yml',
    '.github/workflows/receipt-shadow.yml',
]


def load(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def sha(wt, rev):
    return subprocess.check_output(['git', '-C', str(wt), 'rev-parse',
                                   '--verify', rev + '^{commit}'],
                                   text=True).strip()


def diff_for(wt, head_sha):
    diffbase = load('diffbase', wt / 'scripts/pr-diff-base.py')
    planner = load('planner', wt / 'scripts/pr-gate-plan.py')
    target = sha(BASIS, DEVELOP)
    integration = sha(wt, 'HEAD')
    diff = diffbase.resolve_pr_diff(str(wt), target, head_sha, integration)
    return diff, planner


def surface_plan(wt, head_sha):
    diff, planner = diff_for(wt, head_sha)
    surface = [{'status': 'added', 'path': p} for p in RECEIPT_SURFACE]
    diff = {**diff, 'changes': surface, 'broad_required': False}
    requirements = json.loads((wt / 'scripts/release-requirements.json').read_text())
    mapping = json.loads((wt / 'scripts/pr-gate-map-v1.json').read_text())
    return planner.plan(diff, requirements, mapping, repository=str(wt),
                        context_repository='mcpdev80/baseharbor',
                        context_target_branch='develop')


def ci_policy_plan(wt, head_sha):
    diff, planner = diff_for(wt, head_sha)
    diff = {**diff, 'changes': [{'status': 'modified',
                                 'path': 'scripts/ci-workflow-policy.sh'}],
            'broad_required': False}
    requirements = json.loads((wt / 'scripts/release-requirements.json').read_text())
    mapping = json.loads((wt / 'scripts/pr-gate-map-v1.json').read_text())
    return planner.plan(diff, requirements, mapping, repository=str(wt),
                        context_repository='mcpdev80/baseharbor',
                        context_target_branch='develop')


def main():
    integ1 = sha(INTEG1, 'HEAD')
    integ2 = sha(INTEG2, 'HEAD')
    print('integ1 (basis+PR1)  HEAD=' + integ1)
    print('integ2 (basis+PR2)  HEAD=' + integ2)

    p1 = surface_plan(INTEG1, integ1)
    print()
    print('INTEG1  PR1-surface coverage_status=' + p1['coverage_status'])
    print('INTEG1  PR1-surface unresolved_coverage=' + json.dumps(p1['unresolved_coverage']))
    print('INTEG1  ci-workflow-policy.sh coverage_status=' + ci_policy_plan(INTEG1, integ1)['coverage_status'])
    assert p1['coverage_status'] == 'resolved', p1['unresolved_coverage']
    assert p1['unresolved_coverage'] == [], p1['unresolved_coverage']

    p2 = surface_plan(INTEG2, integ2)
    print()
    print('INTEG2  PR1-surface coverage_status=' + p2['coverage_status'])
    print('INTEG2  PR1-surface unresolved_coverage=' + json.dumps(p2['unresolved_coverage']))
    print('INTEG2  ci-workflow-policy.sh coverage_status=' + ci_policy_plan(INTEG2, integ2)['coverage_status'])
    assert p2['coverage_status'] == 'resolved', p2['unresolved_coverage']
    assert p2['unresolved_coverage'] == [], p2['unresolved_coverage']

    print()
    print('L7-DUAL-SIM-OK')


if __name__ == '__main__':
    main()

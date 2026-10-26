#!/usr/bin/env python3
"""L6 BELEG: run plan() against the real origin/develop...HEAD diff."""
import importlib.util
import json
import subprocess
from pathlib import Path

WT = Path('/home/marcel/dev/baseharbor-ecosystem/worktrees/l6-ustrack')


def load(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


diffbase = load('diffbase', WT / 'scripts/pr-diff-base.py')
planner = load('planner', WT / 'scripts/pr-gate-plan.py')
runner = load('runner', WT / 'scripts/pr-check-runner.py')


def sha(rev):
    return subprocess.check_output(
        ['git', '-C', str(WT), 'rev-parse', '--verify', rev + '^{commit}'],
        text=True).strip()


target, head = sha('origin/develop'), sha('HEAD')
tree = subprocess.check_output(
    ['git', '-C', str(WT), 'merge-tree', '--write-tree', target, head],
    text=True).splitlines()[0]
integration = subprocess.check_output(
    ['git', '-C', str(WT), 'commit-tree', tree, '-p', target, '-p', head,
     '-m', 'L6 local proof integration (dangling, never pushed)'],
    text=True).strip()
diff = diffbase.resolve_pr_diff(str(WT), target, head, integration)
assert not diff['broad_required'], diff['reasons']
requirements = json.loads((WT / 'scripts/release-requirements.json').read_text())
mapping = json.loads((WT / 'scripts/pr-gate-map-v1.json').read_text())
plan = planner.plan(diff, requirements, mapping, repository=str(WT),
                    context_repository='mcpdev80/baseharbor',
                    context_target_branch='develop')
ids = {check['id'] for check in plan['required_validation_checks']}
assert plan['coverage_status'] == 'resolved', plan['unresolved_coverage']
assert plan['unresolved_coverage'] == [], plan['unresolved_coverage']
assert plan['selected_gates'] == [], plan['selected_gates']
assert not plan['broad_required']
assert ids <= runner.VALIDATIONS, sorted(ids - runner.VALIDATIONS)
print('paths=' + str(len(diff['changes'])))
for change in sorted(diff['changes'], key=lambda c: c['path']):
    print('  ' + change['path'] + ' (' + change['status'] + ')')
print('coverage_status=' + plan['coverage_status'])
print('unresolved=' + json.dumps(plan['unresolved_coverage']))
print('selected_gates=' + json.dumps(plan['selected_gates']))
print('validation_ids=' + json.dumps(sorted(ids)))
print('L6-PROOF-OK')

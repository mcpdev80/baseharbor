#!/usr/bin/env python3
"""Z7 coverage-mapping simulation: PR #1096 head qualifies under source-contracts.

Run from the baseharbor repo root:
  python3 evidence/.../scripts/sim_z7_covmap.py
"""
import importlib.util
import json
import sys
from pathlib import Path

ROOT = Path('/home/marcel/dev/baseharbor-ecosystem/baseharbor')
spec = importlib.util.spec_from_file_location('planner', ROOT / 'scripts' / 'pr-gate-plan.py')
planner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(planner)

MAP = json.loads((ROOT / 'scripts' / 'pr-gate-map-v1.json').read_text())
GATES = ['atomic/docker/data-capabilities', 'atomic/podman/data-capabilities',
         'integration/static/public-contracts', 'atomic/docker/lifecycle',
         'atomic/podman/lifecycle', 'journey/docker', 'journey/podman', 'ha/docker/data']
REQ = {'releases': ['v0.4.26'],
       'gates': [{'id': g, 'required_for': ['v0.4.26']} for g in GATES]}


def diff(changes):
    return {'target_base_sha': 'a' * 40, 'head_sha': 'b' * 40,
            'merge_base_sha': 'c' * 40, 'integration_sha': 'd' * 40,
            'broad_required': False, 'reasons': [], 'changes': changes}


def run(label, changes):
    result = planner.plan(diff(changes), REQ, MAP)
    print(f'--- {label} ---')
    print(f'  coverage_status = {result["coverage_status"]}')
    print(f'  unresolved_coverage = {result["unresolved_coverage"]}')
    print(f'  broad_required = {result["broad_required"]}')
    return result


def main():
    proj = 'internal/application/runtime_native_binding_projection.go'
    r1 = run('PR #1096 projection source (added)',
             [{'status': 'added', 'path': proj}])
    assert r1['coverage_status'] == 'resolved', r1['unresolved_coverage']
    assert r1['unresolved_coverage'] == [], r1['unresolved_coverage']
    assert r1['broad_required'] is False
    assert [c['id'] for c in r1['required_validation_checks']] == ['source-contracts']

    r2 = run('PR #1096 full diff (projection + retention runtime + retention test)',
             [{'status': 'modified', 'path': 'internal/application/runtime_environment.go'},
              {'status': 'added', 'path': proj},
              {'status': 'added', 'path': 'internal/application/runtime_placement_retention_test.go'}])
    # The projection source resolves; the other two unreviewed sources stay
    # unresolved (no blanket internal/application/ exemption).
    assert r2['coverage_status'] == 'unresolved', r2['unresolved_coverage']
    assert all(proj not in m for m in r2['unresolved_coverage']), r2['unresolved_coverage']
    assert r2['unresolved_coverage'] == [
        'application lifecycle/provider resolution impact needs targeted qualification: internal/application/runtime_environment.go',
        'application lifecycle/provider resolution impact needs targeted qualification: internal/application/runtime_placement_retention_test.go']

    r3 = run('unmixed unknown application source',
             [{'status': 'added', 'path': 'internal/application/unknown_impl.go'}])
    assert r3['coverage_status'] == 'unresolved', r3['unresolved_coverage']
    assert 'application lifecycle/provider resolution impact needs targeted qualification' in r3['unresolved_coverage'][0]

    print('SIMULATION OK: projection source resolves (resolved/unresolved=[]/broad False); '
          'other application sources stay unresolved.')
    return 0


if __name__ == '__main__':
    sys.exit(main())

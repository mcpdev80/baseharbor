#!/usr/bin/env python3
"""PR-B self-test, fixed integration handling.

Uses git merge-tree --write-tree to obtain the canonical 2-parent integration
commit (matching pr-diff-base.py's clean-merge check), then classifies the
real PR-B diff with the develop (base-copy) planner.
"""
import importlib.util
import json
import subprocess
from pathlib import Path

ROOT = Path('/home/marcel/dev/baseharbor-ecosystem')
BASIS = ROOT / 'baseharbor'


def load(n, p):
    s = importlib.util.spec_from_file_location(n, p)
    m = importlib.util.module_from_spec(s)
    s.loader.exec_module(m)
    return m


def integrate(base, head):
    # Build a canonical 2-parent merge commit whose parents are exactly
    # (base, head) and whose tree is the clean merge of both.
    tree = subprocess.check_output(
        ['git', '-C', str(BASIS), 'merge-tree', base, head],
        text=True).splitlines()[0].strip()
    parents = [subprocess.check_output(
        ['git', '-C', str(BASIS), 'rev-parse', base + '^{commit}'],
        text=True).strip(),
        subprocess.check_output(
            ['git', '-C', str(BASIS), 'rev-parse', head + '^{commit}'],
            text=True).strip()]
    return subprocess.check_output(
        ['git', '-C', str(BASIS), 'commit-tree', tree,
         '-p', parents[0], '-p', parents[1], '-m', 'sim (dangling)'],
        text=True).strip()


def run(base, head, label):
    diffbase = load('diffbase', BASIS / 'scripts' / 'pr-diff-base.py')
    planner = load('planner', BASIS / 'scripts' / 'pr-gate-plan.py')
    reqs = json.loads((BASIS / 'scripts' / 'release-requirements.json').read_text())
    mp = json.loads((BASIS / 'scripts' / 'pr-gate-map-v1.json').read_text())

    tbase = subprocess.check_output(
        ['git', '-C', str(BASIS), 'rev-parse', '--verify', base + '^{commit}'],
        text=True).strip()
    thead = subprocess.check_output(
        ['git', '-C', str(BASIS), 'rev-parse', '--verify', head + '^{commit}'],
        text=True).strip()
    integ = integrate(base, head)

    diff = diffbase.resolve_pr_diff(str(BASIS), tbase, thead, integ)
    if diff['broad_required'] or diff['changes']:
        print(f'[{label}] resolve_pr_diff broad={diff["broad_required"]} '
              f'changes={len(diff["changes"])} reasons={[r["code"] for r in diff["reasons"]]}')
    if not diff['changes']:
        return {'coverage_status': 'SKIP(reasons=' +
                json.dumps([r['code'] for r in diff['reasons']]) + ')',
                'unresolved': diff['reasons'], 'changes': []}
    p = planner.plan(diff, reqs, mp, repository=str(BASIS),
                     context_repository='mcpdev80/baseharbor',
                     context_target_branch='develop')
    return p


if __name__ == '__main__':
    import sys
    argv = sys.argv[1:]
    def paths(r):
        if isinstance(r, dict) and 'changes' in r:
            return [c['path'] + ':' + c['status'] for c in r['changes']]
        return 'n/a'

    if argv and argv[0] == 'prb':
        # PR-B: divergent PR-B base (pre-#1087 receipt CI), head=8f8e5547
        r = run('b7ea46f3dd1ab7f90037c9b373a624a26740b58e', '8f8e5547', 'PR-B')
    elif argv and argv[0] == 'adopt':
        r = run('8f8e5547', '8bcdd527', 'ADOPT')
    else:
        r = run('b7ea46f3dd1ab7f90037c9b373a624a26740b58e', '8f8e5547', 'PR-B')
    print('  diff paths:', paths(r))
    print('  coverage_status :', r.get('coverage_status'))
    print('  unresolved:', json.dumps(r.get('unresolved', [])))
    print('  selected_gates:', r.get('selected_gates'))
    print('  broad_required:', r.get('broad_required'))

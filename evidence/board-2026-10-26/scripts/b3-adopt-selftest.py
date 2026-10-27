#!/usr/bin/env python3
"""Adopt-variant PR self-test (base=HEAD, head=8bcdd527 PR1-Adopt)."""
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


def sha(w, r):
    return subprocess.check_output(['git', '-C', str(w), 'rev-parse',
                                    '--verify', r + '^{commit}'],
                                   text=True).strip()


def tree(w, r):
    return subprocess.check_output(['git', '-C', str(w), 'rev-parse',
                                    r + '^{tree}'], text=True).strip()


def integrate(w, base, head):
    return subprocess.check_output(
        ['git', '-C', str(w), 'commit-tree', tree(w, base),
         '-p', sha(w, base), '-p', sha(w, head),
         '-m', 'adopt integration sim (dangling, never pushed)'],
        text=True).strip()


def run(base, head):
    diffbase = load('diffbase', BASIS / 'scripts' / 'pr-diff-base.py')
    planner = load('planner', BASIS / 'scripts' / 'pr-gate-plan.py')
    reqs = json.loads((BASIS / 'scripts' / 'release-requirements.json').read_text())
    mp = json.loads((BASIS / 'scripts' / 'pr-gate-map-v1.json').read_text())
    b = sha(BASIS, base)
    h = sha(BASIS, head)
    integ = integrate(BASIS, base, head)
    names = subprocess.check_output(
        ['git', '-C', str(BASIS), 'diff', '--name-status', f'{base}..{head}'],
        text=True).splitlines()
    changes = [{'status': n.split()[0], 'path': n.split()[1]}
               for n in names if n.strip()]
    diff = diffbase.resolve_pr_diff(str(BASIS), b, h, integ)
    diff = {**diff, 'changes': changes, 'broad_required': False}
    p = planner.plan(diff, reqs, mp, repository=str(BASIS),
                     context_repository='mcpdev80/baseharbor',
                     context_target_branch='develop')
    return p, integ, changes


if __name__ == '__main__':
    base = '8f8e5547'
    head = '8bcdd527'
    p, integ, changes = run(base, head)
    print('ADOPT-variant: base=%s head=%s' % (base, head))
    print('  integration:', integ[:12])
    print('  diff paths :', [c['path'] + ':' + c['status'] for c in changes])
    print('  coverage_status :', p['coverage_status'])
    print('  unresolved:', json.dumps(p['unresolved_coverage']))
    print('  selected_gates:', p['selected_gates'])
    print('  broad_required:', p['broad_required'])

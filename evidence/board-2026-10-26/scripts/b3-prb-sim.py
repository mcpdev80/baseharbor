#!/usr/bin/env python3
"""PR-B self-test: does the develop (base-copy) planner classify the PR-B diff as resolved?"""
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


def plan(diff, reqs, mp):
    planner = load('planner', BASIS / 'scripts' / 'pr-gate-plan.py')
    return planner.plan(diff, reqs, mp, repository=str(BASIS),
                        context_repository='mcpdev80/baseharbor',
                        context_target_branch='develop')


def main():
    diffbase = load('diffbase', BASIS / 'scripts' / 'pr-diff-base.py')
    reqs = json.loads((BASIS / 'scripts' / 'release-requirements.json').read_text())
    mp = json.loads((BASIS / 'scripts' / 'pr-gate-map-v1.json').read_text())
    target = sha(BASIS, 'develop')
    head = sha(BASIS, '8f8e5547')
    # real 2-parent integration merge of develop..8f8e5547
    integ = subprocess.check_output(
        ['git', '-C', str(BASIS), 'commit-tree',
         git_tree(BASIS, 'develop'), '-p', target, '-p', head,
         '-m', 'PR-B self-test integration sim (dangling, never pushed)'],
        text=True).strip()

    names = subprocess.check_output(
        ['git', '-C', str(BASIS), 'diff', '--name-status', 'develop..8f8e5547'],
        text=True).splitlines()
    changes = [{'status': n.split()[0], 'path': n.split()[1]}
               for n in names if n.strip()]
    diff = diffbase.resolve_pr_diff(str(BASIS), target, head, integ)
    diff = {**diff, 'changes': changes, 'broad_required': False}
    p = plan(diff, reqs, mp)
    print('PR-B self-test (develop base copy)')
    print('  integration:', integ[:12])
    print('  coverage_status :', p['coverage_status'])
    print('  unresolved:', json.dumps(p['unresolved_coverage']))
    print('  selected_gates:', p['selected_gates'])
    print('  broad_required:', p['broad_required'])
    return p


def git_tree(wt, rev):
    return subprocess.check_output(['git', '-C', str(wt), 'rev-parse',
                                    rev + '^{tree}'], text=True).strip()


if __name__ == '__main__':
    main()

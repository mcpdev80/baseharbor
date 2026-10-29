#!/usr/bin/env python3
"""Repro for the #1007 binding-PR trusted-base policy clause (Codex ruling 3a).

Shows BOTH outcomes the clause must produce using two independent repos:

  CASE 1  Fail-closed: with the base development line that carries NO policy
          record (i.e. the current PR branch before the prerequisite policy PR
          lands), the candidate head is left UNRESOLVED.
  CASE 2  Resolved: once the reviewed record is loaded from the authenticated
          base (a `develop` that carries `scripts/ci-binding-qualifications.json`
          on the target branch), the head equals the frozen reviewed candidate
          and the clause resolves.

The record lives only on the trusted target base, never on the candidate head.
"""
import hashlib
import importlib.util
import json
import os
import subprocess
import sys
import tempfile
from pathlib import Path

REPO = Path(sys.argv[1]) if len(sys.argv) > 1 else Path(__file__).parent
spec = importlib.util.spec_from_file_location('planner', REPO / 'pr-gate-plan.py')
planner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(planner)

ROOT_PLANNER = REPO
RECORD_PATH = 'scripts/ci-binding-qualifications.json'


def git(root, *args):
    res = subprocess.run(['git', '-C', str(root), *args],
                         text=True, capture_output=True)
    assert res.returncode == 0, res.stderr
    return res.stdout.strip()


def build(root):
    """develop at baseline (no record); candidate head H on its own branch.

    Returns (target, head) where target = develop tip and head = candidate H.
    """
    rspec = importlib.util.spec_from_file_location('pd', REPO / 'pr-diff-base.py')
    resolver = importlib.util.module_from_spec(rspec)
    rspec.loader.exec_module(resolver)
    git(root, 'init', '-q')
    git(root, 'config', 'user.name', 'Binding repro')
    git(root, 'config', 'user.email', 'repro@example.test')
    for path in planner.BINDING_PR_1007_REVIEWED_FILES:
        (root / path).parent.mkdir(parents=True, exist_ok=True)
        (root / path).write_text('baseline\n')
    git(root, 'add', '.')
    git(root, 'commit', '--allow-empty', '-qm', 'develop baseline')
    git(root, 'branch', 'develop')
    target = git(root, 'rev-parse', 'develop')
    git(root, 'checkout', '-q', '-b', 'candidate')
    for path in planner.BINDING_PR_1007_REVIEWED_FILES:
        (root / path).write_text('reviewed\n')
    git(root, 'add', '.')
    git(root, 'commit', '-qm', 'binding linear head H')
    head = git(root, 'rev-parse', 'HEAD')
    return target, head, root, resolver


def record(root, head):
    hashes = {path: hashlib.sha256(
        subprocess.run(['git', '-C', str(root), 'cat-file', '-p', head + ':' + path],
                       capture_output=True).stdout).hexdigest()
        for path in planner.BINDING_PR_1007_REVIEWED_FILES}
    return {
        'repository': 'mcpdev80/baseharbor',
        'target_branch': 'develop',
        'reviewed_head': head,
        'files': {path: 'modified' for path in planner.BINDING_PR_1007_REVIEWED_FILES},
        'file_hashes': hashes,
        'required_checks': [c['id'] for c in planner.CI_CHECKS],
    }


def resolve(resolver, root, target, head):
    tree = git(root, 'merge-tree', '--write-tree', target, head).splitlines()[0]
    integrated = git(root, 'commit-tree', tree, '-p', target, '-p', head,
                     '-m', 'integration')
    return resolver.resolve_pr_diff(root, target, head, integrated)


def run_plan(root, actual, REQ, MAP):
    prev = os.getcwd()
    os.chdir(str(root))
    try:
        return planner.plan(actual, REQ, MAP)
    finally:
        os.chdir(prev)


def main():
    GATES = ['integration/static/public-contracts', 'atomic/docker/lifecycle',
             'atomic/podman/lifecycle', 'journey/docker', 'journey/podman',
             'ha/docker/data']
    REQ = {'releases': ['v0.4.26'],
           'gates': [{'id': g, 'required_for': ['v0.4.26']} for g in GATES]}
    MAP = json.loads((ROOT_PLANNER / 'pr-gate-map-v1.json').read_text())

    # --- CASE 1: fail-closed, record absent from the base (current PR) ---
    tmp1 = Path(tempfile.mkdtemp())
    target, head, root, resolver = build(tmp1)
    actual = resolve(resolver, root, target, head)
    planned = run_plan(root, actual, REQ, MAP)
    print('CASE 1 (record absent from base -> current PR branch):')
    print('  coverage_status =', planned['coverage_status'])
    print('  reasons =', planned['reasons'])
    assert planned['coverage_status'] == 'unresolved'

    # --- CASE 2: resolved once the reviewed record lands on the trusted base ---
    tmp2 = Path(tempfile.mkdtemp())
    target, head, root, resolver = build(tmp2)
    # Add the reviewed record on the develop (target) branch, never on candidate.
    rec = record(root, head)
    git(root, 'checkout', '-q', 'develop')
    (root / RECORD_PATH).write_text(json.dumps(rec))
    git(root, 'add', '.')
    git(root, 'commit', '-qm', 'prerequisite policy: pin reviewed binding #1007')
    target_with_record = git(root, 'rev-parse', 'develop')
    assert head == git(root, 'rev-parse', 'candidate')
    actual = resolve(resolver, root, target_with_record, head)
    planned = run_plan(root, actual, REQ, MAP)
    print('CASE 2 (record loaded from authenticated base):')
    print('  coverage_status =', planned['coverage_status'])
    print('  reasons =', planned['reasons'])
    assert planned['coverage_status'] == 'resolved', planned
    print('\nBOTH CASES OK: fail-closed unresolved when base has no record, '
          'resolved once the prerequisite policy PR lands on develop.')


if __name__ == '__main__':
    main()

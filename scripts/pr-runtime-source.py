#!/usr/bin/env python3
"""Bind native PR follow-ups to the planner's exact integration source."""
import argparse
import re
import subprocess
from pathlib import Path


def git(repository, *args):
    result = subprocess.run(['git', '-C', str(repository), *args], text=True,
                            stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    if result.returncode:
        raise ValueError('runtime source check failed: git ' + args[0])
    return result.stdout.strip()


def verify(repository, event_base, base, head, candidate):
    for value in (event_base, base, head, candidate):
        if not re.fullmatch('[0-9a-f]{40}', value or ''):
            raise ValueError('runtime source requires complete commit identities')
    if git(repository, 'rev-parse', 'HEAD') != candidate:
        raise ValueError('runtime checkout differs from the selected candidate')
    git(repository, 'fetch', '--no-tags', 'origin', event_base, base, head, candidate,
        '+refs/heads/develop:refs/remotes/origin/runtime-target-tip')
    git(repository, 'merge-base', '--is-ancestor', event_base, base)
    git(repository, 'merge-base', '--is-ancestor', base, 'refs/remotes/origin/runtime-target-tip')
    parents = git(repository, 'rev-list', '--parents', '-n', '1', candidate).split()[1:]
    if len(parents) == 2 and parents == [base, head]:
        return
    if len(parents) == 1 and candidate == head:
        git(repository, 'merge-base', '--is-ancestor', base, head)
        return
    raise ValueError('runtime candidate does not have the selected base and PR head')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--repository', type=Path, required=True)
    parser.add_argument('--event-base', required=True)
    parser.add_argument('--base', required=True)
    parser.add_argument('--head', required=True)
    parser.add_argument('--candidate', required=True)
    args = parser.parse_args()
    try:
        verify(args.repository, args.event_base, args.base, args.head, args.candidate)
    except ValueError as exc:
        raise SystemExit(str(exc)) from exc


if __name__ == '__main__':
    main()

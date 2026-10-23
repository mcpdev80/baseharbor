#!/usr/bin/env python3
"""Normalize a targeted atomic-gate dispatch ref into a checkout-fetchable ref.

actions/checkout v7 classifies its ``ref`` input in ``src/input-helper.ts``: only a full
40/64-hex SHA is fetched as a commit; every other value is fetched via the refspec
``+refs/heads/<value>*`` / ``+refs/tags/<value>*``. A bare (short) commit SHA therefore
matches no ref and fails with silent retry back-offs, and a malformed or nonexistent full
SHA can drop into the same branch-prefix fallback. Both broke the Atomic Acceptance Gate
(run 39856259550; analyses ``o33-atomic-gate-c2.md`` and ``o26-selected-checks.md``).

This resolver runs in the gate's ``resolve`` job BEFORE any candidate checkout so the raw
dispatch input never reaches ``actions/checkout`` unresolved. On success it prints a
checkout-safe ref; on failure it fails fast with an explicit ``::error::`` message.
"""
import argparse
import json
import os
import re
import subprocess
import sys

FULL_SHA = re.compile(r'^(?:[0-9a-fA-F]{40}|[0-9a-fA-F]{64})$')
SHORT_HEX = re.compile(r'^[0-9a-fA-F]{7,39}$')
RECIPE_MESSAGE = ("candidate ref '{ref}' is not a branch/tag; resolve to a ref or a full "
                  'commit SHA before the gate checkout')


class RefResolver:
    def __init__(self, gh, repo):
        self.gh = gh
        self.repo = repo

    def api(self, endpoint):
        result = subprocess.run([self.gh, 'api', endpoint], text=True,
                                capture_output=True, check=False)
        if result.returncode != 0:
            return None
        try:
            return json.loads(result.stdout)
        except json.JSONDecodeError:
            return None

    def branch(self, name):
        return self.api(f'repos/{self.repo}/git/ref/heads/{name}') is not None

    def tag(self, name):
        return self.api(f'repos/{self.repo}/git/ref/tags/{name}') is not None

    def commit_object(self, sha):
        payload = self.api(f'repos/{self.repo}/git/commits/{sha}')
        return isinstance(payload, dict) and payload.get('sha', '').lower() == sha.lower()

    def expand(self, abbreviated):
        payload = self.api(f'repos/{self.repo}/commits/{abbreviated}')
        sha = payload.get('sha', '') if isinstance(payload, dict) else ''
        if FULL_SHA.match(sha or '') and sha.lower().startswith(abbreviated.lower()):
            return sha
        return None


def resolve(ref, resolver):
    if not ref.strip():
        raise ValueError('dispatch ref is empty; pass a branch/tag name, a refs/... path, or a full commit SHA')
    ref = ref.strip()
    if ref.startswith('refs/'):
        return ref
    if FULL_SHA.match(ref):
        if not resolver.commit_object(ref):
            raise ValueError(f"{RECIPE_MESSAGE.format(ref=ref)}: the commit does not exist in {resolver.repo} "
                             'and checkout would fall back to a branch refspec')
        return ref
    if SHORT_HEX.match(ref):
        if resolver.branch(ref):
            return f'refs/heads/{ref}'
        if resolver.tag(ref):
            return f'refs/tags/{ref}'
        expanded = resolver.expand(ref)
        if expanded:
            return expanded
        raise ValueError(f"{RECIPE_MESSAGE.format(ref=ref)}: '{ref}' looks like a short commit SHA "
                         'but is neither a branch/tag nor a resolvable commit')
    if resolver.branch(ref):
        return f'refs/heads/{ref}'
    if resolver.tag(ref):
        return f'refs/tags/{ref}'
    raise ValueError(f"{RECIPE_MESSAGE.format(ref=ref)}: no branch or tag with that name exists in {resolver.repo}")


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument('--ref', required=True, help='raw dispatch ref input')
    parser.add_argument('--repo', default=os.environ.get('GITHUB_REPOSITORY', ''),
                        help='owner/name to resolve against (default: $GITHUB_REPOSITORY)')
    parser.add_argument('--gh', default=os.environ.get('TARGETED_GATE_REF_GH', 'gh'),
                        help='gh executable (test seam)')
    args = parser.parse_args(argv)
    if not args.repo:
        print("::error::no repository set: pass --repo or run where GITHUB_REPOSITORY is set", file=sys.stderr)
        return 2
    try:
        print(resolve(args.ref, RefResolver(args.gh, args.repo)))
    except ValueError as error:
        print(f'::error::{error}', file=sys.stderr)
        return 2
    return 0


if __name__ == '__main__':
    sys.exit(main())

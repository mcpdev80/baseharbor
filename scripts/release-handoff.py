#!/usr/bin/env python3
"""Reject product changes during a frozen candidate's metadata-only promotion."""
import argparse
from pathlib import Path
import re
import subprocess


def validate_handoff(repository, candidate, published, tag):
    if (not re.fullmatch(r'v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?', tag)
            or any(not re.fullmatch('[0-9a-f]{40}', sha) for sha in (candidate, published))):
        raise ValueError('handoff requires a release tag and exact commit SHAs')
    allowed = {'CHANGELOG.md', 'docs/roadmap.md', 'docs/releases/index.md',
               f'docs/releases/{tag}.md', f'docs/internal/release-audits/{tag}.md'}
    changed = subprocess.check_output(
        ['git', 'diff', '--name-only', '-z', candidate, published, '--'],
        cwd=repository).decode().split('\0')
    unexpected = [path for path in changed if path and path not in allowed]
    if unexpected:
        raise ValueError('handoff changes execution inputs: ' + ', '.join(unexpected))
    return [path for path in changed if path]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--candidate', required=True)
    parser.add_argument('--published', required=True)
    parser.add_argument('--tag', required=True)
    args = parser.parse_args()
    try:
        validate_handoff(Path.cwd(), args.candidate, args.published, args.tag)
    except (ValueError, subprocess.CalledProcessError) as exc:
        parser.exit(1, str(exc) + '\n')


if __name__ == '__main__':
    main()

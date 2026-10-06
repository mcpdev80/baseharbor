#!/usr/bin/env python3
"""Validate the v0.4.23 publication handoff without changing execution inputs."""
import argparse
import importlib.util
import pathlib
import re
import subprocess



def validate_handoff(repository, candidate, published, tag):
    if tag != 'v0.4.23' or any(not re.fullmatch('[0-9a-f]{40}', sha) for sha in [candidate, published]):
        raise ValueError('handoff requires the supported release and exact commit SHAs')
    allowed = {'CHANGELOG.md', 'docs/roadmap.md', 'docs/releases/index.md',
               f'docs/releases/{tag}.md', f'docs/internal/release-audits/{tag}.md'}
    # NUL delimiters preserve filenames; renames/deletions do not bypass the list.
    changed = subprocess.check_output(['git', 'diff', '--name-only', '-z', candidate, published, '--'],
                                      cwd=repository).decode().split('\0')
    unexpected = [path for path in changed if path and path not in allowed]
    if unexpected:
        raise ValueError('handoff changes execution inputs: ' + ', '.join(unexpected))
    return [path for path in changed if path]


def requirements(repository, candidate, tag):
    spec = importlib.util.spec_from_file_location('resume', pathlib.Path(__file__).with_name('pre-release-resume.py'))
    resume = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(resume)
    raw = subprocess.check_output(['git', 'show', candidate + ':' + resume.REQUIREMENTS_PATH], cwd=repository)
    return resume.load_requirements(raw, tag)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('mode', choices=['validate', 'count'])
    parser.add_argument('--candidate', required=True)
    parser.add_argument('--published')
    parser.add_argument('--tag', required=True)
    args = parser.parse_args()
    if args.mode == 'validate':
        validate_handoff(pathlib.Path.cwd(), args.candidate, args.published or '', args.tag)
    else:
        print(len(requirements(pathlib.Path.cwd(), args.candidate, args.tag)))


if __name__ == '__main__':
    try:
        main()
    except (ValueError, OSError, subprocess.CalledProcessError) as error:
        raise SystemExit(f'release handoff rejected: {error}')

#!/usr/bin/env python3
"""Validate publication metadata while preserving approved product inputs."""
import argparse
import importlib.util
import pathlib
import re
import subprocess

V024_CANDIDATE = '999838411b3505713d0054d15f3693158147c2e6'
V024_RECOVERY_RUN = 38092223770
V024_RECOVERY_WORKFLOW = 'c98132ce7da2f938bb3963d2f1be66bb35e844a0'
# These exact collector blobs passed the recovery workflow's regressions and
# independently authenticated every original proof. No arbitrary tool delta is allowed.
V024_VERIFIED_COLLECTOR = {
    'scripts/pre-release-resume.py': '3525f680882c3634d34bd0bda7e82649b416103b',
    'scripts/test_pre_release_resume.py': '8b07f749e65d19fae4f512ef39cfcfe27a52b2ef',
}
V024_REVIEWED_DOCUMENTATION = {
    'README.md', 'docs/index.md', 'docs/de/index.md', 'docs/de/roadmap.md',
    'docs/de/releases/index.md', 'docs/releases/v0.4.23.md', 'docs/de/releases/v0.4.23.md',
    'docs/de/releases/v0.4.24.md', 'docs/reference/releases.md', 'docs/de/reference/releases.md',
    'docs/reference/cli-machine-coverage.md', 'docs/de/reference/cli-machine-coverage.md',
    'docs/how-to/developer-access.md', 'docs/de/how-to/developer-access.md',
    'docs/how-to/postgres.md', 'docs/de/how-to/postgres.md',
    'docs/how-to/v0.4.24-package-integration.md', 'docs/de/how-to/v0.4.24-package-integration.md',
    'docs/de/architecture/runtime-standards-audit.md',
    'docs/cli/core.md', 'docs/de/cli/core.md',
    'docs/cli/human-workflows.md', 'docs/de/cli/human-workflows.md',
}


def candidate_for_workflow(tag, workflow_sha, run_id):
    if (tag == 'v0.4.24' and run_id == V024_RECOVERY_RUN and
            workflow_sha == V024_RECOVERY_WORKFLOW):
        return V024_CANDIDATE
    return workflow_sha


def validate_handoff(repository, candidate, published, tag):
    if not re.fullmatch(r'v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?', tag) or any(not re.fullmatch('[0-9a-f]{40}', sha) for sha in [candidate, published]):
        raise ValueError('handoff requires a release tag and exact commit SHAs')
    allowed = {'CHANGELOG.md', 'docs/roadmap.md', 'docs/releases/index.md',
               f'docs/releases/{tag}.md', f'docs/internal/release-audits/{tag}.md'}
    if tag == 'v0.4.24' and candidate == V024_CANDIDATE:
        allowed |= V024_REVIEWED_DOCUMENTATION
        # Reviewed publication control only; runtime workflows, schemas,
        # authorization, inventories, provider pins and shipped code remain exact.
        allowed |= {'.github/workflows/release.yml', 'scripts/release-handoff.py',
                    'scripts/test_release_handoff.py'}
        for path, blob in V024_VERIFIED_COLLECTOR.items():
            entry = subprocess.check_output(['git', 'ls-tree', published, '--', path],
                                            cwd=repository).decode().split()
            original = subprocess.check_output(['git', 'ls-tree', candidate, '--', path],
                                               cwd=repository).decode().split()
            if len(entry) != 4 or len(original) != 4 or entry[:2] != original[:2] or entry[2] != blob:
                raise ValueError('publication collector differs from verified recovery: ' + path)
            allowed.add(path)
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
    parser.add_argument('mode', choices=['validate', 'count', 'candidate'])
    parser.add_argument('--candidate', required=True)
    parser.add_argument('--published')
    parser.add_argument('--tag', required=True)
    parser.add_argument('--run-id', type=int, default=0)
    args = parser.parse_args()
    if args.mode == 'validate':
        validate_handoff(pathlib.Path.cwd(), args.candidate, args.published or '', args.tag)
    elif args.mode == 'count':
        print(len(requirements(pathlib.Path.cwd(), args.candidate, args.tag)))
    else:
        if not re.fullmatch('[0-9a-f]{40}', args.candidate):
            raise ValueError('immutable workflow SHA required')
        print(candidate_for_workflow(args.tag, args.candidate, args.run_id))


if __name__ == '__main__':
    try:
        main()
    except (ValueError, OSError, subprocess.CalledProcessError) as error:
        raise SystemExit(f'release handoff rejected: {error}')

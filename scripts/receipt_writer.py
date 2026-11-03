#!/usr/bin/env python3
"""Publish one merged commit SHA and PR number to Planning."""
import argparse
import base64
import json
import os
import re
import subprocess
from pathlib import Path

MAX_PUSH_RETRIES = 3
PLANNING_URL = 'https://github.com/mcpdev80/baseharbor-plan.git'


class ReceiptError(ValueError):
    pass


def require(condition, message):
    if not condition:
        raise ReceiptError(message)


def full_sha(value):
    return isinstance(value, str) and re.fullmatch('[0-9a-f]{40}', value) is not None


def canonical(value):
    return (json.dumps(value, sort_keys=True, separators=(',', ':')) + '\n').encode()


def git(root, *args):
    result = subprocess.run(['git', '-C', str(root), *args], capture_output=True,
                            env=dict(os.environ, GIT_TERMINAL_PROMPT='0'))
    require(result.returncode == 0, 'Git operation failed: ' + args[0])
    return result.stdout


def _check_commit_record(raw):
    value = json.loads(raw)
    require(isinstance(value, dict) and set(value) == {'commit', 'pull_request'}
            and full_sha(value['commit']) and type(value['pull_request']) is int
            and value['pull_request'] > 0 and raw == canonical(value),
            'invalid canonical commit receipt')
    return value


def commit_record(pr_number, api):
    """Read merged PR identity only; CI results are never queried."""
    require(type(pr_number) is int and pr_number > 0, 'invalid pull request')
    pr = api(pr_number)
    require(pr.get('number') == pr_number and pr.get('state') == 'closed'
            and pr.get('merged') is True and (pr.get('base') or {}).get('ref') == 'develop'
            and ((pr.get('base') or {}).get('repo') or {}).get('id') == 1360667669
            and (pr.get('merged_by') or {}).get('id') == 22537373,
            'authenticated Owner merge into develop required')
    require(full_sha(pr.get('merge_commit_sha')), 'invalid merge commit')
    return {'commit': pr['merge_commit_sha'], 'pull_request': pr_number}


def _record_path(raw):
    value = _check_commit_record(raw)
    return 'evidence/receipts/commits/' + value['commit'] + '.json'


def commit_file(root, path, raw):
    require(path == _record_path(raw), 'receipt path differs from commit')
    require(not git(root, 'status', '--porcelain'), 'clean Planning checkout required')
    target = root / path
    require(not target.is_symlink() and target.parent.resolve().is_relative_to(root.resolve()),
            'receipt path escapes Planning')
    if target.exists():
        require(target.is_file() and target.read_bytes() == raw, 'conflicting existing receipt')
        return git(root, 'rev-parse', 'HEAD').decode().strip()
    target.parent.mkdir(parents=True, exist_ok=True)
    with target.open('xb') as stream:
        stream.write(raw)
    git(root, 'add', '--', path)
    git(root, 'commit', '-m', 'receipt: record merged commit', '--', path)
    return git(root, 'rev-parse', 'HEAD').decode().strip()


def _push_authed(root, args):
    """Supply Basic auth only to the bounded Planning push subprocess."""
    remote = git(root, 'remote', 'get-url', '--push', 'origin').decode().strip()
    env = dict(os.environ, GIT_TERMINAL_PROMPT='0')
    if remote.startswith(('https://', 'http://', 'ssh://', 'git@')):
        require(remote.rstrip('/') in (PLANNING_URL, PLANNING_URL[:-4]),
                'unexpected Planning publication destination')
        token = os.environ.get('PLANNING_RECEIPT_WRITE_TOKEN', '')
        require(bool(token), 'PLANNING_RECEIPT_WRITE_TOKEN required; publication blocked')
        credential = base64.b64encode(('x-access-token:' + token).encode()).decode()
        env.update(GIT_CONFIG_COUNT='1',
                   GIT_CONFIG_KEY_0='http.https://github.com/.extraheader',
                   GIT_CONFIG_VALUE_0='AUTHORIZATION: basic ' + credential)
    # Do not expose downstream stderr: it can contain a token or derived header.
    result = subprocess.run(['git', '-C', str(root), *args], capture_output=True, env=env)
    return result.returncode


def _remote_record(root, path):
    entry = git(root, 'ls-tree', 'refs/remotes/origin/main', '--', path)
    if not entry:
        return None
    require(entry.startswith(b'100644 blob '), 'remote receipt is not a regular file')
    raw = git(root, 'show', 'refs/remotes/origin/main:' + path)
    _check_commit_record(raw)
    return raw


def publish(root, path, local_commit):
    """Retry fast-forward pushes while preserving unrelated concurrent commits."""
    require(not git(root, 'status', '--porcelain'), 'clean Planning checkout required')
    require(git(root, 'rev-parse', 'HEAD').decode().strip() == local_commit,
            'Planning checkout moved before publication')
    raw = (root / path).read_bytes()
    require(path == _record_path(raw), 'receipt path differs from commit')
    for _ in range(MAX_PUSH_RETRIES):
        git(root, 'fetch', '--no-tags', 'origin',
            '+refs/heads/main:refs/remotes/origin/main')
        remote_commit = git(root, 'rev-parse', 'refs/remotes/origin/main').decode().strip()
        existing = _remote_record(root, path)
        if existing is not None:
            require(existing == raw, 'conflicting receipt on remote')
            return path, remote_commit
        require(git(root, 'rev-parse', 'HEAD').decode().strip() == local_commit,
                'Planning checkout moved during publication')
        # Replay only our own single-file commit, never unpublished local work.
        require(git(root, 'diff-tree', '--no-commit-id', '--name-only', '-r',
                    local_commit).decode().splitlines() == [path],
                'unrelated local commit blocks receipt publication')
        git(root, 'merge-base', '--is-ancestor', local_commit + '^', remote_commit)
        # Remote unrelated files remain intact.
        require(not git(root, 'status', '--porcelain'), 'clean Planning checkout required')
        git(root, 'reset', '--hard', remote_commit)
        local_commit = commit_file(root, path, raw)
        if _push_authed(root, ('push', 'origin', local_commit + ':refs/heads/main')) == 0:
            return path, local_commit
    raise ReceiptError('Planning publication push failed after bounded retries')


def publish_commit(root, product, pr_number, api):
    value = commit_record(pr_number, api)
    sha = value['commit']
    git(product, 'cat-file', '-e', sha + '^{commit}')
    git(product, 'merge-base', '--is-ancestor', sha, 'refs/remotes/origin/develop')
    raw = canonical(value)
    path = _record_path(raw)
    return publish(root, path, commit_file(root, path, raw))


def github_pr(pr_number):
    result = subprocess.run(['gh', 'api', f'repos/mcpdev80/baseharbor/pulls/{pr_number}'],
                            capture_output=True, check=False)
    require(result.returncode == 0, 'cannot authenticate pull request')
    value = json.loads(result.stdout)
    require(isinstance(value, dict), 'invalid pull request response')
    return value


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--planning', type=Path, required=True)
    parser.add_argument('--product', type=Path, required=True)
    parser.add_argument('--commit-pr', type=int, required=True)
    parser.add_argument('--publish', action='store_true', required=True)
    args = parser.parse_args()
    try:
        require(os.environ.get('GITHUB_EVENT_NAME') in {'pull_request_target', 'workflow_dispatch'}
                and os.environ.get('GITHUB_REPOSITORY') == 'mcpdev80/baseharbor'
                and os.environ.get('GITHUB_REF') == 'refs/heads/main',
                'trusted merged-PR action on main required')
        print('receipt source: ' + git(args.product, 'rev-parse', 'HEAD').decode().strip())
        path, commit = publish_commit(args.planning, args.product, args.commit_pr, github_pr)
        print(f'receipt: {commit}:{path}')
    except (ReceiptError, OSError, TypeError, ValueError) as exc:
        parser.exit(1, 'receipt blocked: ' + str(exc) + '\n')


if __name__ == '__main__':
    main()

#!/usr/bin/env python3
"""Resolve the real pull-request diff base without selecting CI gates."""
import argparse
import json
import pathlib
import subprocess


def git(repository, *args, check=True):
    return subprocess.run(
        ['git', '-C', str(repository), *args],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        check=check,
    )


def resolve_commit(repository, revision):
    if not revision:
        return None
    result = git(repository, 'rev-parse', '--verify', revision + '^{commit}', check=False)
    if result.returncode != 0:
        return None
    return result.stdout.strip()


def fallback(result, code, message):
    result['broad_required'] = True
    result['reasons'].append({'code': code, 'message': message})
    result['changes'] = []
    return result


def integration_conflicts(repository, target_base, head):
    result = git(repository, 'merge-tree', '--write-tree', target_base, head, check=False)
    if result.returncode == 0:
        return False, result.stdout.strip().splitlines()[0]
    if result.returncode == 1:
        detail = (result.stderr or result.stdout).strip().splitlines()
        suffix = ': ' + detail[-1] if detail else ''
        return True, 'target base and head do not merge cleanly' + suffix
    detail = (result.stderr or result.stdout).strip().splitlines()
    suffix = ': ' + detail[-1] if detail else ''
    return None, 'git merge-tree could not verify integration safety' + suffix


def is_ancestor(repository, ancestor, descendant):
    return git(repository, 'merge-base', '--is-ancestor', ancestor, descendant,
               check=False).returncode == 0


def parse_name_status(raw):
    fields = raw.split('\0')
    if fields and fields[-1] == '':
        fields.pop()
    changes = []
    index = 0
    while index < len(fields):
        status = fields[index]
        index += 1
        if status.startswith(('R', 'C')):
            if index + 1 >= len(fields):
                raise ValueError('truncated rename/copy record from git diff')
            old_path, new_path = fields[index:index + 2]
            index += 2
            changes.append({
                'status': 'renamed' if status.startswith('R') else 'copied',
                'score': int(status[1:]) if status[1:].isdigit() else None,
                'old_path': old_path,
                'new_path': new_path,
            })
            continue
        if index >= len(fields):
            raise ValueError('truncated path record from git diff')
        path = fields[index]
        index += 1
        names = {
            'A': 'added', 'D': 'deleted', 'M': 'modified', 'T': 'type_changed',
            'U': 'unmerged', 'X': 'unknown', 'B': 'broken_pair',
        }
        changes.append({'status': names.get(status[0], status), 'path': path})
    return changes


def resolve_pr_diff(repository, target_base, head, integration=None):
    repository = pathlib.Path(repository)
    result = {
        'target_base_sha': None,
        'head_sha': None,
        'integration_sha': None,
        'merge_base_sha': None,
        'broad_required': False,
        'reasons': [],
        'changes': [],
    }

    target_sha = resolve_commit(repository, target_base)
    head_sha = resolve_commit(repository, head)
    integration_sha = resolve_commit(repository, integration) if integration else None
    result['target_base_sha'] = target_sha
    result['head_sha'] = head_sha
    result['integration_sha'] = integration_sha

    if target_sha is None:
        return fallback(result, 'missing_target_base_history',
                        f'target base {target_base!r} is not available as a local commit')
    if head_sha is None:
        return fallback(result, 'missing_head_history',
                        f'head {head!r} is not available as a local commit')
    if integration and integration_sha is None:
        return fallback(result, 'missing_integration_history',
                        f'integration {integration!r} is not available as a local commit')

    merge_bases = [line for line in git(
        repository, 'merge-base', '--all', target_sha, head_sha, check=False
    ).stdout.splitlines() if line]
    if not merge_bases:
        return fallback(result, 'no_merge_base',
                        'target base and head have no locally provable common ancestor')
    if len(merge_bases) != 1:
        return fallback(result, 'ambiguous_merge_base',
                        'target base and head have multiple best merge bases: ' + ', '.join(merge_bases))
    result['merge_base_sha'] = merge_bases[0]

    conflict, conflict_message = integration_conflicts(repository, target_sha, head_sha)
    if conflict is True:
        return fallback(result, 'integration_conflict', conflict_message)
    if conflict is None:
        return fallback(result, 'integration_check_unavailable', conflict_message)

    if integration_sha:
        # An ancestor-only check would also accept arbitrary later commits.
        # The synthetic integration must merge exactly these two inputs,
        # with precisely the tree Git would produce for a clean merge.
        commit = git(repository, 'rev-list', '--parents', '-n', '1',
                     integration_sha, check=False)
        if commit.returncode != 0:
            return fallback(result, 'integration_check_unavailable',
                            'cannot inspect integration commit parents')
        parents = commit.stdout.strip().split()
        if parents != [integration_sha, target_sha, head_sha]:
            return fallback(result, 'integration_sha_mismatch',
                            'integration commit must have exactly target base and PR head '
                            'as its first and second parents, with no extra parents or commits')
        actual_tree = git(repository, 'rev-parse', '--verify',
                          integration_sha + '^{tree}', check=False)
        if actual_tree.returncode != 0:
            return fallback(result, 'integration_check_unavailable',
                            'cannot inspect integration commit tree')
        if actual_tree.stdout.strip() != conflict_message:
            return fallback(result, 'integration_tree_mismatch',
                            'integration commit tree differs from the clean merge '
                            'of the exact target base and PR head')

    diff = git(repository, 'diff', '--name-status', '-z', '-M', '--no-ext-diff',
               result['merge_base_sha'], head_sha, '--', check=False)
    if diff.returncode != 0:
        detail = diff.stderr.strip() or 'git diff failed'
        return fallback(result, 'diff_failed', detail)
    try:
        result['changes'] = parse_name_status(diff.stdout)
    except ValueError as error:
        return fallback(result, 'diff_parse_failed', str(error))
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--repository', type=pathlib.Path, default=pathlib.Path.cwd())
    parser.add_argument('--target-base', required=True,
                        help='Target branch commit SHA from the PR event')
    parser.add_argument('--head', required=True, help='PR head commit SHA from the PR event')
    parser.add_argument('--integration',
                        help='Integration/synthetic merge commit SHA, when supplied by CI')
    args = parser.parse_args()
    print(json.dumps(resolve_pr_diff(args.repository, args.target_base, args.head,
                                     args.integration), indent=2, sort_keys=True))


if __name__ == '__main__':
    main()

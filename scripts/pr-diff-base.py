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


def resolve_pr_diff(repository, target_base, head, integration=None, base_tip_sha=None):
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
    event_base_sha = target_sha
    head_sha = resolve_commit(repository, head)
    integration_sha = resolve_commit(repository, integration) if integration else None
    base_tip = resolve_commit(repository, base_tip_sha) if base_tip_sha else None
    result['target_base_sha'] = target_sha
    result['event_base_sha'] = event_base_sha
    result['head_sha'] = head_sha
    result['integration_sha'] = integration_sha
    result['base_tip_sha'] = base_tip

    if target_sha is None:
        return fallback(result, 'missing_target_base_history',
                        f'target base {target_base!r} is not available as a local commit')
    if head_sha is None:
        return fallback(result, 'missing_head_history',
                        f'head {head!r} is not available as a local commit')
    if integration and integration_sha is None:
        return fallback(result, 'missing_integration_history',
                        f'integration {integration!r} is not available as a local commit')
    if base_tip_sha and base_tip is None:
        return fallback(result, 'missing_target_tip_history',
                        f'target branch tip {base_tip_sha!r} is not available as a local commit')

    # The PR event can predate the synthetic merge SHA GitHub actually gave
    # this run. Never validate that newer tree against the stale event base.
    # Accept the merge's other parent only when the event base and a freshly
    # fetched target tip prove the complete forward lineage. Its tree is still
    # checked against an independently computed clean merge below.
    if integration_sha:
        parents_result = git(repository, 'rev-list', '--parents', '-n', '1',
                             integration_sha, check=False)
        parts = parents_result.stdout.strip().split()
        if parents_result.returncode == 0 and len(parts) == 3 and head_sha in parts[1:]:
            actual_base = parts[2] if parts[1] == head_sha else parts[1]
            if actual_base != target_sha:
                if base_tip is None:
                    return fallback(result, 'integration_base_unverified',
                                    'advanced integration base requires a fetched target branch tip')
                if not is_ancestor(repository, event_base_sha, actual_base):
                    return fallback(result, 'integration_sha_mismatch',
                                    'integration base must descend from the event base')
                if not is_ancestor(repository, actual_base, base_tip):
                    return fallback(result, 'integration_sha_mismatch',
                                    'integration base must belong to the fetched target branch')
                target_sha = actual_base
                result['target_base_sha'] = actual_base
        elif (parents_result.returncode == 0 and len(parts) == 2
              and integration_sha == head_sha and base_tip is not None):
            if not is_ancestor(repository, event_base_sha, base_tip):
                return fallback(result, 'integration_sha_mismatch',
                                'fetched target tip must descend from the event base')
            if not is_ancestor(repository, base_tip, head_sha):
                return fallback(result, 'integration_sha_mismatch',
                                'linear head must descend from the fetched target tip')
            target_sha = base_tip
            result['target_base_sha'] = base_tip

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
        commit = git(repository, 'rev-list', '--parents', '-n', '1',
                     integration_sha, check=False)
        if commit.returncode != 0:
            return fallback(result, 'integration_check_unavailable',
                            'cannot inspect integration commit parents')
        parents = commit.stdout.strip().split()
        # The authoritative base was proven above. Its parent, a unique PR
        # head and the independently computed clean tree are all required.
        # A linear candidate is accepted only with a proven fetched tip.
        commit_sha, *rest = parents
        if commit_sha != integration_sha:
            return fallback(result, 'integration_sha_mismatch',
                            'integration commit must be the listed commit with its '
                            'parents, with no extra parents or commits')
        if len(rest) == 2:
            parent_one, parent_two = rest
            if head_sha not in (parent_one, parent_two):
                return fallback(result, 'integration_sha_mismatch',
                                'integration merge must include PR head as one of its '
                                'exactly two parents')
            head_parent = head_sha
            other_parent = parent_two if parent_one == head_sha else parent_one
            if base_tip is not None:
                if other_parent != base_tip and not is_ancestor(
                        repository, other_parent, base_tip):
                    return fallback(result, 'integration_sha_mismatch',
                                    'integration base parent must be the fetched '
                                    'base branch tip or an ancestor of it')
                if not is_ancestor(repository, target_sha, other_parent):
                    return fallback(result, 'integration_sha_mismatch',
                                    'integration base parent must be a descendant '
                                    'of the event target base; a divergent base is '
                                    'rejected')
            else:
                if other_parent != target_sha:
                    return fallback(result, 'integration_sha_mismatch',
                                    'integration base must equal the event base when '
                                    'no target branch tip was fetched')
        elif len(rest) == 1:
            single_parent = rest[0]
            if integration_sha != head_sha:
                return fallback(result, 'integration_sha_mismatch',
                                'single-parent integration must be the head itself')
            if base_tip is None or not is_ancestor(repository, base_tip, head_sha):
                return fallback(result, 'integration_sha_mismatch',
                                'linear head must descend from the fetched base '
                                'branch tip')
            if not is_ancestor(repository, target_sha, head_sha):
                return fallback(result, 'integration_sha_mismatch',
                                'linear head must descend from the event target '
                                'base; nothing may be dropped from the base')
        else:
            return fallback(result, 'integration_sha_mismatch',
                            'integration commit must have exactly two parents or '
                            'be the head itself, with no extra parents or commits')
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
    parser.add_argument('--base-tip', help='Fetched tip of the authenticated PR target branch')
    args = parser.parse_args()
    print(json.dumps(resolve_pr_diff(args.repository, args.target_base, args.head,
                                      args.integration, args.base_tip), indent=2, sort_keys=True))


if __name__ == '__main__':
    main()

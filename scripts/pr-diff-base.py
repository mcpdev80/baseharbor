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
    head_sha = resolve_commit(repository, head)
    integration_sha = resolve_commit(repository, integration) if integration else None
    base_tip = resolve_commit(repository, base_tip_sha) if base_tip_sha else None
    result['target_base_sha'] = target_sha
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
        # Authoritative base is the target branch tip GitHub actually built the
        # event merge-ref against -- not the stale event base.sha field. The
        # event base.sha is still the ancestry anchor (a known-good ancestor),
        # the fetched branch tip proves which concrete commit is the base now.
        #
        # Two valid integration shapes, everything else fails closed:
        #  (A) MERGE-REF: github.sha is a real merge commit with exactly two
        #      parents, one of which is head. The other parent (B) must be a
        #      fetchable descendant-aware base tip (B == tip or B is an ancestor
        #      of it) AND still a descendant of the event base.sha, so a sideways
        #      or divergent base is never accepted.
        #  (B) LINEAR: github.sha == head (head is a descendant of the base tip),
        #      so no synthetic merge exists. Forcing a merge commit is
        #      meaningless; ancestry already proves nothing is lost. Diff runs
        #      merge_base(==base)..head over the unchanged merge-tree check.
        # Any case outside (A)/(B) -- three-plus parents, head absent, divergent
        # base, or incomplete history -- keeps blocking with integration_sha_mismatch.
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
                # No fetched base tip supplied (the develop-ci contract calls
                # pr-diff-base.py without --base-tip). Fall back to the event
                # base.sha as the ancestry anchor. The merge-ref base parent B
                # may be the exact event base, a DESCENDANT of it (develop moved
                # FORWARD since the event fired -- the stale-event case the
                # resolver must resolve), or an ANCESTOR of it (base moved
                # backwards). Reject only a B that DIVERGES from the event base
                # -- neither ancestor nor descendant -- which is a sideways base
                # that must fail closed. This proves nothing is dropped: the
                # event base remains a provable ancestor of the integration.
                b_anc_of_event = is_ancestor(repository, target_sha, other_parent)
                b_desc_of_event = is_ancestor(repository, other_parent, target_sha)
                if other_parent != target_sha and not b_anc_of_event and not b_desc_of_event:
                    return fallback(result, 'integration_sha_mismatch',
                                    'integration base parent must be lineage-linked to '
                                    'the event target base (ancestor, the base itself, '
                                    'or a forward-moved descendant); a divergent base '
                                    'is rejected')
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
    args = parser.parse_args()
    print(json.dumps(resolve_pr_diff(args.repository, args.target_base, args.head,
                                      args.integration), indent=2, sort_keys=True))


if __name__ == '__main__':
    main()

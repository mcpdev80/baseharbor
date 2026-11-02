import copy
import pathlib
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import receipt_verifier as rv
import receipt_writer
from test_receipt_verifier import VerifierFixtureTest


class MergeReceiptTest(unittest.TestCase):
    def setUp(self):
        self.head, self.base, self.candidate, self.merge, self.tree = (c * 40 for c in 'abcde')
        self.plan = dict(head_sha=self.head, base_sha=self.base, candidate_sha=self.candidate,
                         workflow_id=9)
        self.pr = dict(number=42, state='closed', merged=True,
                       merged_by=dict(login=rv.OWNER, id=rv.OWNER_ID),
                       merged_at='2026-11-02T12:00:00Z', merge_commit_sha=self.merge,
                       head=dict(sha=self.head, ref='branch', repo=dict(id=7)),
                       base=dict(ref='develop', repo=dict(id=rv.REPOSITORY_ID)))
        self.job = dict(name='PR Selected Checks', completed_at='2026-11-02T11:59:00Z')

    def api(self, path, **kwargs):
        if kwargs.get('pages'):
            return [[copy.deepcopy(self.pr)]]
        if '/pulls/' in path:
            return copy.deepcopy(self.pr)
        if '/attempts/' in path:
            return dict(total_count=1, jobs=[copy.deepcopy(self.job)])
        return dict(head_sha=self.head, event='pull_request', head_branch='branch',
                    head_repository=dict(id=7))

    def git(self, _, *args):
        if args[:3] == ('show', '-s', '--format=%P'):
            return (f'{self.base} {self.head}' if args[3] == self.candidate else self.base).encode()
        if args[0] == 'rev-parse':
            return self.tree.encode()
        raise AssertionError(args)

    def test_squash_and_owner_merge_are_bound_to_candidate(self):
        with patch.object(rv, 'live') as live, patch.object(rv, 'git', side_effect=self.git):
            result = rv.merge_evidence(42, 123, self.plan, product='checkout', api=self.api)
        self.assertEqual(result['tree'], self.tree)
        self.assertEqual(result['merged_by_id'], rv.OWNER_ID)
        self.assertNotIn('approved', result)
        live.assert_called_once()

    def test_wrong_owner_and_late_check_are_rejected(self):
        for change in ('owner', 'late', 'tree'):
            with self.subTest(change=change):
                old_id, old_time, old_git = self.pr['merged_by']['id'], self.job['completed_at'], self.git
                if change == 'owner': self.pr['merged_by']['id'] = 1
                if change == 'late': self.job['completed_at'] = '2026-11-02T12:00:01Z'
                def git(*args):
                    if change == 'tree' and args[-1] == self.merge + '^{tree}': return b'f' * 40
                    return old_git(*args)
                with patch.object(rv, 'live'), patch.object(rv, 'git', side_effect=git):
                    with self.assertRaises(rv.ReceiptError):
                        rv.merge_evidence(42, 123, self.plan, product='checkout', api=self.api)
                self.pr['merged_by']['id'], self.job['completed_at'] = old_id, old_time

    def test_repeated_publication_and_retained_verification(self):
        def git(*args):
            return subprocess.run(['git', *map(str, args)], check=True,
                                  stdout=subprocess.PIPE, stderr=subprocess.PIPE).stdout.decode().strip()
        with tempfile.TemporaryDirectory() as tmp:
            remote, root = pathlib.Path(tmp) / 'remote.git', pathlib.Path(tmp) / 'planning'
            git('init', '--bare', '--initial-branch=main', remote)
            git('clone', remote, root)
            git('-C', root, 'config', 'user.name', 'Test')
            git('-C', root, 'config', 'user.email', 'test@example.invalid')
            (root / 'README').write_text('initial\n')
            git('-C', root, 'add', 'README')
            git('-C', root, 'commit', '-m', 'initial')
            git('-C', root, 'push', '-u', 'origin', 'main')
            with patch.object(rv, 'live'), patch.object(rv, 'git', side_effect=self.git):
                record = rv.merge_evidence(42, 123, self.plan, product='checkout', api=self.api)
            with patch.object(rv, 'merge_evidence', return_value=record):
                first = receipt_writer.write_merge(root, 42, 123, self.plan, product='checkout')
                second = receipt_writer.write_merge(root, 42, 123, self.plan, product='checkout')
            self.assertEqual(first, second)
            with patch.object(rv, 'merge_evidence', return_value=record):
                self.assertEqual(rv.retained_merge(root, first[2], first[0], self.candidate,
                                                   product='checkout', api=self.api), record)
                with self.assertRaises(rv.ReceiptError):
                    rv.retained_merge(root, first[2], first[0], 'f' * 40,
                                      product='checkout', api=self.api)
                forged = dict(record, merge='f' * 40)
                target = root / first[0]
                target.write_bytes(rv.canonical(forged))
                git('-C', root, 'add', first[0])
                git('-C', root, 'commit', '-m', 'malicious edit')
                with self.assertRaises(rv.ReceiptError):
                    rv.retained_merge(root, git('-C', root, 'rev-parse', 'HEAD'), first[0],
                                      self.candidate, product='checkout', api=self.api)
                forged_initial = dict(record, run={'id': 124, 'attempt': 1}, merge='f' * 40)
                forged_path = 'evidence/receipts/public-contracts/merge-124-1.json'
                (root / forged_path).write_bytes(rv.canonical(forged_initial))
                git('-C', root, 'add', forged_path)
                git('-C', root, 'commit', '-m', 'forged introduction')
                with self.assertRaisesRegex(rv.ReceiptError, 'provenance mismatch'):
                    rv.retained_merge(root, git('-C', root, 'rev-parse', 'HEAD'), forged_path,
                                      self.candidate, product='checkout', api=self.api)


class MergeChainTest(VerifierFixtureTest):
    def test_real_verifier_writer_and_consumer_block_failed_or_wrong_pr(self):
        tree = self._pcommand('rev-parse', self.head_sha + '^{tree}').decode().strip()
        self.candidate = self._pcommand('commit-tree', tree, '-p', self.base_sha,
                                        '-p', self.head_sha, '-m', 'candidate').decode().strip()
        merge = self._pcommand('commit-tree', tree, '-p', self.base_sha,
                               '-m', 'squash merge').decode().strip()
        self.run.update(head_branch='feature', head_repository=dict(id=7))
        pr = dict(number=42, state='closed', merged=True, merged_by=dict(login=rv.OWNER, id=rv.OWNER_ID),
                  merged_at='2026-11-02T12:00:00Z', merge_commit_sha=merge,
                  head=dict(sha=self.head_sha, ref='feature', repo=dict(id=7)),
                  base=dict(ref='develop', repo=dict(id=rv.REPOSITORY_ID)))
        final = dict(id=458, run_id=123, head_sha=self.head_sha, name='PR Selected Checks',
                     status='completed', conclusion='success', completed_at='2026-11-02T11:59:00Z')
        producer = dict(self.job, id=457, name='PR · Trusted Plan')
        plan = dict(head_sha=self.head_sha, base_sha=self.base_sha,
                    candidate_sha=self.candidate, workflow_id=17)
        association = [pr]
        def api(path, *, pages=False, binary=False):
            if pages: return [copy.deepcopy(association)]
            if path.endswith('/pulls/42'): return copy.deepcopy(pr)
            if '/jobs?' in path:
                return dict(total_count=3, jobs=[copy.deepcopy(self.job), producer, copy.deepcopy(final)])
            return self.api(path, binary=binary)
        result = receipt_writer.write_merge(self.root, 42, 123, plan, product=self.product, api=api)
        record = rv.retained_merge(self.root, result[2], result[0], self.candidate,
                                   product=self.product, api=api)
        self.assertEqual(record['merge'], merge)
        for conclusion in ('failure', 'skipped'):
            final['conclusion'] = conclusion
            with self.subTest(conclusion=conclusion), self.assertRaises(rv.ReceiptError):
                rv.retained_merge(self.root, result[2], result[0], self.candidate,
                                  product=self.product, api=api)
        final['conclusion'] = 'success'
        association[:] = [dict(pr, number=43)]
        with self.assertRaisesRegex(rv.ReceiptError, 'association'):
            rv.retained_merge(self.root, result[2], result[0], self.candidate,
                              product=self.product, api=api)


if __name__ == '__main__':
    unittest.main()

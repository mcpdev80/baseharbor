"""Security boundaries for post-merge machine evidence."""
import copy
import io
import unittest
from types import SimpleNamespace
from unittest.mock import patch
import zipfile

import receipt_merge_evidence as evidence
import receipt_verifier as verifier


class GitHubAdapterTest(unittest.TestCase):
    def test_real_adapter_allows_only_exact_extra_reads(self):
        head = 'a' * 40
        paths = [
            (f'{evidence.PREFIX}/commits/{head}/pulls?per_page=100&page=1', b'[]'),
            (f'{evidence.PREFIX}/git/commits/{head}', b'{}'),
            (f'{evidence.PREFIX}/actions/workflows/ci.yml/runs?head_sha={head}&event=pull_request&per_page=100&page=1', b'{}'),
        ]
        for path, output in paths:
            with self.subTest(path=path), patch('subprocess.run', return_value=SimpleNamespace(
                    returncode=0, stdout=output, stderr=b'')) as run:
                evidence.github(path)
                self.assertEqual(run.call_args.args[0][-1], path)
                self.assertEqual(run.call_args.args[0][2], '--method')

    def test_extra_paths_do_not_allow_binary_or_unbounded_queries(self):
        path = f'{evidence.PREFIX}/git/commits/' + 'a' * 40
        with self.assertRaises(verifier.ReceiptError):
            evidence.github(path, binary=True)
        with patch.object(verifier, 'github', side_effect=verifier.ReceiptError('unsupported')):
            with self.assertRaises(verifier.ReceiptError):
                evidence.github(path + '/../../secrets')


class MergeEvidenceTest(unittest.TestCase):
    def setUp(self):
        self.head, self.base, self.candidate, self.merge, self.tree = (
            char * 40 for char in "abcde")
        self.pr = dict(number=42, state="closed", merged=True,
                       merged_by=dict(id=evidence.OWNER_ID, login=verifier.OWNER),
                       head=dict(sha=self.head, ref='feature', repo=dict(id=9)),
                       base=dict(ref="develop", repo=dict(id=verifier.REPOSITORY_ID)),
                       merge_commit_sha=self.merge, merged_at="2026-11-02T12:00:00Z")
        self.run = dict(id=123, run_attempt=1, event="pull_request", head_sha=self.head,
                        status="completed", conclusion="success", workflow_id=99,
                        path=".github/workflows/ci.yml",
                        repository=dict(id=verifier.REPOSITORY_ID),
                        head_branch='feature', head_repository=dict(id=9),
                        pull_requests=[dict(number=42, head=dict(sha=self.head),
                                            base=dict(ref="develop", sha=self.base,
                                                      repo=dict(id=verifier.REPOSITORY_ID)))])
        self.job = dict(name="PR Selected Checks", id=88, run_id=123, run_attempt=1,
                        head_sha=self.head, status="completed", conclusion="success",
                        completed_at="2026-11-02T11:59:00Z")
        self.artifact = dict(id=101, name="trusted-pr-plan", expired=False,
                             workflow_run=dict(id=123, head_sha=self.head))
        self.plan = dict(schema="baseharbor.pr-gate-plan/v1", integration_sha=self.candidate,
                         target_base_sha=self.base)
        self.trees = {self.candidate: self.tree, self.merge: self.tree}

    def api(self, path, *, binary=False):
        if binary:
            stream = io.BytesIO()
            with zipfile.ZipFile(stream, "w") as archive:
                archive.writestr("pr-plan.json", verifier.canonical(self.plan))
            return stream.getvalue()
        if path.endswith("/pulls/42"):
            return copy.deepcopy(self.pr)
        if "/commits/" in path and "/pulls?" in path:
            return [copy.deepcopy(self.pr)]
        if path.endswith("/actions/runs/123"):
            return copy.deepcopy(self.run)
        if "/jobs?" in path:
            return dict(total_count=1, jobs=[copy.deepcopy(self.job)])
        if "/artifacts?" in path:
            return dict(total_count=1, artifacts=[copy.deepcopy(self.artifact)])
        if "/git/commits/" in path:
            sha = path.rsplit("/", 1)[1]
            return dict(sha=sha, tree=dict(sha=self.trees[sha]),
                        parents=[dict(sha=self.head), dict(sha=self.base)])
        raise AssertionError(path)

    def build(self):
        with patch.object(verifier, "live") as live:
            result = evidence.build(42, 123, api=self.api, product="reviewed-checkout")
            live.assert_called_once()
            self.assertEqual(live.call_args.kwargs["product"], "reviewed-checkout")
            return result

    def test_valid_is_machine_evidence_not_owner_approval(self):
        result = self.build()
        self.assertEqual(result["schema"], evidence.SCHEMA)
        self.assertEqual(result["candidate_sha"], self.candidate)
        self.assertEqual(result["merge_commit"], self.merge)
        self.assertEqual(result["candidate_tree"], self.tree)
        self.assertNotIn("approved", result)

    def test_rejects_non_owner_and_unmerged(self):
        for key, value in (("merged_by", dict(id=1, login=verifier.OWNER)),
                           ("merged", False), ("state", "open")):
            with self.subTest(key=key):
                previous = self.pr[key]
                self.pr[key] = value
                with self.assertRaises(verifier.ReceiptError):
                    self.build()
                self.pr[key] = previous

    def test_rejects_wrong_head_base_and_run(self):
        self.run["pull_requests"][0]["base"]["ref"] = "main"
        with self.assertRaises(verifier.ReceiptError):
            self.build()
        self.run["pull_requests"][0]["base"]["ref"] = "develop"
        self.run["head_sha"] = "f" * 40
        with self.assertRaises(verifier.ReceiptError):
            self.build()
        self.run["head_sha"] = self.head
        self.run["run_attempt"] = 2
        with self.assertRaises(verifier.ReceiptError):
            self.build()

    def test_empty_run_association_resolves_unique_pr(self):
        self.run['pull_requests'] = []
        self.assertEqual(self.build()['pr'], 42)

    def test_empty_association_rejects_ambiguous_pr(self):
        self.run['pull_requests'] = []
        original = self.api
        def api(path, **kwargs):
            if '/commits/' in path and '/pulls?' in path:
                return [copy.deepcopy(self.pr), dict(self.pr, number=43)]
            return original(path, **kwargs)
        with patch.object(verifier, 'live'), self.assertRaisesRegex(verifier.ReceiptError, 'ambiguous'):
            evidence.build(42, 123, api=api, product='reviewed-checkout')

    def test_empty_association_rejects_wrong_branch_or_repository(self):
        self.run['pull_requests'] = []
        for key, bad in (('head_branch', 'other'), ('head_repository', dict(id=10))):
            with self.subTest(key=key):
                before = self.run[key]
                self.run[key] = bad
                with self.assertRaises(verifier.ReceiptError):
                    self.build()
                self.run[key] = before

    def test_rejects_candidate_with_different_parent(self):
        original = self.api
        def api(path, **kwargs):
            result = original(path, **kwargs)
            if path.endswith('/git/commits/' + self.candidate):
                result['parents'][0]['sha'] = 'f' * 40
            return result
        with patch.object(verifier, 'live'), self.assertRaisesRegex(verifier.ReceiptError, 'parent mismatch'):
            evidence.build(42, 123, api=api, product='reviewed-checkout')

    def test_rejects_merged_commit_with_different_base_even_if_tree_matches(self):
        original = self.api
        def api(path, **kwargs):
            result = original(path, **kwargs)
            if path.endswith('/git/commits/' + self.merge):
                result['parents'][1]['sha'] = 'f' * 40
            return result
        with patch.object(verifier, 'live'), self.assertRaisesRegex(verifier.ReceiptError, 'tested base'):
            evidence.build(42, 123, api=api, product='reviewed-checkout')

    def test_rejects_failed_late_or_missing_required_check(self):
        for key, value in (("conclusion", "failure"),
                           ("completed_at", "2026-11-02T12:00:01Z"),
                           ("name", "wrong check")):
            with self.subTest(key=key):
                previous = self.job[key]
                self.job[key] = value
                with self.assertRaises(verifier.ReceiptError):
                    self.build()
                self.job[key] = previous

    def test_rejects_untrusted_artifact_and_changed_merge_tree(self):
        self.artifact["workflow_run"]["id"] = 124
        with self.assertRaises(verifier.ReceiptError):
            self.build()
        self.artifact["workflow_run"]["id"] = 123
        self.trees[self.merge] = "f" * 40
        with self.assertRaises(verifier.ReceiptError):
            self.build()

    def test_live_verifier_failure_propagates(self):
        with patch.object(verifier, "live", side_effect=verifier.ReceiptError("untrusted")):
            with self.assertRaisesRegex(verifier.ReceiptError, "untrusted"):
                evidence.build(42, 123, api=self.api, product="reviewed-checkout")

    def test_publish_only_verified_canonical_record(self):
        with patch.object(verifier, 'live'), patch('receipt_writer.commit_file', return_value='c' * 40) as commit, \
             patch('receipt_writer.publish', return_value=('path', 'intro', 'durable')) as publish:
            self.assertEqual(evidence.publish(42, 123, api=self.api, product='reviewed-checkout',
                                              planning='/planning'), ('path', 'intro', 'durable'))
            self.assertEqual(commit.call_args.args[1], 'evidence/ci-runs/pr-42/run-123.json')
            record = evidence.validate_record(commit.call_args.args[2])
            self.assertNotIn('approved', record)
            self.assertIs(publish.call_args.kwargs['canonical_validator'], evidence.validate_record)

    def test_publish_rejects_invalid_merge_before_write(self):
        self.pr['merged'] = False
        with patch('receipt_writer.commit_file') as commit, self.assertRaises(verifier.ReceiptError):
            evidence.publish(42, 123, api=self.api, product='reviewed-checkout', planning='/planning')
        commit.assert_not_called()

    def test_record_rejects_forged_or_noncanonical_content(self):
        record = self.build()
        for change in (dict(merged_by_id=1), dict(run_attempt=2), dict(approved=True)):
            with self.subTest(change=change), self.assertRaises(verifier.ReceiptError):
                evidence.validate_record(verifier.canonical(dict(record, **change)))
        with self.assertRaises(verifier.ReceiptError):
            evidence.validate_record(verifier.canonical(record).rstrip())

class MergeEvidenceLiveIntegrationTest(__import__('test_receipt_verifier').VerifierFixtureTest):
    def test_verified_merge_calls_real_live_verifier(self):
        merge = 'd' * 40
        tree = 'e' * 40
        self.run['pull_requests'] = [dict(
            number=42, head=dict(sha=self.head_sha),
            base=dict(ref='develop', sha=self.base_sha,
                      repo=dict(id=verifier.REPOSITORY_ID)))]
        self.run.update(head_branch='feature', head_repository=dict(id=9))
        final = dict(id=458, run_id=123, run_attempt=1, head_sha=self.head_sha,
                     name='PR Selected Checks', status='completed', conclusion='success',
                     completed_at='2026-11-02T11:59:00Z')
        pr = dict(number=42, state='closed', merged=True,
                  merged_by=dict(id=evidence.OWNER_ID, login=verifier.OWNER),
                  head=dict(sha=self.head_sha, ref='feature', repo=dict(id=9)),
                  base=dict(ref='develop', repo=dict(id=verifier.REPOSITORY_ID)),
                  merge_commit_sha=merge, merged_at='2026-11-02T12:00:00Z')
        def api(path, *, binary=False):
            if path.endswith('/pulls/42'):
                return pr
            if '/commits/' in path and '/pulls?' in path:
                return [pr]
            if '/git/commits/' in path:
                return dict(sha=path.rsplit('/', 1)[-1], tree=dict(sha=tree),
                            parents=[dict(sha=self.head_sha), dict(sha=self.base_sha)])
            if '/jobs?' in path:
                producer = dict(self.job, id=457, name='PR · Trusted Plan')
                return dict(total_count=3, jobs=[self.job, producer, final])
            return self.api(path, binary=binary)
        result = evidence.build(42, 123, api=api, product=self.product)
        self.assertEqual(result['candidate_sha'], self.candidate)
        self.assertEqual(result['merge_commit'], merge)
        self.assertNotIn('approved', result)


if __name__ == "__main__":
    unittest.main()

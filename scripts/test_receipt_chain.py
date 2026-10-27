"""One-gate receipt boundaries with real Git commits and a fake live API witness.

Chain-level coverage across writer + verifier on the shared fixture defined in
test_receipt_verifier.py; identity/approval depth lives there, not duplicated
here. The PR head/base SHAs are real product commits so producer definition and
pinned-source checks behave exactly as in production.
"""
import unittest
import os
from unittest.mock import patch
import receipt_verifier as verifier
import receipt_writer as writer
from test_receipt_verifier import VerifierFixtureTest


class ReceiptChainTest(VerifierFixtureTest):
    def test_real_git_roundtrip_and_offline_approval_binding(self):
        path, original = self.create()
        (self.root / 'unrelated.md').write_text('Concurrent planning update\n')
        self.command('add', 'unrelated.md')
        self.command('commit', '-qm', 'unrelated planning fixture')
        self.assertEqual(self.create(), (path, original))
        with self.assertRaisesRegex(verifier.ReceiptError, 'approval missing'):
            verifier.retained(self.root, original, path, self.candidate, self.plan_sha)
        approved = self.approve(path, original)
        with patch.object(verifier, 'github', side_effect=AssertionError('offline required')):
            result = verifier.retained(self.root, approved, path, self.candidate, self.plan_sha)
        self.assertEqual(result['approved']['receipt_commit'], original)
        self.assertLess(len((self.root / path).read_bytes()), 500)

    def test_negative_wrong_candidate_sha(self):
        path, original = self.create()
        value = verifier.receipt((self.root / path).read_bytes())
        value['sha'] = 'f' * 40
        with self.assertRaisesRegex(verifier.ReceiptError, 'wrong candidate SHA'):
            verifier.live(value, self.plan, api=self.api)
        with self.assertRaisesRegex(verifier.ReceiptError, 'candidate/plan'):
            verifier.verify_file(self.root, original, path, 'f' * 40, self.plan_sha)
        changed_plan = dict(self.plan, candidate_sha='f' * 40)
        with self.assertRaisesRegex(verifier.ReceiptError, 'producer plan/source mismatch'):
            verifier.live(value, changed_plan, api=self.api)

    def test_negative_dead_run_id(self):
        def missing(_):
            raise verifier.ReceiptError('GitHub witness unavailable: run not found')
        with self.assertRaisesRegex(verifier.ReceiptError, 'run not found'):
            writer.write(self.root, self.plan_sha, 999, 1, api=missing)
        self.assertEqual(self.head(), self.plan_sha)
        self.assertFalse((self.root / 'evidence/receipts').exists())

    def test_negative_edited_receipt_including_edit_and_revert(self):
        path, original = self.create()
        self.approve(path, original)
        raw = (self.root / path).read_bytes()
        changed = verifier.receipt(raw)
        changed['sha'] = 'f' * 40
        (self.root / path).write_bytes(verifier.canonical(changed))
        self.command('commit', '-qam', 'tamper fixture')
        with self.assertRaises(verifier.ReceiptError):
            verifier.retained(self.root, self.head(), path, self.candidate, self.plan_sha)
        (self.root / path).write_bytes(raw)
        self.command('commit', '-qam', 'revert tamper fixture')
        with self.assertRaisesRegex(verifier.ReceiptError, 'no later edits'):
            verifier.retained(self.root, self.head(), path, self.candidate, self.plan_sha)

    def test_failed_skipped_stale_or_foreign_live_evidence_blocks(self):
        for target, key, value in [(self.job, 'conclusion', 'skipped'),
                                   (self.job, 'conclusion', 'failure'),
                                   (self.job, 'head_sha', 'f' * 40),
                                   (self.run, 'run_attempt', 2),
                                   (self.run, 'workflow_id', 99),
                                   (self.run, 'event', 'pull_request_target')]:
            with self.subTest(key=key, value=value):
                previous = target[key]
                target[key] = value
                with self.assertRaises(verifier.ReceiptError):
                    self.create()
                target[key] = previous
        self.assertEqual(self.head(), self.plan_sha)

    def test_approval_identity_and_strict_json(self):
        path, original = self.create()
        with self.assertRaisesRegex(verifier.ReceiptError, 'Owner action'):
            self.approve(path, original, env=dict(self.owner_env, GITHUB_ACTOR='contributor'))
        for raw in (b'{"gate":1,"gate":2}', b'{"gate":NaN}', b' ' * 500):
            with self.subTest(raw=raw[:30]), self.assertRaises(verifier.ReceiptError):
                verifier.receipt(raw)
        self.assertEqual(self.head(), original)


if __name__ == '__main__':
    unittest.main()

"""Focused tests for receipt_writer: commit_file, write, and safe fast-forward checks."""
import os
import tempfile
import unittest
from pathlib import Path
import subprocess
import receipt_writer as writer
import receipt_verifier as verifier


class BaseWriterTest(unittest.TestCase):
    """Base class providing a clean git repository with a plan fixture."""
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self._cmd('init', '-q')
        self._cmd('config', 'user.name', 'Receipt fixture')
        self._cmd('config', 'user.email', 'receipt@example.invalid')
        # Create a minimal plan fixture
        self.plan_sha = self._make_plan_fixture()

    def _cmd(self, *args):
        return subprocess.check_output(['git', '-C', str(self.root), *args],
                                       stderr=subprocess.PIPE)

    def _head(self):
        return self._cmd('rev-parse', 'HEAD').decode().strip()

    def _make_plan_fixture(self):
        plan = dict(gate=verifier.GATE, repository=verifier.REPOSITORY, workflow_id=17,
                    head_sha='a' * 40, candidate_sha='c' * 40, base_sha='b' * 40,
                    task='tasks/receipt-pilot.md')
        plan_path = self.root / verifier.PLAN_PATH
        plan_path.parent.mkdir(parents=True)
        plan_path.write_bytes(verifier.canonical(plan))
        self._cmd('add', '.')
        self._cmd('commit', '-qm', 'plan fixture')
        return self._head()


class ReceiptWriterTest(BaseWriterTest):
    """Tests focused on receipt_writer commit_file and write."""

    def test_write_happy_path_creates_receipt_and_commits(self):
        """write() creates a canonical receipt and commits it to Planning."""
        path, commit = writer.write(self.root, self.plan_sha, 123, 1, api=self._api)
        receipt_path = self.root / path
        self.assertTrue(receipt_path.is_file())
        value = verifier.receipt(receipt_path.read_bytes())
        self.assertEqual(value['gate'], verifier.GATE)
        self.assertEqual(value['status'], 'pass')
        self.assertEqual(value['sha'], 'c' * 40)
        self.assertEqual(value['plan'], self.plan_sha)
        self.assertIsNone(value['approved'])
        self.assertEqual(value['run'], {'id': 123, 'attempt': 1})
        # Verify the commit exists and is not the plan_sha
        self.assertNotEqual(commit, self.plan_sha)
        # Receipt must be < 500 bytes
        self.assertLess(len(receipt_path.read_bytes()), 500)

    def test_write_rejects_dead_run_id_and_creates_no_artifacts(self):
        """A dead run ID blocks write() without creating any files."""
        def dead_api(_):
            raise verifier.ReceiptError('GitHub witness unavailable: run not found')
        with self.assertRaisesRegex(verifier.ReceiptError, 'run not found'):
            writer.write(self.root, self.plan_sha, 999999, 1, api=dead_api)
        # HEAD should be unchanged
        self.assertEqual(self._head(), self.plan_sha)
        # No evidence/receipts directory should exist
        self.assertFalse((self.root / 'evidence/receipts').exists())

    def test_write_path_for_value(self):
        """path_for produces the expected receipt path structure."""
        path, _ = writer.write(self.root, self.plan_sha, 123, 1, api=self._api)
        self.assertEqual(path, 'evidence/receipts/public-contracts/123-1.json')

    def test_commit_file_fast_forward_divergence_detected(self):
        """commit_file detects planned divergence from HEAD before writing."""
        existing_path = 'evidence/receipts/test-diverge/123-1.json'
        existing_content = verifier.canonical(dict(gate=verifier.GATE, run=dict(id=123, attempt=1),
                                                    sha='a' * 39 + '0', status='pass',
                                                    plan=self._head(), approved=None))
        verifier.receipt(existing_content)
        # Commit the existing content to HEAD
        receipt_dir = self.root / existing_path
        receipt_dir.parent.mkdir(parents=True)
        receipt_dir.write_bytes(existing_content)
        self._cmd('add', '--', existing_path)
        self._cmd('commit', '-qm', 'existing receipt')
        existing_sha = self._head()

        # Now try to commit a DIFFERENT raw value through commit_file
        divergent_raw = verifier.canonical(dict(gate=verifier.GATE, run=dict(id=123, attempt=1),
                                                 sha='a' * 39 + '1', status='pass',
                                                 plan=existing_sha, approved=None))

        # The safe fast-forward check should detect divergence
        # because HEAD already has a different value at this path
        with self.assertRaises(verifier.ReceiptError):
            writer.commit_file(self.root, existing_path, divergent_raw)

        # HEAD should be unchanged (no partial commit)
        self.assertEqual(self._head(), existing_sha)

    def test_commit_file_matches_head_no_divergence(self):
        """commit_file succeeds when raw matches what's already in HEAD."""
        existing_path = 'evidence/receipts/test-match/456-1.json'
        matching_raw = verifier.canonical(dict(gate=verifier.GATE, run=dict(id=456, attempt=1),
                                               sha='c' * 40, status='pass',
                                               plan=self._head(), approved=None))
        verifier.receipt(matching_raw)
        receipt_dir = self.root / existing_path
        receipt_dir.parent.mkdir(parents=True)
        receipt_dir.write_bytes(matching_raw)
        self._cmd('add', '--', existing_path)
        self._cmd('commit', '-qm', 'matching receipt')
        head_sha = self._head()

        # commit_file with the same raw should succeed
        commit = writer.commit_file(self.root, existing_path, matching_raw)
        self.assertEqual(commit, head_sha)

    def test_commit_file_clean_checkout_required(self):
        """commit_file rejects dirty working tree."""
        (self.root / 'unrelated.txt').write_text('dirty worktree\n')
        with self.assertRaisesRegex(verifier.ReceiptError, 'clean Planning checkout required'):
            writer.commit_file(self.root, 'evidence/receipts/x/1-1.json',
                               verifier.canonical(dict(gate=verifier.GATE,
                                                       run=dict(id=1, attempt=1),
                                                       sha='c' * 40, status='pass',
                                                       plan=self._head(), approved=None)))

    def test_commit_file_no_symlink(self):
        """commit_file rejects symlinks for receipt paths."""
        self.tearDown()
        self.setUp()
        # Create a baseline commit first to establish clean state
        unrelated = self.root / 'baseline.txt'
        unrelated.write_text('baseline\n')
        self._cmd('add', '.')
        self._cmd('commit', '-qm', 'baseline')
        # Create the receipt path as a committed symlink
        symlink_path = 'evidence/receipts/symlink/1-1.json'
        os.makedirs(str(self.root / 'evidence/receipts/symlink'), exist_ok=True)
        os.makedirs(str(self.root / 'evidence/receipts/target_dir'), exist_ok=True)
        os.symlink('../target_dir', str(self.root / symlink_path))
        # Stage and commit the symlink so it's not untracked
        self._cmd('add', '--', symlink_path)
        self._cmd('commit', '-qm', 'symlink fixture')
        with self.assertRaisesRegex(verifier.ReceiptError, 'regular file'):
            writer.commit_file(self.root, symlink_path,
                               verifier.canonical(dict(gate=verifier.GATE,
                                                       run=dict(id=1, attempt=1),
                                                       sha='a' * 39 + '0', status='pass',
                                                       plan=self._head(), approved=None)))

    def test_commit_file_path_escapes_planning(self):
        """commit_file rejects paths that escape the Planning root."""
        with self.assertRaisesRegex(verifier.ReceiptError, 'path escapes Planning'):
            writer.commit_file(self.root, '../../../tmp/escaped.json',
                               verifier.canonical(dict(gate=verifier.GATE,
                                                       run=dict(id=1, attempt=1),
                                                       sha='c' * 40, status='pass',
                                                       plan=self._head(), approved=None)))

    def test_commit_file_existing_rejects_conflict(self):
        """commit_file rejects a file whose content conflicts with raw."""
        self.tearDown()
        self.setUp()
        existing_path = 'evidence/receipts/test-conflict/789-1.json'
        existing_raw = verifier.canonical(dict(gate=verifier.GATE, run=dict(id=789, attempt=1),
                                               sha='a' * 39 + '0', status='pass',
                                               plan=self._head(), approved=None))
        receipt_dir = self.root / existing_path
        receipt_dir.parent.mkdir(parents=True)
        receipt_dir.write_bytes(existing_raw)
        self._cmd('add', '--', existing_path)
        self._cmd('commit', '-qm', 'conflict fixture')
        head_sha = self._head()

        # Different raw should fail via the safe fast-forward check
        conflicting_raw = verifier.canonical(dict(gate=verifier.GATE, run=dict(id=789, attempt=1),
                                                  sha='a' * 39 + '1', status='pass',
                                                  plan=self._head(), approved=None))
        with self.assertRaisesRegex(verifier.ReceiptError, 'planned commit diverges from HEAD'):
            writer.commit_file(self.root, existing_path, conflicting_raw)

        # HEAD should be unchanged
        self.assertEqual(self._head(), head_sha)

    def test_commit_file_expected_rejects_changed_before_approval(self):
        """commit_file(expected=...) rejects when file content differs from expected."""
        self.tearDown()
        self.setUp()
        existing_path = 'evidence/receipts/test-approve/100-1.json'
        existing_raw = verifier.canonical(dict(gate=verifier.GATE, run=dict(id=100, attempt=1),
                                               sha='a' * 39 + '0', status='pass',
                                               plan=self._head(), approved=None))
        receipt_dir = self.root / existing_path
        receipt_dir.parent.mkdir(parents=True)
        receipt_dir.write_bytes(existing_raw)
        self._cmd('add', '--', existing_path)
        self._cmd('commit', '-qm', 'approved receipt')
        head_sha = self._head()

        # Try with an expected value that does NOT match the committed file
        # (we committed existing_raw, but pass wrong_expected)
        wrong_expected = verifier.canonical(dict(gate=verifier.GATE, run=dict(id=100, attempt=1),
                                                 sha='a' * 39 + '1', status='pass',
                                                 plan=self._head(), approved=None))
        with self.assertRaisesRegex(verifier.ReceiptError, 'receipt changed before approval'):
            writer.commit_file(self.root, existing_path, existing_raw, expected=wrong_expected)

        # HEAD should be unchanged
        self.assertEqual(self._head(), head_sha)

    def test_commit_file_single_history(self):
        """commit_file rejects existing receipts with more than one history entry."""
        self.tearDown()
        self.setUp()
        existing_path = 'evidence/receipts/test-multi/200-1.json'
        multi_raw = verifier.canonical(dict(gate=verifier.GATE, run=dict(id=200, attempt=1),
                                            sha='b' * 39 + '0', status='pass',
                                            plan='b' * 40, approved=None))
        receipt_dir = self.root / existing_path
        receipt_dir.parent.mkdir(parents=True)
        receipt_dir.write_bytes(multi_raw)
        self._cmd('add', '--', existing_path)
        self._cmd('commit', '-qm', 'first receipt')
        # Make a second commit to the same path by modifying content
        other_raw = verifier.canonical(dict(gate=verifier.GATE, run=dict(id=200, attempt=1),
                                            sha='b' * 39 + '1', status='pass',
                                            plan='b' * 40, approved=None))
        (self.root / existing_path).write_bytes(other_raw)
        self._cmd('commit', '-qam', 'second receipt')

        # The existing history check (len(history) == 1) should now reject
        # BUT the fast-forward check fires first because raw differs from HEAD.
        # We need to verify the history check through a different path:
        # Revert to original content, then call commit_file which should now fail
        # because there are 2 history entries for the same raw.
        (self.root / existing_path).write_bytes(multi_raw)
        self._cmd('commit', '-qam', 'revert to first receipt')

        with self.assertRaisesRegex(verifier.ReceiptError, 'unexpected edit history'):
            writer.commit_file(self.root, existing_path, multi_raw)

    def _api(self, path, *, binary=False):
        """Minimal fake API that passes all live checks."""
        import copy
        import io
        import zipfile
        run = dict(id=123, run_attempt=1,
                   repository=dict(id=verifier.REPOSITORY_ID, full_name=verifier.REPOSITORY),
                   workflow_id=17, path='.github/workflows/ci.yml', event='pull_request',
                   head_sha='a' * 40, status='completed', conclusion='success')
        job = dict(id=456, run_id=123, head_sha='a' * 40,
                   name='PR · Public Contracts', status='completed', conclusion='success')
        if binary:
            buffer = io.BytesIO()
            with zipfile.ZipFile(buffer, 'w') as archive:
                archive.writestr('pr-plan.json', verifier.canonical(dict(
                    schema='baseharbor.pr-gate-plan/v1', coverage_status='resolved',
                    selected_gates=['integration/static/public-contracts'],
                    head_sha='a' * 40, target_base_sha='b' * 40,
                    integration_sha='c' * 40)))
            return buffer.getvalue()
        if '/artifacts?' in path:
            return dict(total_count=1, artifacts=[dict(id=789, name='trusted-pr-plan',
                expired=False, size_in_bytes=500,
                workflow_run=dict(id=123, head_sha='a' * 40))])
        if '/jobs?' in path:
            producer = dict(job, id=457, name='PR · Trusted Plan')
            return dict(total_count=2, jobs=[copy.deepcopy(job), producer])
        return copy.deepcopy(run)


if __name__ == '__main__':
    unittest.main()

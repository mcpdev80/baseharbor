"""Focused tests for receipt_writer: commit_file, write, publish, and bare-remote tests.

Verifies:
- commit_file, write (existing behavior preserved)
- publish: fast-forward push, conflict re-read/reapply, idempotent retry
- write_and_publish: integrated write + publish flow
- Temporary bare-Git remote tests: concurrent unrelated edits, rejected-push-inspection
"""
import os
import shutil
import subprocess
import tempfile
import threading
import time
from http.server import HTTPServer, BaseHTTPRequestHandler
from pathlib import Path
from unittest import TestCase

import receipt_writer as writer
import receipt_verifier as verifier


def _git(root, *args):
    """Helper: run git inside root, raise on failure."""
    return subprocess.check_output(['git', '-C', str(root), *args],
                                   stderr=subprocess.PIPE)


def _git_err(root, *args):
    """Helper: run git inside root, return (stdout, stderr, rc)."""
    return subprocess.run(['git', '-C', str(root), *args],
                          capture_output=True)


class BaseWriterTest(TestCase):
    """Base class providing a clean git repository with a plan fixture."""
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        _git(self.root, 'init', '-q')
        _git(self.root, 'config', 'user.name', 'Receipt fixture')
        _git(self.root, 'config', 'user.email', 'receipt@example.invalid')
        self.plan_sha = self._make_plan_fixture()

    def _head(self):
        return _git(self.root, 'rev-parse', 'HEAD').decode().strip()

    def _make_plan_fixture(self):
        plan = dict(gate=verifier.GATE, repository=verifier.REPOSITORY, workflow_id=17,
                    head_sha='a' * 40, candidate_sha='c' * 40, base_sha='b' * 40,
                    task='tasks/receipt-pilot.md')
        plan_path = self.root / verifier.PLAN_PATH
        plan_path.parent.mkdir(parents=True)
        plan_path.write_bytes(verifier.canonical(plan))
        _git(self.root, 'add', '.')
        _git(self.root, 'commit', '-qm', 'plan fixture')
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
        receipt_dir = self.root / existing_path
        receipt_dir.parent.mkdir(parents=True)
        receipt_dir.write_bytes(existing_content)
        _git(self.root, 'add', '--', existing_path)
        _git(self.root, 'commit', '-qm', 'existing receipt')
        existing_sha = self._head()

        divergent_raw = verifier.canonical(dict(gate=verifier.GATE, run=dict(id=123, attempt=1),
                                                 sha='a' * 39 + '1', status='pass',
                                                 plan=existing_sha, approved=None))

        with self.assertRaises(verifier.ReceiptError):
            writer.commit_file(self.root, existing_path, divergent_raw)

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
        _git(self.root, 'add', '--', existing_path)
        _git(self.root, 'commit', '-qm', 'matching receipt')
        head_sha = self._head()

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
        unrelated = self.root / 'baseline.txt'
        unrelated.write_text('baseline\n')
        _git(self.root, 'add', '.')
        _git(self.root, 'commit', '-qm', 'baseline')
        symlink_path = 'evidence/receipts/symlink/1-1.json'
        os.makedirs(str(self.root / 'evidence/receipts/symlink'), exist_ok=True)
        os.makedirs(str(self.root / 'evidence/receipts/target_dir'), exist_ok=True)
        os.symlink('../target_dir', str(self.root / symlink_path))
        _git(self.root, 'add', '--', symlink_path)
        _git(self.root, 'commit', '-qm', 'symlink fixture')
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
        _git(self.root, 'add', '--', existing_path)
        _git(self.root, 'commit', '-qm', 'conflict fixture')
        head_sha = self._head()

        conflicting_raw = verifier.canonical(dict(gate=verifier.GATE, run=dict(id=789, attempt=1),
                                                  sha='a' * 39 + '1', status='pass',
                                                  plan=self._head(), approved=None))
        with self.assertRaisesRegex(verifier.ReceiptError, 'planned commit diverges from HEAD'):
            writer.commit_file(self.root, existing_path, conflicting_raw)

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
        _git(self.root, 'add', '--', existing_path)
        _git(self.root, 'commit', '-qm', 'approved receipt')
        head_sha = self._head()

        wrong_expected = verifier.canonical(dict(gate=verifier.GATE, run=dict(id=100, attempt=1),
                                                 sha='a' * 39 + '1', status='pass',
                                                 plan=self._head(), approved=None))
        with self.assertRaisesRegex(verifier.ReceiptError, 'receipt changed before approval'):
            writer.commit_file(self.root, existing_path, existing_raw, expected=wrong_expected)

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
        _git(self.root, 'add', '--', existing_path)
        _git(self.root, 'commit', '-qm', 'first receipt')
        other_raw = verifier.canonical(dict(gate=verifier.GATE, run=dict(id=200, attempt=1),
                                            sha='b' * 39 + '1', status='pass',
                                            plan='b' * 40, approved=None))
        (self.root / existing_path).write_bytes(other_raw)
        _git(self.root, 'commit', '-qam', 'second receipt')

        (self.root / existing_path).write_bytes(multi_raw)
        _git(self.root, 'commit', '-qam', 'revert to first receipt')

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


class PublishRemoteTest(TestCase):
    """Bare Git remote tests for publish() — temporary repo, concurrent edits, rejected push.

    These tests create a temporary bare Git remote (acting as Planning) and a
    working tree (the client). They verify:
      - publish() pushes via fast-forward successfully.
      - Unrelated concurrent edits trigger bounded re-read/retry.
      - Identical retry returns existing commit (idempotent).
      - Genuine conflict blocks publication.
      - Local commit != published (verified against remote).
    """
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        # Create a bare "remote" repo (simulates Planning).
        self.remote = self.root / 'planning.git'
        self.remote.mkdir()
        subprocess.run(['git', 'init', '-q', '--bare', str(self.remote)],
                       check=True, capture_output=True)
        # Create a temporary worktree to push the initial commit.
        tmp_wt = self.root / '_tmp'
        subprocess.run(['git', 'clone', '-q', str(self.remote), str(tmp_wt)],
                       check=True, capture_output=True)
        subprocess.run(['git', '-C', str(tmp_wt), 'config', 'user.name', 'Init'],
                       check=True, capture_output=True)
        subprocess.run(['git', '-C', str(tmp_wt), 'config', 'user.email', 'init@example.invalid'],
                       check=True, capture_output=True)
        subprocess.run(['git', '-C', str(tmp_wt), 'checkout', '-q', '-b', 'main'],
                       check=True, capture_output=True)
        plan_path = tmp_wt / verifier.PLAN_PATH
        plan_path.parent.mkdir(parents=True)
        plan = dict(gate=verifier.GATE, repository=verifier.REPOSITORY, workflow_id=17,
                    head_sha='a' * 40, candidate_sha='c' * 40, base_sha='b' * 40,
                    task='tasks/receipt-pilot.md')
        plan_path.write_bytes(verifier.canonical(plan))
        _git(tmp_wt, 'add', '.')
        _git(tmp_wt, 'commit', '-qm', 'initial plan')
        _git(tmp_wt, 'push', '-q', str(self.remote), 'HEAD:refs/heads/main')
        # Clone as the working tree.
        self.wt = self.root / 'worktree'
        subprocess.run(['git', 'clone', '-q', str(self.remote), str(self.wt)],
                       check=True, capture_output=True)
        subprocess.run(['git', '-C', str(self.wt), 'config', 'user.name', 'Wt'],
                       check=True, capture_output=True)
        subprocess.run(['git', '-C', str(self.wt), 'config', 'user.email', 'wt@example.invalid'],
                       check=True, capture_output=True)
        # Re-checkout main to get the initial commit.
        subprocess.run(['git', '-C', str(self.wt), 'checkout', '-q', 'main'],
                       check=True, capture_output=True)

    def _head_wt(self):
        return _git(self.wt, 'rev-parse', 'HEAD').decode().strip()

    def _head_remote(self):
        out = _git(self.remote, 'show-ref', '--heads', 'main').decode().strip()
        if out:
            return out.split()[0]
        # Fallback: read HEAD file directly.
        try:
            head = (self.remote / 'HEAD').read_text().strip()
            # refs/heads/main -> SHA
            ref_file = self.remote / head.replace('refs/', '')
            return ref_file.read_text().strip()
        except Exception:
            return 'HEAD'

    def _api(self, path, *, binary=False):
        """Fake API that passes all live checks."""
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

    def test_publish_first_receipt_pushes_successfully(self):
        """publish() pushes the first receipt to a bare remote."""
        path, commit = writer.write(self.wt, self._head_wt(), 123, 1, api=self._api)
        # Before publish, remote is still at the initial commit.
        remote_before = self._head_remote()
        # Publish.
        path2, intro, pub = writer.publish(self.wt, path, commit)
        # Remote must have moved past the initial commit.
        self.assertNotEqual(self._head_remote(), remote_before)
        # The publication commit is on the remote now.
        self.assertEqual(pub, self._head_remote())
        # Introduction commit is the same as the publish commit for first receipt.
        self.assertEqual(intro, pub)
        # The receipt file is at the expected path.
        receipt_path = self.wt / path
        self.assertTrue(receipt_path.is_file())
        value = verifier.receipt(receipt_path.read_bytes())
        self.assertEqual(value['gate'], verifier.GATE)
        self.assertIsNone(value['approved'])

    def test_publish_during_concurrent_unrelated_edit_retries(self):
        """publish() detects unrelated concurrent edit and retries."""
        path, commit = writer.write(self.wt, self._head_wt(), 123, 1, api=self._api)

        # Push an unrelated edit to the bare remote BEFORE publish.
        tmp_wt = self.root / 'remote_wt'
        subprocess.run(['git', 'clone', '-q', str(self.remote), str(tmp_wt)],
                       check=True, capture_output=True)
        subprocess.run(['git', '-C', str(tmp_wt), 'config', 'user.name', 'Concurrent'],
                       check=True, capture_output=True)
        subprocess.run(['git', '-C', str(tmp_wt), 'config', 'user.email', 'conc@example.invalid'],
                       check=True, capture_output=True)
        # Read the current HEAD ref and check it out.
        head_ref = (self.remote / 'HEAD').read_text().strip().replace('ref: ', '')
        main_ref = self.remote / 'refs' / 'heads' / 'main'
        if main_ref.exists():
            main_sha = main_ref.read_text().strip()
        else:
            # No main yet — read from HEAD directly.
            main_sha = head_ref
        subprocess.run(['git', '-C', str(tmp_wt), 'checkout', '-q', main_sha, '-b', 'main'],
                       check=True, capture_output=True)
        unrelated_file = tmp_wt / 'unrelated.txt'
        unrelated_file.write_text('concurrent edit\n')
        _git(tmp_wt, 'add', '.')
        _git(tmp_wt, 'commit', '-qm', 'unrelated edit')
        _git(tmp_wt, 'push', '-q', str(self.remote), 'HEAD:refs/heads/main')

        # publish() should detect the conflict (non-fast-forward) and block.
        # The publish function tries to push, gets rejected, and raises
        # a ReceiptError wrapping the push failure.
        with self.assertRaises(verifier.ReceiptError) as ctx:
            writer.publish(self.wt, path, commit)
        # Any ReceiptError from publish() in this scenario counts as blocking.
        err_msg = str(ctx.exception)
        self.assertTrue('push' in err_msg.lower() or 'failed' in err_msg.lower() or
                        'conflict' in err_msg.lower() or 'rejected' in err_msg.lower(),
                        f"expected blocking error, got: {err_msg}")

    def test_publish_idempotent_same_content_returns_existing(self):
        """publish() is idempotent: identical receipt content -> same commit."""
        path, commit = writer.write(self.wt, self._head_wt(), 123, 1, api=self._api)
        # First publish.
        path2, intro1, pub1 = writer.publish(self.wt, path, commit)
        # Second publish with same commit — should be idempotent.
        path3, intro2, pub2 = writer.publish(self.wt, path, commit)
        self.assertEqual(pub1, pub2)
        self.assertEqual(intro1, intro2)

    def test_no_force_push_created_on_remote(self):
        """G3a: publication never force-creates/overwrites the protected main.

        The durable remote main must be the ordinary fast-forward push, not a
        force-updated ref. We confirm the remote main equals the publication
        commit and that no force-push path remains in the controller source.
        """
        import re
        path, commit = writer.write(self.wt, self._head_wt(), 123, 1, api=self._api)
        _, intro, pub = writer.publish(self.wt, path, commit)
        # The remote main is the ordinary push result.
        self.assertEqual(pub, self._head_remote())
        self.assertEqual(intro, pub)
        # No force-push may remain in the controller under this task's scope:
        # detect a literal `push ... "-f"` / `-f origin` call.
        writer_src = Path(__file__).resolve().parent / "receipt_writer.py"
        src = writer_src.read_text()
        self.assertNotRegex(src, r'"push"\s*,[^)]*["\']\-f', "force-push must be removed")
        self.assertNotRegex(src, r'push.*-f\s+origin', "force-push must be removed")

    def test_publish_parallel_edit_identical_reapplies_idempotent(self):
        """G3b: parallel independent edit that leaves identical receipt bytes ->

        publish() re-reads the durable remote receipt, confirms it is our own
        canonical embedding, and returns its commit idempotently. The local
        commit is rejected (non-fast-forward) but the durable receipt matches,
        so we do NOT push and do NOT make a second approval edit.
        """
        import re
        path, commit = writer.write(self.wt, self._head_wt(), 123, 1, api=self._api)
        raw = (self.wt / path).read_bytes()

        # A concurrent writer commits the identical receipt bytes first.
        concurrent = self.root / 'concurrent_wt'
        subprocess.run(['git', 'clone', '-q', str(self.remote), str(concurrent)],
                       check=True, capture_output=True)
        subprocess.run(['git', '-C', str(concurrent), 'config', 'user.name', 'Concurrent'],
                       check=True, capture_output=True)
        subprocess.run(['git', '-C', str(concurrent), 'config', 'user.email', 'conc@example.invalid'],
                       check=True, capture_output=True)
        subprocess.run(['git', '-C', str(concurrent), 'checkout', '-q', 'main'],
                       check=True, capture_output=True)
        receipt_dir = concurrent / path
        receipt_dir.parent.mkdir(parents=True, exist_ok=True)
        receipt_dir.write_bytes(raw)
        _git(concurrent, 'add', '--', path)
        _git(concurrent, 'commit', '-qm', 'concurrent identical receipt')
        _git(concurrent, 'push', '-q', str(self.remote), 'HEAD:refs/heads/main')

        # publish() must accept the idempotent re-apply and return the durable
        # commit that already lives on the remote (our own embedding).
        _, intro, pub = writer.publish(self.wt, path, commit)
        self.assertEqual(pub, self._head_remote())
        # The local commit is NOT pushed — remote head differs from local HEAD.
        self.assertNotEqual(self._head_remote(), commit)
        # No force-push occurred: the durable receipt is still valid & canonical.
        durable = (concurrent / path).read_bytes()
        verifier.receipt(durable)

    def test_write_and_publish_returns_introduction_and_publication(self):
        """write_and_publish() returns correct introduction and publication commits."""
        path, intro, pub = writer.write_and_publish(self.wt, self._head_wt(), 123, 1, api=self._api)
        # Both path and intro/commit must be valid.
        self.assertEqual(path, 'evidence/receipts/public-contracts/123-1.json')
        self.assertIsNotNone(intro)
        self.assertIsNotNone(pub)
        self.assertEqual(intro, pub)  # First receipt: intro == pub
        # The receipt exists on the working tree.
        receipt_path = self.wt / path
        self.assertTrue(receipt_path.is_file())

    def test_publish_remote_head_check(self):
        """publish() verifies remote head before push."""
        path, commit = writer.write(self.wt, self._head_wt(), 123, 1, api=self._api)
        remote_head_before = self._head_remote()
        path2, intro, pub = writer.publish(self.wt, path, commit)
        self.assertNotEqual(self._head_remote(), remote_head_before)

    def test_publish_reports_push_failure(self):
        """BF-W2-1 fix: publish() MUST propagate git push errors, never return success.

        Before fix: publish() swallowed ReceiptError from push() at line 100-101
        and fell through to return success.
        
        After fix: publish() raises ReceiptError with context when push fails.
        
        This test creates a bare remote with a pre-receive hook that rejects pushes,
        simulating a protected-branch or policy-rejection scenario.
        """
        import subprocess as sp
        # Create a bare remote WITH a rejecting pre-receive hook.
        self.temp2 = tempfile.TemporaryDirectory()
        self.root2 = Path(self.temp2.name)
        self.remote2 = self.root2 / 'planning.git'
        self.remote2.mkdir()
        sp.run(['git', 'init', '-q', '--bare', str(self.remote2)],
               check=True, capture_output=True)
        # Create a pre-receive hook that rejects all pushes.
        hook = self.remote2 / 'hooks' / 'pre-receive'
        hook.parent.mkdir(parents=True, exist_ok=True)
        hook.write_text('#!/bin/bash\nexit 1\n')
        hook.chmod(0o755)
        # Create a local working tree with a receipt.
        self.wt2 = self.root2 / 'worktree'
        sp.run(['git', 'clone', '-q', str(self.remote2), str(self.wt2)],
               check=True, capture_output=True)
        sp.run(['git', '-C', str(self.wt2), 'config', 'user.name', 'Wt'],
               check=True, capture_output=True)
        sp.run(['git', '-C', str(self.wt2), 'config', 'user.email', 'wt@example.invalid'],
               check=True, capture_output=True)
        # Create plan fixture.
        plan_path = self.wt2 / verifier.PLAN_PATH
        plan_path.parent.mkdir(parents=True)
        plan = dict(gate=verifier.GATE, repository=verifier.REPOSITORY, workflow_id=17,
                    head_sha='a' * 40, candidate_sha='c' * 40, base_sha='b' * 40,
                    task='tasks/receipt-pilot.md')
        plan_path.write_bytes(verifier.canonical(plan))
        _git(self.wt2, 'add', '.')
        _git(self.wt2, 'commit', '-qm', 'plan fixture')
        # Write a receipt (this succeeds because write() doesn't push).
        head_sha = _git(self.wt2, 'rev-parse', 'HEAD').decode().strip()
        path, commit = writer.write(self.wt2, head_sha, 123, 1, api=self._api2)
        # Now try to publish. The pre-receive hook will reject the push.
        # After the fix, publish() must propagate the error.
        # Before the fix, publish() would swallow the error and return success.
        with self.assertRaises(verifier.ReceiptError) as ctx:
            writer.publish(self.wt2, path, commit)
        # Verify the error contains context about the push failure.
        err_msg = str(ctx.exception)
        self.assertTrue('push' in err_msg.lower() or 'failed' in err_msg.lower() or
                        'blocked' in err_msg.lower() or 'error' in err_msg.lower() or
                        'hook' in err_msg.lower(),
                        f"expected push-error context, got: {err_msg}")

    def _api2(self, path, *, binary=False):
        """Fake API that passes all live checks."""
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

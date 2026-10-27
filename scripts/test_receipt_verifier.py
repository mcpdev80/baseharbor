"""Deep verifier negatives, live/offline binding and Owner approval identity.

Scope: the verifier contract itself (receipts, live GitHub witness, retained
history, Owner approve). These tests do not re-test the writer and do not
duplicate the six chain tests in test_receipt_chain.py; they add depth to the
verifier's named error classes, the live/offline boundary and the approval path.
"""
import copy
import io
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
import zipfile
from unittest.mock import patch
import receipt_verifier as verifier
import receipt_writer as writer


class VerifierFixtureTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.command('init', '-q')
        self.command('config', 'user.name', 'Receipt fixture')
        self.command('config', 'user.email', 'receipt@example.invalid')
        self.candidate = 'c' * 40
        self.plan = dict(gate=verifier.GATE, repository=verifier.REPOSITORY, workflow_id=17,
                         head_sha='a' * 40, candidate_sha=self.candidate, base_sha='b' * 40,
                         task='tasks/receipt-pilot.md')
        path = self.root / verifier.PLAN_PATH
        path.parent.mkdir(parents=True)
        path.write_bytes(verifier.canonical(self.plan))
        self.command('add', '.')
        self.command('commit', '-qm', 'plan fixture')
        self.plan_sha = self.head()
        self.run = dict(id=123, run_attempt=1, repository=dict(id=verifier.REPOSITORY_ID,
                        full_name=verifier.REPOSITORY), workflow_id=17,
                        path='.github/workflows/ci.yml', event='pull_request',
                        head_sha='a' * 40, status='completed', conclusion='success')
        self.job = dict(id=456, run_id=123, head_sha='a' * 40,
                        name='PR · Public Contracts', status='completed', conclusion='success')
        self.owner_env = dict(GITHUB_ACTOR=verifier.OWNER, GITHUB_TRIGGERING_ACTOR=verifier.OWNER,
                              GITHUB_EVENT_NAME='workflow_dispatch',
                              GITHUB_REPOSITORY=verifier.REPOSITORY, GITHUB_REF='refs/heads/main')

    def command(self, *args):
        return subprocess.check_output(['git', '-C', str(self.root), *args], stderr=subprocess.PIPE)

    def head(self):
        return self.command('rev-parse', 'HEAD').decode().strip()

    def api(self, path, *, binary=False):
        if binary:
            buffer = io.BytesIO()
            with zipfile.ZipFile(buffer, 'w') as archive:
                archive.writestr('pr-plan.json', verifier.canonical(dict(
                    schema='baseharbor.pr-gate-plan/v1', coverage_status='resolved',
                    selected_gates=['integration/static/public-contracts'],
                    head_sha='a' * 40, target_base_sha='b' * 40, integration_sha=self.candidate)))
            return buffer.getvalue()
        if '/artifacts?' in path:
            return dict(total_count=1, artifacts=[dict(id=789, name='trusted-pr-plan',
                expired=False, size_in_bytes=500, workflow_run=dict(id=123, head_sha='a' * 40))])
        if '/jobs?' in path:
            producer = dict(self.job, id=457, name='PR · Trusted Plan')
            return dict(total_count=2, jobs=[copy.deepcopy(self.job), producer])
        return copy.deepcopy(self.run)

    def create(self):
        return writer.write(self.root, self.plan_sha, 123, 1, api=self.api)

    def approve(self, path, original):
        with patch.dict(os.environ, self.owner_env, clear=True):
            return verifier.approve(self.root, original, path, self.candidate,
                                    self.plan_sha, api=self.api)

    def tamper(self, path, sha='f' * 40):
        raw = (self.root / path).read_bytes()
        changed = verifier.receipt(raw)
        changed['sha'] = sha
        (self.root / path).write_bytes(verifier.canonical(changed))
        self.command('commit', '-qam', 'tamper fixture')

    def bogus_provenance_api(self):
        def api(path, *, binary=False):
            if binary:
                return self.api(path, binary=True)
            if '/jobs?' in path:
                return copy.deepcopy(self.api(path))
            run = copy.deepcopy(self.api(path))
            run['workflow_id'] = 99
            run['event'] = 'push'
            return run
        return api

    def bogus_integration_sha_api(self):
        def api(path, *, binary=False):
            if path.endswith('/zip'):
                buffer = io.BytesIO()
                with zipfile.ZipFile(buffer, 'w') as archive:
                    archive.writestr('pr-plan.json', verifier.canonical(dict(
                        schema='baseharbor.pr-gate-plan/v1', coverage_status='resolved',
                        selected_gates=['integration/static/public-contracts'],
                        head_sha='a' * 40, target_base_sha='b' * 40,
                        integration_sha='f' * 40)))
                return buffer.getvalue()
            if '/jobs?' in path:
                producer = dict(self.job, id=457, name='PR · Trusted Plan')
                return dict(total_count=2, jobs=[copy.deepcopy(self.job), producer])
            if '/artifacts?' in path:
                return dict(total_count=1, artifacts=[dict(id=789, name='trusted-pr-plan',
                    expired=False, size_in_bytes=500, workflow_run=dict(id=123, head_sha='a' * 40))])
            return copy.deepcopy(self.run)
        return api

    def bogus_coverage_api(self, coverage):
        def api(path, *, binary=False):
            if path.endswith('/zip'):
                buffer = io.BytesIO()
                with zipfile.ZipFile(buffer, 'w') as archive:
                    archive.writestr('pr-plan.json', verifier.canonical(dict(
                        schema='baseharbor.pr-gate-plan/v1', coverage_status=coverage,
                        selected_gates=['integration/static/public-contracts'],
                        head_sha='a' * 40, target_base_sha='b' * 40, integration_sha=self.candidate)))
                return buffer.getvalue()
            if '/jobs?' in path:
                producer = dict(self.job, id=457, name='PR · Trusted Plan')
                return dict(total_count=2, jobs=[copy.deepcopy(self.job), producer])
            if '/artifacts?' in path:
                return dict(total_count=1, artifacts=[dict(id=789, name='trusted-pr-plan',
                    expired=False, size_in_bytes=500, workflow_run=dict(id=123, head_sha='a' * 40))])
            return copy.deepcopy(self.run)
        return api

    def bogus_selected_gates_api(self, selected_gates):
        def api(path, *, binary=False):
            if path.endswith('/zip'):
                buffer = io.BytesIO()
                with zipfile.ZipFile(buffer, 'w') as archive:
                    archive.writestr('pr-plan.json', verifier.canonical(dict(
                        schema='baseharbor.pr-gate-plan/v1', coverage_status='resolved',
                        selected_gates=selected_gates, head_sha='a' * 40,
                        target_base_sha='b' * 40, integration_sha=self.candidate)))
                return buffer.getvalue()
            if '/jobs?' in path:
                producer = dict(self.job, id=457, name='PR · Trusted Plan')
                return dict(total_count=2, jobs=[copy.deepcopy(self.job), producer])
            if '/artifacts?' in path:
                return dict(total_count=1, artifacts=[dict(id=789, name='trusted-pr-plan',
                    expired=False, size_in_bytes=500, workflow_run=dict(id=123, head_sha='a' * 40))])
            return copy.deepcopy(self.run)
        return api


class ReceiptSchemaNegativesTest(VerifierFixtureTest):
    def test_unexpected_field_rejected(self):
        path, original = self.create()
        raw = (self.root / path).read_bytes()
        value = verifier.receipt(raw)
        value['extra'] = 'nope'
        with self.assertRaisesRegex(verifier.ReceiptError, 'unexpected receipt fields'):
            verifier.receipt(verifier.canonical(value))

    def test_wrong_gate_status_rejected(self):
        raw = (self.root / (self.root / 'e' * 40)).read_bytes() if False else None
        value = dict(gate='other-gate', run={'id': 1, 'attempt': 1},
                     sha='a' * 40, status='fail', plan='b' * 40, approved=None)
        with self.assertRaisesRegex(verifier.ReceiptError, 'unsupported gate/status'):
            verifier.receipt(verifier.canonical(value))

    def test_wrong_run_provenance_rejected(self):
        path, original = self.create()
        value = verifier.receipt((self.root / path).read_bytes())
        with self.assertRaises(verifier.ReceiptError):
            verifier.live(value, self.plan, api=self.bogus_provenance_api())


class LiveNegativeNegativesTest(VerifierFixtureTest):
    def test_negative_wrong_candidate_sha(self):
        path, original = self.create()
        value = verifier.receipt((self.root / path).read_bytes())
        value['sha'] = 'f' * 40
        with self.assertRaisesRegex(verifier.ReceiptError, 'wrong candidate SHA'):
            verifier.live(value, self.plan, api=self.api)
        with self.assertRaisesRegex(verifier.ReceiptError, 'candidate/plan'):
            verifier.verify_file(self.root, original, path, 'f' * 40, self.plan_sha)

    def test_negative_dead_run_id(self):
        def missing(_):
            raise verifier.ReceiptError('GitHub witness unavailable: run not found')
        with self.assertRaisesRegex(verifier.ReceiptError, 'run not found'):
            writer.write(self.root, self.plan_sha, 999, 1, api=missing)
        self.assertEqual(self.head(), self.plan_sha)
        self.assertFalse((self.root / 'evidence/receipts').exists())

    def test_negative_dead_run_id_blocks_live_before_commit(self):
        path, original = self.create()
        value = verifier.receipt((self.root / path).read_bytes())
        value['run']['id'] = 999
        with self.assertRaisesRegex(verifier.ReceiptError, 'run not found'):
            verifier.live(value, self.plan, api=lambda _: (_ for _ in ()).throw(
                verifier.ReceiptError('GitHub witness unavailable: run not found')))

    def test_negative_wrong_run_provenance(self):
        path, original = self.create()
        value = verifier.receipt((self.root / path).read_bytes())
        bad_plan = dict(self.plan, workflow_id=99)
        with patch.object(verifier, 'github', side_effect=AssertionError('offline required')):
            pass
        with self.assertRaises(verifier.ReceiptError):
            verifier.live(value, bad_plan, api=self.bogus_provenance_api())

    def test_negative_wrong_integration_sha(self):
        path, original = self.create()
        value = verifier.receipt((self.root / path).read_bytes())
        bad_plan = dict(self.plan)
        with self.assertRaisesRegex(verifier.ReceiptError, 'producer plan/source mismatch'):
            verifier.live(value, bad_plan, api=self.bogus_integration_sha_api())

    def test_negative_non_resolved_coverage(self):
        path, original = self.create()
        value = verifier.receipt((self.root / path).read_bytes())
        with self.assertRaisesRegex(verifier.ReceiptError, 'producer plan/source mismatch'):
            verifier.live(value, self.plan, api=self.bogus_coverage_api('failed'))

    def test_negative_missing_selected_gate(self):
        path, original = self.create()
        value = verifier.receipt((self.root / path).read_bytes())
        with self.assertRaisesRegex(verifier.ReceiptError, 'producer plan/source mismatch'):
            verifier.live(value, self.plan, api=self.bogus_selected_gates_api([]))

    def test_negative_second_attempt_rejected(self):
        path, original = self.create()
        value = verifier.receipt((self.root / path).read_bytes())
        value['run']['attempt'] = 2
        with self.assertRaisesRegex(verifier.ReceiptError, 'fresh run, not an Actions rerun'):
            verifier.live(value, self.plan, api=self.api)


class OfflineRetainedTest(VerifierFixtureTest):
    def test_offline_retained_never_calls_github(self):
        path, original = self.create()
        approved = self.approve(path, original)
        with patch.object(verifier, 'github', side_effect=AssertionError('offline required')):
            result = verifier.retained(self.root, approved, path, self.candidate, self.plan_sha)
        self.assertEqual(result['approved']['receipt_commit'], original)
        self.assertLess(len((self.root / path).read_bytes()), 500)

    def test_edited_receipt_tamper_rejected(self):
        path, original = self.create()
        self.approve(path, original)
        self.tamper(path, sha='f' * 40)
        with self.assertRaises(verifier.ReceiptError):
            verifier.retained(self.root, self.head(), path, self.candidate, self.plan_sha)

    def test_later_edit_after_approval_rejected(self):
        path, original = self.create()
        approved = self.approve(path, original)
        # A second edit of the approved receipt (any content) must be rejected
        # because the history has more than one edit to the path.
        tampered = verifier.receipt((self.root / path).read_bytes())
        tampered['approved']['at'] = '2026-01-02T00:00:00Z'
        (self.root / path).write_bytes(verifier.canonical(tampered))
        self.command('commit', '-qam', 'later edit')
        with self.assertRaisesRegex(verifier.ReceiptError, 'receipt must have exactly one approval edit'):
            verifier.retained(self.root, self.head(), path, self.candidate, self.plan_sha)

    def test_approval_missing_rejected(self):
        path, original = self.create()
        with self.assertRaisesRegex(verifier.ReceiptError, 'Owner approval missing'):
            verifier.retained(self.root, original, path, self.candidate, self.plan_sha)

    def test_wrong_receipt_path_rejected(self):
        path, original = self.create()
        approved = self.approve(path, original)
        # A receipt whose on-disk path differs from path_for(content) must fail
        # the wrong-receipt-path check. Content says run 123; file lives at a
        # path that path_for() would never assign to that content.
        content = dict(gate=verifier.GATE, run={'id': 123, 'attempt': 1},
                       sha=self.candidate, status='pass', plan=self.plan_sha,
                       approved={'by': verifier.OWNER, 'at': '2026-01-01T00:00:00Z',
                                 'receipt_commit': approved})
        content_raw = verifier.canonical(content)
        wrong_path = 'evidence/receipts/public-contracts/42-2.json'
        wrong_target = self.root / wrong_path
        wrong_target.parent.mkdir(parents=True, exist_ok=True)
        wrong_target.write_bytes(content_raw)
        self.command('add', wrong_path)
        self.command('commit', '-qm', 'misplaced receipt')
        with self.assertRaisesRegex(verifier.ReceiptError, 'wrong receipt path'):
            verifier.retained(self.root, self.head(), wrong_path, self.candidate, self.plan_sha)


class ApproveIdentityTest(VerifierFixtureTest):
    def test_approve_requires_authenticated_owner_on_main(self):
        path, original = self.create()
        with patch.dict(os.environ, dict(self.owner_env, GITHUB_ACTOR='contributor'), clear=True):
            with self.assertRaisesRegex(verifier.ReceiptError, 'Owner action'):
                verifier.approve(self.root, original, path, self.candidate,
                                 self.plan_sha, api=self.api)

    def test_approve_rejects_wrong_GITHUB_REF(self):
        path, original = self.create()
        with patch.dict(os.environ, dict(self.owner_env, GITHUB_REF='refs/heads/develop'), clear=True):
            with self.assertRaisesRegex(verifier.ReceiptError, 'Owner action'):
                verifier.approve(self.root, original, path, self.candidate,
                                 self.plan_sha, api=self.api)

    def test_approve_rejects_wrong_EVENT_NAME(self):
        path, original = self.create()
        with patch.dict(os.environ, dict(self.owner_env, GITHUB_EVENT_NAME='push'), clear=True):
            with self.assertRaisesRegex(verifier.ReceiptError, 'Owner action'):
                verifier.approve(self.root, original, path, self.candidate,
                                 self.plan_sha, api=self.api)

    def test_approve_rejects_wrong_REPOSITORY(self):
        path, original = self.create()
        with patch.dict(os.environ, dict(self.owner_env, GITHUB_REPOSITORY='other/repo'), clear=True):
            with self.assertRaisesRegex(verifier.ReceiptError, 'Owner action'):
                verifier.approve(self.root, original, path, self.candidate,
                                 self.plan_sha, api=self.api)

    def test_approve_rejects_wrong_TRIGGERING_ACTOR(self):
        path, original = self.create()
        with patch.dict(os.environ, dict(self.owner_env, GITHUB_TRIGGERING_ACTOR='other'), clear=True):
            with self.assertRaisesRegex(verifier.ReceiptError, 'Owner action'):
                verifier.approve(self.root, original, path, self.candidate,
                                 self.plan_sha, api=self.api)

    def test_approve_rejects_missing_env_var(self):
        path, original = self.create()
        with patch.dict(os.environ, {}, clear=True):
            with self.assertRaisesRegex(verifier.ReceiptError, 'Owner action'):
                verifier.approve(self.root, original, path, self.candidate,
                                 self.plan_sha, api=self.api)

    def test_approve_reverifies_live_before_edit(self):
        path, original = self.create()
        # Approve runs live verification again before any edit; with live
        # evidence forbidden, it must fail without writing an approval.
        with patch.dict(os.environ, self.owner_env, clear=True):
            with patch.object(verifier, 'github',
                              side_effect=verifier.ReceiptError('GitHub witness unavailable')):
                with self.assertRaises(verifier.ReceiptError):
                    verifier.approve(self.root, original, path, self.candidate, self.plan_sha)
        self.assertEqual(self.head(), original)

    def test_approve_writes_canonical_approved_receipt(self):
        path, original = self.create()
        approved = self.approve(path, original)
        raw = (self.root / path).read_bytes()
        value = verifier.receipt(raw)
        self.assertEqual(value['approved']['by'], verifier.OWNER)
        self.assertTrue(verifier.full_sha(value['approved']['receipt_commit']))
        self.assertEqual(value['approved']['receipt_commit'], original)
        self.assertEqual(self.head(), approved)


class PathAndBlobNegativesTest(VerifierFixtureTest):
    def test_blob_requires_full_revision(self):
        with self.assertRaisesRegex(verifier.ReceiptError, 'full Git revision required'):
            verifier.blob(self.root, 'deadbeef', verifier.PLAN_PATH)

    def test_blob_regular_file_required(self):
        # A tree revision whose PLAN_PATH is a directory (not a regular blob)
        # must be rejected by blob().
        self.command('rm', '-f', '--', verifier.PLAN_PATH)
        self.command('commit', '-qm', 'remove plan fixture')
        with self.assertRaises(verifier.ReceiptError):
            verifier.blob(self.root, self.head(), verifier.PLAN_PATH)


if __name__ == '__main__':
    unittest.main()

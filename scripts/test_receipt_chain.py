"""One-gate receipt boundaries with real Git commits and a fake live API witness."""
import copy
import io
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
import zipfile
from unittest.mock import patch
import receipt_verifier as verifier
import receipt_writer as writer


class ReceiptChainTest(unittest.TestCase):
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
        with patch.dict(os.environ, dict(self.owner_env, GITHUB_ACTOR='contributor'), clear=True):
            with self.assertRaisesRegex(verifier.ReceiptError, 'Owner action'):
                verifier.approve(self.root, original, path, self.candidate, self.plan_sha, api=self.api)
        for raw in (b'{"gate":1,"gate":2}', b'{"gate":NaN}', b' ' * 500):
            with self.subTest(raw=raw[:30]), self.assertRaises(verifier.ReceiptError):
                verifier.receipt(raw)
        self.assertEqual(self.head(), original)


if __name__ == '__main__':
    unittest.main()

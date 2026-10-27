"""Deep verifier negatives, live/offline binding and Owner approval identity.

Scope: the verifier contract itself (receipts, live GitHub witness, retained
history, Owner approve). These tests do not re-test the writer and do not
duplicate the six chain tests in test_receipt_chain.py; they add depth to the
verifier's named error classes, the live/offline boundary and the approval path.

Identity fixtures (CODE-AUDIT F4/F5): every approval case separates the runner
environment CLAIM from the authenticated run PROOF served by the fake witness.
The five environment-negative cases contributed by PR #1107 are integrated here:
each flips one claim while the proof stays genuine, which is exactly what the
old in-shell GITHUB_REF spoofing made untestable. No case ever reaches a real
gh binary: approval calls always inject the API.
"""
import copy
import io
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest
import zipfile
from unittest.mock import patch
import yaml
import receipt_verifier as verifier
import receipt_writer as writer

REPO_ROOT = Path(__file__).resolve().parent.parent
APPROVAL_RUN = 4001
PR_NUMBER = 7
PR_MARKER = f"{verifier.REPOSITORY}#{PR_NUMBER}"
TASK_PATH = "tasks/receipt-pilot.md"
_UNSET = object()


class VerifierFixtureTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name) / "planning"
        self.root.mkdir()
        self.product = Path(self.temp.name) / "product"
        self.product.mkdir()
        self._build_product()
        self.candidate = 'c' * 40
        self._build_planning()
        self.run = dict(id=123, run_attempt=1, repository=dict(id=verifier.REPOSITORY_ID,
                        full_name=verifier.REPOSITORY), workflow_id=17,
                        path='.github/workflows/ci.yml', event='pull_request',
                        head_sha=self.head_sha, status='completed', conclusion='success')
        self.job = dict(id=456, run_id=123, head_sha=self.head_sha,
                        name='PR · Public Contracts', status='completed', conclusion='success')
        self.approval_run = dict(id=APPROVAL_RUN, run_attempt=1,
                                 event='workflow_dispatch', path=verifier.APPROVAL_WORKFLOW,
                                 repository=dict(id=verifier.REPOSITORY_ID,
                                                 full_name=verifier.REPOSITORY),
                                 head_branch='main', head_sha=self.head_sha,
                                 actor=dict(login=verifier.OWNER))
        self.pull = dict(number=PR_NUMBER, state='open',
                         head=dict(sha=self.head_sha),
                         base=dict(sha=self.base_sha,
                                   repo=dict(full_name=verifier.REPOSITORY)))
        self.owner_env = dict(
            GITHUB_ACTOR=verifier.OWNER, GITHUB_TRIGGERING_ACTOR=verifier.OWNER,
            GITHUB_EVENT_NAME='workflow_dispatch', GITHUB_REPOSITORY=verifier.REPOSITORY,
            GITHUB_REF='refs/heads/main', GITHUB_RUN_ID=str(APPROVAL_RUN),
            GITHUB_SHA=self.head_sha,
            # Presence-only fixture value for the writer credential gate; the
            # verifier never consumes or prints it (addendum cc379b1-1).
            PLANNING_RECEIPT_WRITE_TOKEN='fixture-token-not-a-secret')
        # F5 fixture adaptation: the Planning repository gets a real temporary
        # bare remote, so the card publication's push and read-back are genuine
        # Git operations against durable storage — never an origin-less fixture
        # error and never a network access.
        self.planning_remote = Path(self.temp.name) / 'planning-origin.git'
        subprocess.check_output(['git', 'init', '-q', '--bare', str(self.planning_remote)])
        self.command('remote', 'add', 'origin', str(self.planning_remote))

    def seed_remote_main(self):
        """Create origin/main as an exact copy of the current local planning
        history — the ordinary precondition for fast-forward publication
        pushes. Tests that publish use this; tests that must fail closed
        deliberately leave the remote empty."""
        self.command('push', '-q', 'origin', 'HEAD:refs/heads/main')

    def remote_main(self):
        probe = subprocess.run(['git', 'ls-remote', str(self.planning_remote),
                                'refs/heads/main'], capture_output=True, text=True)
        fields = probe.stdout.split()
        return fields[0] if fields else ''

    def _build_product(self):
        self._pcommand('init', '-q')
        self._pcommand('config', 'user.name', 'Receipt fixture')
        self._pcommand('config', 'user.email', 'receipt@example.invalid')
        self._pwrite('.github/workflows/ci.yml',
                     b'name: CI\njobs:\n  gate:\n    runs-on: ubuntu-latest\n')
        self._pwrite('scripts/receipt_verifier.py',
                     Path(verifier.__file__).resolve().read_bytes())
        self._pwrite('.github/workflows/receipt-approval.yml',
                     (REPO_ROOT / '.github/workflows/receipt-approval.yml').read_bytes())
        self._pcommand('add', '.')
        self._pcommand('commit', '-qm', 'reviewed protected base')
        self.base_sha = self._phead()
        self.pin = self.base_sha
        self._pwrite('feature.txt', b'contributor change that does not touch ci.yml\n')
        self._pcommand('add', '.')
        self._pcommand('commit', '-qm', 'PR head')
        self.head_sha = self._phead()

    def _pwrite(self, rel, raw):
        target = self.product / rel
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes(raw)

    def _pcommand(self, *args):
        return subprocess.check_output(['git', '-C', str(self.product), *args],
                                       stderr=subprocess.PIPE)

    def _phead(self):
        return self._pcommand('rev-parse', 'HEAD').decode().strip()

    def _build_planning(self):
        self.command('init', '-q')
        self.command('config', 'user.name', 'Receipt fixture')
        self.command('config', 'user.email', 'receipt@example.invalid')
        self.plan = dict(gate=verifier.GATE, repository=verifier.REPOSITORY, workflow_id=17,
                         head_sha=self.head_sha, candidate_sha=self.candidate,
                         base_sha=self.base_sha, task=TASK_PATH)
        path = self.root / verifier.PLAN_PATH
        path.parent.mkdir(parents=True)
        path.write_bytes(verifier.canonical(self.plan))
        task = self.root / TASK_PATH
        task.parent.mkdir(exist_ok=True)
        task.write_bytes(f"---\nid: receipt-pilot\nprs: [{PR_MARKER}]\n---\npilot\n".encode())
        self.command('add', '.')
        self.command('commit', '-qm', 'plan and task fixture')
        self.plan_sha = self.head()

    def command(self, *args):
        return subprocess.check_output(['git', '-C', str(self.root), *args],
                                       stderr=subprocess.PIPE)

    def head(self):
        return self.command('rev-parse', 'HEAD').decode().strip()

    def edits(self, path):
        return self.command('log', '--full-history', '--format=%H', 'HEAD',
                            '--', path).decode().splitlines()

    def plan_zip(self):
        buffer = io.BytesIO()
        with zipfile.ZipFile(buffer, 'w') as archive:
            archive.writestr('pr-plan.json', verifier.canonical(dict(
                schema='baseharbor.pr-gate-plan/v1', coverage_status='resolved',
                selected_gates=['integration/static/public-contracts'],
                head_sha=self.head_sha, target_base_sha=self.base_sha,
                integration_sha=self.candidate)))
        return buffer.getvalue()

    def api(self, path, *, binary=False):
        if binary:
            return self.plan_zip()
        prefix = f"repos/{verifier.REPOSITORY}"
        if path.startswith(prefix + "/pulls/"):
            pull = copy.deepcopy(self.pull)
            # The forward PR->task resolver needs a PR body referencing the bound
            # task. Seed a deterministic "Closes #<issue>" closer so the hermetic
            # API resolves the authoritative PR->task binding (F4).
            if pull.get("body") is None:
                pull["body"] = "Implements the pilot.\n\nCloses #%d\n" % (PR_NUMBER + 1)
            return pull
        if "/issues/" in path:
            return {"number": PR_NUMBER + 1, "title": "receipt-pilot.md",
                    "body": "tasks/receipt-pilot.md"}
        if path == prefix + f"/actions/runs/{APPROVAL_RUN}":
            return copy.deepcopy(self.approval_run)
        import re as _re
        matched = _re.match(_re.escape(prefix + "/actions/runs/") + r"([0-9]+)(?=$|[/?])", path)
        if matched:
            if int(matched.group(1)) != self.run['id']:
                return copy.deepcopy(self.approval_run)
            if '/artifacts?' in path:
                return dict(total_count=1, artifacts=[dict(
                    id=789, name='trusted-pr-plan', expired=False, size_in_bytes=500,
                    workflow_run=dict(id=self.run["id"], head_sha=self.head_sha))])
            if '/jobs?' in path:
                producer = dict(self.job, id=457, name='PR · Trusted Plan')
                return dict(total_count=2, jobs=[copy.deepcopy(self.job), producer])
            return copy.deepcopy(self.run)
        raise verifier.ReceiptError("GitHub witness unavailable: run or resource not found")

    def create(self):
        return writer.write(self.root, self.plan_sha, 123, 1, api=self.api)

    def approve(self, path, original, *, env=None, api=None, product=_UNSET, pin=None,
                plan_revision=None):
        with patch.dict(os.environ, self.owner_env if env is None else env, clear=True):
            return verifier.approve(self.root, original, path, self.candidate,
                                    self.plan_sha if plan_revision is None else plan_revision,
                                    api=self.api if api is None else api,
                                    product=self.product if product is _UNSET else product,
                                    pin=self.pin if pin is None else pin)

    def approve_pr(self, pr=PR_NUMBER, *, api=None):
        with patch.dict(os.environ, self.owner_env, clear=True):
            return verifier.approve_pr(self.root, self.product, pr,
                                       api=self.api if api is None else api, pin=self.pin)

    def use_fake_publish(self):
        def publish(root, path, local_commit):
            # W2 interface: (path, introduction, publication) from durable
            # history, so an already-published receipt returns the original
            # introduction and the last receipt edit as publication — never a
            # fresh pseudo-publication that would trigger a second card edit.
            edits = subprocess.check_output(
                ['git', '-C', str(root), 'log', '--full-history', '--format=%H',
                 local_commit, '--', path], stderr=subprocess.PIPE
            ).decode().split()
            if not edits:
                return path, local_commit, local_commit
            return path, edits[-1], edits[0]
        patcher = patch.object(writer, 'publish', publish, create=True)
        patcher.start()
        self.addCleanup(patcher.stop)

    def tamper(self, path, sha='f' * 40):
        raw = (self.root / path).read_bytes()
        changed = verifier.receipt(raw)
        changed['sha'] = sha
        (self.root / path).write_bytes(verifier.canonical(changed))
        self.command('commit', '-qam', 'tamper fixture')

    def bogus_provenance_api(self):
        def api(path, *, binary=False):
            if binary or '/jobs?' in path:
                return self.api(path, binary=binary)
            if path.endswith(f"/actions/runs/{APPROVAL_RUN}") or '/attempts/' not in path:
                if '/attempts/' not in path and not path.startswith(
                        f"repos/{verifier.REPOSITORY}/actions/runs/{self.run['id']}"):
                    return self.api(path, binary=binary)
            run = copy.deepcopy(self.run)
            run['workflow_id'] = 99
            run['event'] = 'push'
            return run
        return api

    def bogus_integration_sha_api(self):
        def api(path, *, binary=False):
            if binary:
                buffer = io.BytesIO()
                with zipfile.ZipFile(buffer, 'w') as archive:
                    archive.writestr('pr-plan.json', verifier.canonical(dict(
                        schema='baseharbor.pr-gate-plan/v1', coverage_status='resolved',
                        selected_gates=['integration/static/public-contracts'],
                        head_sha=self.head_sha, target_base_sha=self.base_sha,
                        integration_sha='f' * 40)))
                return buffer.getvalue()
            return self.api(path, binary=binary)
        return api

    def bogus_coverage_api(self, coverage):
        def api(path, *, binary=False):
            if binary:
                buffer = io.BytesIO()
                with zipfile.ZipFile(buffer, 'w') as archive:
                    archive.writestr('pr-plan.json', verifier.canonical(dict(
                        schema='baseharbor.pr-gate-plan/v1', coverage_status=coverage,
                        selected_gates=['integration/static/public-contracts'],
                        head_sha=self.head_sha, target_base_sha=self.base_sha,
                        integration_sha=self.candidate)))
                return buffer.getvalue()
            return self.api(path, binary=binary)
        return api

    def bogus_selected_gates_api(self, selected_gates):
        def api(path, *, binary=False):
            if binary:
                buffer = io.BytesIO()
                with zipfile.ZipFile(buffer, 'w') as archive:
                    archive.writestr('pr-plan.json', verifier.canonical(dict(
                        schema='baseharbor.pr-gate-plan/v1', coverage_status='resolved',
                        selected_gates=selected_gates, head_sha=self.head_sha,
                        target_base_sha=self.base_sha, integration_sha=self.candidate)))
                return buffer.getvalue()
            return self.api(path, binary=binary)
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
            verifier.live(value, self.plan, api=lambda *_a, **_k: (_ for _ in ()).throw(
                verifier.ReceiptError('GitHub witness unavailable: run not found')))

    def test_negative_wrong_run_provenance(self):
        path, original = self.create()
        value = verifier.receipt((self.root / path).read_bytes())
        bad_plan = dict(self.plan, workflow_id=99)
        with self.assertRaises(verifier.ReceiptError):
            verifier.live(value, bad_plan, api=self.bogus_provenance_api())

    def test_negative_wrong_integration_sha(self):
        path, original = self.create()
        value = verifier.receipt((self.root / path).read_bytes())
        with self.assertRaisesRegex(verifier.ReceiptError, 'producer plan/source mismatch'):
            verifier.live(value, self.plan, api=self.bogus_integration_sha_api())

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
        tampered = verifier.receipt((self.root / path).read_bytes())
        tampered['approved']['at'] = '2026-01-02T00:00:00Z'
        (self.root / path).write_bytes(verifier.canonical(tampered))
        self.command('commit', '-qam', 'later edit')
        with self.assertRaisesRegex(verifier.ReceiptError,
                                    'receipt must have exactly one approval edit'):
            verifier.retained(self.root, self.head(), path, self.candidate, self.plan_sha)

    def test_approval_missing_rejected(self):
        path, original = self.create()
        with self.assertRaisesRegex(verifier.ReceiptError, 'Owner approval missing'):
            verifier.retained(self.root, original, path, self.candidate, self.plan_sha)

    def test_wrong_receipt_path_rejected(self):
        path, original = self.create()
        approved = self.approve(path, original)
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


class PathAndBlobNegativesTest(VerifierFixtureTest):
    def test_blob_requires_full_revision(self):
        with self.assertRaisesRegex(verifier.ReceiptError, 'full Git revision required'):
            verifier.blob(self.root, 'deadbeef', verifier.PLAN_PATH)

    def test_blob_regular_file_required(self):
        self.command('rm', '-f', '--', verifier.PLAN_PATH)
        self.command('commit', '-qm', 'remove plan fixture')
        with self.assertRaises(verifier.ReceiptError):
            verifier.blob(self.root, self.head(), verifier.PLAN_PATH)


class ApproveIdentityTest(VerifierFixtureTest):
    """Environment CLAIM negatives against a genuine PROOF (#1107 integration)."""

    def test_approve_requires_authenticated_owner_on_main(self):
        path, original = self.create()
        with self.assertRaisesRegex(verifier.ReceiptError, 'Owner action'):
            self.approve(path, original, env=dict(self.owner_env, GITHUB_ACTOR='contributor'))

    def test_approve_rejects_wrong_GITHUB_REF(self):
        path, original = self.create()
        with self.assertRaisesRegex(verifier.ReceiptError, 'Owner action'):
            self.approve(path, original, env=dict(self.owner_env, GITHUB_REF='refs/heads/develop'))

    def test_approve_rejects_wrong_EVENT_NAME(self):
        path, original = self.create()
        with self.assertRaisesRegex(verifier.ReceiptError, 'Owner action'):
            self.approve(path, original,
                         env=dict(self.owner_env, GITHUB_EVENT_NAME='push'))

    def test_approve_rejects_wrong_REPOSITORY(self):
        path, original = self.create()
        with self.assertRaisesRegex(verifier.ReceiptError, 'Owner action'):
            self.approve(path, original, env=dict(self.owner_env, GITHUB_REPOSITORY='other/repo'))

    def test_approve_rejects_wrong_TRIGGERING_ACTOR(self):
        path, original = self.create()
        with self.assertRaisesRegex(verifier.ReceiptError, 'Owner action'):
            self.approve(path, original, env=dict(self.owner_env, GITHUB_TRIGGERING_ACTOR='other'))

    def test_approve_rejects_missing_env_var(self):
        path, original = self.create()
        with self.assertRaisesRegex(verifier.ReceiptError, 'Owner action'):
            self.approve(path, original, env={})

    def test_approve_rejects_caller_spoofed_identity_via_env_only(self):
        # A caller may not carry approval by setting env strings alone: a fully
        # self-consistent claim block pointing at another run id fails against
        # the authenticated proof.
        path, original = self.create()
        with self.assertRaisesRegex(verifier.ReceiptError, 'run identity mismatch'):
            self.approve(path, original, env=dict(self.owner_env, GITHUB_RUN_ID='9999'))

    def test_approve_reverifies_live_before_edit(self):
        path, original = self.create()

        def witness_down(path_, *, binary=False):
            if path_ == f"repos/{verifier.REPOSITORY}/actions/runs/{APPROVAL_RUN}":
                return copy.deepcopy(self.approval_run)
            raise verifier.ReceiptError('GitHub witness unavailable: witness transport failure')

        # The failing API is injected; the real gh module function is additionally
        # poisoned so this test can never touch a real binary (audit finding).
        with patch.object(verifier, 'github',
                          side_effect=AssertionError('real gh must never be called')):
            with self.assertRaises(verifier.ReceiptError):
                self.approve(path, original, api=witness_down)
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
        self.assertEqual(len(self.edits(path)), 2)


class AuthenticatedProofTest(VerifierFixtureTest):
    """Proof-side negatives: what the API says wins over any claim."""

    def _approve_with(self, **proof_overrides):
        path, original = self.create()
        previous = {key: self.approval_run[key] for key in proof_overrides}
        self.approval_run.update(proof_overrides)
        try:
            self.approve(path, original)
        finally:
            self.approval_run.update(previous)

    def test_proof_rejects_untrusted_branch(self):
        with self.assertRaisesRegex(verifier.ReceiptError, 'untrusted ref'):
            self._approve_with(head_branch='feature/anything')

    def test_proof_rejects_actor_mismatch(self):
        with self.assertRaisesRegex(verifier.ReceiptError, 'actor proof mismatch'):
            self._approve_with(actor=dict(login='contributor'))

    def test_proof_rejects_event_mismatch(self):
        with self.assertRaisesRegex(verifier.ReceiptError, 'event proof mismatch'):
            self._approve_with(event='schedule')

    def test_proof_rejects_repository_mismatch(self):
        with self.assertRaisesRegex(verifier.ReceiptError, 'repository proof mismatch'):
            self._approve_with(repository=dict(id=999, full_name=verifier.REPOSITORY))

    def test_proof_rejects_wrong_workflow_path(self):
        with self.assertRaisesRegex(verifier.ReceiptError, 'workflow definition proof mismatch'):
            self._approve_with(path='.github/workflows/ci.yml')

    def test_proof_rejects_head_sha_claim_mismatch(self):
        path, original = self.create()
        with self.assertRaisesRegex(verifier.ReceiptError, 'source claim mismatch'):
            self.approve(path, original, env=dict(self.owner_env, GITHUB_SHA='e' * 40))

    def test_approve_rejects_untrusted_ref_dispatch(self):
        # F7 source policy: only main is an authorized ref. An authenticated run
        # whose real ref is develop (even with a matching ref claim) is rejected,
        # not accepted. A transitional or otherwise untrusted ref is never an
        # exception (CODE-AUDIT F7); the allowlist is ("main",).
        path, original = self.create()
        previous_branch = self.approval_run['head_branch']
        previous_ref = self.owner_env['GITHUB_REF']
        self.approval_run['head_branch'] = 'develop'
        self.owner_env['GITHUB_REF'] = 'refs/heads/develop'
        try:
            with self.assertRaisesRegex(verifier.ReceiptError, 'untrusted ref'):
                self.approve(path, original)
        finally:
            self.approval_run['head_branch'] = previous_branch
            self.owner_env['GITHUB_REF'] = previous_ref


class TrustedSourceTest(VerifierFixtureTest):
    def test_unknown_pin_revision_fails_closed(self):
        path, original = self.create()
        with self.assertRaises(verifier.ReceiptError):
            self.approve(path, original, pin='e' * 40)

    def test_non_hex_pin_revision_rejected(self):
        path, original = self.create()
        with self.assertRaisesRegex(verifier.ReceiptError, 'full SHA'):
            self.approve(path, original, pin='main')

    def test_helper_bytes_diverging_from_pin_rejected(self):
        path, original = self.create()
        self._pwrite('scripts/receipt_verifier.py', b'print("edited elsewhere")\n')
        self._pcommand('add', '.')
        self._pcommand('commit', '-qm', 'diverging helper bytes')
        divergent = self._phead()
        with self.assertRaisesRegex(verifier.ReceiptError, 'differs from reviewed pinned source'):
            self.approve(path, original, pin=divergent)

    def test_approval_workflow_diverging_from_pin_rejected(self):
        path, original = self.create()
        self._pwrite('.github/workflows/receipt-approval.yml',
                     b'name: edited approval workflow\n')
        self._pcommand('add', '.')
        self._pcommand('commit', '-qm', 'edited approval workflow')
        edited_head = self._phead()
        previous = self.approval_run['head_sha']
        self.approval_run['head_sha'] = edited_head
        try:
            with self.assertRaisesRegex(verifier.ReceiptError,
                                        'approval workflow differs from reviewed pinned'):
                self.approve(path, original,
                             env=dict(self.owner_env, GITHUB_SHA=edited_head))
        finally:
            self.approval_run['head_sha'] = previous

    def test_missing_product_checkout_fails_closed(self):
        path, original = self.create()
        with self.assertRaisesRegex(verifier.ReceiptError, 'product checkout required'):
            self.approve(path, original, product=None)


class ProducerIdentityTest(VerifierFixtureTest):
    def _product_commit(self, rel, raw, message):
        self._pwrite(rel, raw)
        self._pcommand('add', '.')
        self._pcommand('commit', '-qm', message)
        return self._phead()

    def test_fork_edited_producer_workflow_rejected(self):
        head = self._product_commit('.github/workflows/ci.yml',
                                    b'name: CI\njobs:\n  gate:\n    if: true\n',
                                    'fork edits CI definition')
        with self.assertRaisesRegex(verifier.ReceiptError,
                                    'producer workflow definition differs'):
            verifier.producer_identity(self.product, dict(self.plan, head_sha=head))

    def test_producer_change_outside_workflow_bytes_accepted(self):
        head = self._product_commit('docs/other.md', b'irrelevant change\n', 'no CI change')
        verifier.producer_identity(self.product, dict(self.plan, head_sha=head))

    def test_missing_producer_workflow_fails_closed(self):
        head = self._product_commit('.github/workflows/ci.yml', b'', 'delete CI bytes')
        self._pcommand('rm', '.github/workflows/ci.yml')
        self._pcommand('commit', '-qm', 'remove CI definition')
        with self.assertRaises(verifier.ReceiptError):
            verifier.producer_identity(self.product, dict(self.plan, head_sha=head))

    def test_approval_end_to_end_rejects_workflow_changing_pr(self):
        # The genuine approval path: every authenticated witness stays
        # consistent with the new head; only the ci.yml bytes differ. This is
        # exactly the case trust-by-job-name used to wave through (F8).
        head = self._product_commit('.github/workflows/ci.yml',
                                    b'name: CI\njobs:\n  gate:\n    runs-on: evil\n',
                                    'workflow-changing PR')
        self.head_sha = head
        self.run.update(id=777, head_sha=head)
        self.job.update(run_id=777, head_sha=head)
        self.approval_run['head_sha'] = head
        self.owner_env['GITHUB_SHA'] = head
        self.pull['head']['sha'] = head
        self.plan['head_sha'] = head
        (self.root / verifier.PLAN_PATH).write_bytes(verifier.canonical(self.plan))
        self.command('commit', '-qam', 'plan follows edited head')
        new_plan = self.head()
        path, revision = writer.write(self.root, new_plan, 777, 1, api=self.api)
        with self.assertRaisesRegex(verifier.ReceiptError,
                                    'producer workflow definition differs'):
            self.approve(path, revision, plan_revision=new_plan)


class PrApprovalResolutionTest(VerifierFixtureTest):
    """The Owner selects only the PR; everything else auto-resolves or blocks."""

    def test_approve_pr_happy_path(self):
        path, original = self.create()
        self.seed_remote_main()
        result = self.approve_pr()
        self.assertEqual(result['path'], 'evidence/receipts/public-contracts/123-1.json')
        self.assertEqual(result['introduction'], original)
        # The card publication adds one commit on top of the receipt approval
        # edit; the returned publication is that final durable revision and it
        # equals the pushed remote main — verified against the real bare remote.
        self.assertTrue(verifier.full_sha(result['publication']))
        self.assertEqual(self.remote_main(), result['publication'])
        self.assertEqual(len(self.edits(path)), 2)
        with patch.object(verifier, 'github', side_effect=AssertionError('offline required')):
            verifier.retained(self.root, result['publication'], path, self.candidate,
                              self.plan_sha)

    def test_approve_pr_rejects_stale_pr(self):
        self.create()
        self.use_fake_publish()
        self.pull['state'] = 'closed'
        with self.assertRaisesRegex(verifier.ReceiptError, 'stale or closed'):
            self.approve_pr()

    def test_approve_pr_rejects_foreign_base(self):
        self.create()
        self.use_fake_publish()
        self.pull['base']['repo']['full_name'] = 'fork/unrelated'
        with self.assertRaisesRegex(verifier.ReceiptError, 'base repository is not'):
            self.approve_pr()

    def test_approve_pr_rejects_ambiguous_two_receipts(self):
        self.create()
        self.use_fake_publish()
        self.run.update(id=777)
        self.job.update(run_id=777)
        writer.write(self.root, self.plan_sha, 777, 1, api=self.api)
        with self.assertRaisesRegex(verifier.ReceiptError, 'missing or ambiguous'):
            self.approve_pr()

    def test_approve_pr_rejects_unbound_pr(self):
        self.use_fake_publish()
        with self.assertRaisesRegex(verifier.ReceiptError, 'missing or ambiguous'):
            self.approve_pr()

    def test_approve_pr_rejects_ambiguous_task_claim(self):
        self.use_fake_publish()
        second = self.root / 'tasks/other-pilot.md'
        second.write_bytes(f"---\nprs: [{PR_MARKER}]\n---\n".encode())
        self.command('add', 'tasks/other-pilot.md')
        self.command('commit', '-qm', 'second task claims the same PR')
        self.run.update(id=777)
        self.job.update(run_id=777)
        # Exactly one receipt, bound to the fresh plan revision where two task
        # notes now claim this PR.
        writer.write(self.root, self.head(), 777, 1, api=self.api)
        with self.assertRaisesRegex(verifier.ReceiptError, 'PR-to-task association'):
            self.approve_pr(pr=PR_NUMBER)

    def test_approve_pr_requires_existing_task_note(self):
        self.run.update(id=777)
        self.job.update(run_id=777)
        self.command('rm', '--', TASK_PATH)
        self.command('commit', '-qm', 'delete task note')
        plan2 = self.head()
        writer.write(self.root, plan2, 777, 1, api=self.api)
        with self.assertRaises(verifier.ReceiptError):
            with patch.dict(os.environ, self.owner_env, clear=True):
                verifier.approve(self.root, self.edits(
                    'evidence/receipts/public-contracts/777-1.json')[-1],
                    'evidence/receipts/public-contracts/777-1.json', self.candidate, plan2,
                    api=self.api, product=self.product, pin=self.pin)

    def test_already_approved_is_idempotent_and_never_edits_twice(self):
        path, original = self.create()
        self.seed_remote_main()
        first = self.approve_pr()
        head_before = self.head()
        self.assertEqual(first['publication'], head_before)
        again = self.approve_pr()
        self.assertEqual(again['publication'], head_before)
        self.assertEqual(self.head(), head_before)
        self.assertEqual(self.remote_main(), head_before)
        self.assertEqual(len(self.edits(path)), 2)


    def test_binding_accepts_block_list_and_quoted_entries(self):
        # F4 verifier side: the backward mapping is Planning front-matter
        # (schema), never the removed unquoted inline regex. Block lists and
        # quoted entries must bind identically.
        (self.root / TASK_PATH).write_bytes(
            f"---\nid: receipt-pilot\nprs:\n- \"{PR_MARKER}\"\n---\npilot\n".encode())
        self.command('add', '.')
        self.command('commit', '-qm', 'block-list prs metadata')
        plan2 = self.head()
        self.run.update(id=777)
        self.job.update(run_id=777)
        path, _ = writer.write(self.root, plan2, 777, 1, api=self.api)
        self.seed_remote_main()
        result = self.approve_pr()
        self.assertEqual(result['path'], path)

    def test_binding_rejects_malformed_front_matter(self):
        # A task note whose metadata does not parse is a hard block, never a
        # loose-text fallback.
        (self.root / 'tasks/broken-pilot.md').write_bytes(b"---\nprs: [\n---\nbody\n")
        self.command('add', 'tasks/broken-pilot.md')
        self.command('commit', '-qm', 'malformed front matter')
        self.run.update(id=777)
        self.job.update(run_id=777)
        writer.write(self.root, self.head(), 777, 1, api=self.api)
        with self.assertRaisesRegex(verifier.ReceiptError, 'invalid YAML'):
            self.approve_pr()

    def test_approval_blocks_before_push_without_write_credential(self):
        # Addendum cc379b1-1: a missing write token fails closed BEFORE any
        # push; the publication mechanism is never invoked and the remote is
        # never touched; the message never echoes a credential value.
        path, original = self.create()
        self.seed_remote_main()
        remote_before = self.remote_main()
        calls = []
        real = writer.publish

        def spy(root, p, c):
            calls.append((str(p), c))
            return real(root, p, c)

        env = {k: v for k, v in self.owner_env.items() if k != verifier.PUSH_CREDENTIAL_ENV}
        with patch.dict(os.environ, env, clear=True), patch.object(writer, 'publish', spy):
            with self.assertRaisesRegex(verifier.ReceiptError, 'write credential') as caught:
                verifier.approve_pr(self.root, self.product, PR_NUMBER,
                                     api=self.api, pin=self.pin)
        self.assertEqual(calls, [])
        self.assertEqual(self.remote_main(), remote_before)
        self.assertNotIn('fixture-token-not-a-secret', str(caught.exception))

    def test_approval_pushes_only_through_writer_mechanism(self):
        # Addendum cc379b1-1: both durable pushes of the approval chain (the
        # receipt publication and the card publication) run through the single
        # receipt_writer mechanism — the verifier invents no second push path.
        path, original = self.create()
        self.seed_remote_main()
        calls = []
        real = writer.publish

        def spy(root, p, c):
            calls.append(str(p))
            return real(root, p, c)

        with patch.object(writer, 'publish', spy):
            result = self.approve_pr()
        self.assertEqual(sorted(calls), sorted([path, TASK_PATH]))
        self.assertEqual(self.remote_main(), result['publication'])


class PublishInterfaceTest(VerifierFixtureTest):
    def test_missing_publish_interface_blocks_publication(self):
        # Finding BF-W3-1: on the frozen develop base receipt_writer has no
        # publish(); the approval must fail closed, keep exactly one local edit
        # and export nothing.
        path, original = self.create()
        with self.assertRaisesRegex(verifier.ReceiptError, 'BF-W3-1'):
            self.approve_pr()
        self.assertEqual(len(self.edits(path)), 2)
        self.assertFalse(any(self.root.glob('**/*.patch')))

    def test_retry_after_publish_failure_creates_no_second_edit(self):
        path, original = self.create()
        self.seed_remote_main()
        remote_before = self.remote_main()
        state = {'first': True}
        real = writer.publish

        def publish(root, receipt_path, local_commit):
            # Preserve the deliberate push failure; recover through the real
            # writer mechanism (in-process auth, durable read-back) — never a
            # silently skipped push.
            if state['first']:
                state['first'] = False
                raise verifier.ReceiptError('push rejected: expected-head mismatch')
            return real(root, receipt_path, local_commit)

        patcher = patch.object(writer, 'publish', publish)
        patcher.start()
        self.addCleanup(patcher.stop)
        with self.assertRaisesRegex(verifier.ReceiptError, 'expected-head'):
            self.approve_pr()
        self.assertEqual(self.remote_main(), remote_before)
        result = self.approve_pr()
        self.assertEqual(len(self.edits(path)), 2)
        self.assertEqual(self.remote_main(), result['publication'])
        self.assertTrue(verifier.full_sha(result['publication']))


WORKFLOW = REPO_ROOT / ".github/workflows/receipt-approval.yml"
SHADOW_WORKFLOW = REPO_ROOT / ".github/workflows/receipt-shadow.yml"


class WorkflowContextRegressionTest(unittest.TestCase):
    """Local guard for the F4 correction: the workflow file itself must not
    re-introduce caller-set identity strings, patch exports or manual fields."""

    def setUp(self):
        import yaml
        self.text = WORKFLOW.read_text()
        self.doc = yaml.safe_load(self.text)
        self.job = self.doc['jobs']['approve']

    def triggers(self):
        return self.doc.get('on', self.doc.get(True))

    def env_keys(self):
        keys = set(self.doc.get('env', {})) | set(self.job.get('env', {}))
        for step in self.job['steps']:
            keys |= set(step.get('env', {}))
        return keys

    def test_no_github_env_overrides(self):
        self.assertEqual({k for k in self.env_keys() if k.startswith('GITHUB_')}, set())

    def test_no_in_shell_identity_assignment(self):
        for step in self.job['steps']:
            if 'run' in step:
                self.assertNotRegex(step['run'], r'GITHUB_[A-Z_]+\s*=')

    def test_human_input_is_pr_selection_only(self):
        self.assertEqual(set(self.triggers()['workflow_dispatch']['inputs']),
                         {'pr_number'})

    def test_owner_gate_uses_non_spoofable_context(self):
        self.assertIn("github.actor == 'mcpdev80'", self.job['if'])
        self.assertIn("github.triggering_actor == 'mcpdev80'", self.job['if'])
        self.assertIn('workflow_dispatch', self.job['if'])

    def test_no_patch_export_or_artifact_upload(self):
        self.assertNotIn('format-patch', self.text)
        self.assertNotIn('upload-artifact', self.text)

    def test_no_automatic_trigger_armed(self):
        self.assertEqual(set(self.triggers()), {'workflow_dispatch'})

    def test_product_checkout_follows_authenticated_head(self):
        checkout = self.job['steps'][0]
        self.assertNotIn('ref', checkout.get('with', {}))
        self.assertEqual(checkout['with']['fetch-depth'], 0)

    def test_verifier_module_has_no_hardcoded_ref_spoof(self):
        source = Path(verifier.__file__).read_text()
        self.assertNotIn('refs/heads/main"', source)
        self.assertIn('POLICY_BRANCHES', source)

    def test_approval_carries_publication_and_pins_immutable_source(self):
        # R27-F6 output plumbing: the final durable publication is a declared
        # job output, forwarded into the reusable call, and the reusable
        # source is pinned to a reviewed immutable full SHA (never @develop).
        doc = yaml.safe_load(WORKFLOW.read_text())
        job = doc['jobs']['approve']
        self.assertEqual(set(job['outputs']),
                         {'receipt', 'plan', 'candidate', 'publication'})
        shadow = doc['jobs']['shadow']
        self.assertEqual(set(shadow['with']),
                         {'receipt', 'plan', 'candidate', 'publication'})
        self.assertEqual(shadow['with']['publication'],
                         '${{ needs.approve.outputs.publication }}')
        import re as _re
        pinned = _re.fullmatch(
            r"mcpdev80/baseharbor/\.github/workflows/receipt-shadow\.yml"
            r"@([0-9a-f]{40})", shadow['uses'])
        self.assertIsNotNone(pinned, 'reusable shadow call must be SHA-pinned')
        self.assertNotRegex(WORKFLOW.read_text(),
                            r'receipt-shadow\.yml@(develop|main|master)\b')

    def test_approval_step_fails_explicitly_on_missing_outputs(self):
        # Missing outputs are an explicit step failure, not silent skips.
        step = next(s for s in self.job['steps'] if s.get('id') == 'approval-step')
        self.assertIn('missing or not a full SHA', step['run'])
        self.assertIn("for key in publication plan candidate", step['run'])


class CliOutputContractTest(unittest.TestCase):
    """R27-F6: the CLI must emit each RESULT field under its own name.

    The reproduced defect was `plan: <introduction>` and
    `candidate: <approval>` — so this executes main() with five deliberately
    distinct values and requires the exact printed tuple.
    """

    RESULT = {'path': 'evidence/receipts/public-contracts/123-1.json',
              'introduction': '1' * 40, 'approval': '2' * 40,
              'publication': '9' * 40, 'plan': '4' * 40, 'candidate': '5' * 40}

    def run_main(self, *, result=None, mode='approve'):
        import contextlib
        argv = ['receipt_verifier.py', mode, '--planning', '/dev/null',
                '--product', '/dev/null', '--pr', '7']
        stdout, stderr = io.StringIO(), io.StringIO()
        with patch.object(sys, 'argv', argv), \
                patch.object(verifier, 'approve_pr',
                             return_value=dict(self.RESULT if result is None
                                               else result)), \
                contextlib.redirect_stdout(stdout), \
                contextlib.redirect_stderr(stderr):
            try:
                verifier.main()
                code = 0
            except SystemExit as exc:
                code = exc.code
        return code, stdout.getvalue(), stderr.getvalue()

    def test_approve_emits_exact_field_tuple(self):
        code, out, err = self.run_main()
        self.assertEqual(code, 0, err)
        self.assertEqual(out.splitlines(), [
            'approved: ' + self.RESULT['path'],
            'introduction: ' + '1' * 40,
            'approval: ' + '2' * 40,
            'publication: ' + '9' * 40,
            'receipt: ' + self.RESULT['path'],
            'plan: ' + '4' * 40,
            'candidate: ' + '5' * 40,
        ])

    def test_missing_result_field_is_explicit_failure(self):
        broken = dict(self.RESULT)
        broken.pop('publication')
        code, out, err = self.run_main(result=broken)
        self.assertEqual(code, 1)
        self.assertIn('publication', err)
        self.assertNotIn('publication:', out)

    def test_malformed_plan_field_is_explicit_failure(self):
        broken = dict(self.RESULT, plan='deadbeef')
        code, out, err = self.run_main(result=broken)
        self.assertEqual(code, 1)
        self.assertIn('plan', err)
        self.assertNotIn('plan:', out)


class ShadowWorkflowContractTest(unittest.TestCase):
    def setUp(self):
        self.doc = yaml.safe_load(SHADOW_WORKFLOW.read_text())

    def test_only_workflow_call_trigger(self):
        self.assertEqual(set(self.doc.get('on', self.doc.get(True))), {'workflow_call'})

    def test_publication_is_required_input(self):
        call = self.doc['on' if 'on' in self.doc else True]['workflow_call']['inputs']
        self.assertEqual(set(call), {'receipt', 'plan', 'candidate', 'publication'})
        self.assertTrue(all(spec['required'] for spec in call.values()))

    def test_guard_never_skips_on_missing_inputs(self):
        # R27-F6: the only job-level condition is the registered Owner-gated
        # dispatch dormancy guard. It must not inspect inputs — missing or
        # malformed outputs have to fail explicitly, never skip.
        condition = self.doc['jobs']['receipt_shadow'].get('if', '')
        self.assertNotIn('inputs.', condition)
        self.assertIn("github.event_name == 'workflow_dispatch'", condition)
        self.assertIn("github.actor == 'mcpdev80'", condition)

    def test_consumes_publication_not_head(self):
        step = self.doc['jobs']['receipt_shadow']['steps'][-1]
        self.assertEqual(step['env'].get('PUBLICATION'), '${{ inputs.publication }}')
        run = step['run']
        self.assertIn('${PUBLICATION}^{commit}', run)
        self.assertNotIn('rev-parse HEAD', run)
        self.assertIn('final publication', run)


FAKE_GH = '''#!/usr/bin/env python3
import json, os, re, sys
REPO = "repos/mcpdev80/baseharbor"
FIX = os.environ["FAKE_GH_FIXTURE"]
path = sys.argv[-1]
def out(value):
    sys.stdout.write(json.dumps(value)); raise SystemExit(0)
def fail(text):
    sys.stderr.write(text + "\\n"); raise SystemExit(1)
if os.environ.get("FAKE_GH_TRANSPORT"):
    fail("dial tcp: lookup api.github.com: no such host")
if path.endswith("/zip"):
    sys.stdout.buffer.write(open(FIX + "/plan.zip", "rb").read()); raise SystemExit(0)
if path.startswith(REPO + "/pulls/"):
    out(json.load(open(FIX + "/pull.json")))
if "/issues/" in path:
    out(json.load(open(FIX + "/issue.json")))
m = re.match(re.escape(REPO + "/actions/runs/") + r"([0-9]+)(?=$|[/?])", path)
if m:
    run_id = int(m.group(1))
    if run_id == json.load(open(FIX + "/producer_id.json")):
        if "/jobs?" in path:
            out(json.load(open(FIX + "/jobs.json")))
        if "/artifacts?" in path:
            out(json.load(open(FIX + "/artifacts.json")))
        out(json.load(open(FIX + "/run.json")))
    if run_id == json.load(open(FIX + "/approval_id.json")):
        out(json.load(open(FIX + "/approval_run.json")))
    fail("gh: Not Found (HTTP 404)")
fail("gh: unsupported path")
'''


class ProductionPathCliTest(VerifierFixtureTest):
    """The three controlled negatives plus a positive retained verification run
    through the real CLI entry point (scripts/receipt_verifier.py as a
    subprocess). GitHub access is intercepted by a fixture gh binary only — no
    test ever reaches the network, and the offline case runs without gh."""

    def setUp(self):
        super().setUp()
        self.ghdir = Path(self.temp.name) / 'bin-full'
        self.ghdir.mkdir()
        (self.ghdir / 'git').symlink_to(shutil.which('git'))
        (self.ghdir / 'python3').symlink_to(sys.executable)
        script = self.ghdir / 'gh'
        script.write_text(FAKE_GH)
        script.chmod(0o755)
        self.offline_bin = Path(self.temp.name) / 'bin-offline'
        self.offline_bin.mkdir()
        (self.offline_bin / 'git').symlink_to(shutil.which('git'))
        self.fixture_dir = Path(self.temp.name) / 'gh-fixture'
        self.fixture_dir.mkdir()

    def write_gh_fixture(self):
        fix = str(self.fixture_dir)
        producer = dict(self.run)
        json.dump(producer, open(fix + '/run.json', 'w'))
        json.dump(producer['id'], open(fix + '/producer_id.json', 'w'))
        json.dump(self.approval_run, open(fix + '/approval_run.json', 'w'))
        json.dump(self.approval_run['id'], open(fix + '/approval_id.json', 'w'))
        # The forward PR->task resolver needs a PR body that references the bound
        # task via a "Closes #<issue>" closer. Seed both so the hermetic fixture
        # resolves deterministically (F4). TASK_PATH is "tasks/receipt-pilot.md";
        # the issue title is the bare task stem and the body points to it.
        body = "Implements the pilot.\n\nCloses #%d\n" % (PR_NUMBER + 1)
        self.pull['body'] = body
        json.dump(self.pull, open(fix + '/pull.json', 'w'))
        json.dump({"number": PR_NUMBER + 1,
                   "title": "receipt-pilot.md",
                   "body": TASK_PATH},
                  open(fix + '/issue.json', 'w'))
        producer_job = copy.deepcopy(self.job)
        json.dump(dict(total_count=2, jobs=[producer_job, dict(
            producer_job, id=producer_job['id'] + 1, name='PR · Trusted Plan')]),
            open(fix + '/jobs.json', 'w'))
        json.dump(dict(total_count=1, artifacts=[dict(
            id=789, name='trusted-pr-plan', expired=False, size_in_bytes=500,
            workflow_run=dict(id=producer['id'], head_sha=producer['head_sha']))]),
            open(fix + '/artifacts.json', 'w'))
        (self.fixture_dir / 'plan.zip').write_bytes(self.plan_zip())

    def cli(self, args, *, offline=False, transport=False, identity_env=False):
        env = {'PATH': str(self.offline_bin if offline else self.ghdir),
               'GIT_NO_LAZY_FETCH': '1',
               'FAKE_GH_FIXTURE': str(self.fixture_dir)}
        if transport:
            env['FAKE_GH_TRANSPORT'] = '1'
        if identity_env:
            env.update({k: v for k, v in self.owner_env.items()})
            env['TRUSTED_SOURCE'] = self.pin
        else:
            env['GH_TOKEN'] = 'fixture-token-not-a-secret'
        return subprocess.run([sys.executable, str(REPO_ROOT / 'scripts' /
                                                    'receipt_verifier.py'), *args],
                              capture_output=True, text=True, env=env)

    def test_retained_positive_without_any_api_access(self):
        path, original = self.create()
        self.approve(path, original)
        result = self.cli(['retained', '--planning', str(self.root), '--revision',
                           self.head(), '--receipt', path, '--candidate',
                           self.candidate, '--plan', self.plan_sha], offline=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('checked: public-contracts', result.stdout)

    def test_negative_wrong_candidate_sha_on_production_path(self):
        path, original = self.create()
        self.approve(path, original)
        result = self.cli(['retained', '--planning', str(self.root), '--revision',
                           self.head(), '--receipt', path, '--candidate',
                           'f' * 40, '--plan', self.plan_sha], offline=True)
        self.assertEqual(result.returncode, 1)
        self.assertIn('does not match required candidate', result.stderr)

    def test_negative_genuinely_missing_run_id_via_404(self):
        self.write_gh_fixture()
        value = dict(gate=verifier.GATE, run={'id': 999999, 'attempt': 1},
                     sha=self.candidate, status='pass', plan=self.plan_sha,
                     approved=None)
        target = self.root / verifier.path_for(value)
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes(verifier.canonical(value))
        self.command('add', '.')
        self.command('commit', '-qm', 'receipt for a genuinely absent run')
        result = self.cli(['live', '--planning', str(self.root), '--revision',
                           self.head(), '--receipt', verifier.path_for(value),
                           '--candidate', self.candidate, '--plan', self.plan_sha])
        self.assertEqual(result.returncode, 1)
        self.assertIn('run or resource not found', result.stderr)

    def test_network_outage_is_never_reported_as_missing_run(self):
        path, original = self.create()
        self.write_gh_fixture()
        result = self.cli(['live', '--planning', str(self.root), '--revision',
                           original, '--receipt', path, '--candidate',
                           self.candidate, '--plan', self.plan_sha], transport=True)
        self.assertEqual(result.returncode, 1)
        self.assertIn('witness transport failure', result.stderr)
        self.assertNotIn('not found', result.stderr)

    def test_approve_cli_never_accepts_manual_evidence_fields(self):
        result = self.cli(['approve', '--planning', str(self.root)])
        self.assertEqual(result.returncode, 2)
        self.assertIn('auto-resolved', result.stderr)

    def test_approve_cli_subprocess_emits_exact_resolved_tuple(self):
        # R27-F6 through the real CLI subprocess and the real writer mechanism
        # against the temporary bare remote: plan/candidate are the actual
        # result fields (never introduction/approval), and publication is the
        # durable final revision equal to the pushed remote main.
        path, original = self.create()
        self.write_gh_fixture()
        self.seed_remote_main()
        result = self.cli(['approve', '--planning', str(self.root), '--product',
                           str(self.product), '--pr', str(PR_NUMBER)], identity_env=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        emitted = dict(line.split(': ', 1) for line in result.stdout.splitlines())
        self.assertEqual(emitted['receipt'], path)
        self.assertEqual(emitted['plan'], self.plan_sha)
        self.assertEqual(emitted['candidate'], self.candidate)
        self.assertTrue(verifier.full_sha(emitted['publication']))
        self.assertNotEqual(emitted['publication'], emitted['introduction'])
        self.assertEqual(self.remote_main(), emitted['publication'])

    def test_approve_cli_full_chain_fails_closed_on_missing_publish(self):
        # The F4 fix makes the approval path resolve the forward PR->task binding
        # through the shared tasklink resolver before publishing. With a fixture
        # gh the resolution is hermetic; the chain then fails closed at the
        # missing W2 publish interface (BF-W3-1) without a second edit or patch.
        path, original = self.create()
        self.write_gh_fixture()
        result = self.cli(['approve', '--planning', str(self.root), '--product',
                           str(self.product), '--pr', str(PR_NUMBER)], identity_env=True)
        self.assertEqual(result.returncode, 1)
        self.assertIn('BF-W3-1', result.stderr)
        self.assertEqual(len(self.edits(path)), 2)
        self.assertFalse(list(self.root.glob('**/*.patch')))


class ShadowConsumptionExecutionTest(VerifierFixtureTest):
    """R27-F6: execute the shadow step's real shell block against a real
    repository. The retained verification must consume exactly the final
    durable publication — not the latest Planning HEAD — and missing or
    malformed outputs must fail EXPLICITLY. The PATH never contains gh: the
    retained path has no API fallback."""

    def setUp(self):
        super().setUp()
        self.bin = Path(self.temp.name) / 'bin-offline'
        self.bin.mkdir()
        (self.bin / 'git').symlink_to(shutil.which('git'))
        (self.bin / 'python3').symlink_to(sys.executable)
        self.stage = Path(self.temp.name) / 'shadow-stage'
        self.stage.mkdir()
        (self.stage / 'scripts').symlink_to(REPO_ROOT / 'scripts')
        (self.stage / 'planning').symlink_to(self.root)
        self.summary = Path(self.temp.name) / 'summary.md'

    def script(self):
        doc = yaml.safe_load(SHADOW_WORKFLOW.read_text())
        run = doc['jobs']['receipt_shadow']['steps'][-1]['run']
        path = Path(self.temp.name) / 'shadow-step.sh'
        path.write_text(run)
        return path

    def run_block(self, **values):
        env = {'PATH': str(self.bin), 'GIT_NO_LAZY_FETCH': '1',
               'GITHUB_STEP_SUMMARY': str(self.summary),
               'RECEIPT': values.get('receipt', ''),
               'PLAN': values.get('plan', self.plan_sha),
               'CANDIDATE': values.get('candidate', self.candidate),
               'PUBLICATION': values.get('publication', '')}
        return subprocess.run([shutil.which('bash'), str(self.script())],
                              cwd=str(self.stage), capture_output=True, text=True, env=env)

    def publish_chain(self):
        path, original = self.create()
        self.seed_remote_main()
        result = self.approve_pr()
        return path, result

    def test_consumes_exact_publication_after_head_moved(self):
        path, result = self.publish_chain()
        publication = result['publication']
        # Planning main moves on and the working receipt bytes are tampered:
        # consuming HEAD would fail, consuming the exact publication passes.
        self.tamper(path)
        self.assertNotEqual(self.head(), publication)
        out = self.run_block(receipt=path, publication=publication)
        self.assertEqual(out.returncode, 0, out.stdout + out.stderr)
        self.assertIn('checked: public-contracts', out.stdout)
        self.assertIn(publication, self.summary.read_text())

    def test_missing_publication_output_fails_explicitly(self):
        path, result = self.publish_chain()
        out = self.run_block(receipt=path, publication='')
        self.assertEqual(out.returncode, 1)
        self.assertIn('final publication output missing', out.stderr)

    def test_publication_not_in_planning_history_fails(self):
        path, result = self.publish_chain()
        out = self.run_block(receipt=path, publication='a' * 40)
        self.assertEqual(out.returncode, 1)
        self.assertIn('not in the protected Planning history', out.stderr)

    def test_missing_receipt_output_fails_explicitly(self):
        path, result = self.publish_chain()
        out = self.run_block(receipt='', publication=result['publication'])
        self.assertEqual(out.returncode, 1)
        self.assertIn('receipt path output missing or malformed', out.stderr)


if __name__ == '__main__':
    unittest.main()

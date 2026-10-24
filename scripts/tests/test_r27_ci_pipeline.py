"""Small end-to-end trust matrices with real Git objects and signed receipts."""
import hashlib
import base64
import io
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch
sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
from r27_ci_request import run_name, request_from_name, verify_executor_run
from r27_ci_plan import make_plan, expected_jobs, load_policy, owner
from r27_ci_issuer import issue_completed_run
from r27_commit_receipt import canonical, sign, verify, schedule
from r27_verified_coverage import EvidenceError
from r27_github_origin import OriginError

REPO = "mcpdev80/baseharbor"
SEED = b"k" * 32
PUBLIC = Ed25519PrivateKey.from_private_bytes(SEED).public_key().public_bytes_raw()
EXECUTOR = "a" * 40


class PipelineTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.repo = Path(self.temp.name)
        self.git("init", "-q")
        source = self.repo / "fixture.py"
        source.write_text("base\n")
        self.git("add", ".")
        self.git("commit", "-qm", "base")
        base = self.git("rev-parse", "HEAD")
        source.write_text("candidate\n")
        self.git("commit", "-qam", "candidate")
        head = self.git("rev-parse", "HEAD")
        integration = self.git("commit-tree", "HEAD^{tree}", "-p", base, "-p", head, "-m", "integration")
        self.request = dict(stage="pr", number=1, candidate=head, base=base,
                            integration=integration, artifact="none")
        self.policy = dict(schema="baseharbor.ci-policy/v1", repository=REPO,
                           catalog_release="v0.4.26", demo_sha="",
                           prototype_paths=["fixture.py"], checks={"r27-contracts": {
                               "command": "python3 -m unittest discover -s scripts/tests -p 'test_r27_*.py'",
                               "inputs": ["fixture.py"]}})
        self.digest = hashlib.sha256(canonical(self.policy) + b"\n").hexdigest()
        self.plan = make_plan(self.request, self.policy, repository=self.repo)

    def git(self, *args):
        return subprocess.check_output(
            ["git", "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", *args],
            cwd=self.repo, stderr=subprocess.DEVNULL, text=True).strip()

    def test_protected_policy_pin_and_real_git_plan(self):
        path = self.repo / "policy.json"
        path.write_bytes(canonical(self.policy) + b"\n")
        self.assertEqual(load_policy(path, self.digest), self.policy)
        self.assertEqual(self.plan["checks"], ["r27-contracts"])
        with self.assertRaises(EvidenceError):
            load_policy(path, "0" * 64)
        bad = dict(self.request, integration=self.request["candidate"])
        with self.assertRaises(EvidenceError):
            make_plan(bad, self.policy, repository=self.repo)
        self.git("mv", "fixture.py", "renamed.py")
        self.git("commit", "-qm", "rename")
        bad = dict(self.request, candidate=self.git("rev-parse", "HEAD"),
                   stage="integration", number=0, integration=self.git("rev-parse", "HEAD"))
        with self.assertRaises(EvidenceError):
            make_plan(bad, self.policy, repository=self.repo)

    def test_public_verifier_cannot_sign_and_input_reuse_is_exact(self):
        raw = sign(self.request, self.plan["coverage"], key=SEED,
                   policy_digest=self.digest, executor_sha=EXECUTOR)
        args = dict(key=PUBLIC, policy_digest=self.digest, executor_sha=EXECUTOR)
        self.assertEqual(verify(raw, **args)["request"], self.request)
        self.assertEqual(schedule(self.request, self.plan["coverage"], [raw], **args)["pending"], [])
        changed = {gate: "0" * 64 for gate in self.plan["coverage"]}
        self.assertEqual(schedule(self.request, changed, [raw], **args)["pending"], list(changed))
        self.assertEqual(schedule(self.request, changed, [raw], failed=list(changed), **args)["state"], "ACTION_REQUIRED")
        for mutation in ("signature", "policy", "executor", "key"):
            altered = dict(args)
            data = raw
            if mutation == "signature":
                body = json.loads(raw)
                body["request"]["candidate"] = "f" * 40
                data = canonical(body)
            elif mutation == "policy":
                altered["policy_digest"] = "0" * 64
            elif mutation == "executor":
                altered["executor_sha"] = "b" * 40
            else:
                altered["key"] = SEED
            with self.subTest(mutation=mutation), self.assertRaises(EvidenceError):
                verify(data, **altered)

    def test_request_scope_and_release_smoke(self):
        self.assertEqual(request_from_name(run_name(self.request)), self.request)
        for field, value in (("number", True), ("number", 1.0), ("stage", "other"),
                             ("artifact", "0" * 64), ("candidate", "--help")):
            with self.subTest(field=field), self.assertRaises(EvidenceError):
                run_name(dict(self.request, **{field: value}))
        release = dict(self.request, stage="release", number=0, artifact="b" * 64,
                       integration=self.request["candidate"])
        with self.assertRaises(EvidenceError):
            sign(release, self.plan["coverage"], key=SEED,
                 policy_digest=self.digest, executor_sha=EXECUTOR)
        raw = sign(release, {"release/smoke": "c" * 64}, key=SEED,
                   policy_digest=self.digest, executor_sha=EXECUTOR)
        decision = schedule(dict(release, artifact="d" * 64), {"release/smoke": "c" * 64},
                            [raw], key=PUBLIC, policy_digest=self.digest, executor_sha=EXECUTOR)
        self.assertEqual(decision["pending"], ["release/smoke"])

    def test_dormant_workflows_keep_candidate_and_signer_separate(self):
        import yaml
        root = Path(__file__).resolve().parents[2]
        directory = root / "deploy/ci/r27"
        executor = (directory / "r27-executor.yml").read_text()
        issuer = (directory / "r27-issuer.yml").read_text()
        router = yaml.load((directory / "r27-router.yml").read_text(), Loader=yaml.BaseLoader)
        self.assertNotRegex(executor, r"\$\{\{\s*secrets(?:\.|\[)")
        self.assertNotIn("secrets: inherit", executor)
        self.assertNotIn("self-hosted", executor + issuer)
        self.assertNotIn("synchronize", router["on"]["pull_request_target"]["types"])
        self.assertIn("R27_TRUSTED_EXECUTOR_SHA", issuer)
        self.assertNotIn("actions/download-artifact", issuer)
        self.assertNotIn("inputs.integration", issuer)
        steps = yaml.load(issuer, Loader=yaml.BaseLoader)["jobs"]["issue"]["steps"]
        signers = [step for step in steps if "R27_SIGNING_KEY_HEX" in step.get("env", {})]
        self.assertEqual(len(signers), 1)
        self.assertIn("r27-ci.py issue", signers[0]["run"])

    def test_offline_cli_and_create_only_publication(self):
        cli = owner("r27-ci")
        raw = sign(self.request, self.plan["coverage"], key=SEED,
                   policy_digest=self.digest, executor_sha=EXECUTOR)
        path = self.repo / "receipt.json"
        path.write_bytes(raw)
        args = [sys.executable, str(Path(cli.__file__)), "verify", "--receipt", str(path),
                "--public-key", PUBLIC.hex(), "--policy-digest", self.digest,
                "--executor-sha", EXECUTOR, "--expected-commit", self.request["candidate"],
                "--expected-stage", "pr"]
        self.assertEqual(json.loads(subprocess.check_output(args))["state"], "PASS")
        self.assertNotEqual(subprocess.run(args[:-1] + ["integration"], capture_output=True).returncode, 0)
        with patch.dict("os.environ", {"R27_PUBLIC_KEY_HEX": PUBLIC.hex()}), \
                patch.object(cli, "github_getter"), patch.object(cli, "check_current"), \
                patch.object(cli, "gh") as api, patch("builtins.print"):
            api.side_effect = [None, {}]
            cli.publish(self.policy, EXECUTOR, self.digest, path)
            self.assertEqual([call.args[0] for call in api.call_args_list], ["GET", "PUT"])
            api.reset_mock()
            api.side_effect = None
            api.return_value = {"content": base64.b64encode(raw).decode()}
            cli.publish(self.policy, EXECUTOR, self.digest, path)
            self.assertEqual(api.call_count, 1)
            api.return_value = {"content": base64.b64encode(b"conflict").decode()}
            with self.assertRaises(EvidenceError):
                cli.publish(self.policy, EXECUTOR, self.digest, path)
            api.side_effect = EvidenceError("CI_API: denied")
            with self.assertRaises(EvidenceError):
                cli.publish(self.policy, EXECUTOR, self.digest, path)
            self.assertEqual(path.read_bytes(), raw)

    def test_real_pipeline_issues_only_after_fresh_api_and_all_jobs(self):
        run = dict(id=10, run_attempt=1, head_sha=EXECUTOR, path=".github/workflows/r27-executor.yml",
                   event="workflow_dispatch", status="completed", conclusion="success",
                   repository={"full_name": REPO}, display_title=run_name(self.request))
        jobs = [dict(id=i + 1, run_id=10, run_attempt=1, name=name, status="completed", conclusion="success")
                for i, name in enumerate(sorted(expected_jobs(self.plan)))]
        pr = dict(number=1, state="open", draft=False, merge_commit_sha=self.request["integration"],
                  head={"sha": self.request["candidate"], "repo": {"full_name": REPO}},
                  base={"sha": self.request["base"], "ref": "develop", "repo": {"full_name": REPO}})
        late_drift = False
        pr_reads = 0
        def response(request, timeout):
            nonlocal pr_reads
            if "/pulls/" in request.full_url:
                pr_reads += 1
                body = pr
                if late_drift and pr_reads > 1:
                    body = dict(pr, base=dict(pr["base"], sha="d" * 40))
            elif "/jobs?" in request.full_url:
                body = dict(total_count=len(jobs), jobs=jobs)
            else:
                body = run
            stream = io.BytesIO(json.dumps(body).encode())
            stream.getcode = lambda: 200
            return stream
        with patch("r27_github_transport.urllib.request.build_opener") as build:
            build.return_value.open.side_effect = response
            args = dict(token="fixture-token", key=SEED, run_id=10, attempt=1,
                        executor_sha=EXECUTOR, policy=self.policy, policy_digest=self.digest,
                        repository_path=self.repo)
            receipt = issue_completed_run(**args)
            self.assertEqual(verify(receipt, key=PUBLIC, policy_digest=self.digest,
                                    executor_sha=EXECUTOR)["coverage"], self.plan["coverage"])
            for case in ("source", "attempt", "skipped", "duplicate", "stale", "boolean", "late_base"):
                bad_run, bad_jobs, bad_pr = dict(run), list(jobs), dict(pr)
                pr_reads = 0
                if case == "source":
                    run["head_sha"] = "b" * 40
                elif case == "attempt":
                    run["run_attempt"] = 2
                elif case == "skipped":
                    jobs[0] = dict(jobs[0], conclusion="skipped")
                elif case == "duplicate":
                    jobs.append(dict(jobs[0], id=99))
                elif case == "stale":
                    pr["draft"] = True
                elif case == "boolean":
                    run["id"] = True
                else:
                    late_drift = True
                with self.subTest(case=case), self.assertRaises((EvidenceError, OriginError)):
                    issue_completed_run(**args)
                run.clear(); run.update(bad_run)
                jobs[:] = bad_jobs
                pr.clear(); pr.update(bad_pr)

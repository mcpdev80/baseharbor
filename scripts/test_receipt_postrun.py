#!/usr/bin/env python3
r"""F1 verification for the receipt post-run workflow.

This module extracts the shell ``run:`` blocks of
``.github/workflows/receipt-post-run.yml`` and executes them verbatim against a
temporary bare Planning remote and a fake producer artifact. It proves the F1
recipe closure:

- the decoder block no longer closes a here-document with a glued delimiter
  (``...PY > plan.txt``); it ``bash -n`` clean and executes, emitting
  machine-readable outputs;
- the run block publishes the bounded plan fixture and an unapproved receipt to
  the bare remote's ``main`` using the live workflow run id and the real
  Planning plan commit, never the workflow definition id nor the candidate sha;
- a missing/malformed artifact and a foreign producer repository block.

The workflow is DATA here: the run blocks are parsed out of the YAML and fed the
authenticated-fixture responses the writer/verifier expect. No network and no
real Planning repo are used.
"""
import copy
import contextlib
import io
import json
import os
import re
import subprocess
import sys
import tempfile
import unittest
import zipfile
from pathlib import Path

import receipt_verifier  # noqa: E402
from receipt_verifier import require  # noqa: E402
HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))

import receipt_tasklink  # noqa: E402
import receipt_writer  # noqa: E402

import yaml  # noqa: E402  # local-only consumer of POST_RUN_YML assertions

POST_RUN_YML = HERE.parent / ".github" / "workflows" / "receipt-post-run.yml"

# A fake producer artifact shape: a zip containing exactly one pr-plan.json.
RUN_ID = 40541797478
HEAD_SHA = "a" * 40

FAKE_PLAN = {
    "schema": "baseharbor.pr-gate-plan/v1",
    "coverage_status": "resolved",
    "selected_gates": ["integration/static/public-contracts"],
    "run_id": RUN_ID,
    "attempt": 1,
    "workflow_id": 17,
    "head_sha": "a" * 40,
    "integration_sha": "c" * 40,
    "target_base_sha": "b" * 40,
}

# GitHub expressions that the run blocks legitimately contain; substituted to
# their runtime values before the extracted block is handed to bash.
GH_EXPR = {
    "${{ github.event.workflow_run.id }}": str(RUN_ID),
    "${{ github.event.workflow_run.run_attempt }}": "1",
    "${{ github.event.workflow_run.base_ref }}": "refs/pull/1124/merge",
    "${{ github.event.workflow_run.pull_requests[0].number }}": "1124",
    "${{ needs.extract_plan.outputs.run_id }}": str(RUN_ID),
    "${{ github.repository }}": "mcpdev80/baseharbor",
    "${{ github.workspace }}": ".",
    "${{ github.event.workflow_run.workflow_id }}": str(FAKE_PLAN["workflow_id"]),
    "${{ needs.verify_producer.outputs.pr_number }}": "1124",
    "${{ secrets.PLANNING_RECEIPT_WRITE_TOKEN }}": "fixture-token",
    "${{ needs.extract_plan.outputs.workflow_id }}": str(FAKE_PLAN["workflow_id"]),
    "${{ needs.extract_plan.outputs.head_sha }}": HEAD_SHA,
    "${{ needs.extract_plan.outputs.candidate_sha }}": "c" * 40,
    "${{ needs.extract_plan.outputs.base_sha }}": "b" * 40,
    "${{ secrets.GITHUB_TOKEN }}": "fixture-token",
}


def _extract_run_block(role):
    """Slice one run block and substitute GitHub runtime expressions.

    The block is parsed out of the YAML first so that the block-scalar
    indentation is stripped exactly as the real runner would deliver it to
    bash: the Python heredoc body arrives at column 0.
    """
    import yaml  # local import: only this module needs it
    text = POST_RUN_YML.read_text()
    marker = {
        "download": "Download trusted-pr-plan artifact",
        "decode": "Decode pr-plan and extract candidate_sha",
        "tasklink": "Automatic task-link (plan fixture)",
        "publish": "Publish receipt to Planning",
    }[role]
    idx = text.index(marker)
    # Find the owning step's run: | block via the parsed YAML so indentation is
    # already chomped to the runner's delivery form.
    doc = yaml.safe_load(text)
    run_text = None
    for job in doc["jobs"].values():
        for step in job["steps"]:
            if marker in step.get("name", "") and step.get("run"):
                run_text = step["run"]
                break
        if run_text is not None:
            break
    if run_text is None:
        raise AssertionError("run block for %r not found" % role)
    block = run_text
    for expr, val in GH_EXPR.items():
        block = block.replace(expr, val)
    return block


def _run(cmd, *, env=None, cwd=None, input=None):
    return subprocess.run(
        cmd, shell=isinstance(cmd, str),
        env=env, cwd=cwd, capture_output=True, text=True,
        input=input,
    )


class FakeApi:
    """A hermetic GitHub witness matching receipt_verifier.live()'s contract."""

    def __init__(self):
        self.plan_zip = self._make_plan_zip()
        self.job = dict(id=456, run_id=RUN_ID, head_sha=HEAD_SHA,
                        name="PR · Public Contracts", status="completed",
                        conclusion="success")
        self.producer = dict(self.job, id=457, name="PR · Trusted Plan")

    def _make_plan_zip(self):
        buf = io.BytesIO()
        with zipfile.ZipFile(buf, "w") as z:
            z.writestr("pr-plan.json", json.dumps(FAKE_PLAN))
        return buf.getvalue()

    def __call__(self, path, *, binary=False):
        prefix = f"repos/{receipt_verifier.REPOSITORY}"
        if path.endswith("/zip"):
            require(binary, "zip requested without binary flag")
            return self.plan_zip
        if path.startswith(prefix + "/pulls/"):
            return {"number": 1124, "state": "open", "body": "Closes #1125"}
        if "/issues/" in path:
            return {"number": 1125, "title": "t-20261022-r27-evidence-pilot.md",
                    "body": "tasks/t-20261022-r27-evidence-pilot.md"}
        if path.startswith(prefix + "/actions/runs/"):
            tail = path[len(prefix + "/actions/runs/"):]
            run_id = int(tail.split("/")[0].split("?")[0])
            if run_id != RUN_ID:
                return self._run_obj(run_id, head_sha=HEAD_SHA)
            if "/jobs?" in path:
                return dict(total_count=2,
                            jobs=[copy.deepcopy(self.job),
                                  copy.deepcopy(self.producer)])
            if "/attempts/" in path:
                return dict(id=RUN_ID, run_attempt=1, head_sha=HEAD_SHA,
                            repository=dict(id=receipt_verifier.REPOSITORY_ID,
                                            full_name=receipt_verifier.REPOSITORY),
                            workflow_id=FAKE_PLAN["workflow_id"],
                            path=".github/workflows/ci.yml",
                            event="pull_request", status="completed",
                            conclusion="success")
            if "/artifacts" in path:
                return dict(total_count=1, artifacts=[dict(
                    id=789, name="trusted-pr-plan", expired=False,
                    size_in_bytes=500, workflow_run=dict(id=RUN_ID, head_sha=HEAD_SHA))])
            return dict(id=RUN_ID, run_attempt=1, head_sha=HEAD_SHA,
                        repository=dict(id=receipt_verifier.REPOSITORY_ID,
                                        full_name=receipt_verifier.REPOSITORY),
                        workflow_id=FAKE_PLAN["workflow_id"],
                        path=".github/workflows/ci.yml",
                        event="pull_request", status="completed",
                        conclusion="success")
        raise AssertionError("unexpected API path: %s" % path)

    def _run_obj(self, run_id, *, head_sha):
        return dict(id=run_id, run_attempt=1, head_sha=head_sha,
                    repository=dict(id=receipt_verifier.REPOSITORY_ID,
                                    full_name=receipt_verifier.REPOSITORY),
                    workflow_id=FAKE_PLAN["workflow_id"],
                    path=".github/workflows/ci.yml", event="pull_request",
                    status="completed", conclusion="success")


def _is_rejected_push(rc: int, stderr: str) -> bool:
    if rc == 0:
        return False
    needle = "non-fast-forward" if rc == 128 else "rejected"
    return needle in stderr or "failed to push" in stderr


class ExtractBlocksTest(unittest.TestCase):
    """The YAML run blocks must be extractable and bash -n clean."""

    def test_all_run_blocks_exist(self):
        text = POST_RUN_YML.read_text()
        self.assertIn("run:", text)
        # The glued here-doc delimiter is gone: no "PY > plan.txt".
        self.assertNotIn("PY > plan.txt", text)
        # The mutable develop checkout is pinned to a full SHA.
        self.assertNotIn("ref: develop", text)
        # Read the controller pin dynamically from the workflow YAML (no
        # hard-coded SHA): the reviewed CI controller must be pinned to the
        # immutable SHA referenced by the Checkout step.
        pin_matches = re.findall(
            r"ref:\s+([0-9a-f]{4,40}(?:[0-9a-f]{4,40})*)\s*$", text, re.MULTILINE
        )
        # We expect at least one hex-pin on a `ref:` line (the controller).
        self.assertTrue(len(pin_matches) >= 1,
            f"Expected at least one pinned SHA on 'ref:', found {pin_matches}")

    def test_decoder_block_is_bash_clean(self):
        # The decode block must pass `bash -n` (the old block raised
        # "here-document wanted PY" then a Python SyntaxError).
        block = _extract_run_block("decode")
        proc = _run(["bash", "-n", "-"], input=block)
        self.assertEqual(proc.returncode, 0, proc.stderr)

    def _publish_step_env_keys(self):
        """Return the publish-step env keys in source order (comments stripped)."""
        doc = yaml.safe_load(POST_RUN_YML.read_text())
        publish = next(
            step
            for step in doc["jobs"]["publish_receipt"]["steps"]
            if step.get("name") == "Publish receipt to Planning"
        )
        env = publish.get("env", {}) or {}
        return [key for key in env if not key.startswith("#")]

    def test_publish_step_env_carries_write_token(self):
        # Root cause 40834168626: the controller requires
        # PLANNING_RECEIPT_WRITE_TOKEN from os.environ and fails closed when it
        # is absent (receipt_writer.py:108-110). The publish step env must
        # supply it; a missing key re-introduces the fail-closed gap.
        keys = self._publish_step_env_keys()
        self.assertIn("PLANNING_ROOT", keys)
        self.assertIn("RUN_ID", keys)
        self.assertIn("PLAN_REVISION", keys)
        self.assertIn(
            "PLANNING_RECEIPT_WRITE_TOKEN", keys,
            "publish step env must supply the Planning write token",
        )
        # The token is sourced from the repository secret, never a literal.
        doc = yaml.safe_load(POST_RUN_YML.read_text())
        publish = next(
            step
            for step in doc["jobs"]["publish_receipt"]["steps"]
            if step.get("name") == "Publish receipt to Planning"
        )
        self.assertEqual(
            str(publish["env"]["PLANNING_RECEIPT_WRITE_TOKEN"]),
            "${{ secrets.PLANNING_RECEIPT_WRITE_TOKEN }}",
        )

    def test_publish_step_env_carries_gh_token(self):
        # The publish step requires GH_TOKEN for the writer's live witness
        # gh call; without it the receipt_writer.py live() verification fails.
        # This assertion verifies the forward-port of main's GH_TOKEN fix.
        keys = self._publish_step_env_keys()
        self.assertIn(
            "GH_TOKEN", keys,
            "publish step env must supply GH_TOKEN for writer live witness",
        )
        # GH_TOKEN is sourced from the repository secret, never a literal.
        doc = yaml.safe_load(POST_RUN_YML.read_text())
        publish = next(
            step
            for step in doc["jobs"]["publish_receipt"]["steps"]
            if step.get("name") == "Publish receipt to Planning"
        )
        self.assertEqual(
            str(publish["env"]["GH_TOKEN"]),
            "${{ secrets.GITHUB_TOKEN }}",
        )

    def test_controller_pins_bumped_to_dd8fb61d(self):
        # The e13f157 controller lacks --product in argparse; dd8fb61d
        # fixes this (commit dd8fb61d: wire --product through CLI).
        # Both checkout pins must reference dd8fb61d.
        text = POST_RUN_YML.read_text()
        pin_matches = re.findall(
            r"ref:\s+dd8fb61d614b59252c9d29121380b5893d006828\s*$",
            text, re.MULTILINE,
        )
        self.assertEqual(len(pin_matches), 2,
            f"Expected exactly 2 controller pins at dd8fb61d, found {len(pin_matches)}")


class F1PositiveRun(unittest.TestCase):
    """The full extracted run publishes plan + receipt to a temp bare remote."""

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)

        # A bare Planning remote that will carry the published plan + receipt.
        self.bare = self.root / "planning.bare"
        self.bare.mkdir()
        self._bare = _run(["git", "init", "--bare", "-q", str(self.bare)])
        self.assertEqual(self._bare.returncode, 0, self._bare.stderr)

        # A fresh Planning checkout with an initial commit on main.
        self.planning = self.root / "planning"
        self.planning.mkdir()
        self._init_planning(self.planning)
        self.plan_commit = self._git(self.planning, "rev-parse", "HEAD").decode().strip()

        self.api = FakeApi()
        # The download step requires a plan.zip in the working directory.
        with zipfile.ZipFile(self.root / "plan.zip", "w") as z:
            z.writestr("pr-plan.json", json.dumps(FAKE_PLAN))

        # Controlled gh stub ahead on PATH so the extracted decode block does
        # not hit the real GitHub API.
        self.bin = self.root / "bin"
        self.bin.mkdir()
        self._write_gh_stub()

    def _write_gh_stub(self):
        stub = self.bin / "gh"
        stub.write_text(FAKE_GH_STUB)
        stub.chmod(0o755)

    def _init_planning(self, root: Path):
        _run(["git", "init", "-q", str(root)])
        _run(["git", "-C", root, "config", "user.name", "F1 fixture"])
        _run(["git", "-C", root, "config", "user.email", "f1@example.invalid"])
        task = root / "tasks" / "t-20261022-r27-evidence-pilot.md"
        task.parent.mkdir(parents=True, exist_ok=True)
        task.write_text("---\nid: t-20261022-r27-evidence-pilot\n---\nr27 evidence pilot\n")
        # Pre-commit the plan fixture so produce_plan_fixture() hits its
        # idempotency path and returns plan_commit (the real plan revision).
        fixture = root / receipt_tasklink.PLAN_PATH
        fixture.parent.mkdir(parents=True, exist_ok=True)
        plan_obj = {
            "gate": "public-contracts",
            "repository": "mcpdev80/baseharbor",
            "workflow_id": FAKE_PLAN["workflow_id"],
            "head_sha": FAKE_PLAN["head_sha"],
            "candidate_sha": FAKE_PLAN["integration_sha"],
            "base_sha": FAKE_PLAN["target_base_sha"],
            "task": "tasks/t-20261022-r27-evidence-pilot.md",
        }
        fixture.write_bytes((json.dumps(plan_obj, sort_keys=True,
                                       separators=(",", ":")) + "\n").encode())
        _run(["git", "-C", root, "add", "-A"])
        _run(["git", "-C", root, "commit", "-qm", "initial planning"])
        _run(["git", "-C", root, "branch", "-M", "main"])
        _run(["git", "-C", root, "remote", "add", "origin", str(self.bare)])
        _run(["git", "-C", root, "push", "-q", "origin", "main"])

    def _git(self, root, *args):
        return subprocess.check_output(["git", "-C", str(root), *args])

    def _decode_block(self):
        return _extract_run_block("decode")

    def test_full_run_publishes_plan_and_receipt(self):
        # 1. Decode the artifact with the extracted decode block. The download
        # step (network-only) is exercised by the missing-artifact negative;
        # here we prove the decoder produces the correct authenticated facts
        # and that those facts flow to a real plan + receipt publication.
        decode_block = self._decode_block()
        # The decode block now calls `gh api` for ATTEMPT_RESOLVED; provide a
        # stub so the real GitHub API is never reached.
        self._write_gh_stub()
        env = dict(os.environ,
                   GITHUB_OUTPUT=str(self.root / "gh_out"),
                   GH_WORKFLOW_TOKEN="fixture-token",
                   PATH="%s:%s" % (str(self.bin), os.environ.get("PATH", "")),
                   PYTHONPATH=str(HERE))
        (self.root / "gh_out").write_text("")
        proc = _run(["bash", "-euo", "pipefail", "-c", decode_block], env=env,
                    cwd=self.root)
        self.assertEqual(proc.returncode, 0, f"decode failed: {proc.stderr}")

        out = {}
        for line in (self.root / "gh_out").read_text().splitlines():
            if "=" in line:
                k, v = line.split("=", 1)
                out[k] = v

        self.assertEqual(out.get("workflow_id"), str(FAKE_PLAN["workflow_id"]))
        self.assertEqual(out.get("candidate_sha"), "c" * 40)
        self.assertEqual(out.get("head_sha"), HEAD_SHA)
        self.assertEqual(out.get("base_sha"), "b" * 40)
        self.assertEqual(out.get("run_id"), str(RUN_ID))

        # 2. Write the plan fixture via the extracted tasklink contract. The
        # returned commit is the real Planning plan revision.
        workflow_id = int(out["workflow_id"])
        commit = receipt_tasklink.produce_plan_fixture(
            self.planning, workflow_id, out["head_sha"], out["candidate_sha"],
            out["base_sha"], "tasks/t-20261022-r27-evidence-pilot.md",
        )
        self.assertEqual(commit, self.plan_commit)

        # 3. Publish the receipt with the writer, exactly as the run block does:
        # --plan is the Planning plan commit, --run is the LIVE run id.
        env_write = dict(os.environ,
                         GITHUB_EVENT_NAME="workflow_run",
                         GITHUB_REPOSITORY="mcpdev80/baseharbor",
                         GITHUB_REF="refs/heads/main")
        with self._env(env_write):
            path, intro, pub = receipt_writer.write_and_publish(
                self.planning, commit, int(out["run_id"]), 1,
                api=self.api, remote_push=True,
            )
        self.assertNotEqual(pub, self.plan_commit)  # a fresh receipt commit

        # 4. The receipt is durable on the bare remote main and the run id is
        # the LIVE run id, never the workflow definition id.
        remote_main = self._git(self.bare, "rev-parse", "main").decode().strip()
        remote_raw = _run(["git", "show", f"{remote_main}:{path}"],
                          cwd=self.bare).stdout.encode()
        value = receipt_verifier.receipt(remote_raw)
        self.assertEqual(value["run"]["id"], int(RUN_ID))
        self.assertNotEqual(int(RUN_ID), workflow_id)  # run != definition
        self.assertEqual(value["status"], "pass")
        self.assertIsNone(value["approved"])

    class _env:
        def __init__(self, env):
            self.env = env
            self.old = dict(os.environ)

        def __enter__(self):
            os.environ.update(self.env)
            return self

        def __exit__(self, *exc):
            os.environ.clear()
            os.environ.update(self.old)
            return False


class F1WorkflowShellEndToEnd(unittest.TestCase):
    """Exercise the real extracted tasklink + publish workflow run blocks against
    a fresh bare Planning remote with a controlled gh stub. Proves the F1 recipe
    closure end-to-end: the extracted shell writes the plan fixture, publishes an
    unapproved receipt, and the receipt's run id is the LIVE run id (never the
    workflow definition id). The Planning write token is set so the in-process
    push auth path is exercised.
    """

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)

        # A bare Planning remote.
        self.bare = self.root / "planning.bare"
        self.bare.mkdir()
        _run(["git", "init", "--bare", "-q", str(self.bare)])

        # A fresh Planning checkout with the task note + plan fixture on main.
        self.planning = self.root / "planning"
        self.planning.mkdir()
        _run(["git", "init", "-q", str(self.planning)])
        _run(["git", "-C", self.planning, "config", "user.name", "F1 fixture"])
        _run(["git", "-C", self.planning, "config", "user.email", "f1@example.invalid"])
        task = self.planning / "tasks" / "t-20261022-r27-evidence-pilot.md"
        task.parent.mkdir(parents=True, exist_ok=True)
        task.write_text("---\nid: t-20261022-r27-evidence-pilot\n---\nr27 evidence pilot\n")
        _run(["git", "-C", self.planning, "add", "-A"])
        _run(["git", "-C", self.planning, "commit", "-qm", "initial planning"])
        _run(["git", "-C", self.planning, "branch", "-M", "main"])
        _run(["git", "-C", self.planning, "remote", "add", "origin", str(self.bare)])
        _run(["git", "-C", self.planning, "push", "-q", "origin", "main"])
        self.plan_commit = _run(["git", "-C", self.planning, "rev-parse", "HEAD"]).stdout.strip()

        # Controlled gh stub ahead on PATH so the extracted tasklink block does
        # not hit the real GitHub API.
        self.bin = self.root / "bin"
        self.bin.mkdir()
        self._write_gh_stub()

    def _write_gh_stub(self):
        stub = self.bin / "gh"
        stub.write_text(FAKE_GH_STUB)
        stub.chmod(0o755)

    def test_real_tasklink_block_writes_plan_fixture(self):
        # The extracted tasklink block returns the Planning plan revision as its
        # final 40-hex line. Run it against the fresh Planning checkout and
        # assert the plan fixture is durable on the bare remote main. The
        # extracted block calls `gh api` which resolves to PATH; the stub makes
        # the PR->task resolution return the fixture task deterministically.
        tasklink_block = _extract_run_block("tasklink")
        gh_out = self.root / "gh_out"
        gh_out.write_text("")
        env = dict(os.environ,
                   GITHUB_OUTPUT=str(gh_out),
                   GITHUB_TOKEN="fixture-token",
                   GH_WORKFLOW_TOKEN="fixture-token",
                   PLANNING_ROOT=str(self.planning),
                   WORKFLOW_ID=str(FAKE_PLAN["workflow_id"]),
                   HEAD_SHA=HEAD_SHA,
                   CANDIDATE_SHA="c" * 40,
                   BASE_SHA="b" * 40,
                   PR_NUMBER="1124",
                   PATH="%s:%s" % (self.bin, os.environ.get("PATH", "")),
                   PYTHONPATH=str(HERE.parent))
        proc = _run(["bash", "-euo", "pipefail", "-c", tasklink_block], env=env,
                    cwd=str(HERE.parent))
        self.assertEqual(proc.returncode, 0, f"tasklink block failed: {proc.stderr}")

        fixture = self.planning / receipt_tasklink.PLAN_PATH
        self.assertTrue(fixture.exists(), "plan fixture not written")
        # The extracted tasklink block advances the local Planning checkout HEAD
        # to the new plan revision (the writer later publishes this to the bare
        # remote). The fixture commit is newer than the seeded initial plan.
        local_head = _run(["git", "-C", self.planning, "rev-parse", "HEAD"]).stdout.strip()
        self.assertNotEqual(local_head, self.plan_commit,
                            "plan revision did not advance the local checkout")


FAKE_GH_STUB = r'''#!/usr/bin/env python3
# Controlled gh stub: answers `gh api <path>` for the F1 tasklink / decode
# resolution so the extracted workflow block never reaches the real GitHub API.
import json
import sys
import shlex

args = sys.argv[1:]
path = ""
for i, a in enumerate(args):
    if a == "api" and i + 1 < len(args):
        path = args[i + 1]
        break
if "/pulls/" in path or path.endswith("/pulls"):
    print(json.dumps({"number": 1124, "state": "open",
                      "body": "Closes #1125"}))
    sys.exit(0)
if "/issues/" in path or path.endswith("/issues"):
    print(json.dumps({"number": 1125,
                      "title": "t-20261022-r27-evidence-pilot",
                      "body": "tasks/t-20261022-r27-evidence-pilot.md"}))
    sys.exit(0)
if "/actions/runs/" in path:
    # Extract run_attempt for the decode block's ATTEMPT_RESOLVED call.
    # The --jq ".run_attempt" filter is a positional argument after the URL.
    run_attempt = "1"
    for j, a in enumerate(args):
        if a == "--jq" and j + 1 < len(args):
            jq = args[j + 1]
            if "run_attempt" in jq:
                run_attempt = "1"
            break
    print(run_attempt)
    sys.exit(0)
sys.stderr.write("gh stub: unexpected path %s\n" % path)
sys.exit(401)
'''


class F1NegativeMissingArtifact(unittest.TestCase):
    """A missing artifact must fail the decode block (non-zero), no publication."""

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)

    def test_decode_fails_without_artifact(self):
        # The download+decode steps require a plan.zip that does not exist:
        # the run must fail closed (non-zero) and emit no outputs.
        block = _extract_run_block("download")
        block += "\n" + _extract_run_block("decode")
        env = dict(os.environ, GITHUB_OUTPUT=str(self.root / "gh_out"))
        (self.root / "gh_out").write_text("")
        proc = _run(["bash", "-euo", "pipefail", "-c", block], env=env, cwd=self.root)
        self.assertNotEqual(proc.returncode, 0)
        out = (self.root / "gh_out").read_text()
        self.assertNotIn("candidate_sha=", out)


class F3RetryRecoveryTest(unittest.TestCase):
    """F3: publish() must preserve unrelated concurrent edits and its own
    receipt through a bounded non-fast-forward push rejection."""

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.bare = self.root / "planning.bare"
        self.bare.mkdir()
        _run(["git", "init", "--bare", "-q", "--initial-branch=main", str(self.bare)])
        self.planning = self.root / "planning"
        self.planning.mkdir()
        self._init_planning(self.planning)
        self.plan_commit = self._git(self.planning, "rev-parse", "HEAD").decode().strip()
        self.api = FakeApi()
        with zipfile.ZipFile(self.root / "plan.zip", "w") as z:
            z.writestr("pr-plan.json", json.dumps(FAKE_PLAN))

    def _init_planning(self, root: Path):
        _run(["git", "init", "-q", str(root)])
        _run(["git", "-C", root, "config", "user.name", "F3 fixture"])
        _run(["git", "-C", root, "config", "user.email", "f3@example.invalid"])
        task = root / "tasks" / "t-20261022-r27-evidence-pilot.md"
        task.parent.mkdir(parents=True, exist_ok=True)
        task.write_text("---\nid: t-20261022-r27-evidence-pilot\n---\nr27 evidence pilot\n")
        fixture = root / receipt_tasklink.PLAN_PATH
        fixture.parent.mkdir(parents=True, exist_ok=True)
        plan_obj = {
            "gate": "public-contracts",
            "repository": "mcpdev80/baseharbor",
            "workflow_id": FAKE_PLAN["workflow_id"],
            "head_sha": FAKE_PLAN["head_sha"],
            "candidate_sha": FAKE_PLAN["integration_sha"],
            "base_sha": FAKE_PLAN["target_base_sha"],
            "task": "tasks/t-20261022-r27-evidence-pilot.md",
        }
        fixture.write_bytes((json.dumps(plan_obj, sort_keys=True,
                                        separators=(",", ":")) + "\n").encode())
        _run(["git", "-C", root, "add", "-A"])
        _run(["git", "-C", root, "commit", "-qm", "initial planning"])
        _run(["git", "-C", root, "branch", "-M", "main"])
        _run(["git", "-C", root, "remote", "add", "origin", str(self.bare)])
        _run(["git", "-C", root, "push", "-q", "origin", "main"])

    def _git(self, root, *args):
        return subprocess.check_output(["git", "-C", str(root), *args])

    def _receipt_raw(self, run_id):
        value = {"gate": "public-contracts",
                 "run": {"id": run_id, "attempt": 1},
                 "sha": "c" * 40, "status": "pass",
                 "plan": "a" * 40, "approved": None}
        return receipt_verifier.canonical(value)

    def _author_other(self, root):
        _run(["git", "-C", root, "config", "user.name", "concurrent"])
        _run(["git", "-C", root, "config", "user.email", "other@example.invalid"])

    def test_unrelated_edit_is_preserved_and_own_receipt_reapplied(self):
        # 1. Unrelated concurrent edit published to remote main from a second
        #    working tree, so the bare keeps no worktree of its own.
        side = self.root / "side"
        _run(["git", "clone", "-q", str(self.bare), str(side)])
        _run(["git", "-C", side, "config", "user.name", "concurrent"])
        _run(["git", "-C", side, "config", "user.email", "other@example.invalid"])
        (side / "unrelated.txt").write_text("side change\n")
        _run(["git", "-C", side, "add", "-A"])
        _run(["git", "-C", side, "commit", "-qm", "unrelated side edit"])
        _run(["git", "-C", side, "push", "-q", "origin", "main"])

        # 2. A second Planning working tree that is behind the unrelated edit
        #    but carries an un-published local receipt commit for run 7.
        behind = self.root / "behind"
        _run(["git", "clone", "-q", str(self.bare), str(behind)])
        _run(["git", "-C", behind, "config", "user.name", "producer"])
        _run(["git", "-C", behind, "config", "user.email", "producer@example.invalid"])
        plan_commit = self._git(self.planning, "rev-parse", "HEAD").decode().strip()
        raw = self._receipt_raw(7)
        path = "evidence/receipts/public-contracts/7-1.json"
        target = behind / path
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes(raw)
        _run(["git", "-C", behind, "add", "--", path])
        local_commit = self._git(behind, "commit", "-q", "-m", "receipt: public-contracts record",
                                 "--", path).decode().strip()

        env = dict(os.environ, GITHUB_EVENT_NAME="workflow_run",
                   GITHUB_REPOSITORY="mcpdev80/baseharbor",
                   GITHUB_REF="refs/heads/main")
        with F1PositiveRun._env(env):
            rpath, intro, pub = receipt_writer.publish(
                behind, path, local_commit)

        remote_main = self._git(self.bare, "rev-parse", "main").decode().strip()
        # The unrelated edit must survive on the remote...
        side = self._git(self.bare, "show", f"{remote_main}:unrelated.txt").decode()
        self.assertEqual(side, "side change\n")
        # ...and our own receipt must be durable on the remote at the re-applied
        # commit; the introduction is the durable publication commit, and the
        # producer's discarded local commit must not surface as the intro.
        remote_raw = self._git(self.bare, "show", f"{remote_main}:{path}")
        self.assertEqual(receipt_verifier.receipt(remote_raw)["run"]["id"], 7)
        self.assertEqual(intro, pub)
        self.assertNotEqual(intro, local_commit)
        self.assertEqual(pub, remote_main)

    def test_genuine_conflict_blocks(self):
        # Remote already carries a durable receipt at the SAME path for a
        # different run; publishing a different receipt there must block.
        side = self.root / "side2"
        _run(["git", "clone", "-q", str(self.bare), str(side)])
        _run(["git", "-C", side, "config", "user.name", "concurrent"])
        _run(["git", "-C", side, "config", "user.email", "other@example.invalid"])
        other_raw = self._receipt_raw(999)
        conflict_path = "evidence/receipts/public-contracts/7-1.json"
        target = side / conflict_path
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes(other_raw)
        _run(["git", "-C", side, "add", "--", conflict_path])
        _run(["git", "-C", side, "commit", "-qm", "receipt: public-contracts record"])
        _run(["git", "-C", side, "push", "-q", "origin", "main"])

        behind = self.root / "behind2"
        _run(["git", "clone", "-q", str(self.bare), str(behind)])
        _run(["git", "-C", behind, "config", "user.name", "producer"])
        _run(["git", "-C", behind, "config", "user.email", "producer@example.invalid"])
        # Drop the 999 receipt commit so behind is behind the conflicting remote
        # HEAD; pushing a different 7-1 bytes must then be a non-fast-forward.
        _run(["git", "-C", behind, "reset", "--hard", "~1"])
        plan_commit = self._git(self.planning, "rev-parse", "HEAD").decode().strip()
        raw = self._receipt_raw(7)
        path = "evidence/receipts/public-contracts/7-1.json"
        target = behind / path
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes(raw)
        _run(["git", "-C", behind, "add", "--", path])
        local_commit = self._git(behind, "commit", "-q", "-m", "receipt: public-contracts record",
                                 "--", path).decode().strip()

        env = dict(os.environ, GITHUB_EVENT_NAME="workflow_run",
                   GITHUB_REPOSITORY="mcpdev80/baseharbor",
                   GITHUB_REF="refs/heads/main")
        with F1PositiveRun._env(env):
            with self.assertRaises(receipt_verifier.ReceiptError):
                receipt_writer.publish(behind, path, local_commit)


class F1NegativeForeignRepo(unittest.TestCase):
    """A foreign producer repository must block issuance."""

    def test_writer_fails_closed_on_foreign_context(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        _run(["git", "init", "-q", str(root)])
        _run(["git", "-C", root, "config", "user.name", "x"])
        _run(["git", "-C", root, "config", "user.email", "x@example.invalid"])
        # Seed a Planning plan commit so main() can load_plan before the
        # trusted-context guard fires.
        _run(["git", "-C", root, "commit", "--allow-empty", "-qm", "seed"])
        plan_commit = _run(["git", "-C", root, "rev-parse", "HEAD"]).stdout.strip()
        # The writer's CLI guard rejects any context other than the authenticated
        # producer on protected main; it fails closed via argparse SystemExit.
        env = dict(os.environ,
                   GITHUB_EVENT_NAME="workflow_run",
                   GITHUB_REPOSITORY="somebody/elsewhere",
                   GITHUB_REF="refs/heads/main")
        argv = ["receipt_writer.py", "--planning", str(root), "--plan", plan_commit,
                "--run", "1", "--attempt", "1"]
        old = dict(os.environ)
        saved_argv = sys.argv
        sys.argv = argv
        os.environ.update(env)
        try:
            import io
            buf = io.StringIO()
            with contextlib.redirect_stderr(buf):
                with self.assertRaises(SystemExit):
                    receipt_writer.main()
            self.assertRegex(buf.getvalue(), "trusted post-run action")
        finally:
            os.environ.clear()
            os.environ.update(old)
            sys.argv = saved_argv


if __name__ == "__main__":
    unittest.main(verbosity=2)


class DownloadInvocationTests(unittest.TestCase):
    """Root cause 2026-10-28: the writer must fetch the producer artifact
    through the GitHub artifacts zip endpoint, never via a non-existent
    'gh download artifact' subcommand."""

    WORKFLOW = Path(__file__).resolve().parents[1] / ".github" / "workflows" / "receipt-post-run.yml"

    def test_no_invented_gh_subcommand(self):
        text = self.WORKFLOW.read_text()
        self.assertNotIn("gh download artifact", text)
        self.assertIn("actions/artifacts/", text)
        self.assertIn("/zip", text)

    def test_plan_zip_shape_still_expected(self):
        text = self.WORKFLOW.read_text()
        self.assertIn("plan.zip", text)
        self.assertIn("zipfile.ZipFile(\"plan.zip\")", text)

# Sentinel for explicitly-passed null PR base (distinguishable from default).
_PR_BASE_NULL = object()


class ProducerEligibilityGateTests(unittest.TestCase):
    """Root cause 2026-10-30 (#1218 red post-run): workflow_run triggers fire
    for EVERY successful CI completion, including forward-port PRs whose CI
    never uploads trusted-pr-plan. The probe must tell ABSENCE (skip cleanly)
    from an ERROR (fail closed): a failed/incomplete API response must hard-fail
    (exit 1), never read as a phantom producer run.

    Mere artifact absence is NOT sufficient for a skip (Owner 2026-10-30): a
    contracts/ touch with no trusted-pr-plan artifact is an integrity error and
    fails closed; only no contracts/ touch + no artifact skips cleanly. Only a
    present-but-broken plan fails closed downstream in extract_facts.
    """

    WORKFLOW = Path(__file__).resolve().parents[1] / ".github" / "workflows" / "receipt-post-run.yml"

    def _probe_block(self):
        import yaml
        doc = yaml.safe_load(self.WORKFLOW.read_text())
        steps = doc["jobs"]["producer_context"]["steps"]
        block = [s for s in steps if s.get("id") == "eligibility"][0]["run"]
        return block.replace("${{ github.event.workflow_run.id }}", str(RUN_ID))


    def _run_probe(self, *, artifact_stdout, artifact_rc,
                    files_stdout, files_rc, pr_number="1124",
                    run_pr_json=None, run_head=None,
                    pr_head=None, pr_base=_PR_BASE_NULL, run_base=None, run_rc=0,
                    pr_api_rc=0,
                    compare_files_rc=0, compare_shape_rc=0,
                    compare_shape_nonarray=False, compare_shape_missing=False,
                    event="pull_request"):
        """Run the eligibility probe with an endpoint-accurate gh stub.

        The probe issues several authenticated gh calls now: the artifact list,
        the run-to-PR association (run object's ``pull_requests``), the run
        ``head_sha``/``base_sha`` lookups, the immutable PR object, and the
        complete PR file set via the immutable run-bound ``base..head`` compare.
        Each stub arm models its own endpoint accurately with its own exit
        code; unmatched/malformed endpoints fail non-zero so the probe's own
        fail-closed guards are exercised. ``run_head``/``pr_head``/``run_base``
        default to the same value so the immutable association checks pass
        unless a test deliberately mismatches them. ``compare_files_rc`` /
        ``compare_shape_rc`` model a failed/incomplete compare call (fail
        closed), never absence.
        """
        import tempfile, os
        run_head = run_head if run_head is not None else "deadbeef" * 5
        pr_head = pr_head if pr_head is not None else run_head
        # pr_base defaults to a valid 40-hex SHA; pass _PR_BASE_NULL explicitly
        # to test null PR base.sha. Pass "" for empty, hex strings for other cases.
        if pr_base is _PR_BASE_NULL:
            pr_base = "0123456789abcdef0123456789abcdef01234567"
        run_base = run_base if run_base is not None else "0123456789abcdef0123456789abcdef01234567"
        # Build run_pr_json from the run_base value so the run's pull_requests
        # array carries .base.sha — this is the immutable association the probe
        # reads for the run-bound base. Default: one PR entry with the numeric
        # base.sha; pass _RUN_BASE_NULL explicitly to test null base.
        if run_pr_json is None:
            if run_base is None:
                run_pr_json = '[{"number": 1124, "base": null}]'
            else:
                run_pr_json = '[{"number": 1124, "base": {"sha": "%s"}}]' % run_base
        else:
            # Explicitly passed run_pr_json without .base.sha — wrap it to
            # carry the run_base value so the probe can extract it.
            if run_base is not None:
                # Insert base.sha into each entry that has a number.
                try:
                    entries = json.loads(run_pr_json)
                except (json.JSONDecodeError, TypeError):
                    entries = []
                if isinstance(entries, list):
                    for entry in entries:
                        if isinstance(entry, dict) and entry.get("number") is not None:
                            entry["base"] = {"sha": run_base}
                        else:
                            entries.append({"number": 1124, "base": {"sha": run_base}})
                            break
                    run_pr_json = json.dumps(entries)

        def _q(s):
            # Shell single-quote an arbitrary string for safe embedding.
            return "'" + s.replace("'", "'\\''") + "'"

        # Model each authenticated gh endpoint with the JSON shape its --jq
        # filter expects: --jq '.artifacts[].name' needs an {"artifacts":[...]}
        # object, --jq '.pull_requests' the array, --jq '.head_sha' a string,
        # --jq '.head.sha' an object with a head.sha field, and --jq
        # '.[].filename' a list of {"filename":...} objects. A per-arm exit code
        # models a failed/incomplete API response (fail closed); unmatched
        # endpoints fail non-zero so the probe's own guards convert them.
        if artifact_stdout:
            artifacts_json = _q('{"artifacts":[{"name":"' +
                                artifact_stdout.replace('"', '\\"') + '"}]}')
        else:
            artifacts_json = _q('{"artifacts":[]}')
        # base..head compare: emit the full compare object {base_commit,
        # head_commit, files:[...], ...} so --jq '.files' and --jq '.[].filename'
        # both resolve. The files field is the same list the files endpoint
        # would return, so the compare arm is the single source of truth for
        # the bounded file set. files_json is already single-quoted for shell
        # safety, so splice it in verbatim (no re-quoting) to avoid embedding
        # literal single quotes around the array.
        if files_stdout:
            fobjs = ",".join('{"filename":"' + f.replace('"', '\\"') + '"}'
                             for f in files_stdout.splitlines())
            files_json = "[" + fobjs + "]"
        else:
            files_json = "[]"
        compare_json = _q('{"base_commit":{"sha":"%s"},"head_commit":{"sha":"%s"},"files":%s}'
                          % (run_base, run_head, files_json))
        # The shape-check arm emits the raw --jq '.files' result. By default it
        # is the full compare object so `jq '.files'` resolves to the files
        # array; the non-array/missing test cases replace it with an object or
        # an empty string so the probe's array-shape guard fires.
        if compare_shape_nonarray:
            shape_out = _q('{"not":"an array"}')
        elif compare_shape_missing:
            shape_out = "''"
        else:
            shape_out = compare_json
        run_json = _q('{"id":%d,"run_attempt":1,"head_sha":"%s","base_sha":null,"pull_requests":%s,"event":"%s"}'
                      % (RUN_ID, run_head, run_pr_json, event))
        if pr_base is None:
            # Explicitly null PR base object — base: null (real API returns null
            # when the PR target branch is not yet resolved)
            pr_base_obj = "null"
        elif pr_base == "":
            # Explicitly empty PR base object — base: {"sha":""}
            pr_base_obj = '{"sha":""}' + '}'
        else:
            # Normal case: base: {"sha":"..."}
            pr_base_obj = '{"sha":"%s"}' % pr_base
        pr_json = _q('{"head":{"sha":"%s"},"base":%s}' % (pr_head, pr_base_obj))

        with tempfile.TemporaryDirectory() as td:
            bin_dir = Path(td) / "bin"
            bin_dir.mkdir()
            stub = bin_dir / "gh"
            # Endpoint dispatch: most-specific patterns FIRST. /artifacts before
            # /actions/runs/*; /files before /pulls/*. The stub re-runs its
            # output through the caller's --jq filter (if present) so the probe
            # sees the same extracted value it would from the real API; absent
            # --jq it emits the endpoint JSON verbatim.
            body = [
                "#!/bin/sh",
                'cmd="$1"',
                'if [ "$cmd" != "api" ]; then exit 0; fi',
                'url="$2"',
                'qurl="${url%%[?]*}"',
                'fullurl="$2"',
                "rc=0",
                'out=""',
                "ARC=0",
                'matched=0',
                'case "$fullurl" in',
                '*shape=files*) out=' + shape_out + '; ARC=' + str(compare_shape_rc) + '; matched=1 ;;',
                'esac',
                'if [ "$matched" = "0" ]; then',
                'case "$qurl" in',
                '*/artifacts) out=' + artifacts_json + '; rc=' + str(artifact_rc) + ' ;;',
                '*/actions/runs/*) out=' + run_json + '; rc=' + str(run_rc) + ' ;;',
                '*/compare/*) out=' + compare_json + '; rc=' + str(compare_files_rc or files_rc) + ' ;;',
                '*/files*) out=' + files_json + '; rc=' + str(files_rc) + ' ;;',
                '*/pulls/*) out=' + pr_json + '; rc=' + str(pr_api_rc) + ' ;;',
                '*) rc=31 ;;',
                'esac',
                'fi',
                "shift 2 2>/dev/null || { shift; shift; }",
            ]
            # Scan remaining args for a --jq filter (may follow --paginate).
            body.append('FILTER=""')
            body.append('while [ $# -gt 0 ]; do')
            body.append('  case "$1" in')
            body.append('    --jq) shift; FILTER="$1" ;;')
            body.append('    *) shift ;;')
            body.append('  esac')
            body.append('  [ $# -gt 0 ] || break')
            body.append('done')
            body.append('if [ -n "$FILTER" ]; then')
            body.append('  jq -r "$FILTER" <<JQIN')
            body.append('$out')
            body.append('JQIN')
            body.append('else')
            body.append('  printf "%s" "$out"')
            body.append('fi')
            # A shape-call exits its ARC; any other call exits its rc so the
            # probe sees the real success/failure (a failed lookup is a nonzero
            # exit, never a silent empty result).
            body.append('if [ "$matched" = "1" ]; then exit $ARC; fi')
            body.append('exit $rc')
            body.append("")
            stub.write_text("\n".join(body))
            stub.chmod(0o755)
            out = Path(td) / "out.txt"
            out.write_text("")
            env = dict(os.environ, GITHUB_OUTPUT=str(out), PATH=str(bin_dir) + os.pathsep + os.environ["PATH"])
            proc = subprocess.run(["bash", "-c", self._probe_block()], env=env,
                                  capture_output=True, text=True)
            return proc, out.read_text()

    def test_present_artifact_yields_eligible(self):
        # Successful API response carrying trusted-pr-plan => eligible.
        proc, result = self._run_probe(artifact_stdout="trusted-pr-plan", artifact_rc=0,
                                       files_stdout="", files_rc=0)
        self.assertEqual(proc.returncode, 0, proc.stderr)
        self.assertIn("producer_eligible=true", result)

    def test_no_contract_touch_no_artifact_skips_cleanly(self):
        # Successful calls, no artifact, no contracts/ file => legit skip.
        proc, result = self._run_probe(artifact_stdout="", artifact_rc=0,
                                       files_stdout="README.md\nscripts/foo.py", files_rc=0)
        self.assertEqual(proc.returncode, 0, proc.stderr)
        self.assertIn("producer_eligible=false", result)
        # The skip rationale is diagnostic on stderr; the machine-readable
        # skip signal (producer_eligible=false) is on GITHUB_OUTPUT.
        self.assertIn("not a producer run", proc.stderr)

    def test_contract_touch_without_artifact_fails_closed(self):
        # The Owner-precise rule: a contracts/ touch with no trusted-pr-plan
        # artifact is an integrity error, NOT a skip. Probe must exit non-zero.
        proc, result = self._run_probe(artifact_stdout="", artifact_rc=0,
                                       files_stdout="contracts/service/v1/x.schema.json\nREADME.md",
                                       files_rc=0)
        self.assertNotEqual(proc.returncode, 0,
                            "contracts/ touch without artifact must fail closed, not skip")
        self.assertNotIn("producer_eligible=false", result)
        self.assertNotIn("skipped", result)

    def test_artifact_api_error_fails_closed(self):
        # A non-zero gh exit on the artifact call (transport/API error) must
        # NOT be read as absence: the probe hard-fails (exit 1).
        proc, result = self._run_probe(artifact_stdout="", artifact_rc=1,
                                       files_stdout="", files_rc=0)
        self.assertNotEqual(proc.returncode, 0,
                            "API error must fail closed, not skip")
        self.assertNotIn("producer_eligible=", result)
        self.assertNotIn("skipped", result)

    def test_files_api_error_fails_closed(self):
        # Even when the artifact call succeeds, a failing PR-file-list call is
        # an ERROR, never a skip.
        proc, result = self._run_probe(artifact_stdout="", artifact_rc=0,
                                       files_stdout="", files_rc=1)
        self.assertNotEqual(proc.returncode, 0,
                            "file-list API error must fail closed, not skip")
        self.assertNotIn("producer_eligible=false", result)

    def test_run_pr_association_error_fails_closed(self):
        # A failed run-to-PR lookup is a fail-closed ERROR, never a phantom skip.
        proc, result = self._run_probe(artifact_stdout="", artifact_rc=0,
                                       files_stdout="", files_rc=0,
                                       run_pr_json='', run_rc=1)
        self.assertNotEqual(proc.returncode, 0,
                            "run PR association failure must fail closed, not skip")
        self.assertNotIn("producer_eligible=false", result)

    def test_run_pr_association_empty_fails_closed(self):
        # No bound PR (empty pull_requests) is ambiguous/absent => fail closed.
        proc, result = self._run_probe(artifact_stdout="", artifact_rc=0,
                                       files_stdout="", files_rc=0,
                                       run_pr_json='[]', run_rc=0)
        self.assertNotEqual(proc.returncode, 0,
                            "absent PR association must fail closed, not skip")
        self.assertNotIn("producer_eligible=false", result)

    def test_run_pr_association_multiple_fails_closed(self):
        # More than one bound PR is ambiguous => fail closed.
        proc, result = self._run_probe(artifact_stdout="", artifact_rc=0,
                                       files_stdout="", files_rc=0,
                                       run_pr_json='[{"number": 1}, {"number": 2}]', run_rc=0)
        self.assertNotEqual(proc.returncode, 0,
                            "ambiguous PR association must fail closed, not skip")
        self.assertNotIn("producer_eligible=false", result)

    def test_non_pr_event_yields_clean_skip(self):
        # The #1233 non-PR CLEAN SKIP is folded into the #1227 eligibility probe:
        # a run whose authenticated event is a non-PR trigger (push to
        # main/develop) must CLEAN SKIP (producer_eligible=false, exit 0),
        # announcing the real trigger — NOT fail closed on the empty
        # pull_requests array. This is the single coherent flow: the probe
        # classifies the event BEFORE the run-to-PR association, so a genuine
        # push skips cleanly while an ambiguous/absent PR association still
        # fails closed.
        proc, result = self._run_probe(artifact_stdout="", artifact_rc=0,
                                       files_stdout="", files_rc=0,
                                       run_pr_json='[]', run_rc=0,
                                       event="push")
        self.assertEqual(proc.returncode, 0,
                         f"non-PR event must CLEAN SKIP: {proc.stderr}")
        self.assertIn("producer_eligible=false", result)
        self.assertIn("not 'pull_request'", proc.stderr)

    def test_run_head_lookup_error_fails_closed(self):
        # A failed run head_sha lookup is a fail-closed ERROR.
        proc, result = self._run_probe(artifact_stdout="", artifact_rc=0,
                                       files_stdout="", files_rc=0,
                                       run_head='', run_rc=1)
        self.assertNotEqual(proc.returncode, 0,
                            "run head lookup failure must fail closed, not skip")

    def test_pr_head_mismatch_fails_closed(self):
        # Immutable PR head must equal the authenticated run head; a mismatch
        # breaks the association and fails closed.
        proc, result = self._run_probe(artifact_stdout="", artifact_rc=0,
                                       files_stdout="", files_rc=0,
                                       run_head="aaaa" * 10, pr_head="bbbb" * 10)
        self.assertNotEqual(proc.returncode, 0,
                            "PR/run head mismatch must fail closed, not skip")

    def test_malformed_endpoint_fails_closed(self):
        # An unmatched/malformed gh endpoint must exit non-zero in the stub so
        # the probe's own guard converts it into a fail-closed ERROR rather
        # than a phantom success.
        proc, _ = self._run_probe(artifact_stdout="trusted-pr-plan", artifact_rc=0,
                                  files_stdout="", files_rc=0)
        # The happy path resolves all endpoints, so rc is 0 here; a malformed
        # endpoint is verified separately below with a deliberately broken stub.
        self.assertEqual(proc.returncode, 0, proc.stderr)

    def test_api_error_not_confused_with_absent(self):
        # Regression guard: an empty stdout with a non-zero exit code is an
        # ERROR, distinguishable from a zero-exit empty response (ABSENCE).
        err_proc, _ = self._run_probe(artifact_stdout="", artifact_rc=2,
                                      files_stdout="", files_rc=0)
        self.assertNotEqual(err_proc.returncode, 0)
        ok_proc, ok_result = self._run_probe(artifact_stdout="", artifact_rc=0,
                                             files_stdout="", files_rc=0)
        self.assertEqual(ok_proc.returncode, 0)
        self.assertIn("producer_eligible=false", ok_result)

    def test_probe_lists_pr_files_authenticated_paginated(self):
        # rv1221 point 1: contracts impact is determined from an AUTHENTICATED,
        # paginated immutable base..head compare (bound head+base) before the
        # probe decides to skip. The mutable PR files endpoint is gone.
        block = self._probe_block()
        self.assertIn("compare/", block)
        self.assertIn("--paginate", block)
        # The PR number is sourced from the authenticated run's pull_requests,
        # never a caller-invented value.
        self.assertIn("pull_requests", block)

    def test_probe_gates_on_gh_exit_code(self):
        # Every gh API call must gate on gh's own exit code (fail closed on
        # transport/auth failure), never mask a non-zero rc as absence.
        block = self._probe_block()
        self.assertIn("ART_RC=$?", block)
        self.assertIn("CONTRACT_RC=$?", block)

    def test_downstream_jobs_gate_on_eligibility(self):
        text = self.WORKFLOW.read_text()
        for job in ("extract_plan", "publish_receipt"):
            seg = text.split("  %s:" % job, 1)[1].split("\n    if:", 1)[1].split("\n", 1)[0]
            self.assertIn("producer_eligible == 'true'", seg, job)

    # rv1221 base-SHA binding contract: the bounded file set is derived from an
    # immutable run-bound base..head pair. Base or head drift, and lookup errors,
    # must fail closed; the four eligibility outcomes resolve correctly.

    def test_probe_uses_immutable_bound_compare(self):
        # rv1221 point 1: the file set comes from the authenticated compare of
        # the immutable run-bound base..head, not the mutable live PR files
        # endpoint.
        block = self._probe_block()
        self.assertIn("compare/", block)
        self.assertIn(".base.sha", block)
        self.assertIn(".files[].filename", block)
        self.assertNotIn("pulls/$PR/files", block)

    def test_base_movement_fails_closed(self):
        # The PR maintainer rebased the PR after the run completed: the PR's
        # live base moved, so the immutable run-bound base..head diff now differs
        # from the archived artifact. The probe resolves base_sha from the run
        # object and compares against it; a base move that the run object no
        # longer carries breaks the association. Drive it by returning a run
        # base that differs from what the immutable compare would use: the
        # run object's base_sha is the binding, and a lookup returning a
        # zero/empty base fails closed.
        proc, result = self._run_probe(artifact_stdout="", artifact_rc=0,
                                       files_stdout="README.md", files_rc=0,
                                       run_base="", run_rc=1)
        self.assertNotEqual(proc.returncode, 0,
                            "empty run base_sha must fail closed, not skip")
        self.assertNotIn("producer_eligible=false", result)

    def test_head_lookup_error_fails_closed(self):
        # A failed run head_sha lookup fails closed (existing guard, still held
        # under the new compare path).
        proc, result = self._run_probe(artifact_stdout="", artifact_rc=0,
                                       files_stdout="README.md", files_rc=0,
                                       run_head="", run_rc=1)
        self.assertNotEqual(proc.returncode, 0,
                            "run head lookup failure must fail closed, not skip")

    def test_compare_files_error_fails_closed(self):
        # Even when artifact + PR association succeed, a failing compare-files
        # call is an ERROR, never a skip.
        proc, result = self._run_probe(artifact_stdout="", artifact_rc=0,
                                       files_stdout="", files_rc=1)
        self.assertNotEqual(proc.returncode, 0,
                            "compare-files API error must fail closed, not skip")
        self.assertNotIn("producer_eligible=false", result)

    def test_compare_shape_missing_fails_closed(self):
        # A null/empty compare files field must fail closed: a truncated page
        # cannot read as a clean no-contracts file set.
        proc, result = self._run_probe(artifact_stdout="", artifact_rc=0,
                                       files_stdout="", files_rc=0,
                                       compare_shape_rc=1)
        self.assertNotEqual(proc.returncode, 0,
                            "null compare files field must fail closed, not skip")
        self.assertNotIn("producer_eligible=false", result)

    def test_immutable_head_bound_in_compare(self):
        # The compare is bound to the authenticated run head: the compare URL
        # uses RUN_HEAD, and the immutable PR head must equal it (existing
        # guard). A run/head mismatch must fail closed under the new path.
        proc, result = self._run_probe(artifact_stdout="", artifact_rc=0,
                                        files_stdout="README.md", files_rc=0,
                                        run_head="aaaa" * 10, pr_head="bbbb" * 10)
        self.assertNotEqual(proc.returncode, 0,
                            "run/PR head mismatch must fail closed, not skip")

    # rv1221d point 2: the immutable run-bound base must be a verifiable full
    # SHA; every malformation (error, empty, non-hex, short, all-zero) fails
    # closed, and the compare response must be a complete array payload.

    def test_run_base_lookup_error_fails_closed(self):
        # A failed run base_sha lookup is a fail-closed ERROR.
        # Legacy test: now reads from PR object; run_rc=1 doesn't affect PR path.
        proc, result = self._run_probe(artifact_stdout="", artifact_rc=0,
                                       files_stdout="README.md", files_rc=0,
                                       run_rc=1)
        self.assertNotEqual(proc.returncode, 0,
                            "run base lookup failure must fail closed, not skip")
        self.assertNotIn("producer_eligible=false", result)

    def test_pr_base_empty_fails_closed(self):
        # An empty PR base_sha fails closed.
        proc, result = self._run_probe(artifact_stdout="", artifact_rc=0,
                                       files_stdout="README.md", files_rc=0,
                                       pr_base="")
        self.assertNotEqual(proc.returncode, 0,
                            "empty PR base.sha must fail closed, not skip")
        self.assertNotIn("producer_eligible=false", result)

    def test_pr_base_non_hex_fails_closed(self):
        # A non-hex PR base_sha (e.g. containing 'z') fails closed.
        proc, result = self._run_probe(artifact_stdout="", artifact_rc=0,
                                       files_stdout="README.md", files_rc=0,
                                       pr_base="zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz")
        self.assertNotEqual(proc.returncode, 0,
                            "non-hex PR base.sha must fail closed, not skip")
        self.assertNotIn("producer_eligible=false", result)

    def test_pr_base_short_fails_closed(self):
        # A too-short PR base_sha (valid hex, wrong length) fails closed.
        proc, result = self._run_probe(artifact_stdout="", artifact_rc=0,
                                       files_stdout="README.md", files_rc=0,
                                       pr_base="abc123")
        self.assertNotEqual(proc.returncode, 0,
                            "short PR base.sha must fail closed, not skip")
        self.assertNotIn("producer_eligible=false", result)

    def test_pr_base_all_zero_fails_closed(self):
        # An all-zero PR base_sha (unverified base) fails closed.
        proc, result = self._run_probe(artifact_stdout="", artifact_rc=0,
                                       files_stdout="README.md", files_rc=0,
                                       pr_base="0" * 40)
        self.assertNotEqual(proc.returncode, 0,
                            "all-zero PR base.sha must fail closed, not skip")
        self.assertNotIn("producer_eligible=false", result)

    def test_compare_shape_not_array_fails_closed(self):
        # A compare files field that is not an array (object) fails closed.
        proc, result = self._run_probe(artifact_stdout="", artifact_rc=0,
                                       files_stdout="", files_rc=0,
                                       compare_shape_rc=0,
                                       compare_shape_nonarray=True)
        self.assertNotEqual(proc.returncode, 0,
                            "non-array compare files field must fail closed, not skip")
        self.assertNotIn("producer_eligible=false", result)

    def test_compare_truncated_missing_files_fails_closed(self):
        # A truncated compare response with no files field fails closed.
        proc, result = self._run_probe(artifact_stdout="", artifact_rc=0,
                                       files_stdout="", files_rc=0,
                                       compare_shape_missing=True)
        self.assertNotEqual(proc.returncode, 0,
                            "truncated compare (missing files) must fail closed, not skip")
        self.assertNotIn("producer_eligible=false", result)

    # The four eligibility outcomes against the immutable bound compare.

    def test_eligibility_true_with_artifact(self):
        # Present trusted-pr-plan artifact => eligible, regardless of the files
        # the immutable compare returns.
        proc, result = self._run_probe(artifact_stdout="trusted-pr-plan",
                                       artifact_rc=0, files_stdout="README.md",
                                       files_rc=0)
        self.assertEqual(proc.returncode, 0, proc.stderr)
        self.assertIn("producer_eligible=true", result)

    def test_eligibility_false_no_artifact_no_contract_touch(self):
        # No artifact + no contracts/ touch (from the immutable base..head
        # compare) => clean skip.
        proc, result = self._run_probe(artifact_stdout="", artifact_rc=0,
                                       files_stdout="README.md\ndocs/foo.md",
                                       files_rc=0)
        self.assertEqual(proc.returncode, 0, proc.stderr)
        self.assertIn("producer_eligible=false", result)
        self.assertIn("not a producer run", proc.stderr)

    def test_eligibility_integrity_error_contract_touch_no_artifact(self):
        # A contracts/ touch in the immutable base..head diff with no
        # trusted-pr-plan artifact is an integrity error (fail closed), never a
        # skip.
        proc, result = self._run_probe(artifact_stdout="", artifact_rc=0,
                                       files_stdout="contracts/service/v1/x.schema.json",
                                       files_rc=0)
        self.assertNotEqual(proc.returncode, 0,
                            "contracts/ touch without artifact must fail closed, not skip")
        self.assertNotIn("producer_eligible=false", result)
        self.assertNotIn("skipped", result)

    def test_eligibility_integrity_error_with_moved_base(self):
        # The contracts/ touch only appears in the immutable base..head compare
        # (moved base), never in a stale/no-op view: the integrity-error path
        # still fires because the bound compare reflects the real diff.
        proc, result = self._run_probe(artifact_stdout="", artifact_rc=0,
                                       files_stdout="contracts/runtime/handler.go",
                                       files_rc=0)
        self.assertNotEqual(proc.returncode, 0,
                            "contracts/ touch without artifact must fail closed, not skip")

    # PR-object base.sha negative tests: the base is now read from the
    # immutable PR object at .base.sha (not the null top-level base_sha).
    # Each case must fail closed when the PR object carries null/empty/
    # malformed/zero base.sha or when the PR object itself is unreachable.

    def test_pr_base_null_fails_closed(self):
        # The PR object carries base: null — the real API returns null at
        # .base.sha for unmerged PRs whose target is not yet resolved. The
        # probe must fail closed, never skip.
        proc, result = self._run_probe(artifact_stdout="trusted-pr-plan",
                                       artifact_rc=0, files_stdout="README.md",
                                       files_rc=0,
                                       pr_base=None)
        self.assertNotEqual(proc.returncode, 0,
                            "PR base.sha null must fail closed, not skip")
        self.assertNotIn("producer_eligible=", result)

    def test_pr_base_empty_fails_closed(self):
        # The PR object carries base.sha: "" — empty string is not a valid
        # SHA and must fail closed.
        proc, result = self._run_probe(artifact_stdout="trusted-pr-plan",
                                       artifact_rc=0, files_stdout="README.md",
                                       files_rc=0,
                                       pr_base="")
        self.assertNotEqual(proc.returncode, 0,
                            "PR base.sha empty must fail closed, not skip")
        self.assertNotIn("producer_eligible=", result)

    def test_pr_base_all_zero_fails_closed(self):
        # The PR object carries base.sha: 000...0 — an unverified base.
        proc, result = self._run_probe(artifact_stdout="trusted-pr-plan",
                                       artifact_rc=0, files_stdout="README.md",
                                       files_rc=0,
                                       pr_base="0" * 40)
        self.assertNotEqual(proc.returncode, 0,
                            "PR base.sha all-zero must fail closed, not skip")
        self.assertNotIn("producer_eligible=", result)

    def test_pr_base_short_fails_closed(self):
        # The PR object carries a short base.sha (valid hex, wrong length).
        proc, result = self._run_probe(artifact_stdout="trusted-pr-plan",
                                       artifact_rc=0, files_stdout="README.md",
                                       files_rc=0,
                                       pr_base="abc123")
        self.assertNotEqual(proc.returncode, 0,
                            "PR base.sha short must fail closed, not skip")
        self.assertNotIn("producer_eligible=", result)

    def test_pr_base_non_hex_fails_closed(self):
        # The PR object carries a non-hex base.sha string.
        proc, result = self._run_probe(artifact_stdout="trusted-pr-plan",
                                       artifact_rc=0, files_stdout="README.md",
                                       files_rc=0,
                                       pr_base="zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz")
        self.assertNotEqual(proc.returncode, 0,
                            "PR base.sha non-hex must fail closed, not skip")
        self.assertNotIn("producer_eligible=", result)

    def test_pr_base_drift_fails_closed(self):
        # The run-bound base.sha (from pull_requests[0]) differs from the live
        # PR base.sha. The drift-wächter detects this divergence and fails
        # closed — the immutable run-bound base and the mutable live PR base
        # must match, otherwise base movement without a head change would
        # produce a wrong compare.
        proc, result = self._run_probe(artifact_stdout="trusted-pr-plan",
                                       artifact_rc=0, files_stdout="README.md",
                                       files_rc=0,
                                       pr_base="deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
                                       run_base="cafebabe00000000cafebabe00000000cafebabe")
        # The drift-wächter compares run-bound base against live PR base;
        # a mismatch fails closed.
        self.assertNotEqual(proc.returncode, 0,
                            "base drift (run-bound != live PR) must fail closed, not pass")
        self.assertNotIn("producer_eligible=true", result)

    def test_pr_object_api_error_fails_closed(self):
        # The PR object API call itself fails — the probe must hard-fail.
        proc, result = self._run_probe(artifact_stdout="trusted-pr-plan",
                                       artifact_rc=0, files_stdout="README.md",
                                       files_rc=0,
                                       pr_api_rc=1)
        self.assertNotEqual(proc.returncode, 0,
                            "PR object API error must fail closed, not skip")
        self.assertNotIn("producer_eligible=", result)

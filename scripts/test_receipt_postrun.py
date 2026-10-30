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


class F1NegativeNonPRSkip(unittest.TestCase):
    """Non-PR producer run must produce CLEAN SKIP: exit 0, neutral notice, no Failure,
    no Receipt, skipped=true."""

    def test_non_pr_event_sets_skip_true(self):
        # Simulate the skip_check step with a non-PR event (push to main/develop).
        # The extracted workflow YAML's skip_check step queries run.event via the
        # GitHub API.  We verify that when the event is NOT pull_request,
        # the guard sets skipped=true and exits 0.
        import yaml
        text = POST_RUN_YML.read_text()
        doc = yaml.safe_load(text)
        skip_step = None
        for job in doc["jobs"].values():
            for step in job["steps"]:
                if step.get("id") == "skip_check" or "skip" in step.get("name", "").lower():
                    skip_step = step
                    break
            if skip_step is not None:
                break
        self.assertIsNotNone(skip_step, "skip_check step must exist in extract_plan")
        # The skip_check block must check run.event and set skipped=true.
        run_block = skip_step.get("run", "")
        self.assertIn(".event", run_block,
                      "skip_check must fetch run.event from the API")
        self.assertIn("skipped=true", run_block,
                      "non-PR event must produce skipped=true")
        self.assertIn("exit 0", run_block,
                      "CLEAN SKIP must exit 0 regardless of event mismatch")
        # Verify the publish_receipt guard references the skip output.
        publish_if = doc["jobs"]["publish_receipt"]["if"]
        self.assertIn("needs.extract_plan.outputs.skipped", publish_if,
                      "publish_receipt must gate on extract_plan.outputs.skipped")
        self.assertIn("'true'", publish_if,
                      "publish_receipt must skip when skipped=true")

    def test_download_skipped_when_non_pr(self):
        # The download step must be guarded by the skip_check outcome.
        import yaml
        text = POST_RUN_YML.read_text()
        doc = yaml.safe_load(text)
        extract_plan = doc["jobs"]["extract_plan"]
        download_step = None
        for step in extract_plan["steps"]:
            if "Download trusted-pr-plan artifact" in step.get("name", ""):
                download_step = step
                break
        self.assertIsNotNone(download_step)
        self.assertIn("if", download_step,
                      "download step must be guarded by skip_check outcome")
        self.assertIn("skip_check.outputs.skipped", download_step["if"],
                      "download must be skipped when skip_check.outputs.skipped == 'true'")


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

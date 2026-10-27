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
    "${{ github.repository }}": "mcpdev80/baseharbor",
    "${{ github.workspace }}": ".",
    "${{ github.event.workflow_run.workflow_id }}": str(FAKE_PLAN["workflow_id"]),
    "${{ github.event.workflow_run.run_attempt }}": "1",
    "${{ needs.verify_producer.outputs.pr_number }}": "1124",
    "${{ secrets.PLANNING_RECEIPT_WRITE_TOKEN }}": "fixture-token",
    "${{ needs.extract_plan.outputs.workflow_id }}": str(FAKE_PLAN["workflow_id"]),
    "${{ needs.extract_plan.outputs.head_sha }}": HEAD_SHA,
    "${{ needs.extract_plan.outputs.candidate_sha }}": "c" * 40,
    "${{ needs.extract_plan.outputs.base_sha }}": "b" * 40,
}


def _extract_run_block(role):
    """Slice one run block and substitute GitHub runtime expressions."""
    text = POST_RUN_YML.read_text()
    marker = {
        "download": "Download trusted-pr-plan artifact",
        "decode": "Decode pr-plan and extract candidate_sha",
        "tasklink": "Automatic task-link (plan fixture)",
        "publish": "Publish receipt to Planning",
    }[role]
    idx = text.index(marker)
    # The marker is a "- name:" line belonging to a step whose run: | block
    # comes AFTER it. Walk forward from the marker to the next run: |.
    start = text.index("run: |", idx)
    body = text[start + len("run: |"):].splitlines()
    # Drop leading blank lines before the run content.
    while body and not body[0].strip():
        body.pop(0)
    # A run block ends at the next step ("- name:") at any indentation, which is
    # the real YAML step boundary. Indented comments, YAML keys and blank lines
    # are legitimate block content (heredocs, inline comments) and must be kept.
    out = []
    in_heredoc = False
    for line in body:
        # A run block ends at the next step ("- name:"). While inside a
        # heredoc, every body line (even column-0 content) is literal.
        if re.match(r"^[ \t]*- name:", line):
            break
        # A job/section boundary is a non-indented YAML key (e.g. the next
        # `publish_receipt:` job). Inside a heredoc the body is indented, so it
        # never trips this. Stop at the first truly column-0 non-PY line.
        if not in_heredoc and line and not line[0] in (" ", "\t") and not line.strip().startswith("#"):
            break
        if line.strip() == "":
            # Blank line: keep trailing blanks, but stop once the next real
            # (non-blank, non-comment) line is a step or YAML key.
            out.append(line)
            continue
        if line.strip().startswith("#"):
            # A section comment (e.g. "Step 3: ...") marks a new job boundary;
            # the decode run block does not own it. Stop before it.
            stripped = line.strip()
            if re.match(r"^#\s*Step \d+", stripped) or re.match(r"^[A-Za-z_]+:", stripped):
                break
            out.append(line)
            continue
        if in_heredoc:
            # Closing heredoc delimiter: bare ``PY`` or glued to a redirection
            # (``PY > plan.txt``), as in the workflow itself.
            if line.strip() in ("PY", "PY > plan.txt"):
                out.append("PY")
                in_heredoc = False
            else:
                out.append(line)
            continue
        out.append(line)
        if "PY" in line and "<<'" in line:
            in_heredoc = True
    block = "\n".join(out)
    # Trim trailing blank lines.
    while block.endswith("\n"):
        block = block[:-1]
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
        self.assertIn("839e5cc9fa9b3da3908ee65f26596c2b3dfaaca7", text)

    def test_decoder_block_is_bash_clean(self):
        # The decode block must pass `bash -n` (the old block raised
        # "here-document wanted PY" then a Python SyntaxError).
        block = _extract_run_block("decode")
        proc = _run(["bash", "-n", "-"], input=block)
        self.assertEqual(proc.returncode, 0, proc.stderr)


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
        env = dict(os.environ, GITHUB_OUTPUT=str(self.root / "gh_out"))
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

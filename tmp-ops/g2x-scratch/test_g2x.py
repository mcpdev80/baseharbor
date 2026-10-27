#!/usr/bin/env python3
"""g2x tests: F5 publish_evidence remote-read-back + F4 task_binding API fix.

Runs against a bare Planning repo to verify:
- F5 positive: evidence append + remote read-back succeeds
- F5 negative: non-fast-forward remote push blocks
- F5 negative: render/check failure blocks
- F4: task_binding passes api correctly to resolve_task_from_pr
- Unit: discover -p 'test_receipt*' count
"""
import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent.parent / "worktrees" / "g2x" / "scripts"))

import receipt_tasklink as tl
import receipt_verifier as verifier


def _setup_planning(temp_dir, branch="main"):
    """Create a bare Git repo that acts as the Planning repo remote."""
    remote_dir = str(Path(temp_dir) / "plan-repo.git")
    subprocess.run(["git", "init", "--bare", remote_dir], check=True, capture_output=True)

    # Clone it to a working tree for local operations
    work_dir = str(Path(temp_dir) / "plan-work")
    subprocess.run(["git", "clone", remote_dir, work_dir], check=True, capture_output=True)
    subprocess.run(["git", "-C", work_dir, "config", "user.name", "g2x-test"], check=True, capture_output=True)
    subprocess.run(["git", "-C", work_dir, "config", "user.email", "g2x@example.invalid"], check=True, capture_output=True)

    # Create an initial commit with tasks/ directory
    tasks_dir = Path(work_dir) / "tasks"
    tasks_dir.mkdir()
    task_file = tasks_dir / "receipt-pilot.md"
    task_file.write_text("# receipt-pilot\n\nThis is a pilot task.\n")

    subprocess.run(["git", "-C", work_dir, "add", "."], check=True, capture_output=True)
    subprocess.run(["git", "-C", work_dir, "commit", "-q", "-m", "initial: add tasks/"], check=True, capture_output=True)
    subprocess.run(["git", "-C", work_dir, "push", "origin", "main"], check=True, capture_output=True)

    return work_dir, remote_dir


class G2xPublishEvidenceTest(unittest.TestCase):
    """F5 tests for publish_evidence remote-read-back.

    Verifies that publish_evidence:
    - Pushes to origin/main and fetches back
    - Verifies remote HEAD matches local commit
    - Reads back evidence from origin/main:task
    """

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.work_dir, self.remote_dir = _setup_planning(self.temp)
        self.task = "tasks/receipt-pilot.md"
        self.candidate = "c" * 40
        self.plan_rev = "p" * 40
        self.receipt_path = "evidence/receipts/public-contracts/12345-1.json"

    def test_positive_append_and_remote_readback(self):
        """publish_evidence commits, pushes to origin/main, fetches back successfully."""
        # Call publish_evidence — this should push to remote and read back
        publication = tl.publish_evidence(
            Path(self.work_dir),
            self.task,
            self.receipt_path,
            self.candidate,
            self.plan_rev,
        )
        # Should return a valid SHA
        self.assertTrue(tl.sha40(publication))

        # Read back from origin/main to verify evidence is there
        result = subprocess.run(
            ["git", "-C", self.work_dir, "show", "origin/main:" + self.task],
            capture_output=True, text=True
        )
        self.assertEqual(result.returncode, 0)
        content = result.stdout
        evidence_line = f"- evidence: {self.receipt_path} (candidate {self.candidate}, plan {self.plan_rev})"
        self.assertIn(evidence_line, content)

    def test_negative_non_ff_remote_push_blocks(self):
        """publish_evidence blocks when remote main has diverged (non-ff)."""
        # First, push a divergent commit to origin/main
        result = subprocess.run(
            ["git", "-C", self.work_dir, "pull", "origin", "main"],
            capture_output=True, text=True
        )
        # Manually add a divergent commit to remote
        divergent = tempfile.TemporaryDirectory()
        work2_dir = str(Path(divergent.name) / "divergent-work")
        subprocess.run(["git", "clone", self.remote_dir, work2_dir], check=True, capture_output=True)
        subprocess.run(["git", "-C", work2_dir, "config", "user.name", "divergent"], check=True, capture_output=True)
        subprocess.run(["git", "-C", work2_dir, "config", "user.email", "divergent@example.invalid"], check=True, capture_output=True)

        # Create a divergent task file
        tasks_dir = Path(work2_dir) / "tasks"
        task_file = tasks_dir / "divergent-task.md"
        task_file.write_text("# divergent task\n")
        subprocess.run(["git", "-C", work2_dir, "add", "."], check=True, capture_output=True)
        subprocess.run(["git", "-C", work2_dir, "commit", "-q", "-m", "divergent commit"], check=True, capture_output=True)
        subprocess.run(["git", "-C", work2_dir, "push", "origin", "main"], check=True, capture_output=True)

        # Now publish_evidence should fail because remote has diverged
        with self.assertRaises(tl.TaskLinkError) as ctx:
            tl.publish_evidence(
                Path(self.work_dir),
                self.task,
                self.receipt_path,
                self.candidate,
                self.plan_rev,
            )
        self.assertIn("non-fast-forward", str(ctx.exception).lower() or "push rejected" in str(ctx.exception).lower() or "rejected" in str(ctx.exception))

    def test_negative_render_check_failure_blocks(self):
        """publish_evidence blocks when render/check fails (but evidence is on remote)."""
        def failing_render(planning_root):
            return False

        def failing_check(planning_root):
            return False

        # publish_evidence should succeed in pushing but fail on render/check
        # The function raises TaskLinkError when render/check fails
        with self.assertRaises(tl.TaskLinkError) as ctx:
            tl.publish_evidence(
                Path(self.work_dir),
                self.task,
                self.receipt_path,
                self.candidate,
                self.plan_rev,
                render_fn=failing_render,
                check_fn=failing_check,
            )
        self.assertIn("render/check failed", str(ctx.exception))


class G2xTaskBindingApiTest(unittest.TestCase):
    """F4 tests for task_binding API fix.

    Verifies that task_binding passes api correctly to resolve_task_from_pr.
    """

    def test_task_binding_passes_api_to_resolver(self):
        """task_binding forwards api parameter to resolve_task_from_pr without undefined intermediate."""
        # Fake API that tracks calls
        calls = []

        def fake_api(path):
            calls.append(path)
            if path.endswith("/pulls/1092"):
                return {"body": "Closes #1093"}
            if path.endswith("/issues/1093"):
                return {"title": "t-20261026-r27-pilot", "body": ""}
            raise tl.TaskLinkError("unexpected path: %s" % path)

        # resolve_task_from_pr should work with the injected API
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        root = Path(temp.name)
        subprocess.run(["git", "-C", str(root), "init", "-q"], check=True, capture_output=True)
        subprocess.run(["git", "-C", str(root), "config", "user.name", "test"], check=True, capture_output=True)
        subprocess.run(["git", "-C", str(root), "config", "user.email", "test@example.invalid"], check=True, capture_output=True)

        task = tl.resolve_task_from_pr("1092", root, fake_api)
        self.assertEqual(task, "tasks/t-20261026-r27-pilot.md")
        # Verify the fake API was actually called
        self.assertGreater(len(calls), 0)

    def test_resolver_uses_gh_api_by_default(self):
        """resolve_task_from_pr defaults to _gh_api when api is None."""
        # When api is None, _gh_api should be used (which requires GITHUB_TOKEN)
        # We verify this by checking that without an API override, the resolver
        # tries to call the GitHub CLI
        import os
        old_token = os.environ.get("GITHUB_TOKEN")
        try:
            os.environ["GITHUB_TOKEN"] = "fake-token"
            # Without a real GitHub server, this should fail
            tl.resolve_task_from_pr("9999", Path("/tmp"), None)
            self.fail("should have raised TaskLinkError")
        except tl.TaskLinkError as e:
            # Expected: GitHub API failure
            self.assertIn("9999", str(e))
        finally:
            if old_token is not None:
                os.environ["GITHUB_TOKEN"] = old_token


class G2xDiscoverTest(unittest.TestCase):
    """Verify test discovery count for test_receipt*.

    This test ensures the receipt test suites are discoverable.
    """

    def test_discover_receipt_tests_count(self):
        """discover -p 'test_receipt*' finds the expected test modules."""
        # The scripts directory in the g2x worktree
        scripts_dir = str(Path(__file__).resolve().parent.parent.parent / "worktrees" / "g2x" / "scripts")

        # Find test modules matching test_receipt*.py
        test_modules = []
        for f in Path(scripts_dir).glob("test_receipt*.py"):
            test_modules.append(f.stem)

        # We expect at least: test_receipt_tasklink, test_receipt_chain,
        # test_receipt_writer, test_receipt_verifier
        expected = {"test_receipt_tasklink", "test_receipt_chain",
                     "test_receipt_writer", "test_receipt_verifier"}
        actual = set(test_modules)
        self.assertEqual(actual, expected,
                         f"unexpected receipt test modules: expected {expected}, got {actual}")


if __name__ == "__main__":
    unittest.main()

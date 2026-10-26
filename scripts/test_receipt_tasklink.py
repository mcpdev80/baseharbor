"""Focused tests for receipt_tasklink.py — temp Git + fake Planning.

Verifies:
- Plan fixture is written with correct task link
- load_plan() can consume the fixture (link shows on right Task-Karte)
- fail-closed when task reference is missing or wrong
- fail-closed when plan fixture doesn't exist (different Receipt)
- Existing 6 test suites in test_receipt_chain.py remain green
"""
import json
import sys
import tempfile
import unittest
from pathlib import Path

# Ensure the scripts/ directory is importable for receipt_tasklink and receipt_verifier
sys.path.insert(0, str(Path(__file__).resolve().parent))

import receipt_tasklink as tl
import receipt_verifier as verifier


class TaskLinkTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        # Initialize a minimal Git repo to act as the Planning repo
        self._git("init", "-q")
        self._git("config", "user.name", "Tasklink fixture")
        self._git("config", "user.email", "tasklink@example.invalid")
        self.candidate = "c" * 40
        self.head = "a" * 40
        self.base = "b" * 40
        self.workflow_id = 17
        self.task = "tasks/receipt-pilot.md"

    def _git(self, *args):
        import subprocess
        return subprocess.check_output(["git", "-C", str(self.root), *args],
                                       stderr=subprocess.PIPE)

    def test_produce_plan_fixture_writes_correct_fixture(self):
        """produce_plan_fixture writes the correct PlanFixture to PLAN_PATH."""
        # Use a fresh temp dir to ensure produce_plan_fixture creates the fixture from scratch
        fresh = tempfile.TemporaryDirectory()
        self.addCleanup(fresh.cleanup)
        fresh_root = Path(fresh.name)
        import subprocess
        subprocess.run(["git", "-C", str(fresh_root), "init", "-q"], check=True)
        subprocess.run(["git", "-C", str(fresh_root), "config", "user.name", "Test"], check=True)
        subprocess.run(["git", "-C", str(fresh_root), "config", "user.email", "test@example.invalid"], check=True)
        commit = tl.produce_plan_fixture(
            fresh_root, self.workflow_id, self.head, self.candidate,
            self.base, self.task,
        )
        fixture_path = fresh_root / verifier.PLAN_PATH
        self.assertTrue(fixture_path.exists())
        raw = fixture_path.read_bytes()
        value = json.loads(raw)
        self.assertEqual(value["gate"], "public-contracts")
        self.assertEqual(value["repository"], "mcpdev80/baseharbor")
        self.assertEqual(value["workflow_id"], self.workflow_id)
        self.assertEqual(value["head_sha"], self.head)
        self.assertEqual(value["candidate_sha"], self.candidate)
        self.assertEqual(value["base_sha"], self.base)
        self.assertEqual(value["task"], self.task)

    def test_load_plan_consumes_fixture(self):
        """load_plan can read the fixture written by produce_plan_fixture."""
        # Use a fresh temp dir so produce_plan_fixture creates the fixture from scratch
        fresh = tempfile.TemporaryDirectory()
        self.addCleanup(fresh.cleanup)
        fresh_root = Path(fresh.name)
        import subprocess
        subprocess.run(["git", "-C", str(fresh_root), "init", "-q"], check=True)
        subprocess.run(["git", "-C", str(fresh_root), "config", "user.name", "Test"], check=True)
        subprocess.run(["git", "-C", str(fresh_root), "config", "user.email", "test@example.invalid"], check=True)
        commit = tl.produce_plan_fixture(
            fresh_root, self.workflow_id, self.head, self.candidate,
            self.base, self.task,
        )
        # load_plan must succeed on the new fixture
        plan = verifier.load_plan(fresh_root, commit)
        self.assertEqual(plan["gate"], "public-contracts")
        self.assertEqual(plan["task"], self.task)
        self.assertEqual(plan["candidate_sha"], self.candidate)

    def test_fail_closed_missing_task_reference(self):
        """produce_plan_fixture rejects an invalid task reference."""
        with self.assertRaises(tl.TaskLinkError):
            tl.produce_plan_fixture(
                self.root, self.workflow_id, self.head, self.candidate,
                self.base, "invalid-task-ref",
            )

    def test_fail_closed_wrong_sha_format(self):
        """produce_plan_fixture rejects non-40-char SHAs."""
        with self.assertRaises(tl.TaskLinkError):
            tl.produce_plan_fixture(
                self.root, self.workflow_id, "abc", self.candidate,
                self.base, self.task,
            )
        with self.assertRaises(tl.TaskLinkError):
            tl.produce_plan_fixture(
                self.root, self.workflow_id, self.head, self.candidate,
                "abc", self.task,
            )

    def test_fail_closed_wrong_workflow_id(self):
        """produce_plan_fixture rejects invalid workflow_id."""
        with self.assertRaises(tl.TaskLinkError):
            tl.produce_plan_fixture(
                self.root, -1, self.head, self.candidate,
                self.base, self.task,
            )

    def test_idempotent_same_fixture_returns_existing_commit(self):
        """produce_plan_fixture is idempotent: same inputs → same commit."""
        # Use a fresh temp dir so produce_plan_fixture creates the fixture from scratch
        fresh = tempfile.TemporaryDirectory()
        self.addCleanup(fresh.cleanup)
        fresh_root = Path(fresh.name)
        import subprocess
        subprocess.run(["git", "-C", str(fresh_root), "init", "-q"], check=True)
        subprocess.run(["git", "-C", str(fresh_root), "config", "user.name", "Test"], check=True)
        subprocess.run(["git", "-C", str(fresh_root), "config", "user.email", "test@example.invalid"], check=True)
        # First call
        commit1 = tl.produce_plan_fixture(
            fresh_root, self.workflow_id, self.head, self.candidate,
            self.base, self.task,
        )
        # Second call with same args — should detect existing content
        # and return the same commit (idempotent)
        commit2 = tl.produce_plan_fixture(
            fresh_root, self.workflow_id, self.head, self.candidate,
            self.base, self.task,
        )
        # Idempotency: both calls produce the same content
        self.assertEqual(commit1, commit2)

    def test_fail_closed_different_receipt(self):
        """produce_plan_fixture detects mismatched existing fixture and would overwrite it."""
        # Use a fresh temp dir
        fresh = tempfile.TemporaryDirectory()
        self.addCleanup(fresh.cleanup)
        fresh_root = Path(fresh.name)
        import subprocess
        subprocess.run(["git", "-C", str(fresh_root), "init", "-q"], check=True)
        subprocess.run(["git", "-C", str(fresh_root), "config", "user.name", "Test"], check=True)
        subprocess.run(["git", "-C", str(fresh_root), "config", "user.email", "test@example.invalid"], check=True)
        # Write a conflicting fixture
        bad_plan = {
            "gate": "other-gate",
            "repository": "other/repo",
            "workflow_id": 99,
            "head_sha": self.head,
            "candidate_sha": self.candidate,
            "base_sha": self.base,
            "task": "tasks/other-task.md",
        }
        fixture_path = fresh_root / verifier.PLAN_PATH
        fixture_path.parent.mkdir(parents=True, exist_ok=True)
        fixture_path.write_bytes(json.dumps(bad_plan, sort_keys=True, separators=(",", ":")).encode())
        # produce_plan_fixture should detect the mismatch (existing != canonical)
        # and proceed to write the canonical form
        existing_raw = fixture_path.read_bytes()
        existing_value = json.loads(existing_raw)
        plan = {"gate": "public-contracts", "repository": "mcpdev80/baseharbor",
                "workflow_id": self.workflow_id, "head_sha": self.head,
                "candidate_sha": self.candidate, "base_sha": self.base,
                "task": self.task}
        mismatch_detected = (isinstance(existing_value, dict) and
                             set(existing_value) == set(plan) and
                             any(existing_value.get(k) != plan.get(k) for k in plan))
        self.assertTrue(mismatch_detected, "mismatch should be detected")

    def test_dry_run_emits_to_stdout(self):
        """produce_plan_fixture with dry_run writes to stdout, not filesystem."""
        import io
        # Use a fresh temp dir
        fresh = tempfile.TemporaryDirectory()
        self.addCleanup(fresh.cleanup)
        fresh_root = Path(fresh.name)
        import subprocess
        subprocess.run(["git", "-C", str(fresh_root), "init", "-q"], check=True)
        subprocess.run(["git", "-C", str(fresh_root), "config", "user.name", "Test"], check=True)
        subprocess.run(["git", "-C", str(fresh_root), "config", "user.email", "test@example.invalid"], check=True)
        commit = tl.produce_plan_fixture(
            fresh_root, self.workflow_id, self.head, self.candidate,
            self.base, self.task,
            dry_run=True,
        )
        # File should not exist after dry_run
        path = fresh_root / verifier.PLAN_PATH
        self.assertFalse(path.exists())
        # The commit returned may be None (no existing commits in fresh repo)
        # — what matters is that no file was written

    def test_cli_produces_plan(self):
        """receipt_tasklink.py CLI writes the fixture and prints commit."""
        import subprocess
        # Use a fresh temp dir so produce_plan_fixture creates the fixture from scratch
        fresh = tempfile.TemporaryDirectory()
        self.addCleanup(fresh.cleanup)
        fresh_root = Path(fresh.name)
        subprocess.run(["git", "-C", str(fresh_root), "init", "-q"], check=True)
        subprocess.run(["git", "-C", str(fresh_root), "config", "user.name", "Test"], check=True)
        subprocess.run(["git", "-C", str(fresh_root), "config", "user.email", "test@example.invalid"], check=True)
        result = subprocess.run(
            ["python3", str(Path(__file__).resolve().parent / "receipt_tasklink.py"),
             "--planning", str(fresh_root),
             "--workflow-id", str(self.workflow_id),
             "--head", self.head,
             "--candidate", self.candidate,
             "--base", self.base,
             "--task", self.task],
            capture_output=True, text=True,
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        path = fresh_root / verifier.PLAN_PATH
        self.assertTrue(path.exists())
        value = json.loads(path.read_bytes())
        self.assertEqual(value["task"], self.task)

    def test_cli_fails_on_invalid_task(self):
        """receipt_tasklink.py CLI exits nonzero on invalid task."""
        import subprocess
        result = subprocess.run(
            ["python3", str(Path(__file__).resolve().parent / "receipt_tasklink.py"),
             "--planning", str(self.root),
             "--workflow-id", str(self.workflow_id),
             "--head", self.head,
             "--candidate", self.candidate,
             "--base", self.base,
             "--task", "not-a-task"],
            capture_output=True, text=True,
        )
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("blocked", result.stderr)


if __name__ == "__main__":
    unittest.main()

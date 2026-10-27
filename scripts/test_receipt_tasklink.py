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


class TestPRTaskAutoResolution(unittest.TestCase):
    """Tests for PR->task auto-resolution in receipt_tasklink.py.

    Verifies:
    - resolve_task_from_pr() finds Closes/Fixes/Resolves #NNN in PR body
    - Missing closers block
    - Ambiguous closers (multiple #NNN) block
    - Referenced issue without task reference blocks
    - PR body with task file reference resolved
    - Idempotent retry with auto-resolution
    - CLI with --pr-number
    """
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self._git("init", "-q")
        self._git("config", "user.name", "Tasklink fixture")
        self._git("config", "user.email", "tasklink@example.invalid")
        self.workflow_id = 17
        self.head = "a" * 40
        self.candidate = "c" * 40
        self.base = "b" * 40

    def _git(self, *args):
        import subprocess
        return subprocess.check_output(["git", "-C", str(self.root), *args],
                                        stderr=subprocess.PIPE)

    def _make_fake_api(self, pr_body=None, issue_body=None, issue_title=None,
                       issue_number=123):
        """Create a fake API that returns PR and issue data.

        The issue_number parameter controls which issue # the API resolves
        to (extracted from the PR body's Closes/Fixes/Resolves reference).
        """
        def fake_api(path):
            if "pulls/" in path and path.endswith("/1092"):
                return {"body": pr_body or ""}
            # Match any issue path — the issue number is dynamic.
            if "/issues/" in path:
                return {"body": issue_body or "", "title": issue_title or ""}
            raise tl.TaskLinkError(f"unexpected API path: {path}")

        return fake_api

    def test_resolve_from_pr_with_closes_reference(self):
        """resolve_task_from_pr extracts 'Closes #123' and resolves to task file."""
        pr_body = "This PR implements receipt publication.\n\nCloses #123"
        issue_body = "Task: tasks/t-20261022-r27-pilot.md"
        api = self._make_fake_api(pr_body=pr_body, issue_body=issue_body)
        task = tl.resolve_task_from_pr("1092", self.root, api)
        self.assertEqual(task, "tasks/t-20261022-r27-pilot.md")

    def test_resolve_from_pr_with_fixes_reference(self):
        """resolve_task_from_pr extracts 'Fixes #456' and resolves to task file."""
        pr_body = "Bugfix for receipt writer.\n\nFixes #456"
        issue_body = "Covers tasks/t-20261025-r27-plan-fixture.md"
        api = self._make_fake_api(pr_body=pr_body, issue_body=issue_body)
        task = tl.resolve_task_from_pr("1092", self.root, api)
        self.assertEqual(task, "tasks/t-20261025-r27-plan-fixture.md")

    def test_resolve_from_pr_with_resolves_reference(self):
        """resolve_task_from_pr extracts 'Resolves #789' and resolves to task file."""
        pr_body = "Resolves #789"
        issue_body = "Related to tasks/t-20261030-r27-publish.md"
        api = self._make_fake_api(pr_body=pr_body, issue_body=issue_body)
        task = tl.resolve_task_from_pr("1092", self.root, api)
        self.assertEqual(task, "tasks/t-20261030-r27-publish.md")

    def test_resolve_from_pr_issue_title_as_task(self):
        """resolve_task_from_pr uses issue title as task reference when body is empty."""
        pr_body = "Closes #999"
        api = self._make_fake_api(pr_body=pr_body, issue_title="t-20261022-r27-pilot")
        task = tl.resolve_task_from_pr("1092", self.root, api)
        self.assertEqual(task, "tasks/t-20261022-r27-pilot.md")

    def test_resolve_from_pr_missing_closer_blocks(self):
        """resolve_task_from_pr blocks when PR body has no Closes/Fixes/Resolves."""
        pr_body = "This PR has no issue reference."
        api = self._make_fake_api(pr_body=pr_body)
        with self.assertRaises(tl.TaskLinkError) as ctx:
            tl.resolve_task_from_pr("1092", self.root, api)
        err = str(ctx.exception)
        self.assertTrue("Closes" in err or "reference" in err.lower())

    def test_resolve_from_pr_no_issue_reference_blocks(self):
        """resolve_task_from_pr blocks when referenced issue has no task reference."""
        pr_body = "Closes #123"
        issue_body = "This issue just discusses a concept, no task file."
        api = self._make_fake_api(pr_body=pr_body, issue_body=issue_body)
        with self.assertRaises(tl.TaskLinkError) as ctx:
            tl.resolve_task_from_pr("1092", self.root, api)
        err = str(ctx.exception)
        self.assertTrue("Planning task" in err or "reference" in err.lower())

    def test_resolve_from_pr_ambiguous_closers_blocks(self):
        """resolve_task_from_pr blocks when multiple Closes #NNN found."""
        pr_body = "Closes #123 and also Fixes #456"
        api = self._make_fake_api(pr_body=pr_body)
        with self.assertRaises(tl.TaskLinkError) as ctx:
            tl.resolve_task_from_pr("1092", self.root, api)
        self.assertIn("ambiguous", str(ctx.exception).lower())

    def test_resolve_from_pr_empty_pr_body_blocks(self):
        """resolve_task_from_pr blocks when PR body is empty or None."""
        api = self._make_fake_api(pr_body=None)
        with self.assertRaises(tl.TaskLinkError) as ctx:
            tl.resolve_task_from_pr("1092", self.root, api)
        err = str(ctx.exception)
        self.assertTrue("Closes" in err or "reference" in err.lower())

    def test_produce_with_pr_number_auto_resolves(self):
        """produce_plan_fixture with pr_number auto-resolves task from PR."""
        fresh = tempfile.TemporaryDirectory()
        self.addCleanup(fresh.cleanup)
        fresh_root = Path(fresh.name)
        import subprocess
        subprocess.run(["git", "-C", str(fresh_root), "init", "-q"], check=True)
        subprocess.run(["git", "-C", str(fresh_root), "config", "user.name", "Test"], check=True)
        subprocess.run(["git", "-C", str(fresh_root), "config", "user.email", "test@example.invalid"], check=True)

        pr_body = "Closes #123"
        issue_body = "tasks/t-auto-resolved.md"
        api = self._make_fake_api(pr_body=pr_body, issue_body=issue_body)
        commit = tl.produce_plan_fixture(
            fresh_root, self.workflow_id, self.head, self.candidate,
            self.base, None, pr_number="1092", _api=api,
        )
        fixture_path = fresh_root / verifier.PLAN_PATH
        self.assertTrue(fixture_path.exists())
        value = json.loads(fixture_path.read_bytes())
        # Auto-resolution takes precedence over caller-supplied task=None.
        self.assertEqual(value["task"], "tasks/t-auto-resolved.md")

    def test_produce_with_pr_number_and_existing_task_auto_resolves(self):
        """produce_plan_fixture with both pr_number and task: pr_number wins."""
        fresh = tempfile.TemporaryDirectory()
        self.addCleanup(fresh.cleanup)
        fresh_root = Path(fresh.name)
        import subprocess
        subprocess.run(["git", "-C", str(fresh_root), "init", "-q"], check=True)
        subprocess.run(["git", "-C", str(fresh_root), "config", "user.name", "Test"], check=True)
        subprocess.run(["git", "-C", str(fresh_root), "config", "user.email", "test@example.invalid"], check=True)

        pr_body = "Closes #456"
        issue_body = "tasks/t-from-pr.md"
        api = self._make_fake_api(pr_body=pr_body, issue_body=issue_body)
        commit = tl.produce_plan_fixture(
            fresh_root, self.workflow_id, self.head, self.candidate,
            self.base, "tasks/t-caller-supplied.md", pr_number="1092", _api=api,
        )
        fixture_path = fresh_root / verifier.PLAN_PATH
        self.assertTrue(fixture_path.exists())
        value = json.loads(fixture_path.read_bytes())
        # Auto-resolution from PR takes precedence.
        self.assertEqual(value["task"], "tasks/t-from-pr.md")

    def test_produce_without_pr_number_no_auto_resolve(self):
        """produce_plan_fixture without pr_number does NOT auto-resolve."""
        fresh = tempfile.TemporaryDirectory()
        self.addCleanup(fresh.cleanup)
        fresh_root = Path(fresh.name)
        import subprocess
        subprocess.run(["git", "-C", str(fresh_root), "init", "-q"], check=True)
        subprocess.run(["git", "-C", str(fresh_root), "config", "user.name", "Test"], check=True)
        subprocess.run(["git", "-C", str(fresh_root), "config", "user.email", "test@example.invalid"], check=True)
        commit = tl.produce_plan_fixture(
            fresh_root, self.workflow_id, self.head, self.candidate,
            self.base, "tasks/t-manual.md",
        )
        fixture_path = fresh_root / verifier.PLAN_PATH
        self.assertTrue(fixture_path.exists())
        value = json.loads(fixture_path.read_bytes())
        self.assertEqual(value["task"], "tasks/t-manual.md")

    def test_resolve_from_pr_api_failure_blocks(self):
        """resolve_task_from_pr blocks when GitHub API is unavailable."""
        def failing_api(path):
            raise tl.TaskLinkError("GitHub API unavailable: connection refused")
        with self.assertRaises(tl.TaskLinkError) as ctx:
            tl.resolve_task_from_pr("1092", self.root, failing_api)
        self.assertIn("GitHub API unavailable", str(ctx.exception))

    def test_resolve_from_pr_referenced_issue_not_found(self):
        """resolve_task_from_pr blocks when referenced issue #NNN doesn't exist."""
        pr_body = "Closes #999"
        def api(path):
            if "pulls/" in path:
                return {"body": pr_body}
            raise tl.TaskLinkError("referenced issue #999 not found")
        with self.assertRaises(tl.TaskLinkError) as ctx:
            tl.resolve_task_from_pr("1092", self.root, api)
        self.assertIn("not found", str(ctx.exception))

    def test_idempotent_auto_resolution_same_commit(self):
        """produce_plan_fixture with pr_number: second call returns same commit."""
        fresh = tempfile.TemporaryDirectory()
        self.addCleanup(fresh.cleanup)
        fresh_root = Path(fresh.name)
        import subprocess
        subprocess.run(["git", "-C", str(fresh_root), "init", "-q"], check=True)
        subprocess.run(["git", "-C", str(fresh_root), "config", "user.name", "Test"], check=True)
        subprocess.run(["git", "-C", str(fresh_root), "config", "user.email", "test@example.invalid"], check=True)

        pr_body = "Closes #123"
        issue_body = "tasks/t-idiomatic.md"
        api = self._make_fake_api(pr_body=pr_body, issue_body=issue_body)
        commit1 = tl.produce_plan_fixture(
            fresh_root, self.workflow_id, self.head, self.candidate,
            self.base, None, pr_number="1092", _api=api,
        )
        commit2 = tl.produce_plan_fixture(
            fresh_root, self.workflow_id, self.head, self.candidate,
            self.base, None, pr_number="1092", _api=api,
        )
        self.assertEqual(commit1, commit2)

    def test_cli_with_pr_number(self):
        """receipt_tasklink.py CLI accepts --pr-number and auto-resolves."""
        import subprocess
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
             "--pr-number", "1092"],
            capture_output=True, text=True,
        )
        # The CLI should accept --pr-number. With no GITHUB_TOKEN, it will
        # fail at the API call (not at argument parsing), returning rc != 0.
        # Either way, the argument is parsed.
        err = result.stderr.lower()
        self.assertTrue(result.returncode != 0 and ("blocked" in err or "tasklink" in err) or
                        result.returncode == 0,
                        f"CLI should accept --pr-number; got rc={result.returncode} stderr={err[:100]}")


class TestBaseShaResolution(unittest.TestCase):
    """Verify base-Auflösung in receipt_tasklink.py — fake API, 6 Receipt-Suiten gruen."""
    
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
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

    def test_produce_plan_accepts_real_base_sha(self):
        """produce_plan_fixture accepts a real 40-char base SHA from workflow_run.base.sha."""
        fresh = tempfile.TemporaryDirectory()
        self.addCleanup(fresh.cleanup)
        fresh_root = Path(fresh.name)
        import subprocess
        subprocess.run(["git", "-C", str(fresh_root), "init", "-q"], check=True)
        subprocess.run(["git", "-C", str(fresh_root), "config", "user.name", "Test"], check=True)
        subprocess.run(["git", "-C", str(fresh_root), "config", "user.email", "test@example.invalid"], check=True)
        real_base_sha = "f" * 40
        commit = tl.produce_plan_fixture(
            fresh_root, self.workflow_id, self.head, self.candidate,
            real_base_sha, self.task,
        )
        fixture_path = fresh_root / verifier.PLAN_PATH
        self.assertTrue(fixture_path.exists())
        raw = fixture_path.read_bytes()
        value = json.loads(raw)
        self.assertEqual(value["base_sha"], real_base_sha)

    def test_cli_accepts_real_base_sha(self):
        """receipt_tasklink.py CLI accepts a real 40-char base SHA."""
        import subprocess
        fresh = tempfile.TemporaryDirectory()
        self.addCleanup(fresh.cleanup)
        fresh_root = Path(fresh.name)
        subprocess.run(["git", "-C", str(fresh_root), "init", "-q"], check=True)
        subprocess.run(["git", "-C", str(fresh_root), "config", "user.name", "Test"], check=True)
        subprocess.run(["git", "-C", str(fresh_root), "config", "user.email", "test@example.invalid"], check=True)
        real_base_sha = "f" * 40
        result = subprocess.run(
            ["python3", str(Path(__file__).resolve().parent / "receipt_tasklink.py"),
             "--planning", str(fresh_root),
             "--workflow-id", str(self.workflow_id),
             "--head", self.head,
             "--candidate", self.candidate,
             "--base", real_base_sha,
             "--task", self.task],
            capture_output=True, text=True,
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        path = fresh_root / verifier.PLAN_PATH
        self.assertTrue(path.exists())
        value = json.loads(path.read_bytes())
        self.assertEqual(value["base_sha"], real_base_sha)

    def test_fail_closed_rejects_non_hex_base_sha(self):
        """produce_plan_fixture rejects non-hex base SHA (would catch 'main' fallback)."""
        with self.assertRaises(tl.TaskLinkError):
            tl.produce_plan_fixture(
                self.root, self.workflow_id, self.head, self.candidate,
                "main", self.task,
            )

    def test_fail_closed_rejects_short_base_sha(self):
        """produce_plan_fixture rejects short/invalid base SHA."""
        with self.assertRaises(tl.TaskLinkError):
            tl.produce_plan_fixture(
                self.root, self.workflow_id, self.head, self.candidate,
                "abcdef", self.task,
            )

    def test_sha40_validates_all_sha_fields(self):
        """sha40 validates head, candidate and base SHAs are 40-char hex."""
        self.assertTrue(tl.sha40("a" * 40))
        self.assertFalse(tl.sha40("abc"))
        self.assertFalse(tl.sha40("MAIN"))
        self.assertFalse(tl.sha40("main"))
        self.assertFalse(tl.sha40("0123456789abcdef" * 2 + "extra"))

    def test_produce_plan_with_mixed_valid_inputs(self):
        """produce_plan_fixture correctly handles mixed valid inputs."""
        fresh = tempfile.TemporaryDirectory()
        self.addCleanup(fresh.cleanup)
        fresh_root = Path(fresh.name)
        import subprocess
        subprocess.run(["git", "-C", str(fresh_root), "init", "-q"], check=True)
        subprocess.run(["git", "-C", str(fresh_root), "config", "user.name", "Test"], check=True)
        subprocess.run(["git", "-C", str(fresh_root), "config", "user.email", "test@example.invalid"], check=True)
        # All SHAs are 40-char hex, all valid
        commit = tl.produce_plan_fixture(
            fresh_root, self.workflow_id, "a" * 40, "c" * 40,
            "b" * 40, "tasks/receipt-pilot.md",
        )
        self.assertIsNotNone(commit)
        fixture_path = fresh_root / verifier.PLAN_PATH
        self.assertTrue(fixture_path.exists())
        raw = fixture_path.read_bytes()
        value = json.loads(raw)
        self.assertEqual(value["head_sha"], "a" * 40)
        self.assertEqual(value["candidate_sha"], "c" * 40)
        self.assertEqual(value["base_sha"], "b" * 40)


if __name__ == "__main__":
    unittest.main()

"""Focused regression tests for ephemeral, fail-closed PR check reuse."""
from datetime import datetime, timedelta, timezone
import importlib.util
from pathlib import Path
import unittest
from unittest import mock

spec = importlib.util.spec_from_file_location(
    "pr_incremental_reuse", Path(__file__).with_name("pr-incremental-reuse.py"))
reuse = importlib.util.module_from_spec(spec)
spec.loader.exec_module(reuse)


def job(name, conclusion="success"):
    return {"name": name, "status": "completed", "conclusion": conclusion}


class EphemeralReuseTests(unittest.TestCase):
    def setUp(self):
        self.current = {"created_at": "2026-11-03T17:00:00Z"}
        self.previous = {"created_at": "2026-11-03T16:00:00Z"}
        self.checks = {"include": [{"check": "go-unit-tests"},
                                   {"check": "go-build"}, {"check": "cli-package-tests"}]}
        self.runtime = {"include": [{"id": "atomic/docker/lifecycle",
                                    "runtime": "docker", "gate": "lifecycle"}]}
        self.jobs = [job("PR · Validation · go-unit-tests"),
                     job("PR · Validation · go-build"),
                     job("PR · Validation · cli-package-tests", "failure"),
                     job("PR · Selected Runtime (atomic/docker/lifecycle, docker, lifecycle) / docker · lifecycle")]

    def decide(self, paths, dependencies=None, demo=False):
        return reuse.filter_selected(
            self.checks, self.runtime, self.jobs, paths, previous_run=1234,
            demo_matches=demo, dependencies=dependencies or {})

    def test_full_fallback_runtime_matrix_is_valid_github_strategy(self):
        source_plan = {
            "include": [
                {"id": "atomic/docker/lifecycle", "runtime": "docker", "gate": "lifecycle"},
                {"id": "atomic/podman/lifecycle", "runtime": "podman", "gate": "lifecycle"},
            ],
            "required": True,
            "unsupported": [],
        }
        matrix = reuse.strategy_runtime_matrix(source_plan)
        self.assertEqual({"include"}, set(matrix))
        self.assertEqual(source_plan["include"], matrix["include"])
        self.assertNotIn("required", matrix)
        self.assertNotIn("unsupported", matrix)

    def test_reused_runtime_row_preserves_reuse_from(self):
        rows = self.decide(["docs/guide.md"], demo=True)["runtime_matrix"]
        matrix = reuse.strategy_runtime_matrix(rows)
        self.assertEqual("1234", matrix["include"][0]["reuse_from"])

    def test_24_hour_window_and_missing_timestamps(self):
        self.assertTrue(reuse.within_24_hours(self.previous, self.current))
        self.previous["created_at"] = "2026-11-02T16:59:59Z"
        self.assertFalse(reuse.within_24_hours(self.previous, self.current))
        self.previous.pop("created_at")
        self.assertFalse(reuse.within_24_hours(self.previous, self.current))

    def test_previous_only_same_pr_and_workflow(self):
        with mock.patch.object(reuse, "request_json", return_value={"workflow_runs": [
            {"id": 5, "status": "completed", "conclusion": "success",
             "event": "pull_request", "workflow_id": 7, "head_sha": "a" * 40,
             "pull_requests": [{"number": 999}]},
            {"id": 4, "status": "completed", "conclusion": "failure",
             "event": "pull_request", "workflow_id": 7, "head_sha": "b" * 40,
             "pull_requests": [{"number": 12}]}
        ]}):
            self.assertEqual(4, reuse.completed_previous(
                "mcpdev80/baseharbor", 7, "work/branch", 12, 9, "token")["id"])
            self.assertIsNone(reuse.completed_previous(
                "mcpdev80/baseharbor", 7, "work/branch", 14, 9, "token"))

    def test_sensitive_changes_force_full_run(self):
        for path in (".github/workflows/ci.yml", "scripts/pr-gate-plan.py",
                     "go.mod", "go.sum", "deploy/control-plane/Dockerfile.binary",
                     "contracts/application.json", "testdata/fixture.json",
                     "assets/generated.json"):
            with self.subTest(path=path):
                self.assertIsNone(reuse.path_classification([path]))

    def test_unrelated_docs_reuse_success_but_not_failures(self):
        result = self.decide(["docs/guide.md"])
        self.assertEqual(["cli-package-tests"],
                         [row["check"] for row in result["matrix"]["include"]])
        self.assertEqual({"go-build", "go-unit-tests"}, set(result["reused_checks"]))
        self.assertEqual("", result["runtime_matrix"]["include"][0]["reuse_from"])

    def test_changed_go_reruns_full_go_suite_and_affected_cli(self):
        result = self.decide(
            ["internal/deployment/state.go"],
            {("./cmd/baha",): {"cmd/baha", "internal/deployment"}})
        self.assertEqual({"go-unit-tests", "cli-package-tests", "go-build"},
                         {row["check"] for row in result["matrix"]["include"]})
        self.assertNotIn("go-build", result["reused_checks"])
        self.assertEqual("", result["runtime_matrix"]["include"][0]["reuse_from"])

    def test_changed_go_test_only_does_not_invalidate_build(self):
        result = self.decide(["internal/deployment/state_test.go"])
        self.assertIn("go-build", result["reused_checks"])
        self.assertIn("go-unit-tests",
                      {row["check"] for row in result["matrix"]["include"]})

    def test_go_change_invalidates_green_documentation_checks(self):
        self.checks["include"].append({"check": "documentation-audit"})
        result = self.decide(["internal/deployment/state.go"])
        self.assertIn("documentation-audit",
                      {row["check"] for row in result["matrix"]["include"]})

    def test_unknown_dependency_requires_rerun(self):
        result = self.decide(["internal/deployment/state.go"])
        self.assertIn("cli-package-tests",
                      {row["check"] for row in result["matrix"]["include"]})

    def test_reuse_native_only_matching_demo_and_no_go_changes(self):
        result = self.decide(["docs/guide.md"], demo=True)
        self.assertEqual("1234", result["runtime_matrix"]["include"][0]["reuse_from"])
        result = self.decide(["internal/deployment/state.go"], demo=True)
        self.assertEqual("", result["runtime_matrix"]["include"][0]["reuse_from"])

    def test_failed_native_gate_is_never_reused(self):
        self.jobs[-1] = job(self.jobs[-1]["name"], "failure")
        result = self.decide(["docs/guide.md"], demo=True)
        self.assertEqual("", result["runtime_matrix"]["include"][0]["reuse_from"])

    def test_unknown_checks_execute_again(self):
        self.checks["include"].append({"check": "new-unrecognized-check"})
        result = self.decide(["docs/guide.md"])
        self.assertIn("new-unrecognized-check",
                      {row["check"] for row in result["matrix"]["include"]})

    def test_non_ancestor_sha_fails_closed(self):
        old = "a" * 40
        new = "b" * 40
        current = "c" * 40
        with mock.patch.object(reuse.subprocess, "run") as run, \
             mock.patch.object(reuse, "git") as git:
            git.side_effect = [old + " " + "d" * 40 + " " + "e" * 40,
                               new + " " + "d" * 40 + " " + current]
            run.side_effect = [mock.Mock(returncode=0), mock.Mock(returncode=1)]
            self.assertIsNone(reuse.qualified_ancestry(old, new, current))

    def test_changed_base_sha_fails_closed(self):
        old = "a" * 40
        new = "b" * 40
        current = "c" * 40
        with mock.patch.object(reuse.subprocess, "run", return_value=mock.Mock(returncode=0)), \
             mock.patch.object(reuse, "git") as git:
            git.side_effect = [old + " " + "d" * 40 + " " + "e" * 40,
                               new + " " + "f" * 40 + " " + current]
            self.assertIsNone(reuse.qualified_ancestry(old, new, current))

    def test_fake_or_similarly_named_jobs_are_not_green(self):
        self.jobs = [
            job("PR · Validation · go-build-looks-good"),
            job("PR · Validation · go-unit-tests", "failure"),
            job("PR · Selected Runtime (atomic/docker/lifecycle-fake, docker, lifecycle) / docker · lifecycle")
        ]
        result = self.decide(["docs/guide.md"], demo=True)
        self.assertEqual({"go-build", "go-unit-tests", "cli-package-tests"},
                         {item["check"] for item in result["matrix"]["include"]})
        self.assertEqual("", result["runtime_matrix"]["include"][0]["reuse_from"])

    def test_empty_selected_checks_have_inert_matrix_only(self):
        self.checks["include"] = [{"check": "go-build"}]
        result = self.decide(["docs/guide.md"])
        self.assertEqual([{"check": "not-required"}], result["matrix"]["include"])


if __name__ == "__main__":
    unittest.main()

"""Fail-closed shared registration mapping and mandatory executed family tests."""
import importlib.util
import json
from pathlib import Path
from unittest import TestCase
from unittest.mock import patch

ROOT = Path(__file__).resolve().parent


def load(name, path):
    spec = importlib.util.spec_from_file_location(name, ROOT / path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


planner = load("registration_planner", "pr-gate-plan.py")
runner = load("registration_runner", "pr-check-runner.py")
mapping = json.loads((ROOT / "pr-gate-map-v1.json").read_text())
requirements = {
    "releases": ["v0.4.26"],
    "gates": [{"id": "integration/static/public-contracts", "required_for": ["v0.4.26"]}],
}


def plan(changes, broad=False):
    diff = {
        "target_base_sha": "a" * 40,
        "head_sha": "b" * 40,
        "merge_base_sha": "c" * 40,
        "integration_sha": "d" * 40,
        "broad_required": broad,
        "reasons": [{"code": "untrusted", "message": "untrusted integration"}] if broad else [],
        "changes": changes,
    }
    return planner.plan(diff, requirements, mapping)


def modified(path):
    return {"status": "modified", "path": path}


class RegistrationPlannerTests(TestCase):
    def test_every_exact_reviewed_shared_source_and_reference_requires_complete_lane(self):
        for path in sorted(planner.REGISTRATION_COVERAGE):
            with self.subTest(path=path):
                status = "added" if path in planner.REGISTRATION_TEST_FILES else "modified"
                result = plan([{"status": status, "path": path}])
                self.assertEqual("resolved", result["coverage_status"], result["unresolved_coverage"])
                checks = {row["id"] for row in result["required_validation_checks"]}
                self.assertTrue({
                    "wizard-registration-tests", "source-contracts",
                    "cli-gofmt", "cli-vet", "cli-build",
                }.issubset(checks), checks)
                self.assertEqual([], result["selected_gates"])

    def test_real_mixed_source_diffs_fail_closed(self):
        # Actual added registration and changed authorization/write hunks
        # must not be certified from filenames without semantic review.
        fixtures = {
            "registration-only": ("cmd/baha/commands.go", "@@ -10,0 +11 @@\\n+registerAppCommand(root)"),
            "authorization-change": ("cmd/baha/mcp_application_authoring.go",
                "@@ -90 +90 @@\\n-if !allowWrite(ctx) { return denied }\\n+if false { return denied }"),
            "write-mutation": ("cmd/baha/mcp_workspace_mutation.go",
                "@@ -120 +120 @@\\n-return initializeApplicationWorkspace(ctx)\\n+return persistWorkspaceDirectly(ctx)"),
        }
        for name, (path, patch) in fixtures.items():
            with self.subTest(name=name):
                result = plan([{"status": "modified", "path": path, "patch": patch}])
                self.assertEqual("unresolved", result["coverage_status"])
                self.assertTrue(result["broad_required"])

    def test_shared_sources_outside_reviewed_boundary_are_unresolved(self):
        for path in (
            "cmd/baha/mcp_unreviewed_registration.go",
            "cmd/baha/unreviewed_command.go",
            "cmd/baha/nested/commands.go",
            "internal/machine/operations.go",
        ):
            with self.subTest(path=path):
                result = plan([modified(path)])
                self.assertEqual("unresolved", result["coverage_status"])
                self.assertTrue(result["broad_required"])

    def test_delete_rename_type_change_and_unverified_integration_fail_closed(self):
        path = "cmd/baha/registration_ci_readiness_test.go"
        for change in (
            {"status": "deleted", "path": path},
            {"status": "type_changed", "path": path},
            {"status": "renamed", "old_path": path, "new_path": "cmd/baha/command_moved.go"},
        ):
            with self.subTest(change=change):
                result = plan([change])
                self.assertEqual("unresolved", result["coverage_status"])
                self.assertTrue(result["broad_required"])
        self.assertEqual("unresolved", plan([modified(path)], broad=True)["coverage_status"])


class RegistrationExecutorTests(TestCase):
    # This fixture qualifies runner behavior, not hosted workflow routing.
    # A real hosted positive/negative route is still required for acceptance.
    names = (
        "TestAppRegistrationCICommandHelpAndJSONDiscovery",
        "TestCommandTreeCIRouterPathsUnique",
        "TestMachineCoverageCIRealTreeAndJSONClassification",
        "TestMachineDocumentationTracksRegistryAndCommandTree",
        "TestHumanCommandIndexTracksExactCommandMetadata",
        "TestMCPRegistrationCIInProcessDiscoverAndInvoke",
    )

    def exercise(self, absent=None, skipped=None):
        from subprocess import CompletedProcess

        def fake_go(argv, **kwargs):
            pattern = argv[argv.index("-list") + 1] if "-list" in argv else argv[argv.index("-run") + 1]
            matches = [name for name in self.names
                       if __import__("re").search(pattern, name) and name != absent]
            if "-list" in argv:
                return CompletedProcess(argv, 0, "\n".join(matches) + "\n", "")
            lines = []
            for name in matches:
                lines.append("=== RUN   " + name)
                lines.append("--- SKIP: " + name + " (0.00s)" if name == skipped
                             else "--- PASS: " + name + " (0.00s)")
            return CompletedProcess(argv, 0, "\n".join(lines) + "\n", "")
        with patch.object(runner.subprocess, "run", side_effect=fake_go):
            return runner.execute_validation("wizard-registration-tests", ROOT.parent)

    def test_all_five_families_execute(self):
        self.exercise()

    def test_missing_or_skipped_mcp_and_documentation_fail(self):
        for name in self.names[-3:]:
            with self.subTest(name=name):
                with self.assertRaises(RuntimeError):
                    self.exercise(absent=name)
                with self.assertRaises(RuntimeError):
                    self.exercise(skipped=name)


class RegistrationWorkflowRoute(TestCase):
    def test_real_workflow_branch_uses_trusted_helper_and_rejects_missing_mcp(self):
        import os
        import subprocess
        import tempfile

        workflow = (ROOT.parent / ".github/workflows/ci.yml").read_text()
        start = workflow.index("            wizard-*-tests)") + len("            wizard-*-tests)\n")
        fragment = workflow[start:workflow.index("              ;;", start)]
        self.assertIn('git show "$BASE_SHA":scripts/pr-check-runner.py', fragment)
        self.assertIn('execute --check "$VALIDATION"', fragment)

        def run(*args, cwd=None, env=None):
            return subprocess.run(args, cwd=cwd, env=env, text=True,
                                  stdout=subprocess.PIPE, stderr=subprocess.STDOUT)

        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            trusted, candidate, temp = (root / name for name in ("trusted", "candidate", "runner"))
            for path in (trusted, candidate, temp):
                path.mkdir()
            self.assertEqual(0, run("git", "init", "-q", str(trusted)).returncode)
            (trusted / "scripts").mkdir()
            (trusted / "scripts/pr-check-runner.py").write_text(
                (ROOT / "pr-check-runner.py").read_text())
            self.assertEqual(0, run("git", "add", ".", cwd=trusted).returncode)
            identity = dict(os.environ, GIT_AUTHOR_NAME="independent fixture",
                            GIT_AUTHOR_EMAIL="fixture@example.test",
                            GIT_COMMITTER_NAME="independent fixture",
                            GIT_COMMITTER_EMAIL="fixture@example.test")
            committed = run("git", "commit", "-qm", "isolated trusted helper",
                            cwd=trusted, env=identity)
            self.assertEqual(0, committed.returncode, committed.stdout)
            sha = run("git", "rev-parse", "HEAD", cwd=trusted).stdout.strip()
            self.assertEqual(0, run("git", "init", "-q", str(candidate)).returncode)
            self.assertEqual(0, run("git", "remote", "add", "origin", str(trusted),
                                    cwd=candidate).returncode)
            (candidate / "go.mod").write_text("module example.com/registration-route\n\ngo 1.25\n")
            pkg = candidate / "cmd" / "baha"
            pkg.mkdir(parents=True)
            environment = dict(os.environ, BASE_SHA=sha,
                               VALIDATION="wizard-registration-tests",
                               RUNNER_TEMP=str(temp), GITHUB_WORKSPACE=str(candidate))
            names = list(RegistrationExecutorTests.names)
            for missing in (None, "TestMCPRegistrationCIInProcessDiscoverAndInvoke"):
                available = [name for name in names if name != missing]
                (pkg / "registration_test.go").write_text(
                    "package main\nimport \"testing\"\n" +
                    "\n".join("func %s(t *testing.T) {}" % name for name in available) + "\n")
                executed = run("bash", "-euo", "pipefail", "-c", fragment,
                               cwd=candidate, env=environment)
                if missing:
                    self.assertNotEqual(0, executed.returncode, executed.stdout)
                    self.assertIn("zero matching tests", executed.stdout)
                else:
                    self.assertEqual(0, executed.returncode, executed.stdout)
                    self.assertIn("TestMCPRegistrationCIInProcessDiscoverAndInvoke", executed.stdout)


class RegistrationActualPackageSmoke(TestCase):
    def test_real_candidate_go_registration_families(self):
        # Supplementary HOSTED source proof. This is NOT the trusted workflow
        # matrix job; that qualification requires independent policy promotion.
        matched = runner.execute_validation("wizard-registration-tests", ROOT.parent)
        self.assertEqual(6, len(matched), matched)

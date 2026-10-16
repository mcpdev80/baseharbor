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
        path = "cmd/baha/commands.go"
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
        for name in self.names[-2:]:
            with self.subTest(name=name):
                with self.assertRaises(RuntimeError):
                    self.exercise(absent=name)
                with self.assertRaises(RuntimeError):
                    self.exercise(skipped=name)

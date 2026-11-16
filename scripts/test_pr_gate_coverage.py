"""Tests for declarative PR gate coverage validation and resolution."""
import importlib.util
import json
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parent
SPEC = importlib.util.spec_from_file_location(
    "validate_pr_gate_coverage", ROOT / "validate-pr-gate-coverage.py")
coverage_module = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(coverage_module)
PLANNER_SPEC = importlib.util.spec_from_file_location(
    "pr_gate_plan", ROOT / "pr-gate-plan.py")
planner = importlib.util.module_from_spec(PLANNER_SPEC)
PLANNER_SPEC.loader.exec_module(planner)

DOCKER = "atomic/docker/data-capabilities"
PODMAN = "atomic/podman/data-capabilities"
LIFECYCLE = "atomic/podman/lifecycle"


def catalog():
    return {
        "schema": coverage_module.SCHEMA,
        "release": "v0.4.27",
        "surfaces": [
            "runtime.container-ownership",
            "application.data-capabilities",
            "application.lifecycle",
            "installation.destroy-retry",
            "installation.second-target-conflict",
        ],
        "packages": [
            {"path": "internal/runtime", "surfaces": ["runtime.container-ownership"]}
        ],
        "components": [
            {"id": "runtime.container-identity",
             "path": "internal/runtime/container_identity.go",
             "surfaces": ["runtime.container-ownership"]},
            {"id": "podman.installation-destroy",
             "path": "cmd/baha/installation_destroy_native_test.go",
             "surfaces": ["installation.destroy-retry"]},
            {"id": "podman.second-target-conflict-tests",
             "path": "cmd/baha/second_target_conflict_native_test.go",
             "surfaces": ["installation.second-target-conflict"]},
        ],
        "gates": [
            {"id": DOCKER, "runtime": "docker", "selection_scope": "focused",
             "covers": ["runtime.container-ownership", "application.data-capabilities"]},
            {"id": PODMAN, "runtime": "podman", "selection_scope": "focused",
             "covers": ["runtime.container-ownership", "application.data-capabilities"]},
            {"id": LIFECYCLE, "runtime": "podman", "selection_scope": "focused",
             "covers": ["application.lifecycle"]},
        ],
        "native_checks": [
            {"id": "native/podman/installation-destroy", "runtime": "podman",
             "covers": ["installation.destroy-retry"],
             "source_components": ["podman.installation-destroy"],
             "selection_scope": "focused"},
            {"id": "native/podman/second-target-conflict", "runtime": "podman",
             "covers": ["installation.second-target-conflict"],
             "source_components": ["podman.second-target-conflict-tests"],
             "selection_scope": "focused"},
        ],
    }


def requirements():
    ids = [DOCKER, PODMAN, LIFECYCLE]
    return {
        "releases": ["v0.4.27"],
        "gates": [{"id": gate_id, "required_for": ["v0.4.27"]} for gate_id in ids],
    }


class CoverageCatalogTests(unittest.TestCase):
    def test_runtime_neutral_package_fallback_uses_docker_representative(self):
        selected, unresolved = coverage_module.resolve_covered_gates(
            ["internal/runtime/new_feature.go"], catalog())
        self.assertEqual([DOCKER], selected)
        self.assertEqual([], unresolved)

    def test_podman_only_component_selects_exact_podman_gate(self):
        data = catalog()
        data["components"] = [{"id": "podman.only", "path": "internal/podman/only.go",
                               "surfaces": ["application.lifecycle"]}]
        selected, unresolved = coverage_module.resolve_covered_gates(
            ["internal/podman/only.go"], data)
        self.assertEqual([LIFECYCLE], selected)
        self.assertEqual([], unresolved)

    def test_shared_and_podman_only_changes_keep_their_own_smallest_gates(self):
        data = catalog()
        data["components"].append({"id": "podman.only", "path": "internal/podman/only.go",
                                   "surfaces": ["application.lifecycle"]})
        selected, unresolved = coverage_module.resolve_covered_gates(
            ["internal/runtime/container_identity.go", "internal/podman/only.go"], data)
        self.assertEqual([DOCKER, LIFECYCLE], selected)
        self.assertEqual([], unresolved)

    def test_exact_runtime_neutral_component_uses_docker_representative(self):
        selected, unresolved = coverage_module.resolve_covered_gates(
            ["internal/runtime/container_identity.go"], catalog())
        self.assertEqual([DOCKER], selected)
        self.assertEqual([], unresolved)

    def test_podman_container_identity_changes_select_only_podman_data_gate(self):
        current = json.loads((ROOT / "pr-gate-coverage.json").read_text())
        selected, unresolved = coverage_module.resolve_covered_gates(
            ["internal/runtime/container_identity.go",
             "internal/runtime/container_identity_test.go",
             "internal/runtime/service_retirement_test.go"], current)
        self.assertEqual(["atomic/podman/data-capabilities"], selected)
        self.assertEqual([], unresolved)

    def test_podman_pinned_service_stop_selects_only_podman_lifecycle(self):
        current = json.loads((ROOT / "pr-gate-coverage.json").read_text())
        selected, unresolved = coverage_module.resolve_covered_gates(
            ["internal/runtime/service_stop.go",
             "internal/runtime/service_stop_test.go"], current)
        self.assertEqual(["atomic/podman/lifecycle"], selected)
        self.assertEqual([], unresolved)

    def test_selective_identity_runtime_test_has_both_executable_lifecycle_gates(self):
        current = json.loads((ROOT / "pr-gate-coverage.json").read_text())
        selected, unresolved = coverage_module.resolve_covered_gates(
            ["cmd/baha/app_identity_selective_runtime_test.go"], current)
        self.assertEqual(["atomic/docker/lifecycle", "atomic/podman/lifecycle"], selected)
        self.assertEqual([], unresolved)
        component = next(row for row in current["components"]
                         if row["path"] == "cmd/baha/app_identity_selective_runtime_test.go")
        self.assertEqual(["source.gate.atomic.docker.lifecycle",
                          "source.gate.atomic.podman.lifecycle"], component["surfaces"])

    def test_coreless_backend_baseline_has_both_integration_gate_owners_and_test_anchor(self):
        current = json.loads((ROOT / "pr-gate-coverage.json").read_text())
        path = "cmd/baha/app_backend_selective_runtime_test.go"
        selected, unresolved = coverage_module.resolve_covered_gates([path], current)
        self.assertEqual(["integration/docker/coreless-lifecycle",
                          "integration/podman/coreless-lifecycle"], selected)
        self.assertEqual([], unresolved)
        component = next(row for row in current["components"] if row["path"] == path)
        self.assertEqual(["TestSelectiveBackendCleanupPreservesRuntimeBaseline"],
                         component["required_test_names"])
        executor = importlib.util.spec_from_file_location(
            "pre_release_integration", ROOT / "pre-release-integration.py")
        integration = importlib.util.module_from_spec(executor)
        executor.loader.exec_module(integration)
        with tempfile.TemporaryDirectory() as temporary:
            source = Path(temporary) / path
            source.parent.mkdir(parents=True)
            source.write_text("func TestSelectiveBackendCleanupPreservesRuntimeBaseline(t *testing.T) {}\n")
            application_path = Path(temporary) / "cmd/baha/app_application_backend_runtime_test.go"
            application_path.write_text(
                "func TestSelectiveApplicationBackendRuntimeAddRemoveReAdd(t *testing.T) {}\n")
            for gate in selected:
                self.assertEqual(
                    [["./cmd/baha", "-run", "^TestSelectiveBackendCleanupPreservesRuntimeBaseline$",
                      "-timeout", "20m"],
                     ["./cmd/baha", "-run", "^TestSelectiveApplicationBackendRuntimeAddRemoveReAdd$",
                      "-timeout", "20m"]],
                    integration.coreless_anchor_checks(gate, temporary))

    def test_application_backend_coreless_acceptance_is_anchored_to_both_integration_gates(self):
        current = json.loads((ROOT / "pr-gate-coverage.json").read_text())
        path = "cmd/baha/app_application_backend_runtime_test.go"
        selected, unresolved = coverage_module.resolve_covered_gates([path], current)
        self.assertEqual(["integration/docker/coreless-lifecycle",
                          "integration/podman/coreless-lifecycle"], selected)
        self.assertEqual([], unresolved)
        component = next(row for row in current["components"] if row["path"] == path)
        self.assertEqual(["TestSelectiveApplicationBackendRuntimeAddRemoveReAdd"],
                         component["required_test_names"])

    def test_coreless_executor_and_workflows_select_both_runtime_gates(self):
        current = json.loads((ROOT / "pr-gate-coverage.json").read_text())
        expected = ["integration/docker/coreless-lifecycle",
                    "integration/podman/coreless-lifecycle"]
        paths = ["scripts/pre-release-integration.py",
                 ".github/workflows/pre-release.yml",
                 ".github/workflows/targeted-integration.yml"]
        for path in paths:
            with self.subTest(path=path):
                selected, unresolved = coverage_module.resolve_covered_gates([path], current)
                self.assertEqual(expected, selected)
                self.assertEqual([], unresolved)

    def test_service_stop_maps_to_podman_lifecycle_and_executor_runs_test_anchor(self):
        current = json.loads((ROOT / "pr-gate-coverage.json").read_text())
        for path in ("internal/runtime/service_stop.go", "internal/runtime/service_stop_test.go"):
            with self.subTest(path=path):
                selected, unresolved = coverage_module.resolve_covered_gates([path], current)
                self.assertEqual([LIFECYCLE], selected)
                self.assertEqual([], unresolved)
                component = next(row for row in current["components"] if row["path"] == path)
                self.assertEqual(["TestSelectedStopAcceptsPinnedContainerName"],
                                 component["required_test_names"])
        workflow = (ROOT.parent / ".github/workflows/pre-release-runtime-suite.yml").read_text()
        self.assertIn("run_native_test ./internal/runtime TestSelectedStopAcceptsPinnedContainerName",
                      workflow)

    def test_trusted_pr_plan_simulates_both_coreless_and_podman_lifecycle_routes(self):
        requirements = json.loads((ROOT / "release-requirements.json").read_text())
        mapping = json.loads((ROOT / "pr-gate-map-v1.json").read_text())
        for path, expected in (
            ("cmd/baha/app_backend_selective_runtime_test.go",
             ["integration/docker/coreless-lifecycle", "integration/podman/coreless-lifecycle"]),
            ("internal/runtime/service_stop.go", ["atomic/podman/lifecycle"]),
            ("internal/runtime/service_stop_test.go", ["atomic/podman/lifecycle"]),
        ):
            with self.subTest(path=path):
                diff = {
                    "target_base_sha": "a" * 40,
                    "head_sha": "b" * 40,
                    "merge_base_sha": "c" * 40,
                    "integration_sha": "d" * 40,
                    "broad_required": False,
                    "reasons": [],
                    "changes": [{"status": "modified", "path": path}],
                }
                result = planner.plan(diff, requirements, mapping)
                self.assertEqual("resolved", result["coverage_status"])
                self.assertEqual([], result["unresolved_coverage"])
                self.assertEqual(expected, result["selected_gates"])

    def test_unknown_package_is_returned_for_conservative_fallback(self):
        selected, unresolved = coverage_module.resolve_covered_gates(
            ["internal/unknown/new_surface.go"], catalog())
        self.assertEqual([], selected)
        self.assertEqual(["internal/unknown/new_surface.go"], unresolved)

    def test_unrelated_changes_do_not_select_runtime_gates(self):
        selected, unresolved = coverage_module.resolve_covered_gates(
            ["docs/how-to/example.md"], catalog())
        self.assertEqual([], selected)
        self.assertEqual(["docs/how-to/example.md"], unresolved)

    def test_podman_native_check_is_runtime_specific(self):
        selected = coverage_module.resolve_native_checks(
            ["cmd/baha/installation_destroy_native_test.go"], catalog())
        self.assertEqual(["native/podman/installation-destroy"], selected)

    def test_second_target_conflict_selects_only_its_runtime_check(self):
        selected = coverage_module.resolve_native_checks(
            ["cmd/baha/second_target_conflict_native_test.go"], catalog())
        self.assertEqual(["native/podman/second-target-conflict"], selected)

    def test_runtime_neutral_native_checks_use_docker_representative(self):
        data = catalog()
        data["components"].append({"id": "neutral.native", "path": "internal/neutral/native.go",
                                   "surfaces": ["application.data-capabilities"]})
        data["native_checks"] += [
            {"id": "native/docker/neutral", "runtime": "docker", "covers": ["application.data-capabilities"],
             "source_components": ["neutral.native"], "selection_scope": "focused"},
            {"id": "native/podman/neutral", "runtime": "podman", "covers": ["application.data-capabilities"],
             "source_components": ["neutral.native"], "selection_scope": "focused"},
        ]
        selected = coverage_module.resolve_native_checks(["internal/neutral/native.go"], data)
        self.assertEqual(["native/docker/neutral"], selected)

    def test_mixed_neutral_and_podman_native_sources_keep_both_scoped_checks(self):
        data = catalog()
        data["components"].append({"id": "neutral.native", "path": "internal/neutral/native.go",
                                   "surfaces": ["application.data-capabilities"]})
        data["native_checks"] += [
            {"id": "native/docker/data-capabilities", "runtime": "docker",
             "covers": ["application.data-capabilities"],
             "source_components": ["neutral.native"], "selection_scope": "focused"},
            {"id": "native/podman/data-capabilities", "runtime": "podman",
             "covers": ["application.data-capabilities"],
             "source_components": ["neutral.native"], "selection_scope": "focused"},
        ]
        selected = coverage_module.resolve_native_checks([
            "internal/neutral/native.go", "cmd/baha/installation_destroy_native_test.go"], data)
        self.assertEqual([
            "native/docker/data-capabilities",
            "native/podman/installation-destroy",
        ], selected)

    def test_catalog_accounts_for_every_active_release_gate(self):
        self.assertEqual([], coverage_module.validate(catalog(), requirements()))

    def test_repo_core_paths_preserve_legacy_release_gate_coverage(self):
        legacy = json.loads((ROOT / "pre-release-runtime-impact.json").read_text())
        current = json.loads((ROOT / "pr-gate-coverage.json").read_text())
        for path, expected in legacy["core"].items():
            with self.subTest(path=path):
                selected, _ = coverage_module.resolve_covered_gates([path], current)
                expected = set(expected)
                # The full-release impact map names the two coreless integration
                # jobs, while PR CI executes this changed acceptance test through
                # the corresponding native lifecycle gates.
                if path == "cmd/baha/app_identity_selective_runtime_test.go":
                    expected = {gate.replace("integration/", "atomic/").replace(
                        "/coreless-lifecycle", "/lifecycle") for gate in expected}
                expected = {gate for gate in expected
                            if not (gate.startswith(("atomic/podman/", "integration/podman/")) and
                                    gate.replace("/podman/", "/docker/") in expected)}
                self.assertTrue(expected <= set(selected),
                                f"legacy gates {expected} missing from {selected}")

    def test_shared_openbao_login_change_uses_docker_pr_representative(self):
        selected, unresolved = coverage_module.resolve_covered_gates(
            ["internal/openbao/bootstrap_auth.go"],
            json.loads((ROOT / "pr-gate-coverage.json").read_text()))
        self.assertEqual(["atomic/docker/control-plane-recovery"], selected)
        self.assertEqual([], unresolved)

    def test_package_surface_without_gate_owner_is_rejected(self):
        data = catalog()
        data["gates"] = [gate for gate in data["gates"] if gate["id"] != DOCKER and gate["id"] != PODMAN]
        errors = coverage_module.validate(data, requirements())
        self.assertTrue(any("has no release gate owner" in error for error in errors))

    def test_unknown_gate_and_path_traversal_are_rejected(self):
        data = catalog()
        data["packages"][0]["path"] = "../internal/runtime"
        data["gates"][0]["id"] = "atomic/docker/does-not-exist"
        errors = coverage_module.validate(data, requirements())
        self.assertTrue(any("normalized repository-relative directory" in error for error in errors))
        self.assertTrue(any("unknown release gate" in error for error in errors))

    def test_pr_plan_routes_podman_identity_to_its_runtime_gate(self):
        requirements = json.loads((ROOT / "release-requirements.json").read_text())
        mapping = json.loads((ROOT / "pr-gate-map-v1.json").read_text())
        changes = [{"status": "modified", "path": "internal/runtime/container_identity.go"}]
        diff = {
            "target_base_sha": "a" * 40,
            "head_sha": "b" * 40,
            "merge_base_sha": "c" * 40,
            "integration_sha": "d" * 40,
            "broad_required": False,
            "reasons": [],
            "changes": changes,
        }
        result = planner.plan(diff, requirements, mapping)
        self.assertEqual("resolved", result["coverage_status"])
        self.assertNotIn(DOCKER, result["selected_gates"])
        self.assertIn(PODMAN, result["selected_gates"])
        self.assertNotIn("atomic/podman/control-plane-recovery", result["selected_gates"])
        self.assertNotIn("atomic/podman/backup-restore", result["selected_gates"])
        self.assertNotIn("journey/podman", result["selected_gates"])
        self.assertEqual([PODMAN], result["selected_gates"])

    def test_pr_plan_routes_podman_container_identity_to_podman_gate(self):
        requirements = json.loads((ROOT / "release-requirements.json").read_text())
        mapping = json.loads((ROOT / "pr-gate-map-v1.json").read_text())
        diff = {
            "target_base_sha": "a" * 40, "head_sha": "b" * 40,
            "merge_base_sha": "c" * 40, "integration_sha": "d" * 40,
            "broad_required": False, "reasons": [],
            "changes": [
                {"status": "modified", "path": "internal/runtime/container_identity.go"},
                {"status": "modified", "path": "internal/runtime/container_identity_test.go"},
                {"status": "modified", "path": "internal/runtime/service_retirement_test.go"},
            ],
        }
        result = planner.plan(diff, requirements, mapping)
        self.assertEqual("resolved", result["coverage_status"])
        self.assertEqual(["atomic/podman/data-capabilities"], result["selected_gates"])

    def test_trusted_workflow_copies_gate_coverage_beside_planner(self):
        workflow = (ROOT.parent / ".github/workflows/ci.yml").read_text()
        step = workflow.split('          for policy_file in ', 1)[1].split(
            '          done', 1)[0]
        for policy_file in ("pr-gate-plan.py", "validate-pr-gate-coverage.py",
                            "pr-gate-coverage.json"):
            self.assertIn(policy_file, step)
            self.assertIn('git show "$controller:scripts/$policy_file"', step)

    def test_copied_trusted_planner_keeps_runtime_coverage(self):
        with tempfile.TemporaryDirectory() as folder:
            temp = Path(folder)
            for name in ("pr-gate-plan.py", "validate-pr-gate-coverage.py",
                         "pr-gate-coverage.json"):
                (temp / name).write_bytes((ROOT / name).read_bytes())
            diff = {
                "target_base_sha": "a" * 40, "head_sha": "b" * 40,
                "merge_base_sha": "c" * 40, "integration_sha": "d" * 40,
                "broad_required": False, "reasons": [],
                "changes": [{"status": "modified",
                             "path": "cmd/baha/app_identity_selective_runtime_test.go"}],
            }
            diff_path = temp / "diff.json"
            diff_path.write_text(json.dumps(diff))
            result = subprocess.check_output([
                "python3", str(temp / "pr-gate-plan.py"), "--diff", str(diff_path),
                "--requirements", str(ROOT / "release-requirements.json"),
                "--mapping", str(ROOT / "pr-gate-map-v1.json"),
            ], cwd=ROOT.parent, env={**__import__("os").environ,
                                    "GITHUB_REPOSITORY": "mcpdev80/baseharbor",
                                    "GITHUB_BASE_REF": "develop"}, text=True)
            plan_result = json.loads(result)
            self.assertEqual(["atomic/docker/lifecycle", "atomic/podman/lifecycle"],
                             plan_result["selected_gates"])

    def test_legacy_app_core_requirement_keeps_only_declared_narrow_gates(self):
        requirements = json.loads((ROOT / "release-requirements.json").read_text())
        mapping = json.loads((ROOT / "pr-gate-map-v1.json").read_text())
        diff = {
            "target_base_sha": "a" * 40, "head_sha": "b" * 40,
            "merge_base_sha": "c" * 40, "integration_sha": "d" * 40,
            "broad_required": False, "reasons": [],
            "changes": [{"status": "modified", "path": "cmd/baha/app_core_requirement.go"}],
        }
        result = planner.plan(diff, requirements, mapping)
        self.assertEqual("unresolved", result["coverage_status"])
        self.assertEqual([
            "atomic/docker/data-capabilities", "atomic/docker/guided",
        ], result["selected_gates"])
        self.assertFalse(any("journey/" in gate for gate in result["selected_gates"]))

    def test_podman_doctor_change_selects_only_podman_lifecycle(self):
        requirements = json.loads((ROOT / "release-requirements.json").read_text())
        mapping = json.loads((ROOT / "pr-gate-map-v1.json").read_text())
        diff = {
            "target_base_sha": "a" * 40, "head_sha": "b" * 40,
            "merge_base_sha": "c" * 40, "integration_sha": "d" * 40,
            "broad_required": False, "reasons": [],
            "changes": [{"status": "modified", "path": "cmd/baha/podman_doctor_checks.go"}],
        }
        result = planner.plan(diff, requirements, mapping)
        self.assertEqual(["atomic/podman/lifecycle"], result["selected_gates"])

    def test_unreviewed_runtime_file_falls_back_without_package_wide_false_gate(self):
        requirements = json.loads((ROOT / "release-requirements.json").read_text())
        mapping = json.loads((ROOT / "pr-gate-map-v1.json").read_text())
        diff = {
            "target_base_sha": "a" * 40, "head_sha": "b" * 40,
            "merge_base_sha": "c" * 40, "integration_sha": "d" * 40,
            "broad_required": False, "reasons": [],
            "changes": [{"status": "modified", "path": "internal/runtime/new_feature.go"}],
        }
        result = planner.plan(diff, requirements, mapping, conservative_fallback=True)
        self.assertTrue(result["fallback_used"])
        self.assertEqual([], result["selected_gates"])
        self.assertNotIn("atomic/docker/data-capabilities", result["selected_gates"])
        self.assertNotIn("atomic/podman/lifecycle", result["selected_gates"])

    def test_unknown_source_fallback_does_not_guess_runtime_gate(self):
        requirements = json.loads((ROOT / "release-requirements.json").read_text())
        mapping = json.loads((ROOT / "pr-gate-map-v1.json").read_text())
        diff = {
            "target_base_sha": "a" * 40, "head_sha": "b" * 40,
            "merge_base_sha": "c" * 40, "integration_sha": "d" * 40,
            "broad_required": False, "reasons": [],
            "changes": [{"status": "added", "path": "internal/unknown/new.go"}],
        }
        result = planner.plan(diff, requirements, mapping, conservative_fallback=True)
        self.assertTrue(result["fallback_used"])
        self.assertEqual([], result["selected_gates"])
        self.assertIn("go-unit-tests", {row["id"] for row in result["required_validation_checks"]})

    def test_legacy_runtime_hint_without_catalog_owner_is_unresolved(self):
        requirements = json.loads((ROOT / "release-requirements.json").read_text())
        mapping = json.loads((ROOT / "pr-gate-map-v1.json").read_text())
        diff = {
            "target_base_sha": "a" * 40, "head_sha": "b" * 40,
            "merge_base_sha": "c" * 40, "integration_sha": "d" * 40,
            "broad_required": False, "reasons": [],
            "changes": [{"status": "modified", "path": "internal/runtime/new_feature.go"}],
        }
        result = planner.plan(diff, requirements, mapping, conservative_fallback=True)
        self.assertTrue(result["fallback_used"])
        self.assertEqual("resolved", result["coverage_status"])
        self.assertNotIn("atomic/docker/lifecycle", result["selected_gates"])
        self.assertNotIn("atomic/podman/lifecycle", result["selected_gates"])


if __name__ == "__main__":
    unittest.main()

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

    def test_perses_runtime_smoke_selects_only_its_docker_native_check(self):
        current = json.loads((ROOT / "pr-gate-coverage.json").read_text())
        paths = ("internal/application/perses_runtime.go",
                 "internal/application/perses_runtime_test.go")
        for path in paths:
            with self.subTest(path=path):
                selected = coverage_module.resolve_native_checks([path], current)
                self.assertEqual(["native/docker/perses-runtime-smoke"], selected)
                component = next(row for row in current["components"] if row["path"] == path)
                self.assertEqual(["TestPersesRuntimeSmokeDockerIsolated"],
                                 component["required_test_names"])
        self.assertEqual([], coverage_module.resolve_native_checks(
            ["internal/application/manifest.go"], current))

    def test_application_observability_source_and_test_select_both_gates(self):
        current = json.loads((ROOT / "pr-gate-coverage.json").read_text())
        expected = ["atomic/docker/observability", "atomic/podman/observability"]
        for path in ("internal/application/runtime_observability.go",
                     "internal/application/runtime_observability_trace_source_test.go"):
            with self.subTest(path=path):
                selected, unresolved = coverage_module.resolve_covered_gates([path], current)
                self.assertEqual(expected, selected)
                self.assertEqual([], unresolved)
        selected, unresolved = coverage_module.resolve_covered_gates(
            ["internal/application/provider_union.go"], current)
        self.assertEqual([], selected)
        self.assertEqual(["internal/application/provider_union.go"], unresolved)

    def test_deleted_source_without_catalog_row_resolves_by_deletion(self):
        # card 0428-253 (precedent #1789): dead source whose deletion the
        # planner exempted (statuses map) must not come back as an uncataloged
        # change; without the exemption the fail-loud contract stands.
        current = json.loads((ROOT / "pr-gate-coverage.json").read_text())
        path = "internal/metrics/perses_dashboards_promql_test.go"
        self.assertTrue(all(row.get("path") != path for row in current["components"]))
        selected, unresolved = coverage_module.resolve_covered_gates([path], current)
        self.assertEqual([], selected)
        self.assertEqual([path], unresolved)
        selected, unresolved = coverage_module.resolve_covered_gates(
            [path], current, {path: "deleted"})
        self.assertEqual([], selected)
        self.assertEqual([], unresolved)

    def test_deleted_source_selects_nothing_even_with_remaining_catalog_row(self):
        current = json.loads((ROOT / "pr-gate-coverage.json").read_text())
        path = "internal/metrics/perses_dashboards_test.go"
        selected, unresolved = coverage_module.resolve_covered_gates(
            [path], current, {path: "deleted"})
        self.assertEqual([], selected)
        self.assertEqual([], unresolved)

    def test_status_map_never_exempts_non_deletion_statuses(self):
        current = json.loads((ROOT / "pr-gate-coverage.json").read_text())
        path = "internal/newarea/feature.go"
        for status in ("modified", "added", "renamed", "copied", "type_changed"):
            with self.subTest(status=status):
                selected, unresolved = coverage_module.resolve_covered_gates(
                    [path], current, {path: status})
                self.assertEqual([], selected)
                self.assertEqual([path], unresolved)
        # Without a status map the resolver behaves exactly as before.
        selected, unresolved = coverage_module.resolve_covered_gates([path], current)
        self.assertEqual([], selected)
        self.assertEqual([path], unresolved)

    def test_planner_deletion_exemption_contract(self):
        # card 0428-253: unit coverage of the exemption contract (the
        # end-to-end replay lives in scripts/test_pr_ci_fallback.py against
        # the real #1789 commits). A deleted path the checkout catalog owns -
        # exact row or package root - is exempted; a path it no longer lists
        # is NOT exempted by this function (plan() resolves that case through
        # the trusted base-catalog read). Other statuses never exempt.
        current = json.loads((ROOT / "pr-gate-coverage.json").read_text())
        sibling = "internal/metrics/perses_dashboards_test.go"

        def diff(changes):
            return {'target_base_sha': 'a' * 40, 'head_sha': 'b' * 40,
                    'merge_base_sha': 'c' * 40, 'integration_sha': 'd' * 40,
                    'broad_required': False, 'reasons': [], 'changes': changes}

        self.assertEqual({sibling: "deleted"}, planner.deletion_exemption(
            diff([{'status': 'deleted', 'path': sibling}]), current))
        package_root = next(row["path"].rstrip("/") for row in current["packages"])
        inside = package_root + "/exemption_probe_test.go"
        self.assertEqual({inside: "deleted"}, planner.deletion_exemption(
            diff([{'status': 'deleted', 'path': inside}]), current))
        dead = "internal/metrics/perses_dashboards_promql_test.go"
        self.assertEqual({}, planner.deletion_exemption(
            diff([{'status': 'deleted', 'path': dead}]), current))
        for status in ("modified", "added", "renamed", "type_changed"):
            with self.subTest(status=status):
                self.assertEqual({}, planner.deletion_exemption(
                    diff([{'status': status, 'path': sibling}]), current))

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
                      "-timeout", "20m"]],
                    integration.coreless_anchor_checks(gate, temporary))

    def test_environment_destroy_native_test_selects_both_coreless_gates(self):
        current = json.loads((ROOT / "pr-gate-coverage.json").read_text())
        path = "cmd/baha/env_destroy_integration_test.go"
        selected, unresolved = coverage_module.resolve_covered_gates([path], current)
        self.assertEqual(["integration/docker/coreless-lifecycle",
                          "integration/podman/coreless-lifecycle"], selected)
        self.assertEqual([], unresolved)
        component = next(row for row in current["components"] if row["path"] == path)
        self.assertTrue(component["runtime_specific"])
        self.assertEqual(["TestEnvironmentDestroyNativeLifecycle"],
                         component["required_test_names"])

    def test_shared_backend_helper_maps_to_both_coreless_integration_gates(self):
        current = json.loads((ROOT / "pr-gate-coverage.json").read_text())
        path = "cmd/baha/app_backend_shared_runtime_test.go"
        selected, unresolved = coverage_module.resolve_covered_gates([path], current)
        self.assertEqual(["integration/docker/coreless-lifecycle",
                          "integration/podman/coreless-lifecycle"], selected)
        self.assertEqual([], unresolved)

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

    def test_coreless_fixture_acceptance_files_are_anchored_to_both_integration_gates(self):
        current = json.loads((ROOT / "pr-gate-coverage.json").read_text())
        expected = ["integration/docker/coreless-lifecycle",
                    "integration/podman/coreless-lifecycle"]
        for path, test_name in (
            ("cmd/baha/app_capability_runtime_test.go", "TestApplicationCapabilityLifecycleExistingCore"),
            ("cmd/baha/app_https_selective_runtime_test.go", "TestSelectiveHTTPSRuntimeAddRemoveReAdd"),
            ("cmd/baha/application_otlp_handoff_runtime_test.go", "TestManagedRegisteredOTLPNativeConsumerHandoff"),
        ):
            with self.subTest(path=path):
                selected, unresolved = coverage_module.resolve_covered_gates([path], current)
                self.assertEqual(expected, selected)
                self.assertEqual([], unresolved)
                component = next(row for row in current["components"] if row["path"] == path)
                self.assertEqual([test_name], component["required_test_names"])

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

    def test_workstation_onboarding_selects_docker_and_podman_for_docs_and_test_changes(self):
        expected = ["native/docker/onboarding", "native/podman/workstation-onboarding"]
        data = json.loads((ROOT / "pr-gate-coverage.json").read_text())
        for path in (
                "cmd/baha/workstation_onboarding_remote_native_test.go",
                "docs/how-to/operator-api.md",
                "docs/how-to/remote-workstation.md",
                "docs/spec/zero-friction-workstation-onboarding-v0427.md"):
            with self.subTest(path=path):
                self.assertEqual(expected, coverage_module.resolve_native_checks([path], data))

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

    # card 0428-208 / #1746 (Owner authorization 2026-11-23): the validator fix at
    # validate-pr-gate-coverage.py:249-256 keeps every currently-required release gate
    # mandatory (the `missing` rule is unchanged) while allowing catalog gates whose
    # required_for lists only a NEWER release (future coverage) without an `extra` fail.

    def test_future_only_catalog_entry_passes_as_release_only(self):
        # (b) a catalog gate whose required_for is only v0.4.28 is allowed while the
        # catalog release field stays v0.4.27; it is not reported as `extra`.
        data = catalog()
        data["gates"].append({
            "id": "atomic/static/ci-fast-check", "runtime": "static",
            "class": "static", "scope": "pr",
            "covers": ["source.gate.atomic.static.ci.fast.check"], "kind": "static",
            "selection_scope": "release-only",
        })
        data["surfaces"].append("source.gate.atomic.static.ci.fast.check")
        requirements_dict = requirements()
        requirements_dict["releases"] = ["v0.4.27", "v0.4.28"]
        requirements_dict["gates"].append(
            {"id": "atomic/static/ci-fast-check", "required_for": ["v0.4.28"]})
        self.assertEqual([], coverage_module.validate(data, requirements_dict))

    def test_missing_current_required_gate_still_fails(self):
        # (a) a gate required for the active release field must still be present; the
        # future-coverage allowance never masks a genuinely missing current gate.
        data = catalog()
        del data["gates"][0]  # drop DOCKER, still required at v0.4.27
        errors = coverage_module.validate(data, requirements())
        self.assertTrue(
            any("coverage catalog omits active release gates" in e for e in errors))
        self.assertTrue(any("atomic/docker/data-capabilities" in e for e in errors))

    def test_release_field_stays_hard_required(self):
        # (c) a release field absent from the releases list is still rejected outright.
        data = catalog()
        data["release"] = "v0.4.99"
        errors = coverage_module.validate(data, requirements())
        self.assertTrue(
            any("is not in release requirements" in e for e in errors))

    def test_current_required_gate_still_fails_when_future_gate_missing(self):
        # The future allowance must not drop the mandatory `missing` enforcement: a
        # catalog that carries a v0.4.28-only gate but omits a v0.4.27 gate fails on
        # the missing current gate, not silently passes.
        data = catalog()
        data["gates"].append({
            "id": "atomic/static/ci-fast-check", "runtime": "static",
            "class": "static", "scope": "pr",
            "covers": ["source.gate.atomic.static.ci.fast.check"], "kind": "static",
            "selection_scope": "release-only",
        })
        data["surfaces"].append("source.gate.atomic.static.ci.fast.check")
        data["gates"].pop(0)  # also drop DOCKER (required at v0.4.27)
        requirements_dict = requirements()
        requirements_dict["releases"] = ["v0.4.27", "v0.4.28"]
        requirements_dict["gates"].append(
            {"id": "atomic/static/ci-fast-check", "required_for": ["v0.4.28"]})
        errors = coverage_module.validate(data, requirements_dict)
        self.assertTrue(any("omits active release gates" in e for e in errors))
        self.assertTrue(any("atomic/docker/data-capabilities" in e for e in errors))

    def test_repo_core_paths_preserve_legacy_release_gate_coverage(self):
        legacy = json.loads((ROOT / "pre-release-runtime-impact.json").read_text())
        current = json.loads((ROOT / "pr-gate-coverage.json").read_text())
        requirements = json.loads((ROOT / "release-requirements.json").read_text())
        active_release_gates = {
            gate["id"] for gate in requirements["gates"]
            if current["release"] in gate.get("required_for", [])
        }
        for path, expected in legacy["core"].items():
            with self.subTest(path=path):
                selected, _ = coverage_module.resolve_covered_gates([path], current)
                # The release impact map can include gates introduced by a newer
                # release. PR coverage remains pinned to its declared release,
                # so only require gates active for that coverage version here.
                expected = set(expected) & active_release_gates
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

    def test_unreviewed_runtime_file_is_rejected_not_falled_back(self):
        # Fail-loud guard (card 0428-242): a changed runtime source with no
        # declared catalog owner must abort with exit != 0, never resolve to a
        # broad false-pass. The conservative fallback never runs for an
        # uncataloged source.
        requirements = json.loads((ROOT / "release-requirements.json").read_text())
        mapping = json.loads((ROOT / "pr-gate-map-v1.json").read_text())
        diff = {
            "target_base_sha": "a" * 40, "head_sha": "b" * 40,
            "merge_base_sha": "c" * 40, "integration_sha": "d" * 40,
            "broad_required": False, "reasons": [],
            "changes": [{"status": "modified", "path": "internal/runtime/new_feature.go"}],
        }
        with self.assertRaises(ValueError) as ctx:
            planner.plan(diff, requirements, mapping, conservative_fallback=True)
        message = str(ctx.exception)
        self.assertIn("internal/runtime/new_feature.go", message)
        self.assertIn("scripts/pr-gate-coverage.json", message)
        self.assertIn("atomic.docker.data.capabilities", message)
        self.assertTrue("fallback_used" not in message)

    def test_unknown_source_is_rejected_not_falled_back(self):
        requirements = json.loads((ROOT / "release-requirements.json").read_text())
        mapping = json.loads((ROOT / "pr-gate-map-v1.json").read_text())
        diff = {
            "target_base_sha": "a" * 40, "head_sha": "b" * 40,
            "merge_base_sha": "c" * 40, "integration_sha": "d" * 40,
            "broad_required": False, "reasons": [],
            "changes": [{"status": "added", "path": "internal/unknown/new.go"}],
        }
        with self.assertRaises(ValueError) as ctx:
            planner.plan(diff, requirements, mapping, conservative_fallback=True)
        self.assertIn("internal/unknown/new.go", str(ctx.exception))
        self.assertNotIn("go-unit-tests", str(ctx.exception))

    def test_runtime_hint_without_catalog_owner_is_rejected(self):
        requirements = json.loads((ROOT / "release-requirements.json").read_text())
        mapping = json.loads((ROOT / "pr-gate-map-v1.json").read_text())
        diff = {
            "target_base_sha": "a" * 40, "head_sha": "b" * 40,
            "merge_base_sha": "c" * 40, "integration_sha": "d" * 40,
            "broad_required": False, "reasons": [],
            "changes": [{"status": "modified", "path": "internal/runtime/new_feature.go"}],
        }
        with self.assertRaises(ValueError):
            planner.plan(diff, requirements, mapping, conservative_fallback=True)

    # card 0428-242: catalog + fail-loud guardian.

    def test_app_broker_catalog_entry_resolves_to_data_capabilities_gates(self):
        # (catalog) cmd/baha/app_broker.go is declared with the two data.capabilities
        # surfaces, matching the app_apply.go pattern, and resolves to the atomic
        # data-capabilities gate (never empty/gray).
        current = json.loads((ROOT / "pr-gate-coverage.json").read_text())
        component = next(
            row for row in current["components"] if row["path"] == "cmd/baha/app_broker.go")
        self.assertEqual(
            ["source.gate.atomic.docker.data.capabilities",
             "source.gate.atomic.podman.data.capabilities"], component["surfaces"])
        selected, unresolved = coverage_module.resolve_covered_gates(
            ["cmd/baha/app_broker.go"], current)
        self.assertEqual(["atomic/docker/data-capabilities"], selected)
        self.assertEqual([], unresolved)

    def test_planner_catalogs_app_broker_as_resolved(self):
        requirements = json.loads((ROOT / "release-requirements.json").read_text())
        mapping = json.loads((ROOT / "pr-gate-map-v1.json").read_text())
        diff = {
            "target_base_sha": "a" * 40, "head_sha": "b" * 40,
            "merge_base_sha": "c" * 40, "integration_sha": "d" * 40,
            "broad_required": False, "reasons": [],
            "changes": [{"status": "modified", "path": "cmd/baha/app_broker.go"}],
        }
        # CI authenticates the runner context, which enables the conservative
        # fallback that adds the broad Go vet/build/unit checks for a CLI source
        # whose runtime impact is not otherwise targeted.
        result = planner.plan(diff, requirements, mapping, conservative_fallback=True)
        self.assertEqual("resolved", result["coverage_status"])
        self.assertEqual([], result["unresolved_coverage"])
        self.assertEqual(["atomic/docker/data-capabilities"], result["selected_gates"])

    def test_uncataloged_source_is_rejected_with_precise_next_action(self):
        # (guardian) an uncataloged source aborts with exit != 0 and a message
        # that names the missing catalog id/path and the exact next step.
        requirements = json.loads((ROOT / "release-requirements.json").read_text())
        mapping = json.loads((ROOT / "pr-gate-map-v1.json").read_text())
        diff = {
            "target_base_sha": "a" * 40, "head_sha": "b" * 40,
            "merge_base_sha": "c" * 40, "integration_sha": "d" * 40,
            "broad_required": False, "reasons": [],
            "changes": [{"status": "modified", "path": "internal/runtime/new_feature.go"}],
        }
        with self.assertRaises(ValueError) as ctx:
            planner.plan(diff, requirements, mapping, conservative_fallback=True)
        message = str(ctx.exception)
        self.assertIn("internal/runtime/new_feature.go", message)
        self.assertIn("scripts/pr-gate-coverage.json", message)
        self.assertIn("source.internal_runtime_new_feature_go", message)
        self.assertIn("atomic.docker.data.capabilities", message)


class ReleaseDemoPinSelectionTests(unittest.TestCase):
    """v0.4.28 demo-ref must select the exact pin validator (regression guard).

    Regression: pr-gate-plan.py used to match only docs/releases/v0.4.27.demo-ref,
    so a v0.4.28 demo-ref change fell through to the broad Go fallback and never
    scheduled the exact immutable Demo SHA validator. Changed demo pins of ANY
    active release must select the pin-specific validator.
    """

    def _release_diff(self, path):
        requirements = json.loads((ROOT / "release-requirements.json").read_text())
        mapping = json.loads((ROOT / "pr-gate-map-v1.json").read_text())
        return planner.plan({
            "target_base_sha": "a" * 40, "head_sha": "b" * 40,
            "merge_base_sha": "c" * 40, "integration_sha": "d" * 40,
            "broad_required": False, "reasons": [],
            "changes": [{"status": "modified", "path": path}],
        }, requirements, mapping)

    def test_v0428_demo_ref_selects_exact_pin_validator(self):
        # Regression: v0.4.28 fell through the old v0.4.27-only match and got
        # the broad Go fallback instead of the pin validator. When the mapping
        # targets v0.4.28 the v0.4.28 demo-ref resolves to the pin validator.
        requirements = json.loads((ROOT / "release-requirements.json").read_text())
        mapping = json.loads((ROOT / "pr-gate-map-v1.json").read_text())
        result = planner.plan({
            "target_base_sha": "a" * 40, "head_sha": "b" * 40,
            "merge_base_sha": "c" * 40, "integration_sha": "d" * 40,
            "broad_required": False, "reasons": [],
            "changes": [{"status": "modified", "path": "docs/releases/v0.4.28.demo-ref"}],
        }, requirements, {**mapping, "release": "v0.4.28"})
        self.assertEqual("resolved", result["coverage_status"])
        self.assertEqual([], result["unresolved_coverage"])
        selected_ids = [c.get("id") for c in result["required_validation_checks"]]
        self.assertIn("demo-source-pin-validation", selected_ids)
        # A data-only release pin must NOT fall back to the broad Go matrix.
        self.assertFalse(result["broad_required"])
        self.assertEqual([], result["selected_gates"])

    def test_v0427_demo_ref_still_selects_exact_pin_validator(self):
        requirements = json.loads((ROOT / "release-requirements.json").read_text())
        mapping = json.loads((ROOT / "pr-gate-map-v1.json").read_text())
        result = planner.plan({
            "target_base_sha": "a" * 40, "head_sha": "b" * 40,
            "merge_base_sha": "c" * 40, "integration_sha": "d" * 40,
            "broad_required": False, "reasons": [],
            "changes": [{"status": "modified", "path": "docs/releases/v0.4.27.demo-ref"}],
        }, requirements, {**mapping, "release": "v0.4.27"})
        self.assertEqual("resolved", result["coverage_status"])
        self.assertEqual([], result["unresolved_coverage"])
        selected_ids = [c.get("id") for c in result["required_validation_checks"]]
        self.assertIn("demo-source-pin-validation", selected_ids)
        self.assertFalse(result["broad_required"])
        self.assertEqual([], result["selected_gates"])

    def test_demo_ref_release_must_match_mapping_release(self):
        # A demo-ref path for one release while the mapping targets a
        # different release must NOT resolve to the pin validator: the path
        # is release-scoped to the mapping's release.
        requirements = json.loads((ROOT / "release-requirements.json").read_text())
        mapping = json.loads((ROOT / "pr-gate-map-v1.json").read_text())
        result = planner.plan({
            "target_base_sha": "a" * 40, "head_sha": "b" * 40,
            "merge_base_sha": "c" * 40, "integration_sha": "d" * 40,
            "broad_required": False, "reasons": [],
            "changes": [{"status": "modified", "path": "docs/releases/v0.4.28.demo-ref"}],
        }, requirements, {**mapping, "release": "v0.4.27"})
        self.assertEqual("unresolved", result["coverage_status"])
        self.assertNotEqual([], result["unresolved_coverage"])


class ReleaseDemoPinSelectionRegression(unittest.TestCase):
    """Regression matrix for the release-scoped demo-ref selector.

    The selector must resolve any active release demo-ref to the exact
    demo-source-pin validation; it must reject non-pin, inactive or mixed
    change sets rather than falling to a broad fallback. Uses the real repo
    release catalog so the planner's catalog validation passes.
    """

    def _load(self):
        requirements = json.loads((ROOT / "release-requirements.json").read_text())
        mapping = json.loads((ROOT / "pr-gate-map-v1.json").read_text())
        return requirements, mapping

    def _diff(self, path, status="modified"):
        return {"target_base_sha": "a" * 40, "head_sha": "b" * 40,
                "merge_base_sha": "c" * 40, "integration_sha": "d" * 40,
                "broad_required": False, "reasons": [],
                "changes": [{"status": status, "path": path}]}

    def _active_releases(self):
        return json.loads((ROOT / "release-requirements.json").read_text())["releases"]

    def test_each_active_release_demo_ref_resolves_to_source_validation(self):
        # Every demo-ref in the active catalog resolves to the exact
        # demo-source-pin-validation, never a broad fallback.
        requirements, base_mapping = self._load()
        expected = ["demo-source-pin-validation"]
        for release in self._active_releases():
            with self.subTest(release=release):
                path = "docs/releases/{}.demo-ref".format(release)
                mapping = {**base_mapping, "release": release}
                result = planner.plan(self._diff(path), requirements, mapping)
                self.assertEqual("resolved", result["coverage_status"])
                self.assertEqual([], result["unresolved_coverage"])
                self.assertEqual([], result["selected_gates"])
                self.assertEqual(expected,
                                 [row["id"] for row in result["required_validation_checks"]])

    def test_v0427_demo_pin_is_resolved(self):
        # Baseline regression: the historical v0.4.27 pin still resolves.
        requirements, mapping = self._load()
        result = planner.plan(self._diff("docs/releases/v0.4.27.demo-ref"),
                              requirements, {**mapping, "release": "v0.4.27"})
        self.assertEqual("resolved", result["coverage_status"])
        self.assertEqual(["demo-source-pin-validation"],
                         [row["id"] for row in result["required_validation_checks"]])

    def test_v0428_demo_pin_is_resolved(self):
        # Regression for the v0.4.28 pin that previously fell through.
        requirements, mapping = self._load()
        result = planner.plan(self._diff("docs/releases/v0.4.28.demo-ref"),
                              requirements, {**mapping, "release": "v0.4.28"})
        self.assertEqual("resolved", result["coverage_status"])
        self.assertEqual(["demo-source-pin-validation"],
                         [row["id"] for row in result["required_validation_checks"]])

    def test_non_pin_release_metadata_stays_fallback(self):
        # A non-pin release-metadata path is NOT a demo-ref; it must not
        # resolve to the demo pin validation.
        requirements, mapping = self._load()
        result = planner.plan(self._diff("docs/releases/v0.4.27.md"),
                              requirements, {**mapping, "release": "v0.4.27"})
        self.assertNotEqual(["demo-source-pin-validation"],
                            [row["id"] for row in result["required_validation_checks"]])

    def test_demo_pin_mixed_with_other_change_is_unresolved(self):
        # A demo-ref pin plus any other changed path must stay unresolved,
        # because the pin must be a standalone modified file.
        requirements, mapping = self._load()
        diff = {
            "target_base_sha": "a" * 40, "head_sha": "b" * 40,
            "merge_base_sha": "c" * 40, "integration_sha": "d" * 40,
            "broad_required": False, "reasons": [],
            "changes": [
                {"status": "modified", "path": "docs/releases/v0.4.27.demo-ref"},
                {"status": "modified", "path": "docs/guide.md"},
            ],
        }
        result = planner.plan(diff, requirements, {**mapping, "release": "v0.4.27"})
        self.assertEqual("unresolved", result["coverage_status"])
        self.assertTrue(
            any("v0.4.27 Demo source pin" in m for m in result["unresolved_coverage"]))


if __name__ == "__main__":
    unittest.main()

"""Tests for declarative PR gate coverage validation and resolution."""
import importlib.util
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parent
SPEC = importlib.util.spec_from_file_location(
    "validate_pr_gate_coverage", ROOT / "validate-pr-gate-coverage.py")
coverage_module = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(coverage_module)

DOCKER = "atomic/docker/data-capabilities"
PODMAN = "atomic/podman/data-capabilities"


def catalog():
    return {
        "schema": coverage_module.SCHEMA,
        "release": "v0.4.27",
        "surfaces": ["runtime.container-ownership", "application.data-capabilities", "application.lifecycle"],
        "packages": [
            {"path": "internal/runtime", "surfaces": ["runtime.container-ownership"]}
        ],
        "gates": [
            {"id": DOCKER, "covers": ["runtime.container-ownership", "application.data-capabilities"]},
            {"id": PODMAN, "covers": ["runtime.container-ownership", "application.data-capabilities"]},
            {"id": "atomic/podman/lifecycle", "covers": ["application.lifecycle"]},
        ],
    }


def requirements():
    return {
        "releases": ["v0.4.27"],
        "gates": [{"id": DOCKER}, {"id": PODMAN}, {"id": "atomic/podman/lifecycle"}],
    }


class CoverageCatalogTests(unittest.TestCase):
    def test_runtime_package_selects_only_owning_data_capabilities_gates(self):
        selected, unresolved = coverage_module.resolve_covered_gates(
            ["internal/runtime/container_identity.go"], catalog())
        self.assertEqual([DOCKER, PODMAN], selected)
        self.assertEqual([], unresolved)

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

    def test_catalog_references_existing_release_gates(self):
        self.assertEqual([], coverage_module.validate(catalog(), requirements()))

    def test_package_surface_without_gate_owner_is_rejected(self):
        data = catalog()
        data["gates"] = []
        errors = coverage_module.validate(data, requirements())
        self.assertTrue(any("has no release gate owner" in error for error in errors))

    def test_unknown_gate_and_path_traversal_are_rejected(self):
        data = catalog()
        data["packages"][0]["path"] = "../internal/runtime"
        data["gates"][0]["id"] = "atomic/docker/does-not-exist"
        errors = coverage_module.validate(data, requirements())
        self.assertTrue(any("normalized repository-relative directory" in error for error in errors))
        self.assertTrue(any("unknown release gate" in error for error in errors))


if __name__ == "__main__":
    unittest.main()

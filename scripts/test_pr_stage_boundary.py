"""PR stage separation never converts integration debt into qualified proof."""
import copy
import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location("stage", Path(__file__).with_name("pr-stage-boundary.py"))
stage = importlib.util.module_from_spec(spec)
spec.loader.exec_module(stage)


class PRStageBoundaryTests(unittest.TestCase):
    def setUp(self):
        self.plan = {
            "schema": "baseharbor.pr-gate-plan/v1",
            "head_sha": "a" * 40, "integration_sha": "b" * 40,
            "coverage_status": "resolved", "unresolved_coverage": [],
            "selected_gates": ["atomic/docker/lifecycle", "atomic/podman/lifecycle",
                               "integration/static/public-contracts"],
            "required_validation_checks": [{"id": "go-unit-tests"}],
            "required_consumer_dependencies": ["core", "demo", "console", "connector"]
        }

    def test_defer_all_cross_repo_consumer_gates_without_claiming_success(self):
        original = copy.deepcopy(self.plan)
        result = stage.stage(self.plan)
        self.assertEqual([], result["selected_gates"])
        self.assertEqual(original["selected_gates"], result["deferred_integration_gates"])
        self.assertEqual(original["required_consumer_dependencies"],
                         result["deferred_consumer_dependencies"])
        self.assertEqual(original["required_validation_checks"], result["required_validation_checks"])
        self.assertNotIn("integration_result", result)
        self.assertEqual(original, self.plan)

    def test_local_only_retains_mandatory_validations(self):
        self.plan["selected_gates"] = []
        self.assertEqual([], stage.stage(self.plan)["deferred_integration_gates"])

    def test_unresolved_coverage_cannot_be_laundered(self):
        self.plan.update(coverage_status="unresolved",
                         unresolved_coverage=["unknown changed path"])
        result = stage.stage(self.plan)
        self.assertEqual("unresolved", result["coverage_status"])
        self.assertEqual(["unknown changed path"], result["unresolved_coverage"])

    def test_failure_injection_rejects_unclassified_or_duplicate_gates(self):
        for gates in [["integration/unknown"], ["atomic/docker/lifecycle", "atomic/docker/lifecycle"],
                      ["atomic/static/lifecycle"], [None], ["atomic/docker"], "atomic/docker/lifecycle"]:
            with self.subTest(gates=gates):
                self.plan["selected_gates"] = gates
                with self.assertRaises(ValueError):
                    stage.stage(self.plan)

    def test_failure_injection_rejects_missing_checks_dependencies_and_double_stage(self):
        variants = [
            {"required_validation_checks": []},
            {"required_validation_checks": [{"id": None}]},
            {"required_consumer_dependencies": ["core", "unknown"]},
            {"required_consumer_dependencies": None},
            {"schema": "unknown"},
            {"deferred_integration_gates": []},
        ]
        for change in variants:
            with self.subTest(change=change):
                with self.assertRaises(ValueError):
                    stage.stage({**self.plan, **change})


if __name__ == "__main__":
    unittest.main()

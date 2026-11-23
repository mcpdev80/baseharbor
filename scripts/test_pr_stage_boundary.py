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
        self.assertEqual(["atomic/docker/lifecycle", "atomic/podman/lifecycle"],
                         result["selected_gates"])
        self.assertEqual(["integration/static/public-contracts"],
                         result["deferred_integration_gates"])
        self.assertEqual(original["required_consumer_dependencies"],
                         result["deferred_consumer_dependencies"])
        self.assertEqual(original["required_validation_checks"], result["required_validation_checks"])
        self.assertNotIn("integration_result", result)
        self.assertEqual(original, self.plan)

    def test_executed_guided_docker_gate_is_not_deferred(self):
        self.plan["selected_gates"] = ["atomic/docker/guided"]
        result = stage.stage(self.plan)
        self.assertEqual(["atomic/docker/guided"], result["selected_gates"])
        self.assertEqual([], result["deferred_integration_gates"])

    def test_local_only_retains_mandatory_validations(self):
        self.plan["selected_gates"] = []
        self.assertEqual([], stage.stage(self.plan)["deferred_integration_gates"])

    def test_coreless_integration_gates_remain_selected_for_pr_executor(self):
        gates=["integration/docker/coreless-lifecycle", "integration/podman/coreless-lifecycle"]
        self.plan["selected_gates"] = gates
        result=stage.stage(self.plan)
        self.assertEqual(gates, result["selected_gates"])
        self.assertEqual([], result["deferred_integration_gates"])

    def test_atomic_static_release_gates_are_pr_executed_not_unclassified(self):
        # Finding (#1746 run 45041976030): the trusted planner selects the
        # release-only atomic/static/shell-ux gate, but the stage boundary
        # classified it as an unclassified gate and aborted. The PR-executed
        # static class (atomic/static/shell-ux and, once #1746 lands,
        # atomic/static/ci-fast-check) must stay selected, must never be
        # deferred, and must leave native acceptance unchanged.
        for gate in ("atomic/static/shell-ux", "atomic/static/ci-fast-check"):
            with self.subTest(gate=gate):
                self.plan["selected_gates"] = [gate]
                result = stage.stage(self.plan)
                self.assertEqual([gate], result["selected_gates"])
                self.assertEqual([], result["deferred_integration_gates"])
        # atomic/static/mcp gained the same PR-executed class as its siblings:
        # it stays selected, is never deferred, and leaves native acceptance
        # unchanged (mirrors the shell-ux/ci-fast-check regression below).
        self.plan["selected_gates"] = ["atomic/static/mcp"]
        result = stage.stage(self.plan)
        self.assertEqual(["atomic/static/mcp"], result["selected_gates"])
        self.assertEqual([], result["deferred_integration_gates"])
        # Mixed with a native gate: the native selection/deferral behaviour is
        # asserted UNCHANGED (native stays selected, nothing deferred).
        self.plan["selected_gates"] = ["atomic/static/shell-ux",
                                       "atomic/docker/lifecycle",
                                       "atomic/podman/lifecycle"]
        result = stage.stage(self.plan)
        self.assertEqual(["atomic/static/shell-ux", "atomic/docker/lifecycle",
                          "atomic/podman/lifecycle"], result["selected_gates"])
        self.assertEqual([], result["deferred_integration_gates"])

    def test_other_atomic_static_names_stay_unclassified(self):
        # Only the enumerated PR-executed static gates gained a class; the
        # failure-injection list for invented atomic/static gates stands.
        for gate in ["atomic/static/lifecycle", "atomic/static/config-matrix",
                     "atomic/static/init"]:
            with self.subTest(gate=gate):
                self.plan["selected_gates"] = [gate]
                with self.assertRaises(ValueError):
                    stage.stage(self.plan)

    def test_only_coreless_lifecycle_gates_stay_selected(self):
        coreless = ["integration/docker/coreless-lifecycle",
                    "integration/podman/coreless-lifecycle"]
        deferred = ["integration/static/public-contracts"]
        self.plan["selected_gates"] = coreless + deferred
        result = stage.stage(self.plan)
        self.assertEqual(coreless, result["selected_gates"])
        self.assertEqual(deferred, result["deferred_integration_gates"])

    def test_unresolved_coverage_cannot_be_laundered(self):
        self.plan.update(coverage_status="unresolved",
                         unresolved_coverage=["unknown changed path"])
        result = stage.stage(self.plan)
        self.assertEqual("unresolved", result["coverage_status"])
        self.assertEqual(["unknown changed path"], result["unresolved_coverage"])

    def test_failure_injection_rejects_unclassified_or_duplicate_gates(self):
        for gates in [["integration/unknown"], ["integration/docker/unknown"],
                      ["atomic/docker/lifecycle", "atomic/docker/lifecycle"],
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

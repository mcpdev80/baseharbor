"""Consumer follow-up notices are derived from the trusted PR plan."""
import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location("notice", Path(__file__).with_name("pr-consumer-followup.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class NoticeTests(unittest.TestCase):
    def setUp(self):
        self.plan = {
            "schema": "baseharbor.pr-gate-plan/v1",
            "deferred_consumer_dependencies": ["core", "demo", "connector"],
            "deferred_integration_gates": ["atomic/docker/lifecycle"],
        }

    def test_only_named_consumers_are_shown(self):
        message, affected, gates = module.notice(self.plan)
        self.assertEqual(["demo", "connector"], affected)
        self.assertIn("baseharbor-demo", message)
        self.assertIn("baseharbor-node-connector", message)
        self.assertNotIn("baseharbor-console", message)
        self.assertIn("not a PR merge blocker", message)
        self.assertEqual(["atomic/docker/lifecycle"], gates)

    def test_no_followup_without_dependencies_or_gates(self):
        self.plan["deferred_consumer_dependencies"] = ["core"]
        self.plan["deferred_integration_gates"] = []
        message, affected, gates = module.notice(self.plan)
        self.assertEqual(([], []), (affected, gates))
        self.assertIn("No cross-repository follow-up", message)

    def test_unclassified_deferred_gate_is_still_reported(self):
        self.plan["deferred_consumer_dependencies"] = ["core"]
        message, affected, _ = module.notice(self.plan)
        self.assertEqual([], affected)
        self.assertIn("Deferred integration gates", message)

    def test_missing_or_untrusted_metadata_is_not_silently_ignored(self):
        for modification in (
            {"deferred_consumer_dependencies": None},
            {"deferred_consumer_dependencies": ["core", "unknown"]},
            {"deferred_consumer_dependencies": ["demo", "demo"]},
            {"deferred_integration_gates": None},
            {"schema": "other"},
        ):
            with self.subTest(modification=modification), self.assertRaises(ValueError):
                module.notice({**self.plan, **modification})


if __name__ == "__main__":
    unittest.main()

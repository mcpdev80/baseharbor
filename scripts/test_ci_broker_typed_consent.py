"""Exercise the broker fixture's no-impact and consent boundaries."""
import importlib.util
import unittest
from pathlib import Path


spec = importlib.util.spec_from_file_location(
    "broker_consent", Path(__file__).with_name("ci-broker-typed-consent.py"))
helper = importlib.util.module_from_spec(spec)
spec.loader.exec_module(helper)


def plan(**updates):
    value = {"version": "baseharbor.provider-plan/v1", "target": "ci", "complete": True,
             "consumers": {}, "required": [], "released_bindings": [], "changes": []}
    value.update(updates)
    return {"provider_plan": value}


class BrokerConsentTests(unittest.TestCase):
    def test_complete_non_mutating_plan_needs_no_synthetic_consent(self):
        self.assertIsNone(helper.consent_from_plan(plan()))

    def test_no_impact_does_not_fabricate_consent_for_incomplete_plan(self):
        self.assertIsNone(helper.consent_from_plan(plan(complete=False, impact=None)))

    def test_invalid_plans_are_rejected(self):
        for value in (plan(complete=None), plan(version="unknown"), plan(target=""),
                      plan(changes=None), plan(changes=[{"action": "add"}]), {}):
            with self.subTest(value=value), self.assertRaises(ValueError):
                helper.consent_from_plan(value)

    def test_present_impact_uses_existing_non_destructive_consent_validation(self):
        impact = {"version": "baseharbor.provider-impact/v1", "plan_id": "fresh",
                  "confirmations_required": 2, "destructive": False}
        self.assertEqual("fresh", helper.consent_from_plan(plan(impact=impact))["plan_id"])
        impact["destructive"] = True
        with self.assertRaises(ValueError):
            helper.consent_from_plan(plan(impact=impact))


if __name__ == "__main__":
    unittest.main()

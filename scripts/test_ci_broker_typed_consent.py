"""Exercise the broker fixture's no-impact and consent boundaries."""
import importlib.util
import unittest
from pathlib import Path


spec = importlib.util.spec_from_file_location(
    "broker_consent", Path(__file__).with_name("ci-broker-typed-consent.py"))
helper = importlib.util.module_from_spec(spec)
spec.loader.exec_module(helper)


def plan(**updates):
    value = {"version": "baseharbor.provider-plan/v1", "complete": True,
             "changes": [{"action": "reuse"}, {"action": "preserve"}]}
    value.update(updates)
    return {"provider_plan": value}


class BrokerConsentTests(unittest.TestCase):
    def test_complete_non_mutating_plan_needs_no_synthetic_consent(self):
        self.assertIsNone(helper.consent_from_plan(plan()))

    def test_missing_impact_never_hides_incomplete_or_mutating_plan(self):
        for value in (plan(complete=False), plan(version="unknown"),
                      plan(changes=None), plan(changes=[{"action": "add"}]),
                      plan(impact=None), {}):
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

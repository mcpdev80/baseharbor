"""Fail-closed regressions for the existing Podman native acceptance helper."""
import importlib.util
import json
from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("podman_consent", ROOT / "ci-podman-typed-consent.py")
helper = importlib.util.module_from_spec(spec)
spec.loader.exec_module(helper)


def plan(**changes):
    impact = dict(version="baseharbor.provider-impact/v1", plan_id="fresh-plan-987",
                  confirmations_required=2, destructive=False, complete=False)
    impact.update(changes)
    return {"provider_plan": {"impact": impact}}


class PodmanTypedConsentTests(unittest.TestCase):
    def test_fresh_two_stage_consent_preserves_incomplete_inventory(self):
        self.assertEqual(
            {"plan_id": "fresh-plan-987", "first": True, "second": True, "delete_data": False},
            helper.consent_from_plan(plan()))

    def test_single_stage_only_if_required_by_plan(self):
        self.assertEqual(False, helper.consent_from_plan(
            plan(confirmations_required=1))["second"])

    def test_missing_invalid_or_destructive_impact_blocks(self):
        invalid = [None, {}, {"provider_plan": {}},
                   {"provider_plan": {"impact": None}},
                   plan(plan_id=""), plan(plan_id=None),
                   plan(confirmations_required=0),
                   plan(confirmations_required=3),
                   plan(confirmations_required=True),
                   plan(destructive=None), plan(destructive=True),
                   plan(version="unknown"), plan(plan_id="x" * 513)]
        for candidate in invalid:
            with self.subTest(candidate=candidate):
                with self.assertRaises(ValueError):
                    helper.consent_from_plan(candidate)

    def test_workflow_uses_fresh_json_plan_and_limits_consent_to_apply(self):
        workflow = (ROOT.parent / ".github/workflows/podman-acceptance.yml").read_text()
        self.assertIn("app plan podman-ci -o json > \"$plan_file\"", workflow)
        self.assertIn('consent="$(python3 scripts/ci-podman-typed-consent.py "$plan_file")"', workflow)
        self.assertIn('BASEHARBOR_PROVIDER_CHANGE_CONSENT="$consent" timeout 180s', workflow)
        self.assertIn("app preflight podman-ci", workflow)
        self.assertIn("authenticated SELECT 1 succeeded", workflow)
        self.assertIn("semantic Valkey verification passed", workflow)
        self.assertNotIn("--skip-consent", workflow)


if __name__ == "__main__":
    unittest.main()

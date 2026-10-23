import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from r27_evidence_coverage import plan

SHA = "a" * 40


class CoverageTests(unittest.TestCase):
    def test_reuses_authenticated_coverage(self):
        decision = plan(["sql", "cli", "sql"], ["sql"], stage="pr", exact_candidate=SHA)
        self.assertEqual(decision.state, "WAITING")
        self.assertEqual(decision.reusable, ("sql",))
        self.assertEqual(decision.pending, ("cli",))
        self.assertEqual(decision.code, "EVIDENCE_GATES_PENDING")

    def test_release_requires_smoke(self):
        with self.assertRaisesRegex(ValueError, "EVIDENCE_SMOKE"):
            plan(["sql"], ["sql"], stage="release", exact_candidate=SHA)
        decision = plan(["sql", "release/smoke"], ["sql"], stage="release", exact_candidate=SHA)
        self.assertEqual(decision.pending, ("release/smoke",))
        self.assertNotEqual(decision.state, "PASS")

    def test_release_complete(self):
        decision = plan(["sql", "release/smoke"], ["sql", "release/smoke"],
                        stage="release", exact_candidate=SHA)
        self.assertEqual(decision.state, "WAITING")\n        self.assertEqual(decision.pending, ("release/smoke",))

    def test_trust_failure_blocks_even_when_covered(self):
        decision = plan(["sql"], ["sql"], stage="integration", exact_candidate=SHA, trusted=False)
        self.assertEqual(decision.state, "BLOCKED")
        self.assertEqual(decision.code, "EVIDENCE_TRUST")

    def test_failure_is_actionable(self):
        decision = plan(["sql", "cli"], ["sql"], failed=["sql"],
                        stage="pr", exact_candidate=SHA)
        self.assertEqual(decision.state, "ACTION_REQUIRED")
        self.assertIn("sql", decision.summary)
        self.assertEqual(decision.pending, ("cli", "sql"))

    def test_unknown_coverage_fails_closed(self):
        with self.assertRaisesRegex(ValueError, "EVIDENCE_INPUT"):
            plan(["sql"], ["unknown"], stage="pr", exact_candidate=SHA)

    def test_invalid_candidate_fails_closed(self):
        with self.assertRaisesRegex(ValueError, "EVIDENCE_TARGET"):
            plan(["sql"], [], stage="pr", exact_candidate="wrong")


if __name__ == "__main__":
    unittest.main()

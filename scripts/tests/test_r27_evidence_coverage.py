import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from r27_evidence_coverage import GateProof, plan

SHA = "a" * 40
OLD = "c" * 40
POLICY = "b" * 64
ARTIFACT = "d" * 64


def proof(gate, candidate=SHA, policy=POLICY, artifact=None, authenticated=True):
    return GateProof(gate, candidate, policy, artifact, authenticated)


class CoverageTests(unittest.TestCase):
    def test_only_missing_gates(self):
        d = plan(["sql", "docs"], [proof("sql")], stage="pr",
                 candidate=SHA, policy_digest=POLICY)
        self.assertEqual((d.state, d.reusable, d.pending),
                         ("WAITING", (), ("docs", "sql")))

    def test_stale_or_untrusted_cannot_pass(self):
        for p in [proof("sql", OLD), proof("sql", policy="f" * 64),
                  proof("sql", authenticated=False)]:
            with self.subTest(p=p):
                d = plan(["sql"], [p], stage="integration",
                         candidate=SHA, policy_digest=POLICY)
                self.assertEqual(d.state, "WAITING")

    def test_release_smoke_bound_to_artifact(self):
        required = ["sql", "release/smoke"]
        for p in [proof("release/smoke", artifact="e" * 64),
                  proof("release/smoke", OLD, artifact=ARTIFACT)]:
            d = plan(required, [proof("sql"), p], stage="release",
                     candidate=SHA, policy_digest=POLICY, artifact_digest=ARTIFACT)
            self.assertEqual(d.pending, ("release/smoke", "sql"))
        d = plan(required, [proof("sql"), proof("release/smoke", artifact=ARTIFACT)],
                 stage="release", candidate=SHA, policy_digest=POLICY,
                 artifact_digest=ARTIFACT)
        self.assertEqual((d.state, d.pending), ("WAITING", ("release/smoke", "sql")))

    def test_release_without_smoke_or_artifact_rejected(self):
        with self.assertRaisesRegex(ValueError, "EVIDENCE_ARTIFACT"):
            plan(["release/smoke"], [], stage="release", candidate=SHA,
                 policy_digest=POLICY)
        with self.assertRaisesRegex(ValueError, "EVIDENCE_SMOKE"):
            plan(["sql"], [], stage="release", candidate=SHA,
                 policy_digest=POLICY, artifact_digest=ARTIFACT)

    def test_failed_gate_and_trust(self):
        args = dict(stage="pr", candidate=SHA, policy_digest=POLICY)
        d = plan(["sql", "docs"], [proof("sql")], failed=["sql"], **args)
        self.assertEqual((d.state, d.failed, d.pending),
                         ("ACTION_REQUIRED", ("sql",), ("docs",)))
        self.assertEqual(plan(["sql"], [proof("sql")],
                              trust_available=False, **args).state, "BLOCKED")

    def test_caller_claimed_authentication_never_skips_gate(self):
        for claimed in (True, False):
            d = plan(["sql"], [proof("sql", authenticated=claimed)],
                     stage="pr", candidate=SHA, policy_digest=POLICY)
            self.assertEqual((d.state, d.reusable, d.pending),
                             ("WAITING", (), ("sql",)))

    def test_unknown_gate_rejected(self):
        with self.assertRaisesRegex(ValueError, "EVIDENCE_INPUT"):
            plan(["sql"], [proof("other")], stage="pr",
                 candidate=SHA, policy_digest=POLICY)


if __name__ == "__main__":
    unittest.main()

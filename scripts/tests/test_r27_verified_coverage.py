import hashlib
import hmac
import json
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from r27_verified_coverage import EvidenceError, verify, SCHEMA

KEY = b"k" * 32
SHA = "a" * 40
POLICY = "b" * 64
ARTIFACT = "c" * 64


def signed(**changes):
    body = dict(schema=SCHEMA, stage="release", candidate=SHA,
                policy_digest=POLICY, gate="release/smoke",
                artifact_digest=ARTIFACT, result="PASS",
                run_id=123, attempt=1, job_id=456)
    body.update(changes)
    mac = hmac.new(KEY, json.dumps(body, sort_keys=True,
                   separators=(",", ":")).encode(), hashlib.sha256).hexdigest()
    return json.dumps({**body, "mac": mac}).encode()


def check(raw, **changes):
    args = dict(key=KEY, stage="release", candidate=SHA,
                policy_digest=POLICY, gate="release/smoke",
                artifact_digest=ARTIFACT)
    args.update(changes)
    return verify(raw, **args)


class VerifiedCoverageTests(unittest.TestCase):
    def test_exact_signed_gate_accepted(self):
        self.assertEqual(check(signed())["job_id"], 456)

    def test_tampering_rejected(self):
        obj = json.loads(signed())
        obj["result"] = "PASS "
        with self.assertRaisesRegex(EvidenceError, "EVIDENCE_RESULT"):
            check(json.dumps(obj).encode())
        obj = json.loads(signed())
        obj["candidate"] = "d" * 40
        with self.assertRaisesRegex(EvidenceError, "EVIDENCE_SIGNATURE"):
            check(json.dumps(obj).encode())

    def test_scope_and_artifact_rejected(self):
        for args in (dict(candidate="d" * 40), dict(policy_digest="d" * 64),
                     dict(gate="sql"), dict(artifact_digest="d" * 64),
                     dict(stage="pr")):
            with self.subTest(args=args), self.assertRaises(EvidenceError):
                check(signed(), **args)

    def test_origin_and_untrusted_key_rejected(self):
        for bad in (0, True, "123", -1):
            with self.subTest(bad=bad), self.assertRaisesRegex(EvidenceError, "EVIDENCE_ORIGIN"):
                check(signed(run_id=bad))
        with self.assertRaisesRegex(EvidenceError, "EVIDENCE_TRUST"):
            check(signed(), key=b"weak")

    def test_empty_and_non_string_result_rejected(self):
        with self.assertRaisesRegex(EvidenceError, "EVIDENCE_SIZE"):
            check(b"")
        for value in (None, True, 1, ["PASS"]):
            with self.subTest(value=value), self.assertRaisesRegex(
                    EvidenceError, "EVIDENCE_RESULT"):
                check(signed(result=value))

    def test_non_dict_json_rejected(self):
        for raw in (b"[]", b"null", b"42", b'"hello"'):
            with self.subTest(raw=raw), self.assertRaisesRegex(
                    EvidenceError, "EVIDENCE_SCHEMA"):
                check(raw)

    def test_duplicate_fields_and_large_payload_rejected(self):
        with self.assertRaisesRegex(EvidenceError, "EVIDENCE_SCHEMA"):
            check(signed().replace(b'"result": "PASS"', b'"result": "PASS", "result": "PASS"'))
        with self.assertRaisesRegex(EvidenceError, "EVIDENCE_SIZE"):
            check(b" " * 4097)

    def test_rejected_signed_failure(self):
        with self.assertRaisesRegex(EvidenceError, "EVIDENCE_RESULT"):
            check(signed(result="FAIL"))

    def test_cross_key_signature_rejected(self):
        with self.assertRaisesRegex(EvidenceError, "EVIDENCE_SIGNATURE"):
            check(signed(), key=b"z" * 32)

    def test_signed_gate_mismatch_rejected(self):
        with self.assertRaisesRegex(EvidenceError, "EVIDENCE_TARGET"):
            check(signed(gate="sql"))

    def test_signed_policy_and_candidate_mismatch_rejected(self):
        for changes, code in ((dict(policy_digest="d" * 64), "EVIDENCE_POLICY"),
                              (dict(candidate="d" * 40), "EVIDENCE_TARGET")):
            with self.subTest(changes=changes), self.assertRaisesRegex(EvidenceError, code):
                check(signed(**changes))

    def test_every_release_gate_binds_artifact(self):
        for gate in ("sql", "release/smoke"):
            with self.subTest(gate=gate):
                self.assertEqual(check(signed(gate=gate), gate=gate)["gate"], gate)
                with self.assertRaisesRegex(EvidenceError, "EVIDENCE_ARTIFACT"):
                    check(signed(gate=gate, artifact_digest="d" * 64), gate=gate)

    def test_smoke_not_reusable_as_pr_or_integration(self):
        for stage in ("pr", "integration"):
            with self.subTest(stage=stage), self.assertRaisesRegex(
                    EvidenceError, "EVIDENCE_ARTIFACT"):
                check(signed(stage=stage), stage=stage)

    def test_nonrelease_artifact_scope_is_enforced_when_supplied(self):
        for stage in ("pr", "integration"):
            with self.subTest(stage=stage):
                raw = signed(stage=stage, gate="sql")
                with self.assertRaisesRegex(EvidenceError, "EVIDENCE_ARTIFACT"):
                    check(raw, stage=stage, gate="sql", artifact_digest=None)
                self.assertEqual(
                    check(raw, stage=stage, gate="sql",
                          artifact_digest=ARTIFACT)["job_id"], 456)
                with self.assertRaisesRegex(EvidenceError, "EVIDENCE_ARTIFACT"):
                    check(raw, stage=stage, gate="sql", artifact_digest="d" * 64)

    def test_nonfinite_receipt_rejected_at_parser_boundary(self):
        raw = signed()
        for constant in (b"NaN", b"Infinity", b"-Infinity"):
            with self.subTest(constant=constant):
                mutated = raw.replace(b'"attempt":1', b'"attempt":' + constant)
                self.assertNotEqual(mutated, raw)
                with self.assertRaisesRegex(EvidenceError, "EVIDENCE_SCHEMA"):
                    check(mutated)

    def test_invalid_expected_scope_rejected_before_comparison(self):
        cases = [
            ({"stage": []}, "EVIDENCE_SCHEMA"),
            ({"candidate": None}, "EVIDENCE_TARGET"),
            ({"policy_digest": 42}, "EVIDENCE_POLICY"),
            ({"gate": ["sql"]}, "EVIDENCE_GATE"),
            ({"artifact_digest": False}, "EVIDENCE_ARTIFACT"),
        ]
        for changed, code in cases:
            with self.subTest(changed=changed), self.assertRaisesRegex(EvidenceError, code):
                check(signed(), **changed)

    def test_missing_release_artifact_rejected(self):
        with self.assertRaisesRegex(EvidenceError, "EVIDENCE_ARTIFACT"):
            check(signed(), artifact_digest=None)


if __name__ == "__main__":
    unittest.main()

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

    def test_duplicate_fields_and_large_payload_rejected(self):
        with self.assertRaisesRegex(EvidenceError, "EVIDENCE_SCHEMA"):
            check(signed().replace(b'"result": "PASS"', b'"result": "PASS", "result": "PASS"'))
        with self.assertRaisesRegex(EvidenceError, "EVIDENCE_SIZE"):
            check(b" " * 4097)

    def test_rejected_signed_failure(self):
        with self.assertRaisesRegex(EvidenceError, "EVIDENCE_RESULT"):
            check(signed(result="FAIL"))

    def test_missing_release_artifact_rejected(self):
        with self.assertRaisesRegex(EvidenceError, "EVIDENCE_ARTIFACT"):
            check(signed(), artifact_digest=None)


if __name__ == "__main__":
    unittest.main()

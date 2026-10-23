import json
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from r27_evidence_receipt import ReceiptError, issue, verify

KEY = b"k" * 32
SHA = "a" * 40
POLICY = "b" * 64


class ReceiptTests(unittest.TestCase):
    def test_offline_roundtrip_all_stages(self):
        for kind in ("pr", "integration", "release"):
            with self.subTest(kind=kind):
                raw = issue(kind, SHA, POLICY, KEY)
                self.assertLess(len(raw), 1024)
                self.assertTrue(verify(raw, kind, SHA, POLICY, KEY))
                self.assertNotIn(b"run_id", raw)

    def test_wrong_commit_policy_or_kind(self):
        raw = issue("pr", SHA, POLICY, KEY)
        for kind, sha, policy in [
            ("release", SHA, POLICY), ("pr", "c" * 40, POLICY),
            ("pr", SHA, "d" * 64)
        ]:
            with self.assertRaises(ReceiptError):
                verify(raw, kind, sha, policy, KEY)

    def test_forged_pass_fails(self):
        raw = issue("pr", SHA, POLICY, KEY)
        obj = json.loads(raw)
        obj["commit"] = "c" * 40
        with self.assertRaisesRegex(ReceiptError, "EVIDENCE_SIGNATURE"):
            verify(json.dumps(obj).encode(), "pr", "c" * 40, POLICY, KEY)

    def test_wrong_key_fails(self):
        with self.assertRaisesRegex(ReceiptError, "EVIDENCE_SIGNATURE"):
            verify(issue("pr", SHA, POLICY, KEY), "pr", SHA, POLICY, b"z" * 32)

    def test_untrusted_inputs_rejected(self):
        for kind, sha, policy in [
            ("unknown", SHA, POLICY), ("pr", "short", POLICY),
            ("pr", SHA, "not-a-digest")
        ]:
            with self.assertRaises(ReceiptError):
                issue(kind, sha, policy, KEY)
        with self.assertRaises(ReceiptError):
            issue("pr", SHA, POLICY, b"weak")

    def test_duplicate_keys_rejected(self):
        raw = issue("pr", SHA, POLICY, KEY).decode()
        duplicated = raw.replace('"result":"PASS"', '"result":"PASS","result":"PASS"')
        with self.assertRaisesRegex(ReceiptError, "EVIDENCE_SCHEMA"):
            verify(duplicated.encode(), "pr", SHA, POLICY, KEY)

    def test_size_bound(self):
        with self.assertRaisesRegex(ReceiptError, "EVIDENCE_SIZE"):
            verify(b" " * 4097, "pr", SHA, POLICY, KEY)


if __name__ == "__main__":
    unittest.main()

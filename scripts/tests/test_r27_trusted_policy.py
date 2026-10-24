import hashlib,json,sys,unittest
from pathlib import Path
sys.path.insert(0,str(Path(__file__).resolve().parents[1]))
from r27_trusted_policy import load_pinned_policy
from r27_verified_coverage import EvidenceError

class ProtectedPolicyTests(unittest.TestCase):
 def test_valid_canonical_policy(self):
  raw=b'{"sql":{"job_name":"PR Selected Checks","workflow_path":".github/workflows/ci.yml"}}'
  mapping,digest=load_pinned_policy(raw,protected_digest=hashlib.sha256(raw).hexdigest())
  self.assertEqual(mapping["sql"]["job_name"],"PR Selected Checks")
 def test_untrusted_modified_bytes_rejected(self):
  raw=b'{"sql":{}}'
  with self.assertRaisesRegex(EvidenceError,"source mismatch"):
   load_pinned_policy(raw,protected_digest="a"*64)
 def test_missing_pin_rejected(self):
  with self.assertRaisesRegex(EvidenceError,"input unavailable"):
   load_pinned_policy(b"{}",protected_digest=None)
 def test_noncanonical_rejected(self):
  raw=b'{ "sql":{} }'
  with self.assertRaisesRegex(EvidenceError,"noncanonical"):
   load_pinned_policy(raw,protected_digest=hashlib.sha256(raw).hexdigest())


class PolicyIssuerBoundaryTests(unittest.TestCase):
 def test_caller_cannot_override_policy_or_job(self):
  from r27_trusted_policy import issue_with_pinned_policy
  raw=b'{"sql":{"job_name":"PR Selected Checks","workflow_path":".github/workflows/ci.yml"}}'
  pin=hashlib.sha256(raw).hexdigest()
  for arg in ("policy_jobs","policy_digest","trusted_policy_digest",
              "expected_job_name","workflow_path"):
   with self.subTest(arg=arg):
    with self.assertRaisesRegex(EvidenceError,"override forbidden"):
     issue_with_pinned_policy(policy_bytes=raw,protected_digest=pin,
                              gate="sql",**{arg:"fake"})
 def test_issuer_called_only_with_protected_policy(self):
  from r27_trusted_policy import issue_with_pinned_policy
  from unittest.mock import patch
  raw=b'{"sql":{"job_name":"PR Selected Checks","workflow_path":".github/workflows/ci.yml"}}'
  pin=hashlib.sha256(raw).hexdigest()
  with patch("r27_verified_issuer.issue_from_github",return_value=b"receipt") as issuer:
   result=issue_with_pinned_policy(policy_bytes=raw,protected_digest=pin,gate="sql")
  self.assertEqual(result,b"receipt")
  self.assertEqual(issuer.call_args.kwargs["policy_digest"],pin)
  self.assertEqual(issuer.call_args.kwargs["expected_job_name"],"PR Selected Checks")

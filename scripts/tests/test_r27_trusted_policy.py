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

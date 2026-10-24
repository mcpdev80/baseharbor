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


class ProtectedPolicySchemaTests(unittest.TestCase):
 def test_invalid_mapping_rejected_at_loader_boundary(self):
  for mapping in ({"sql":{}},{"sql":{"job_name":"a","workflow_path":"../../evil.yml"}},
      {"sql":{"job_name":"a\nwrong","workflow_path":".github/workflows/ci.yml"}},
      {"sql":{"job_name":"ok","workflow_path":".github/workflows/ci.yml","extra":1}}):
   raw=json.dumps(mapping,sort_keys=True,separators=(",",":")).encode()
   with self.subTest(mapping=mapping),self.assertRaisesRegex(EvidenceError,"invalid protected gate mapping"):
    load_pinned_policy(raw,protected_digest=hashlib.sha256(raw).hexdigest())


class DuplicatePolicyKeyTests(unittest.TestCase):
 def test_duplicate_gate_or_job_property_is_rejected(self):
  for raw in (
      b'{"sql":{"job_name":"Job","workflow_path":".github/workflows/ci.yml"},"sql":{"job_name":"Other","workflow_path":".github/workflows/other.yml"}}',
      b'{"sql":{"job_name":"Job","job_name":"Other","workflow_path":".github/workflows/ci.yml"}}'
  ):
   with self.subTest(raw=raw), self.assertRaisesRegex(EvidenceError,"duplicate protected key"):
    load_pinned_policy(raw,protected_digest=hashlib.sha256(raw).hexdigest())


class FileReaderTests(unittest.TestCase):
 def test_rejects_symlink_and_relative_file(self):
  import tempfile
  from r27_trusted_policy import read_pinned_policy_file
  with tempfile.TemporaryDirectory() as d:
   source=Path(d)/"policy.json"
   source.write_bytes(b"{}")
   link=Path(d)/"link.json"
   link.symlink_to(source)
   for candidate in ("policy.json",str(link)):
    with self.subTest(candidate=candidate), self.assertRaisesRegex(EvidenceError,"protected path|cannot read"):
     read_pinned_policy_file(candidate,"a"*64)
 def test_canonical_file_roundtrip(self):
  import tempfile
  from r27_trusted_policy import read_pinned_policy_file
  data=b'{"sql":{"job_name":"PR Selected Checks","workflow_path":".github/workflows/ci.yml"}}'
  with tempfile.TemporaryDirectory() as d:
   file=Path(d)/"policy.json"
   file.write_bytes(data)
   mapping,pin=read_pinned_policy_file(str(file),hashlib.sha256(data).hexdigest())
   self.assertEqual(mapping["sql"]["job_name"],"PR Selected Checks")

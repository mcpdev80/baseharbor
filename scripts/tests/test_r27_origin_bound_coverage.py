import hashlib,hmac,json,sys,unittest
from pathlib import Path
from unittest.mock import patch
sys.path.insert(0,str(Path(__file__).resolve().parents[1]))
from r27_origin_bound_coverage import check_origin_bound_coverage
from r27_github_origin import OriginError
from r27_verified_coverage import SCHEMA
SHA="a"*40
POLICY="b"*64
KEY=b"k"*32
JOBS={"sql":{"workflow_path":".github/workflows/ci.yml","job_name":"PR Selected Checks"}}
def receipt(gate="sql",job_id=5):
 body=dict(schema=SCHEMA,stage="pr",candidate=SHA,policy_digest=POLICY,gate=gate,
           artifact_digest=None,result="PASS",run_id=3,attempt=2,job_id=job_id)
 mac=hmac.new(KEY,json.dumps(body,sort_keys=True,separators=(",",":")).encode(),hashlib.sha256).hexdigest()
 return json.dumps(dict(body,mac=mac)).encode()
ARGS=dict(key=KEY,token="placeholder",repository="mcpdev80/baseharbor",
          stage="pr",candidate=SHA,policy_digest=POLICY)
class OriginBoundCoverageTests(unittest.TestCase):
 def test_complete_correlated_job_and_receipt(self):
  with patch("r27_origin_bound_coverage.fetch_authenticated_origin") as fetch:
   result=check_origin_bound_coverage(["sql"],{"sql":receipt()},JOBS,**ARGS)
  self.assertEqual(result.state,"READY_FOR_TRUSTED_ISSUER")
  self.assertEqual(fetch.call_args.kwargs["attempt"],2)
  self.assertEqual(fetch.call_args.kwargs["job_id"],5)
  self.assertEqual(fetch.call_args.kwargs["expected_event"],"pull_request")
 def test_missing_receipt_remains_pending_without_fetch(self):
  with patch("r27_origin_bound_coverage.fetch_authenticated_origin") as fetch:
   result=check_origin_bound_coverage(["sql"],{},JOBS,**ARGS)
  self.assertEqual(result.pending,("sql",))
  fetch.assert_not_called()
 def test_invalid_signature_never_fetches(self):
  raw=receipt().replace(b'"job_id": 5',b'"job_id": 6')
  with patch("r27_origin_bound_coverage.fetch_authenticated_origin") as fetch:
   with self.assertRaises(ValueError):
    check_origin_bound_coverage(["sql"],{"sql":raw},JOBS,**ARGS)
  fetch.assert_not_called()
 def test_failed_origin_blocks_coverage(self):
  with patch("r27_origin_bound_coverage.fetch_authenticated_origin",side_effect=OriginError("EVIDENCE_ORIGIN: wrong job")):
   with self.assertRaisesRegex(OriginError,"wrong job"):
    check_origin_bound_coverage(["sql"],{"sql":receipt()},JOBS,**ARGS)
 def test_incomplete_policy_never_fetches(self):
  with patch("r27_origin_bound_coverage.fetch_authenticated_origin") as fetch:
   with self.assertRaisesRegex(OriginError,"incomplete"):
    check_origin_bound_coverage(["sql"],{"sql":receipt()}, {},**ARGS)
  fetch.assert_not_called()

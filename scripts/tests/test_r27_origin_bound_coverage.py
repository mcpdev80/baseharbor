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
ARGS["policy_digest"]=hashlib.sha256(json.dumps(JOBS,sort_keys=True,separators=(",",":")).encode()).hexdigest()
POLICY=ARGS["policy_digest"]
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


class PolicyPreflightTests(unittest.TestCase):
 def test_invalid_missing_gate_policy_never_fetches(self):
  for mapping in ({}, {"workflow_path":"not-a-workflow","job_name":"job"},
                  {"workflow_path":".github/workflows/ci.yml","job_name":" "},
                  {"workflow_path":".github/workflows/ci.yml","job_name":"job","unexpected":1}):
   policy=dict(JOBS, docs=mapping)
   with self.subTest(mapping=mapping), patch("r27_origin_bound_coverage.fetch_authenticated_origin") as fetch:
    with self.assertRaisesRegex(OriginError,"invalid job mapping"):
     check_origin_bound_coverage(["sql","docs"],{"sql":receipt()},policy,**ARGS)
    fetch.assert_not_called()


class StrictPolicyMappingTests(unittest.TestCase):
 def test_path_traversal_and_bad_job_names_fail_before_fetch(self):
  for workflow,job in ((".github/workflows/../ci.yml","Job"),
                       (".github/workflows//ci.yml","Job"),
                       (".github/workflows/ci.yaml/evil.yml","Job"),
                       (".github/workflows/ci.yml","job\nmalicious")):
   with self.subTest(workflow=workflow,job=job),patch("r27_origin_bound_coverage.fetch_authenticated_origin") as fetch:
    with self.assertRaisesRegex(OriginError,"invalid job mapping"):
     check_origin_bound_coverage(["sql"],{"sql":receipt()},
       {"sql":{"workflow_path":workflow,"job_name":job}},**ARGS)
    fetch.assert_not_called()


class MalformedRequiredTests(unittest.TestCase):
 def test_invalid_collection_never_raises_unhandled_exception(self):
  for value in (None, 5, True, {"sql":1}, "sql", [None], [[]]):
   with self.subTest(required=repr(value)), patch("r27_origin_bound_coverage.fetch_authenticated_origin") as fetch:
    with self.assertRaisesRegex(OriginError, "invalid required gates"):
     check_origin_bound_coverage(value, {}, JOBS, **ARGS)
    fetch.assert_not_called()


class PolicyDistinctGateTests(unittest.TestCase):
 def test_duplicate_required_gate_is_rejected(self):
  with patch("r27_origin_bound_coverage.fetch_authenticated_origin") as fetch:
   with self.assertRaisesRegex(OriginError, "duplicate required gate"):
    check_origin_bound_coverage(["sql","sql"],{"sql":receipt()},JOBS,**ARGS)
   fetch.assert_not_called()


class CoveragePolicyDigestTests(unittest.TestCase):
 def test_complete_policy_mismatch_rejected_before_network(self):
  with patch("r27_origin_bound_coverage.fetch_authenticated_origin") as api:
   with self.assertRaisesRegex(OriginError,"coverage policy digest mismatch"):
    check_origin_bound_coverage(["sql"],{"sql":receipt()},JOBS,
       **dict(ARGS,policy_digest="f"*64))
   api.assert_not_called()

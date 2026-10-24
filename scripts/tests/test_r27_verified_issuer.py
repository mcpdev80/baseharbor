import sys,unittest,hashlib,json
from pathlib import Path
from unittest.mock import patch
sys.path.insert(0,str(Path(__file__).resolve().parents[1]))
from r27_verified_issuer import issue_from_github
from r27_verified_coverage import verify,EvidenceError
from r27_github_origin import OriginError
ARGS=dict(key=b"k"*32,token="trusted-token",repository="mcpdev80/baseharbor",
          stage="pr",candidate="a"*40,policy_digest="b"*64,gate="sql",
          run_id=11,attempt=2,job_id=12,expected_job_name="PR Selected Checks",
          workflow_path=".github/workflows/ci.yml")
JOBS={"sql":{"workflow_path":".github/workflows/ci.yml",
             "job_name":"PR Selected Checks"}}
ARGS["policy_jobs"]=JOBS
ARGS["policy_digest"]=hashlib.sha256(json.dumps(JOBS,sort_keys=True,separators=(",",":")).encode()).hexdigest()
ARGS["trusted_policy_digest"]=ARGS["policy_digest"]
PROOF=dict(run_id=11,attempt=2,job_id=12,head_sha="a"*40,name="PR Selected Checks")
class IssuerTests(unittest.TestCase):
 def test_success_correlated_to_authenticated_job(self):
  with patch("r27_verified_issuer.fetch_authenticated_origin",return_value=PROOF) as origin:
   raw=issue_from_github(**ARGS)
  self.assertEqual(verify(raw,key=ARGS["key"],stage="pr",candidate=ARGS["candidate"],
    policy_digest=ARGS["policy_digest"],gate="sql")["job_id"],12)
  self.assertEqual(origin.call_args.kwargs["expected_event"],"pull_request")
 def test_no_issue_when_github_origin_fails(self):
  with patch("r27_verified_issuer.fetch_authenticated_origin",side_effect=OriginError("missing job")):
   with self.assertRaises(OriginError):issue_from_github(**ARGS)
 def test_no_issue_when_origin_result_is_unmatched(self):
  for proof in ({},None,dict(PROOF,job_id=9)):
   with self.subTest(proof=proof),patch("r27_verified_issuer.fetch_authenticated_origin",return_value=proof):
    with self.assertRaisesRegex(EvidenceError,"EVIDENCE_ORIGIN"):
     issue_from_github(**ARGS)
 def test_bad_scope_rejected_before_fetch(self):
  for changes in ({"stage":"invalid"},{"key":b"short"},{"candidate":"bad"},
                  {"artifact_digest":"x"},{"stage":"release"},{"gate":"release/smoke"}):
   with self.subTest(changes=changes),patch("r27_verified_issuer.fetch_authenticated_origin") as fetch:
    with self.assertRaises((EvidenceError,OriginError)):
     issue_from_github(**dict(ARGS,**changes))
    fetch.assert_not_called()


class IssuerPolicyTests(unittest.TestCase):
 def test_unmapped_gate_denied_before_api(self):
  for policy in (None, {}, {"sql":{"workflow_path":".github/workflows/ci.yml",
                                    "job_name":"Unrelated Job"}}):
   with self.subTest(policy=policy),patch("r27_verified_issuer.fetch_authenticated_origin") as origin:
    with self.assertRaisesRegex(EvidenceError,"EVIDENCE_POLICY"):
     issue_from_github(**dict(ARGS,policy_jobs=policy))
    origin.assert_not_called()


class DigestBindingTests(unittest.TestCase):
 def test_policy_mutation_invalidates_digest_without_api(self):
  mutated={"sql":{"workflow_path":".github/workflows/ci.yml","job_name":"PR Selected Checks"},
           "docs":{"workflow_path":".github/workflows/docs.yml","job_name":"Docs"}}
  with patch("r27_verified_issuer.fetch_authenticated_origin") as api:
   with self.assertRaisesRegex(EvidenceError,"policy digest mismatch"):
    issue_from_github(**dict(ARGS,policy_jobs=mutated))
   api.assert_not_called()


class IndependentPolicyPinTests(unittest.TestCase):
 def test_policy_pin_required(self):
  for pin in (None, "x"*64, "", True):
   with self.subTest(pin=pin),patch("r27_verified_issuer.fetch_authenticated_origin") as api:
    with self.assertRaisesRegex(EvidenceError,"independent digest pin required"):
     issue_from_github(**dict(ARGS,trusted_policy_digest=pin))
    api.assert_not_called()


class StageEventBindingTests(unittest.TestCase):
 def test_mismatched_stage_and_event_block_before_fetch(self):
  for stage,event in (("pr","push"),("pr","workflow_dispatch"),
                      ("integration","pull_request"),("release","push")):
   change=dict(stage=stage,expected_event=event)
   if stage=="release":change["artifact_digest"]="c"*64
   with self.subTest(change=change),patch("r27_verified_issuer.fetch_authenticated_origin") as api:
    with self.assertRaisesRegex(EvidenceError,"event does not match evidence stage"):
     issue_from_github(**dict(ARGS,**change))
    api.assert_not_called()

import sys
import unittest
from pathlib import Path
sys.path.insert(0,str(Path(__file__).resolve().parents[1]))
from r27_github_fetch import fetch_verified_origin, fetch_authenticated_origin
from r27_github_origin import OriginError
REPO="mcpdev80/baseharbor"
SHA="a"*40
PATH=".github/workflows/ci.yml"
NAME="PR Selected Checks"
RUN={"id":123,"run_attempt":2,"head_sha":SHA,"path":PATH,
     "repository":{"full_name":REPO},"status":"completed","conclusion":"success"}
JOB={"id":456,"run_id":123,"run_attempt":2,"name":NAME,
     "status":"completed","conclusion":"success"}
ARGS=dict(repository=REPO,head_sha=SHA,run_id=123,attempt=2,job_id=456,
          expected_job_name=NAME,workflow_path=PATH)
class FetchTests(unittest.TestCase):
 def test_authenticated_entrypoint_rejects_missing_token(self):
  with self.assertRaisesRegex(OriginError,"EVIDENCE_TRUST"):
   fetch_authenticated_origin("",**ARGS)
 def test_exact_success(self):
  seen=[]
  def get(path):
   seen.append(path)
   return RUN if path.endswith("/123") else {"total_count":1,"jobs":[JOB]}
  self.assertEqual(fetch_verified_origin(get,**ARGS)["job_id"],456)
  self.assertIn("/attempts/2/jobs?per_page=100&page=1",seen[-1])
 def test_partial_inventory_fail_closed(self):
  def get(path):
   return RUN if path.endswith("/123") else {"total_count":2,"jobs":[JOB]}
  with self.assertRaises(OriginError):fetch_verified_origin(get,**ARGS)
 def test_fetch_exception_fail_closed(self):
  def get(path):raise RuntimeError("transport")
  with self.assertRaisesRegex(OriginError,"fetch failed"):
   fetch_verified_origin(get,**ARGS)
 def test_wrong_run_fail_closed(self):
  def get(path):
   return dict(RUN,head_sha="b"*40) if path.endswith("/123") else {"total_count":1,"jobs":[JOB]}
  with self.assertRaises(OriginError):fetch_verified_origin(get,**ARGS)

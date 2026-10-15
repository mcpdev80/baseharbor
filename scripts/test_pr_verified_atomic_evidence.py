"""Regression tests call the actual existing pre-release evidence validator."""
import hashlib
import importlib.util
import io
import json
from pathlib import Path
import unittest
import zipfile

spec=importlib.util.spec_from_file_location("atomic",Path(__file__).with_name("pr-verified-atomic-evidence.py"))
mod=importlib.util.module_from_spec(spec)
spec.loader.exec_module(mod)
BASE="a"*40
HEAD="b"*40
MERGE="c"*40
DEMO="d"*40
RUN=123
ATTEMPT=1
KEY="atomic/docker/backup-restore"
PLAN={"head_sha":HEAD,"integration_sha":MERGE,"selected_gates":[KEY]}
CATALOG={"gates":[{"id":KEY,"required_for":["v0.4.26"],"dependencies":["core","demo"]}]}
RESULTS={"results":[]}
JOBS=[{"name":"PR · Selected Runtime / docker · backup-restore","run_id":RUN,"run_attempt":ATTEMPT,
       "head_sha":HEAD,"status":"completed","conclusion":"success"}]


def fixture(outcome="success",cleanup="success",candidate=MERGE):
    manifest={"schema":"baseharbor.pre-release.gate-evidence/v1","id":"docker/backup-restore",
              "candidate_sha":candidate,"demo_ref":DEMO,
              "workflow_run_id":str(RUN),"workflow_run_attempt":ATTEMPT,
              "outcome":outcome,"cleanup_outcome":cleanup,"runtime":"docker","gate":"backup-restore"}
    file=io.BytesIO()
    with zipfile.ZipFile(file,"w",zipfile.ZIP_DEFLATED) as archive:
        archive.writestr("manifest.json",json.dumps(manifest))
    data=file.getvalue()
    artifact={"id":88,"name":f"pre-release-gate-docker-backup-restore-evidence-{RUN}-{ATTEMPT}",
              "expired":False,"digest":"sha256:"+hashlib.sha256(data).hexdigest(),
              "workflow_run":{"id":RUN,"head_sha":HEAD}}
    return data, artifact


def check(jobs=JOBS,artifacts=None,data=None,plan=PLAN):
    if artifacts is None:
        data,artifact=fixture()
        artifacts=[artifact]
    return mod.authenticate(plan,{"results":[]},CATALOG,RUN,ATTEMPT,HEAD,MERGE,DEMO,
                            jobs,artifacts,lambda _:data)


class RealVerifierFixtures(unittest.TestCase):
    def test_exact_authentic_original_manifest(self):
        self.assertEqual([{"kind":"gate","id":KEY,"conclusion":"success"}],check()["results"])

    def test_missing_forged_and_failed_jobs_block(self):
        self.assertEqual([],check(jobs=[])["results"])
        for outcome in ("failure","cancelled","timed_out","skipped"):
            with self.subTest(outcome=outcome):
                jobs=[dict(JOBS[0],conclusion=outcome)]
                self.assertEqual([],check(jobs=jobs)["results"])
        self.assertEqual([],check(jobs=[dict(JOBS[0],head_sha=MERGE)])["results"])

    def test_missing_or_duplicated_artifact(self):
        data,artifact=fixture()
        self.assertEqual([],check(artifacts=[],data=data)["results"])
        self.assertEqual([],check(artifacts=[artifact,artifact],data=data)["results"])

    def test_bad_manifest_never_passes(self):
        for data,artifact in [fixture(outcome="failure"),fixture(cleanup="skipped"),
                              fixture(candidate=BASE)]:
            with self.subTest(candidate=BASE in data.decode(errors="ignore")):
                with self.assertRaises(ValueError):
                    check(artifacts=[artifact],data=data)

    def test_digest_tampering_rejected(self):
        data,artifact=fixture()
        bad=dict(artifact,digest="sha256:"+"0"*64)
        with self.assertRaises(ValueError):
            check(artifacts=[bad],data=data)

if __name__=="__main__":
    unittest.main()

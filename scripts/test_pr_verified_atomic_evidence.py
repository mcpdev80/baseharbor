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

    def test_public_contracts_owner_artifact_and_denial(self):
        key="integration/static/public-contracts"
        manifest={"schema":"baseharbor.pre-release.integration-evidence/v1",
                  "id":"static/public-contracts","runtime":"static","gate":"public-contracts",
                  "candidate_sha":MERGE,"demo_ref":DEMO,
                  "workflow_run_id":str(RUN),"workflow_run_attempt":ATTEMPT,
                  "outcome":"success","cleanup_outcome":"skipped"}
        def archive(value):
            stream=io.BytesIO()
            with zipfile.ZipFile(stream,"w",zipfile.ZIP_DEFLATED) as z:
                z.writestr("manifest.json",json.dumps(value))
            return stream.getvalue()
        content=archive(manifest)
        origin={"id":99,"name":f"pre-release-integration-static-public-contracts-evidence-{RUN}-{ATTEMPT}",
                "expired":False,"digest":"sha256:"+hashlib.sha256(content).hexdigest(),
                "workflow_run":{"id":RUN,"head_sha":HEAD}}
        job={"name":"PR · Public Contracts","run_id":RUN,"head_sha":HEAD,
             "status":"completed","conclusion":"success"}
        plan=dict(PLAN,selected_gates=[key])
        requirements={"gates":[{"id":key,"required_for":["v0.4.26"],"dependencies":["core"]}]}
        def check(jobs,artifacts,raw=content):
            return mod.authenticate(plan,{"results":[]},requirements,RUN,ATTEMPT,HEAD,MERGE,DEMO,
                                    jobs,artifacts,lambda _:raw)["results"]
        self.assertEqual([{"kind":"gate","id":key,"conclusion":"success"}],check([job],[origin]))
        self.assertEqual([],check([], [origin]))
        self.assertEqual([],check([dict(job,conclusion="failure")],[origin]))
        self.assertEqual([],check([job],[]))
        with self.assertRaises(ValueError):
            check([job],[dict(origin,digest="sha256:"+"0"*64)])
        failed=dict(manifest,outcome="failure")
        corrupt=archive(failed)
        invalid=dict(origin,digest="sha256:"+hashlib.sha256(corrupt).hexdigest())
        with self.assertRaises(ValueError):
            check([job],[invalid],corrupt)

if __name__=="__main__":
    unittest.main()

import copy
import io
import json
import subprocess
import unittest
import zipfile
from unittest.mock import patch

import receipt_closed_run as helper

HEAD = "a" * 40
BASE = "b" * 40
CANDIDATE = "c" * 40
REPO = 1360667669
RUN = 123


def fixture():
    plan = {"schema": "baseharbor.pr-gate-plan/v1", "coverage_status": "resolved",
            "selected_gates": [], "head_sha": HEAD, "target_base_sha": BASE,
            "integration_sha": CANDIDATE}
    data = io.BytesIO()
    with zipfile.ZipFile(data, "w") as z:
        z.writestr("pr-plan.json", json.dumps(plan))
    run = {"id": RUN, "event": "pull_request", "status": "completed", "conclusion": "success",
           "run_attempt": 1, "path": ".github/workflows/ci.yml", "workflow_id": 7,
           "repository": {"id": REPO}, "head_repository": {"id": REPO},
           "head_sha": HEAD, "head_branch": "branch", "pull_requests": []}
    jobs = [{"name": name, "run_id": RUN, "run_attempt": 1, "head_sha": HEAD,
             "status": "completed", "conclusion": "success"}
            for name in ("PR · Trusted Plan", "PR Selected Checks")]
    artifact = {"id": 5, "name": "trusted-pr-plan", "expired": False,
                "workflow_run": {"id": RUN, "head_sha": HEAD}}
    pr = {"number": 42, "state": "closed", "head": {"sha": HEAD, "ref": "branch", "repo": {"id": REPO}},
          "base": {"sha": BASE, "ref": "develop", "repo": {"id": REPO}}}
    return run, jobs, artifact, pr, data.getvalue()


class ClosedRunTests(unittest.TestCase):
    def test_authenticated_tuple_and_fail_closed_negatives(self):
        original = fixture()
        cases = ("ok", "open", "ambiguous", "wrong_base", "failed_job", "wrong_run",
                 "artifact_expired", "wrong_parents", "wrong_identity", "missing_plan", "pilot")
        for case in cases:
            with self.subTest(case=case):
                run, jobs, artifact, pr, archive = copy.deepcopy(original)
                prs = [pr]
                if case == "open": pr["state"] = "open"
                if case == "ambiguous": prs.append(copy.deepcopy(pr))
                if case == "wrong_base": pr["base"]["sha"] = "d" * 40
                if case == "failed_job": jobs[0]["conclusion"] = "failure"
                if case == "wrong_run": artifact["workflow_run"]["id"] = 456
                if case == "artifact_expired": artifact["expired"] = True
                if case == "missing_plan": archive = b"invalid"
                if case == "pilot":
                    with zipfile.ZipFile(io.BytesIO(archive)) as source:
                        plan = json.loads(source.read("pr-plan.json"))
                    plan["selected_gates"] = ["integration/static/public-contracts"]
                    buffer = io.BytesIO()
                    with zipfile.ZipFile(buffer, "w") as target:
                        target.writestr("pr-plan.json", json.dumps(plan))
                    archive = buffer.getvalue()
                def api(path, *, pages=False, binary=False):
                    if binary: return archive
                    if "/attempts/1/jobs" in path: return [{"jobs": jobs}]
                    if path.endswith("/artifacts?per_page=100"): return [{"artifacts": [artifact]}]
                    if "/commits/" in path: return [prs]
                    return run
                def command(args, **kwargs):
                    if "rev-list" in args:
                        parents = f"{CANDIDATE} {BASE} {'d' * 40 if case == 'wrong_parents' else HEAD}\n"
                        return subprocess.CompletedProcess(args, 0, stdout=parents)
                    return subprocess.CompletedProcess(args, 0)
                with patch.object(helper.subprocess, "run", side_effect=command), patch.object(
                        helper.rv, "producer_identity", side_effect=ValueError() if case == "wrong_identity" else None):
                    if case == "ok":
                        self.assertEqual(helper.closed_run(RUN, ".", api), 42)
                    else:
                        with self.assertRaises((ValueError, zipfile.BadZipFile)):
                            helper.closed_run(RUN, ".", api)


if __name__ == "__main__":
    unittest.main()

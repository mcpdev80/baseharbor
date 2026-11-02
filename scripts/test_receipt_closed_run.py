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
    def test_pilot_is_forwarded_to_existing_writer_only_with_planning(self):
        run, jobs, artifact, pr, archive = fixture()
        pr['merge_commit_sha'] = 'd' * 40
        with zipfile.ZipFile(io.BytesIO(archive)) as source:
            plan = json.loads(source.read('pr-plan.json'))
        plan['selected_gates'] = ['integration/static/public-contracts']
        output = io.BytesIO()
        with zipfile.ZipFile(output, 'w') as target:
            target.writestr('pr-plan.json', json.dumps(plan))
        def api(path, *, pages=False, binary=False):
            if binary: return output.getvalue()
            if '/attempts/1/jobs' in path: return [{'jobs': jobs}]
            if path.endswith('/artifacts?per_page=100'): return [{'artifacts': [artifact]}]
            if '/commits/' in path: return [[pr]]
            return run
        def command(args, **kwargs):
            return subprocess.CompletedProcess(args, 0, stdout=f'{CANDIDATE} {BASE} {HEAD}\n')
        with patch.object(helper.subprocess, 'run', side_effect=command), \
             patch.object(helper.rv, 'producer_identity'), \
             patch('receipt_writer.write_merge', return_value=('path', 'intro', 'durable')) as write:
            self.assertEqual(helper.closed_run(RUN, '.', api, planning='planning'), 42)
            self.assertEqual(write.call_args.args[1:3], (42, RUN))

    def test_authenticated_tuple_and_fail_closed_negatives(self):
        original = fixture()
        cases = ("ok", "associated", "wrong_association", "open", "ambiguous", "wrong_base", "failed_job", "wrong_run",
                 "artifact_expired", "wrong_parents", "wrong_identity", "missing_plan", "pilot", "associated_pilot")
        for case in cases:
            with self.subTest(case=case):
                run, jobs, artifact, pr, archive = copy.deepcopy(original)
                prs = [pr]
                if case in ("associated", "wrong_association", "associated_pilot"):
                    run["pull_requests"] = [{"number": 43 if case == "wrong_association" else 42,
                                             "head": {"sha": HEAD}, "base": {"sha": BASE, "ref": "develop"}}]
                if case == "open": pr["state"] = "open"
                if case == "ambiguous": prs.append(copy.deepcopy(pr))
                if case == "wrong_base": pr["base"]["sha"] = "d" * 40
                if case == "failed_job": jobs[0]["conclusion"] = "failure"
                if case == "wrong_run": artifact["workflow_run"]["id"] = 456
                if case == "artifact_expired": artifact["expired"] = True
                if case == "missing_plan": archive = b"invalid"
                if case in ("pilot", "associated_pilot"):
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
                    if case in ("ok", "associated"):
                        self.assertEqual(helper.closed_run(RUN, ".", api), 42)
                    else:
                        with self.assertRaises((ValueError, zipfile.BadZipFile)):
                            helper.closed_run(RUN, ".", api)

    def test_closed_main_pr_requires_authenticated_non_pilot_shape(self):
        for case in ("ok", "open", "ambiguous", "failed_policy", "planner_ran",
                     "final_missing", "unexpected_plan", "bad_base", "wrong_identity"):
            with self.subTest(case=case):
                run, _, artifact, pr, _ = copy.deepcopy(fixture())
                pr["base"]["ref"] = "main"
                prs = [pr]
                jobs = [{"name": name, "run_id": RUN, "run_attempt": 1, "head_sha": HEAD,
                         "status": "completed", "conclusion": conclusion}
                        for name, conclusion in (("Workflow policy", "success"),
                                                 ("PR · Trusted Plan", "skipped"),
                                                 ("PR Selected Checks", "skipped"))]
                artifacts = []
                if case == "open": pr["state"] = "open"
                if case == "ambiguous": prs.append(copy.deepcopy(pr))
                if case == "failed_policy": jobs[0]["conclusion"] = "failure"
                if case == "planner_ran": jobs[1]["conclusion"] = "success"
                if case == "final_missing": jobs.pop()
                if case == "unexpected_plan": artifacts.append(artifact)
                if case == "bad_base": pr["base"]["sha"] = "bad"
                def api(path, *, pages=False, binary=False):
                    if "/attempts/1/jobs" in path: return [{"jobs": jobs}]
                    if path.endswith("/artifacts?per_page=100"): return [{"artifacts": artifacts}]
                    if "/commits/" in path: return [prs]
                    return run
                with patch.object(helper.subprocess, "run", return_value=subprocess.CompletedProcess([], 0)), patch.object(
                        helper.rv, "producer_identity", side_effect=ValueError() if case == "wrong_identity" else None):
                    if case == "ok":
                        self.assertEqual(helper.closed_run(RUN, ".", api), 42)
                    else:
                        with self.assertRaises(ValueError):
                            helper.closed_run(RUN, ".", api)


if __name__ == "__main__":
    unittest.main()

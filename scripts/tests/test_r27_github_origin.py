import copy
import sys
import unittest
from pathlib import Path
sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from r27_github_origin import OriginError, verify_origin, verify_complete_inventory

REPO = "mcpdev80/baseharbor"
SHA = "a" * 40
PATH = ".github/workflows/ci.yml"
NAME = "PR Selected Checks"
RUN = {"id": 123, "run_attempt": 2, "head_sha": SHA, "path": PATH,
       "repository": {"full_name": REPO}, "status": "completed", "conclusion": "success"}
JOB = {"id": 456, "run_id": 123, "run_attempt": 2, "name": NAME,
       "status": "completed", "conclusion": "success"}
EXPECTED = dict(repository=REPO, head_sha=SHA, run_id=123, attempt=2,
                job_id=456, expected_job_name=NAME, workflow_path=PATH)


def checked(run=None, jobs=None, **changes):
    args = dict(EXPECTED)
    args.update(changes)
    return verify_origin(run=RUN if run is None else run,
                         jobs=[JOB] if jobs is None else jobs, **args)


class GitHubOriginTests(unittest.TestCase):
    def test_complete_job_inventory(self):
        self.assertEqual(verify_complete_inventory(
            [{"total_count": 1, "jobs": [JOB]}]), [JOB])
        batch = [dict(JOB, id=i + 1, name=f"job-{i}") for i in range(100)]
        tail = dict(JOB, id=200, name=NAME)
        self.assertEqual(len(verify_complete_inventory([
            {"total_count": 101, "jobs": batch},
            {"total_count": 101, "jobs": [tail]}])), 101)

    def test_partial_or_inconsistent_job_inventory_is_rejected(self):
        batch = [dict(JOB, id=i + 1) for i in range(100)]
        cases = [
            [{"total_count": 2, "jobs": [JOB]}],
            [{"total_count": 2, "jobs": [JOB, JOB]}],
            [{"total_count": 101, "jobs": batch}],
            [{"total_count": 101, "jobs": batch},
             {"total_count": 102, "jobs": [dict(JOB, id=200)]}],
            [{"total_count": 2, "jobs": [JOB]},
             {"total_count": 2, "jobs": [dict(JOB, id=200)]}],
            [{"total_count": True, "jobs": [JOB]}],
            [{"total_count": 0, "jobs": []}],
        ]
        for pages in cases:
            with self.subTest(pages=len(pages)), self.assertRaises(OriginError):
                verify_complete_inventory(pages)

    def test_untrusted_api_shapes_fail_with_origin_error(self):
        for value in (None, True, "unexpected", []):
            with self.subTest(repository=value), self.assertRaisesRegex(
                    OriginError, "EVIDENCE_ORIGIN"):
                checked(run=dict(RUN, repository=value))
        for limit in (None, True, 0, -1, 1001, "1000"):
            with self.subTest(max_jobs=limit), self.assertRaisesRegex(
                    OriginError, "EVIDENCE_ORIGIN"):
                verify_complete_inventory([{"total_count": 1, "jobs": [JOB]}],
                                          max_jobs=limit)

    def test_exact_run_and_job_success(self):
        self.assertEqual(checked()["job_id"], 456)

    def test_bad_run_and_job_fields_fail_closed(self):
        for field, value in (("id", 999), ("run_attempt", 3), ("head_sha", "b" * 40),
                             ("path", ".github/workflows/other.yml"),
                             ("repository", {"full_name": "other/repo"}),
                             ("status", "in_progress"), ("conclusion", "failure")):
            with self.subTest(run=field):
                data = copy.deepcopy(RUN)
                data[field] = value
                with self.assertRaises(OriginError):
                    checked(run=data)
        for field, value in (("id", 999), ("run_id", 9), ("run_attempt", 1),
                             ("name", "other"), ("status", "queued"),
                             ("conclusion", "skipped")):
            with self.subTest(job=field):
                data = dict(JOB, **{field: value})
                with self.assertRaises(OriginError):
                    checked(jobs=[data])

    def test_ambiguous_job_identity_fails_closed(self):
        with self.assertRaises(OriginError):
            checked(jobs=[JOB, dict(JOB)])
        with self.assertRaises(OriginError):
            checked(jobs=[JOB, dict(JOB, id=457)])

    def test_malformed_expected_arguments_fail_closed(self):
        for changes in ({"repository": "../other"}, {"head_sha": "short"},
                        {"run_id": True}, {"attempt": 0}, {"job_id": -1},
                        {"expected_job_name": ""}, {"workflow_path": "../ci.yml"}):
            with self.subTest(changes=changes), self.assertRaises(OriginError):
                checked(**changes)


if __name__ == "__main__":
    unittest.main()

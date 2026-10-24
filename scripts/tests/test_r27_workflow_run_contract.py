"""Negative contract tests for isolated workflow_run eligibility."""
import sys
import json
import unittest
from unittest.mock import patch
from pathlib import Path
sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from r27_verified_coverage import EvidenceError
from r27_workflow_run_contract import (
    validate_workflow_run_event, verify_authenticated_workflow_run,
)
from r27_github_origin import OriginError

REPO = "mcpdev80/baseharbor"
PATH = ".github/workflows/selected.yml"
RUN_ID = 1234


def event():
    return {
        "action": "completed",
        "repository": {"full_name": REPO},
        "workflow_run": {
            "id": RUN_ID,
            "event": "pull_request",
            "status": "completed",
            "conclusion": "success",
            "path": PATH,
            "head_repository": {"full_name": REPO},
            "head_sha": "a" * 40,
        },
    }


class WorkflowRunEligibility(unittest.TestCase):
    def test_accepts_valid_shape(self):
        result = validate_workflow_run_event(event(), repository=REPO, workflow_path=PATH, run_id=RUN_ID)
        self.assertEqual(result["head_sha"], "a" * 40)

    def test_rejects_untrusted_shapes(self):
        for key, value in (
            ("action", "requested"),
            ("repository", {"full_name": "other/baseharbor"}),
        ):
            bad = event()
            bad[key] = value
            with self.subTest(key=key), self.assertRaises(EvidenceError):
                validate_workflow_run_event(bad, repository=REPO, workflow_path=PATH, run_id=RUN_ID)
        for key, value in (
            ("id", 999), ("event", "push"), ("status", "in_progress"),
            ("conclusion", "failure"), ("path", ".github/workflows/other.yml"),
            ("head_repository", {"full_name": "attacker/baseharbor"}),
            ("head_sha", "not-a-sha"),
        ):
            bad = event()
            bad["workflow_run"][key] = value
            with self.subTest(key=key), self.assertRaises(EvidenceError):
                validate_workflow_run_event(bad, repository=REPO, workflow_path=PATH, run_id=RUN_ID)


    def test_api_run_must_match_event(self):
        from r27_workflow_run_contract import verify_workflow_run_against_api
        api = {"id": RUN_ID, "repository": {"full_name": REPO},
               "path": PATH, "head_sha": "a" * 40,
               "event": "pull_request", "status": "completed", "conclusion": "success"}
        self.assertEqual(verify_workflow_run_against_api(
            event(), lambda _: api, repository=REPO,
            workflow_path=PATH, run_id=RUN_ID)["run_id"], RUN_ID)
        for changed in ("head_sha", "conclusion", "path"):
            mismatched = dict(api)
            mismatched[changed] = "wrong"
            with self.subTest(changed=changed), self.assertRaises(EvidenceError):
                verify_workflow_run_against_api(
                    event(), lambda _: mismatched, repository=REPO,
                    workflow_path=PATH, run_id=RUN_ID)

    def test_run_ids_require_exact_integers(self):
        from r27_workflow_run_contract import verify_workflow_run_against_api
        valid = event()
        valid["workflow_run"]["id"] = 1
        api = dict(valid["workflow_run"], repository={"full_name": REPO})
        for invalid in (True, 1.0, "1", None):
            for source in ("expected", "event", "api"):
                payload = event()
                payload["workflow_run"]["id"] = 1
                response = dict(api)
                expected = 1
                if source == "expected":
                    expected = invalid
                elif source == "event":
                    payload["workflow_run"]["id"] = invalid
                else:
                    response["id"] = invalid
                with self.subTest(source=source, value=invalid), self.assertRaises(EvidenceError):
                    verify_workflow_run_against_api(
                        payload, lambda _: response, repository=REPO,
                        workflow_path=PATH, run_id=expected)

    def test_authenticated_transport_composition(self):
        payload = event()
        api = dict(payload["workflow_run"], repository={"full_name": REPO})
        # Exercise the real getter, JSON decoder and identity verifier; replace
        # only the network boundary. No signing key or issuer is involved.
        with patch("r27_github_transport.urllib.request.build_opener") as build:
            response = build.return_value.open.return_value.__enter__.return_value
            response.getcode.return_value = 200
            response.read.side_effect = lambda _: json.dumps(api).encode()
            result = verify_authenticated_workflow_run(
                payload, "test-token", repository=REPO,
                workflow_path=PATH, run_id=RUN_ID)
            self.assertEqual(result, {"run_id": RUN_ID, "head_sha": "a" * 40,
                                      "workflow_path": PATH})
            request = build.return_value.open.call_args.args[0]
            self.assertEqual(request.full_url,
                             f"https://api.github.com/repos/{REPO}/actions/runs/{RUN_ID}")
            self.assertEqual(request.get_header("Authorization"), "Bearer test-token")
            for failure in ("mismatch", "transport", "invalid_event", "missing_token"):
                build.return_value.open.reset_mock()
                build.return_value.open.side_effect = None
                api["head_sha"] = "a" * 40
                bad_payload = event()
                token = "test-token"
                if failure == "mismatch":
                    api["head_sha"] = "b" * 40
                elif failure == "transport":
                    build.return_value.open.side_effect = OSError("unavailable")
                elif failure == "invalid_event":
                    bad_payload["action"] = "requested"
                else:
                    token = ""
                expected_error = OriginError if failure == "missing_token" else EvidenceError
                with self.subTest(failure=failure), self.assertRaises(expected_error):
                    verify_authenticated_workflow_run(
                        bad_payload, token, repository=REPO,
                        workflow_path=PATH, run_id=RUN_ID)
                if failure in ("invalid_event", "missing_token"):
                    build.return_value.open.assert_not_called()

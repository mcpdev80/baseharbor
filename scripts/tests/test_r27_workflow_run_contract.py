"""Negative contract tests for isolated workflow_run eligibility."""
import sys
import unittest
from pathlib import Path
sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from r27_verified_coverage import EvidenceError
from r27_workflow_run_contract import validate_workflow_run_event

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

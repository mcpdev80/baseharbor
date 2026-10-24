"""Validate untrusted workflow_run payload before requesting trusted verification.

This is an input filter, NOT proof of webhook authenticity, default-branch
workflow provenance, or protected signer isolation. Those are executor duties.
"""
import re
from r27_verified_coverage import EvidenceError, SHA

_REPO = re.compile(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+\Z")
_WORKFLOW = re.compile(r"\.github/workflows/[A-Za-z0-9_.-]+\.ya?ml\Z")


def validate_workflow_run_event(payload, *, repository, workflow_path, run_id):
    if not isinstance(repository, str) or not _REPO.fullmatch(repository):
        raise EvidenceError("EVIDENCE_TRIGGER: invalid expected repository")
    if not isinstance(workflow_path, str) or not _WORKFLOW.fullmatch(workflow_path):
        raise EvidenceError("EVIDENCE_TRIGGER: invalid expected workflow")
    if type(run_id) is not int or run_id < 1 or not isinstance(payload, dict):
        raise EvidenceError("EVIDENCE_TRIGGER: invalid event")
    run = payload.get("workflow_run")
    repo = payload.get("repository")
    if (payload.get("action") != "completed" or not isinstance(run, dict)
            or not isinstance(repo, dict) or repo.get("full_name") != repository
            or run.get("event") != "pull_request"
            or run.get("status") != "completed" or run.get("conclusion") != "success"
            or run.get("id") != run_id
            or run.get("path") != workflow_path
            or not isinstance(run.get("head_repository"), dict)
            or run["head_repository"].get("full_name") != repository
            or not isinstance(run.get("head_sha"), str)
            or not SHA.fullmatch(run["head_sha"])):
        raise EvidenceError("EVIDENCE_TRIGGER: workflow run is not eligible")
    return {"run_id": run_id, "head_sha": run["head_sha"], "workflow_path": workflow_path}

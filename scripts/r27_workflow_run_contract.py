"""Validate untrusted workflow_run payload before requesting trusted verification.

This is an input filter, NOT proof of webhook authenticity, default-branch
workflow provenance, or protected signer isolation. Those are executor duties.
"""
import re
from r27_verified_coverage import EvidenceError, SHA
from r27_github_transport import github_getter

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
            or type(run.get("id")) is not int or run["id"] != run_id
            or run.get("path") != workflow_path
            or not isinstance(run.get("head_repository"), dict)
            or run["head_repository"].get("full_name") != repository
            or not isinstance(run.get("head_sha"), str)
            or not SHA.fullmatch(run["head_sha"])):
        raise EvidenceError("EVIDENCE_TRIGGER: workflow run is not eligible")
    return {"run_id": run_id, "head_sha": run["head_sha"], "workflow_path": workflow_path}


def verify_workflow_run_against_api(payload, get_json, *, repository,
                                    workflow_path, run_id):
    """Cross-check untrusted event identity with independently fetched API run."""
    event = validate_workflow_run_event(payload, repository=repository,
                                        workflow_path=workflow_path, run_id=run_id)
    if not callable(get_json):
        raise EvidenceError("EVIDENCE_TRIGGER: authenticated API transport required")
    try:
        verified = get_json(f"repos/{repository}/actions/runs/{run_id}")
    except Exception as exc:
        raise EvidenceError("EVIDENCE_TRIGGER: API lookup failed") from exc
    if not isinstance(verified, dict) or not isinstance(verified.get("repository"), dict):
        raise EvidenceError("EVIDENCE_TRIGGER: invalid API run")
    if (type(verified.get("id")) is not int or verified["id"] != run_id
            or verified["repository"].get("full_name") != repository
            or verified.get("path") != workflow_path
            or verified.get("head_sha") != event["head_sha"]
            or verified.get("event") != "pull_request"
            or verified.get("status") != "completed"
            or verified.get("conclusion") != "success"):
        raise EvidenceError("EVIDENCE_TRIGGER: API run mismatch")
    return event


def verify_authenticated_workflow_run(payload, token, *, repository,
                                      workflow_path, run_id):
    """Re-fetch event identity using the restricted authenticated GitHub transport.

    Call from independently protected executor code with a scoped credential.
    Returns validated identity only; does not issue evidence or establish
    workflow provenance, attempt/job authorization or signing-key isolation.
    """
    return verify_workflow_run_against_api(
        payload, github_getter(token), repository=repository,
        workflow_path=workflow_path, run_id=run_id)

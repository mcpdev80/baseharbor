"""Independently resolve GitHub Actions job outcomes; never issue signed receipts.

The caller must supply an authenticated API fetcher controlled by trusted CI.
Returned proof is data for a separate policy/verifier, NOT promotion authority.
"""
import re

SHA = re.compile(r"[0-9a-f]{40}\Z")


class OriginError(ValueError):
    pass


def verify_origin(*, run, jobs, repository, head_sha, run_id, attempt, job_id,
                  expected_job_name, workflow_path):
    """Validate exact run, attempt, job and workflow identity using API-fetched objects."""
    if not isinstance(repository, str) or not re.fullmatch(
            r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", repository):
        raise OriginError("EVIDENCE_ORIGIN: invalid repository")
    if not isinstance(head_sha, str) or not SHA.fullmatch(head_sha):
        raise OriginError("EVIDENCE_TARGET: invalid SHA")
    if any(type(n) is not int or n < 1 for n in (run_id, attempt, job_id)):
        raise OriginError("EVIDENCE_ORIGIN: invalid expected ids")
    if not isinstance(expected_job_name, str) or not expected_job_name.strip():
        raise OriginError("EVIDENCE_ORIGIN: missing expected job")
    if not isinstance(workflow_path, str) or not workflow_path.startswith(
            ".github/workflows/") or not workflow_path.endswith(".yml"):
        raise OriginError("EVIDENCE_ORIGIN: invalid workflow path")
    if not isinstance(run, dict) or not isinstance(jobs, list):
        raise OriginError("EVIDENCE_ORIGIN: invalid API response")
    expected_path = repository + "/" + workflow_path
    if (run.get("id") != run_id or type(run.get("id")) is not int
            or run.get("run_attempt") != attempt
            or type(run.get("run_attempt")) is not int
            or run.get("head_sha") != head_sha
            or run.get("path") != workflow_path
            or run.get("repository", {}).get("full_name") != repository
            or run.get("status") != "completed"
            or run.get("conclusion") != "success"):
        raise OriginError("EVIDENCE_ORIGIN: run identity or success mismatch")
    matches = [job for job in jobs if isinstance(job, dict)
               and job.get("id") == job_id]
    if len(matches) != 1:
        raise OriginError("EVIDENCE_ORIGIN: missing or duplicate job")
    job = matches[0]
    if (type(job.get("id")) is not int
            or job.get("run_id") != run_id or type(job.get("run_id")) is not int
            or job.get("run_attempt") != attempt
            or type(job.get("run_attempt")) is not int
            or job.get("name") != expected_job_name
            or job.get("status") != "completed"
            or job.get("conclusion") != "success"):
        raise OriginError("EVIDENCE_ORIGIN: job identity or success mismatch")
    if sum(isinstance(j, dict) and j.get("name") == expected_job_name
           for j in jobs) != 1:
        raise OriginError("EVIDENCE_ORIGIN: ambiguous job name")
    return {"run_id": run_id, "attempt": attempt, "job_id": job_id,
            "head_sha": head_sha, "name": expected_job_name}

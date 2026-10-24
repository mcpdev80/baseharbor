"""Read exact run and complete job inventory from GitHub through a trusted transport.

The injected transport must supply authenticated HTTPS GET responses. This
module validates fetch shape and pagination, not token trust or CI authority.
"""
from r27_github_origin import OriginError, verify_complete_inventory, verify_origin
from r27_github_transport import github_getter
import re


def fetch_verified_origin(get_json, *, repository, head_sha, run_id, attempt,
                          job_id, expected_job_name, workflow_path):
    if not callable(get_json):
        raise OriginError("EVIDENCE_ORIGIN: authenticated transport unavailable")
    if (not isinstance(repository, str) or repository.count("/") != 1
            or any(p in (".", "..") for p in repository.split("/"))
            or not re.fullmatch(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", repository)
            or type(run_id) is not int or run_id < 1
            or type(attempt) is not int or attempt < 1):
        raise OriginError("EVIDENCE_ORIGIN: invalid API request identity")
    root = f"repos/{repository}/actions/runs/{run_id}"
    try:
        run = get_json(root)
        pages = []
        page = 1
        while page <= 10:
            response = get_json(
                f"{root}/attempts/{attempt}/jobs?per_page=100&page={page}")
            if (not isinstance(response, dict)
                    or type(response.get("total_count")) is not int
                    or response["total_count"] < 1
                    or response["total_count"] > 1000
                    or not isinstance(response.get("jobs"), list)
                    or not response["jobs"]
                    or len(response["jobs"]) > 100):
                raise OriginError("EVIDENCE_ORIGIN: invalid job inventory page")
            pages.append(response)
            jobs = verify_complete_inventory(pages) if (
                isinstance(response, dict)
                and type(response.get("total_count")) is int
                and sum(len(p.get("jobs", [])) for p in pages if isinstance(p, dict))
                    == response["total_count"]) else None
            if jobs is not None:
                return verify_origin(
                    run=run, jobs=jobs, repository=repository, head_sha=head_sha,
                    run_id=run_id, attempt=attempt, job_id=job_id,
                    expected_job_name=expected_job_name, workflow_path=workflow_path)
            page += 1
    except OriginError:
        raise
    except Exception as exc:
        raise OriginError("EVIDENCE_ORIGIN: authenticated fetch failed") from exc
    raise OriginError("EVIDENCE_ORIGIN: job inventory never completed")


def fetch_authenticated_origin(token, **scope):
    """Use the pinned GitHub API origin with caller-provided trusted credential."""
    return fetch_verified_origin(github_getter(token), **scope)

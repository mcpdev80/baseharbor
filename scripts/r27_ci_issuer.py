"""Protected whole-run issuer, separate from every candidate execution job."""
import hashlib
from r27_ci_request import check_current, verify_executor_run, WORKFLOW
from r27_ci_plan import expected_jobs, make_plan
from r27_commit_receipt import sign, canonical
from r27_github_fetch import fetch_run_inventory
from r27_github_origin import verify_origin
from r27_github_transport import github_getter
from r27_verified_coverage import EvidenceError


def issue_completed_run(*, token, key, run_id, attempt, executor_sha,
                        policy, policy_digest, repository_path, demo_repository=None):
    """Only fixed-source CI calls this; it never checks out or executes PR code.

    Recompute the entire plan from immutable Git objects and protected policy.
    A candidate artifact, claimed plan or caller job list is never accepted.
    """
    if hashlib.sha256(canonical(policy) + b"\n").hexdigest() != policy_digest:
        raise EvidenceError("CI_POLICY: protected issuer policy digest mismatch")
    get_json = github_getter(token)
    repository = policy["repository"]
    run, jobs = fetch_run_inventory(get_json, repository=repository,
                                    run_id=run_id, attempt=attempt)
    request = verify_executor_run(run, executor_sha=executor_sha,
                                  repository=repository, run_id=run_id, attempt=attempt)
    check_current(request, get_json, repository)
    plan = make_plan(request, policy, repository=repository_path,
                     demo_repository=demo_repository)
    required = expected_jobs(plan)
    for name in sorted(required):
        matches = [job for job in jobs if job.get("name") == name]
        if len(matches) != 1:
            raise EvidenceError("CI_COVERAGE: missing or ambiguous required job: " + name)
        verify_origin(run=run, jobs=jobs, repository=repository, head_sha=executor_sha,
                      run_id=run_id, attempt=attempt, job_id=matches[0]["id"],
                      expected_job_name=name, workflow_path=WORKFLOW,
                      expected_event="workflow_dispatch")
    # No signature is produced if GitHub changed during plan reconstruction.
    check_current(request, get_json, repository)
    return sign(request, plan["coverage"], key=key, policy_digest=policy_digest,
                executor_sha=executor_sha)

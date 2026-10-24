"""Advisory receipt coverage requiring fresh GitHub job-origin corroboration.

The caller must use a trusted policy mapping; no signature issuance or promotion.
"""
from r27_evidence_coverage import plan_verified
from r27_verified_coverage import verify
from r27_github_fetch import fetch_authenticated_origin
from r27_github_origin import OriginError

def check_origin_bound_coverage(required, receipts, policy_jobs, *, key, token,
                                repository, stage, candidate, policy_digest,
                                artifact_digest=None, expected_event="pull_request"):
    if not isinstance(policy_jobs, dict) or set(policy_jobs) != set(required):
        raise OriginError("EVIDENCE_POLICY: incomplete trusted gate-job mapping")
    if not isinstance(receipts, dict) or set(receipts) - set(required):
        raise OriginError("EVIDENCE_INPUT: unexpected receipt")
    coverage = plan_verified(required, receipts, key=key, stage=stage,
                             candidate=candidate, policy_digest=policy_digest,
                             artifact_digest=artifact_digest)
    for gate in sorted(receipts):
        mapping = policy_jobs[gate]
        if (not isinstance(mapping, dict)
                or set(mapping) != {"workflow_path", "job_name"}
                or not all(isinstance(x, str) and x for x in mapping.values())):
            raise OriginError("EVIDENCE_POLICY: invalid job mapping")
        identity = verify(receipts[gate], key=key, stage=stage, candidate=candidate,
                          policy_digest=policy_digest, gate=gate,
                          artifact_digest=artifact_digest)
        fetch_authenticated_origin(token, repository=repository, head_sha=candidate,
            run_id=identity["run_id"], attempt=identity["attempt"],
            job_id=identity["job_id"], expected_job_name=mapping["job_name"],
            workflow_path=mapping["workflow_path"], expected_event=expected_event)
    return coverage

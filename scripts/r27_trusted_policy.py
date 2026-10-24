"""Load a trusted gate policy from an exact immutable source.

The caller MUST obtain the bytes and expected digest independently from
protected CI configuration. This loader does not authenticate the caller.
"""
import hashlib
import hmac
import json
from r27_verified_coverage import EvidenceError, DIGEST, MAX_BYTES

def load_pinned_policy(raw, *, protected_digest):
    if (type(raw) is not bytes or len(raw) > MAX_BYTES
            or not isinstance(protected_digest, str)
            or not DIGEST.fullmatch(protected_digest)):
        raise EvidenceError("EVIDENCE_POLICY: protected policy input unavailable")
    actual = hashlib.sha256(raw).hexdigest()
    if not hmac.compare_digest(actual, protected_digest):
        raise EvidenceError("EVIDENCE_POLICY: protected policy source mismatch")
    try:
        decoded = json.loads(raw)
    except (ValueError, UnicodeError) as exc:
        raise EvidenceError("EVIDENCE_POLICY: invalid protected policy") from exc
    if not isinstance(decoded, dict) or not decoded:
        raise EvidenceError("EVIDENCE_POLICY: empty protected policy")
    canonical = json.dumps(decoded,sort_keys=True,separators=(",",":"),
                           ensure_ascii=True,allow_nan=False).encode("ascii")
    if raw != canonical:
        raise EvidenceError("EVIDENCE_POLICY: noncanonical protected policy")
    return decoded,actual


def issue_with_pinned_policy(*, policy_bytes, protected_digest, **kwargs):
    """Only the policy bytes are accepted from the policy source, not caller mappings."""
    from r27_verified_issuer import issue_from_github
    if "policy_jobs" in kwargs or "policy_digest" in kwargs or "trusted_policy_digest" in kwargs:
        raise EvidenceError("EVIDENCE_POLICY: caller policy override forbidden")
    mappings, digest = load_pinned_policy(policy_bytes, protected_digest=protected_digest)
    gate = kwargs.get("gate")
    if gate not in mappings or not isinstance(mappings[gate], dict):
        raise EvidenceError("EVIDENCE_POLICY: unmapped gate")
    job = mappings[gate]
    if "expected_job_name" in kwargs or "workflow_path" in kwargs:
        raise EvidenceError("EVIDENCE_POLICY: caller job override forbidden")
    return issue_from_github(**kwargs, policy_jobs=mappings, policy_digest=digest,
                             trusted_policy_digest=digest,
                             expected_job_name=job.get("job_name"),
                             workflow_path=job.get("workflow_path"))

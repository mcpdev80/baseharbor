"""Isolated gate issuer: a successful authenticated GitHub job is mandatory.

Requires trusted caller to pin gate policy and provision secret credentials.
Never a substitute for policy authorization or promotion qualification.
"""
import hashlib
import hmac
import json
from r27_verified_coverage import SCHEMA, SHA, DIGEST, GATE, MAX_BYTES, EvidenceError
from r27_github_fetch import fetch_authenticated_origin

def issue_from_github(*, key, token, repository, stage, candidate, policy_digest,
                      gate, run_id, attempt, job_id, expected_job_name,
                      workflow_path, expected_event="pull_request",
                      artifact_digest=None, policy_jobs=None, trusted_policy_digest=None):
    if type(key) is not bytes or len(key) < 32:
        raise EvidenceError("EVIDENCE_TRUST: trusted signing key unavailable")
    if stage not in ("pr", "integration", "release"):
        raise EvidenceError("EVIDENCE_SCHEMA: invalid stage")
    if not isinstance(candidate, str) or not SHA.fullmatch(candidate):
        raise EvidenceError("EVIDENCE_TARGET: invalid candidate")
    if not isinstance(policy_digest, str) or not DIGEST.fullmatch(policy_digest):
        raise EvidenceError("EVIDENCE_POLICY: invalid policy digest")
    if not isinstance(gate, str) or not GATE.fullmatch(gate):
        raise EvidenceError("EVIDENCE_GATE: invalid gate")
    if artifact_digest is not None and (not isinstance(artifact_digest,str)
                                        or not DIGEST.fullmatch(artifact_digest)):
        raise EvidenceError("EVIDENCE_ARTIFACT: invalid artifact digest")
    if stage == "release" and artifact_digest is None:
        raise EvidenceError("EVIDENCE_ARTIFACT: release requires artifact")
    if gate == "release/smoke" and stage != "release":
        raise EvidenceError("EVIDENCE_ARTIFACT: smoke requires release")
    if (not isinstance(policy_jobs, dict) or gate not in policy_jobs
            or not isinstance(policy_jobs[gate], dict)
            or set(policy_jobs[gate]) != {"job_name", "workflow_path"}
            or policy_jobs[gate] != {"job_name": expected_job_name,
                                    "workflow_path": workflow_path}):
        raise EvidenceError("EVIDENCE_POLICY: trusted gate-to-job mapping required")
    # Reject malformed identities in every policy entry, not only the selected
    # gate. A digest alone cannot validate the policy's schema.
    import re
    if (not isinstance(policy_jobs, dict) or not policy_jobs
            or any(not isinstance(name,str) or not GATE.fullmatch(name)
                   or not isinstance(value,dict)
                   or set(value)!={"job_name","workflow_path"}
                   or not isinstance(value["job_name"],str)
                   or not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9 _./:()-]{0,127}",
                                       value["job_name"])
                   or not isinstance(value["workflow_path"],str)
                   or not re.fullmatch(r"\.github/workflows/[A-Za-z0-9_.-]+\.ya?ml",
                                       value["workflow_path"])
                   for name,value in policy_jobs.items())):
        raise EvidenceError("EVIDENCE_POLICY: invalid complete mapping")
    # A caller may not attach an arbitrary digest to an unrelated job mapping.
    # All mappings in the policy participate in the canonical digest.
    if not isinstance(policy_jobs, dict) or not policy_jobs:
        raise EvidenceError("EVIDENCE_POLICY: trusted policy mappings unavailable")
    try:
        policy_bytes = json.dumps(policy_jobs, sort_keys=True,
                                  separators=(",", ":"), ensure_ascii=True,
                                  allow_nan=False).encode("ascii")
    except (TypeError, ValueError, UnicodeError) as exc:
        raise EvidenceError("EVIDENCE_POLICY: malformed mapping") from exc
    actual_digest = hashlib.sha256(policy_bytes).hexdigest()
    if not hmac.compare_digest(actual_digest, policy_digest):
        raise EvidenceError("EVIDENCE_POLICY: policy digest mismatch")
    if (not isinstance(trusted_policy_digest, str)
            or not DIGEST.fullmatch(trusted_policy_digest)
            or not hmac.compare_digest(actual_digest, trusted_policy_digest)):
        raise EvidenceError("EVIDENCE_POLICY: independent digest pin required")
    # A receipt bound to a release artifact may not be issued based on an
    # unrelated PR event; release and integration require explicit event scope.
    allowed_events = {"pr": ("pull_request",),
                      "integration": ("push",),
                      "release": ("workflow_dispatch",)}
    if expected_event not in allowed_events[stage]:
        raise EvidenceError("EVIDENCE_ORIGIN: event does not match evidence stage")
    identity = fetch_authenticated_origin(token, repository=repository, head_sha=candidate,
        run_id=run_id, attempt=attempt, job_id=job_id,
        expected_job_name=expected_job_name, workflow_path=workflow_path,
        expected_event=expected_event)
    if not isinstance(identity,dict) or any(identity.get(k) != v for k,v in
            (("run_id",run_id),("attempt",attempt),("job_id",job_id),
             ("head_sha",candidate),("name",expected_job_name))):
        raise EvidenceError("EVIDENCE_ORIGIN: invalid authenticated result")
    body = dict(schema=SCHEMA,stage=stage,candidate=candidate,
                policy_digest=policy_digest,gate=gate,artifact_digest=artifact_digest,
                result="PASS",run_id=run_id,attempt=attempt,job_id=job_id)
    canonical = json.dumps(body,sort_keys=True,separators=(",",":")).encode()
    mac = hmac.new(key,canonical,hashlib.sha256).hexdigest()
    raw = json.dumps(dict(body,mac=mac),sort_keys=True,separators=(",",":")).encode()
    if len(raw) > MAX_BYTES:
        raise EvidenceError("EVIDENCE_SIZE: oversized receipt")
    return raw

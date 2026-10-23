"""Verify signed gate-scoped evidence, never authorize issuance."""
import hashlib
import hmac
import json
import re

SCHEMA = "baseharbor.gate-evidence/v1"
SHA = re.compile(r"[0-9a-f]{40}\Z")
DIGEST = re.compile(r"[0-9a-f]{64}\Z")
GATE = re.compile(r"[a-zA-Z0-9][a-zA-Z0-9_./-]{0,127}\Z")
FIELDS = frozenset(("schema", "stage", "candidate", "policy_digest", "gate",
                    "artifact_digest", "result", "run_id", "attempt", "job_id"))
MAX_BYTES = 4096


class EvidenceError(ValueError):
    pass


def _unique(pairs):
    obj = {}
    for k, v in pairs:
        if k in obj:
            raise EvidenceError("EVIDENCE_SCHEMA: duplicate key")
        obj[k] = v
    return obj


def verify(raw, *, key, stage, candidate, policy_digest, gate, artifact_digest=None):
    """Validate MAC, exact scope and CI provenance from a trusted issuer."""
    if not isinstance(key, bytes) or len(key) < 32:
        raise EvidenceError("EVIDENCE_TRUST: verifier key unavailable")
    if not isinstance(raw, bytes) or len(raw) > MAX_BYTES:
        raise EvidenceError("EVIDENCE_SIZE: invalid receipt size")
    try:
        record = json.loads(raw, object_pairs_hook=_unique)
    except (UnicodeError, ValueError) as exc:
        raise EvidenceError("EVIDENCE_SCHEMA: malformed JSON") from exc
    if not isinstance(record, dict) or set(record) != FIELDS | {"mac"}:
        raise EvidenceError("EVIDENCE_SCHEMA: unexpected fields")
    mac = record["mac"]
    body = {k: v for k, v in record.items() if k != "mac"}
    if record["schema"] != SCHEMA or record["stage"] not in ("pr", "integration", "release"):
        raise EvidenceError("EVIDENCE_SCHEMA: invalid stage")
    if not isinstance(record["candidate"], str) or not SHA.fullmatch(record["candidate"]):
        raise EvidenceError("EVIDENCE_TARGET: invalid candidate")
    if not isinstance(record["policy_digest"], str) or not DIGEST.fullmatch(record["policy_digest"]):
        raise EvidenceError("EVIDENCE_POLICY: invalid digest")
    if not isinstance(record["gate"], str) or not GATE.fullmatch(record["gate"]):
        raise EvidenceError("EVIDENCE_GATE: invalid gate")
    artifact = record["artifact_digest"]
    if artifact is not None and (not isinstance(artifact, str) or not DIGEST.fullmatch(artifact)):
        raise EvidenceError("EVIDENCE_ARTIFACT: invalid digest")
    if record["result"] != "PASS":
        raise EvidenceError("EVIDENCE_RESULT: only PASS reusable")
    if any(type(record[k]) is not int or record[k] < 1 for k in ("run_id", "attempt", "job_id")):
        raise EvidenceError("EVIDENCE_ORIGIN: invalid CI identity")
    if not isinstance(mac, str) or not DIGEST.fullmatch(mac):
        raise EvidenceError("EVIDENCE_SIGNATURE: invalid MAC")
    canonical = json.dumps(body, sort_keys=True, separators=(",", ":"), allow_nan=False).encode()
    expected = hmac.new(key, canonical, hashlib.sha256).hexdigest()
    if not hmac.compare_digest(mac, expected):
        raise EvidenceError("EVIDENCE_SIGNATURE: authentication failed")
    if (record["stage"], record["candidate"], record["gate"]) != (stage, candidate, gate):
        raise EvidenceError("EVIDENCE_TARGET: receipt scope mismatch")
    if record["policy_digest"] != policy_digest:
        raise EvidenceError("EVIDENCE_POLICY: stale policy")
    if stage == "release" and artifact_digest is None:
        raise EvidenceError("EVIDENCE_ARTIFACT: release digest required")
    if gate == "release/smoke" and (stage != "release" or artifact != artifact_digest):
        raise EvidenceError("EVIDENCE_ARTIFACT: smoke artifact mismatch")
    return {"gate": gate, "run_id": record["run_id"], "attempt": record["attempt"],
            "job_id": record["job_id"]}

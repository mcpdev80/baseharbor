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

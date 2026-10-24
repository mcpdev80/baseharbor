"""One bounded offline-verifiable receipt for complete trusted CI coverage.

Fingerprints must be computed by protected canonical input owners. Neither a
caller boolean nor unsigned job output grants reuse. Signing keys never enter
the candidate executor. This module performs no promotion or network access.
"""
import json
import re
from cryptography.exceptions import InvalidSignature
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey, Ed25519PublicKey
from r27_verified_coverage import DIGEST, GATE, EvidenceError, _unique, _reject_nonfinite
from r27_ci_request import validate_request

SCHEMA = "baseharbor.commit-evidence/v1"
MAX_BYTES = 16384
FIELDS = {"schema", "request", "policy_digest", "executor_sha", "coverage", "signature"}


def canonical(value):
    return json.dumps(value, sort_keys=True, separators=(",", ":"), allow_nan=False).encode()


def validate_coverage(coverage):
    if not isinstance(coverage, dict) or not coverage or len(coverage) > 128:
        raise EvidenceError("CI_COVERAGE: missing or oversized requirement set")
    for gate, fingerprint in coverage.items():
        if (not isinstance(gate, str) or not GATE.fullmatch(gate)
                or not isinstance(fingerprint, str) or not DIGEST.fullmatch(fingerprint)):
            raise EvidenceError("CI_COVERAGE: invalid gate input fingerprint")


def _key(key):
    if type(key) is not bytes or len(key) != 32:
        raise EvidenceError("CI_TRUST: protected receipt key unavailable")


def sign(request, coverage, *, key, policy_digest, executor_sha):
    """Protected issuer only, after authenticating every required job and input."""
    from r27_verified_coverage import SHA
    _key(key)
    request = validate_request(request)
    validate_coverage(coverage)
    if (not isinstance(policy_digest, str) or not DIGEST.fullmatch(policy_digest)
            or not isinstance(executor_sha, str) or not SHA.fullmatch(executor_sha)):
        raise EvidenceError("CI_TRUST: invalid source or policy pin")
    if request["stage"] == "release" and "release/smoke" not in coverage:
        raise EvidenceError("CI_SMOKE: artifact-bound release smoke is mandatory")
    body = dict(schema=SCHEMA, request=request, policy_digest=policy_digest,
                executor_sha=executor_sha, coverage=coverage)
    body["signature"] = Ed25519PrivateKey.from_private_bytes(key).sign(canonical(body)).hex()
    raw = canonical(body) + b"\n"
    if len(raw) > MAX_BYTES:
        raise EvidenceError("CI_SIZE: compact receipt too large")
    return raw


def verify(raw, *, key, policy_digest, executor_sha):
    """Verify independently from run retention. Returns authenticated coverage."""
    _key(key)
    if type(raw) is not bytes or not raw or len(raw) > MAX_BYTES:
        raise EvidenceError("CI_SIZE: invalid compact receipt")
    try:
        body = json.loads(raw, object_pairs_hook=_unique, parse_constant=_reject_nonfinite)
    except (ValueError, UnicodeError) as exc:
        raise EvidenceError("CI_SCHEMA: invalid compact receipt JSON") from exc
    if not isinstance(body, dict) or set(body) != FIELDS or body.get("schema") != SCHEMA:
        raise EvidenceError("CI_SCHEMA: invalid compact receipt fields")
    signature = body.pop("signature")
    if not isinstance(signature, str) or not re.fullmatch(r"[0-9a-f]{128}", signature):
        raise EvidenceError("CI_SIGNATURE: invalid receipt signature")
    try:
        Ed25519PublicKey.from_public_bytes(key).verify(bytes.fromhex(signature), canonical(body))
    except (ValueError, InvalidSignature) as exc:
        raise EvidenceError("CI_SIGNATURE: compact receipt authentication failed") from exc
    validate_request(body["request"])
    validate_coverage(body["coverage"])
    if body["policy_digest"] != policy_digest or body["executor_sha"] != executor_sha:
        raise EvidenceError("CI_POLICY: receipt policy or executor changed")
    if body["request"]["stage"] == "release" and "release/smoke" not in body["coverage"]:
        raise EvidenceError("CI_SMOKE: release receipt lacks smoke")
    return body


def schedule(request, required, receipts, *, key, policy_digest, executor_sha, failed=()):
    """Reuse identical inputs only; exact artifact/candidate smoke never carries over."""
    request = validate_request(request)
    validate_coverage(required)
    if (not isinstance(receipts, (tuple, list)) or len(receipts) > 64
            or not isinstance(failed, (tuple, list, set, frozenset))
            or any(not isinstance(gate, str) for gate in failed)
            or not set(failed) <= set(required)):
        raise EvidenceError("CI_COVERAGE: invalid prior receipts or failures")
    reusable = set()
    for raw in receipts:
        old = verify(raw, key=key, policy_digest=policy_digest, executor_sha=executor_sha)
        # Release proofs cannot be consumed as PR/integration authorization.
        if old["request"]["stage"] == "release" and request["stage"] != "release":
            continue
        for gate, digest in required.items():
            if gate == "release/smoke" and old["request"] != request:
                continue
            if old["coverage"].get(gate) == digest:
                reusable.add(gate)
    reusable -= set(failed)
    pending = set(required) - reusable
    return {"state": "ACTION_REQUIRED" if failed else "WAITING" if pending else "PASS",
            "code": "CI_FAILED" if failed else "CI_PENDING" if pending else "CI_COVERED",
            "reusable": sorted(reusable), "pending": sorted(pending), "failed": sorted(set(failed))}

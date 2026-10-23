"""Compact, offline-verifiable evidence receipts (isolated v0.4.27 prototype).

HMAC trust requires a separately provisioned secret shared with the verifier.
The caller must be an authorized, trusted qualification owner; this module
does not decide whether CI gates passed.
"""
import hashlib
import hmac
import json
import re

SCHEMA = "baseharbor.evidence-receipt/v1"
SHA = re.compile(r"[0-9a-f]{40}\Z")
DIGEST = re.compile(r"[0-9a-f]{64}\Z")
MAX_BYTES = 4096


class ReceiptError(ValueError):
    pass


def _canonical(value):
    return json.dumps(value, sort_keys=True, separators=(",", ":"), allow_nan=False).encode("utf-8")


def _validate_fields(body):
    if not isinstance(body, dict) or set(body) != {"schema", "kind", "commit", "policy_digest", "result"}:
        raise ReceiptError("EVIDENCE_SCHEMA: unexpected receipt fields")
    if body["schema"] != SCHEMA or body["kind"] not in ("pr", "integration", "release"):
        raise ReceiptError("EVIDENCE_SCHEMA: invalid receipt schema or kind")
    if not isinstance(body["commit"], str) or not SHA.fullmatch(body["commit"]):
        raise ReceiptError("EVIDENCE_COMMIT: invalid exact commit SHA")
    if not isinstance(body["policy_digest"], str) or not DIGEST.fullmatch(body["policy_digest"]):
        raise ReceiptError("EVIDENCE_POLICY: invalid policy digest")
    if type(body["result"]) is not str or body["result"] != "PASS":
        raise ReceiptError("EVIDENCE_RESULT: only verified PASS may be recorded")


def _validate_key(key):
    if not isinstance(key, bytes) or len(key) < 32:
        raise ReceiptError("EVIDENCE_TRUST: trusted key must contain at least 32 bytes")


def issue(kind, commit, policy_digest, key):
    """Call ONLY after a separate trusted verifier has accepted every required gate."""
    _validate_key(key)
    body = {"schema": SCHEMA, "kind": kind, "commit": commit,
            "policy_digest": policy_digest, "result": "PASS"}
    _validate_fields(body)
    signature = hmac.new(key, _canonical(body), hashlib.sha256).hexdigest()
    encoded = _canonical({**body, "mac": signature})
    if len(encoded) > MAX_BYTES:
        raise ReceiptError("EVIDENCE_SIZE: receipt exceeds 4 KiB")
    return encoded + b"\n"


def verify(raw, expected_kind, expected_commit, expected_policy_digest, key):
    """Verify a receipt offline; no GitHub run, API or artifact lookup."""
    _validate_key(key)
    if not isinstance(raw, bytes) or len(raw) > MAX_BYTES:
        raise ReceiptError("EVIDENCE_SIZE: invalid receipt bytes")
    try:
        def unique(pairs):
            result = {}
            for k, v in pairs:
                if k in result:
                    raise ReceiptError("EVIDENCE_SCHEMA: duplicate JSON key")
                result[k] = v
            return result
        envelope = json.loads(raw, object_pairs_hook=unique)
    except (UnicodeError, ValueError) as exc:
        raise ReceiptError("EVIDENCE_SCHEMA: malformed receipt JSON") from exc
    if not isinstance(envelope, dict) or set(envelope) != {
            "schema", "kind", "commit", "policy_digest", "result", "mac"}:
        raise ReceiptError("EVIDENCE_SCHEMA: invalid receipt envelope")
    mac = envelope.pop("mac")
    _validate_fields(envelope)
    if not isinstance(mac, str) or not DIGEST.fullmatch(mac):
        raise ReceiptError("EVIDENCE_SIGNATURE: invalid MAC")
    actual = hmac.new(key, _canonical(envelope), hashlib.sha256).hexdigest()
    if not hmac.compare_digest(mac, actual):
        raise ReceiptError("EVIDENCE_SIGNATURE: authentication failed")
    if envelope["kind"] != expected_kind or envelope["commit"] != expected_commit:
        raise ReceiptError("EVIDENCE_TARGET: receipt belongs to another candidate")
    if envelope["policy_digest"] != expected_policy_digest:
        raise ReceiptError("EVIDENCE_POLICY: receipt was qualified under another policy")
    return True

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
    def unique_pairs(pairs):
        result = {}
        for name, value in pairs:
            if name in result:
                raise EvidenceError("EVIDENCE_POLICY: duplicate protected key")
            result[name] = value
        return result
    try:
        decoded = json.loads(raw, object_pairs_hook=unique_pairs)
    except EvidenceError:
        raise
    except (ValueError, UnicodeError) as exc:
        raise EvidenceError("EVIDENCE_POLICY: invalid protected policy") from exc
    if not isinstance(decoded, dict) or not decoded:
        raise EvidenceError("EVIDENCE_POLICY: empty protected policy")
    canonical = json.dumps(decoded,sort_keys=True,separators=(",",":"),
                           ensure_ascii=True,allow_nan=False).encode("ascii")
    if raw != canonical:
        raise EvidenceError("EVIDENCE_POLICY: noncanonical protected policy")
    import re
    if any(not isinstance(gate, str) or not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9_./:-]{0,127}", gate)
           or not isinstance(job, dict)
           or set(job) != {"job_name", "workflow_path"}
           or not isinstance(job["job_name"], str)
           or not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9 _./:()-]{0,127}", job["job_name"])
           or not isinstance(job["workflow_path"], str)
           or not re.fullmatch(r"\.github/workflows/[A-Za-z0-9_.-]+\.ya?ml", job["workflow_path"])
           for gate, job in decoded.items()):
        raise EvidenceError("EVIDENCE_POLICY: invalid protected gate mapping")
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

def read_pinned_policy_file(policy_path, protected_digest, *, trusted_owner_uid=None):
    """Read a pinned policy file; trusted executor must own parent directories."""
    import os
    import stat
    if not isinstance(policy_path, str) or not os.path.isabs(policy_path):
        raise EvidenceError("EVIDENCE_POLICY: absolute protected path required")
    if not hasattr(os, "O_NOFOLLOW"):
        raise EvidenceError("EVIDENCE_POLICY: nofollow unavailable")
    if type(trusted_owner_uid) is not int or trusted_owner_uid < 0:
        raise EvidenceError("EVIDENCE_POLICY: invalid trusted owner")
    def check_protection(info):
        if info.st_uid not in (0, trusted_owner_uid) or info.st_mode & 0o022:
            raise EvidenceError("EVIDENCE_POLICY: unprotected owner or permissions")
    try:
        parts = policy_path.split("/")[1:]
        if not parts or any(part in ("", ".", "..") for part in parts):
            raise EvidenceError("EVIDENCE_POLICY: unsafe protected path")
        dirfd = os.open("/", os.O_RDONLY | os.O_DIRECTORY)
        try:
            check_protection(os.fstat(dirfd))
            for component in parts[:-1]:
                nextfd = os.open(component, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW, dir_fd=dirfd)
                os.close(dirfd)
                dirfd = nextfd
                check_protection(os.fstat(dirfd))
            fd = os.open(parts[-1], os.O_RDONLY | os.O_NOFOLLOW | getattr(os, "O_CLOEXEC", 0), dir_fd=dirfd)
        finally:
            os.close(dirfd)
        try:
            info = os.fstat(fd)
            check_protection(info)
            if not stat.S_ISREG(info.st_mode) or info.st_size > MAX_BYTES:
                raise EvidenceError("EVIDENCE_POLICY: invalid protected file")
            raw = os.read(fd, MAX_BYTES + 1)
        finally:
            os.close(fd)
    except OSError as exc:
        raise EvidenceError("EVIDENCE_POLICY: cannot read protected file") from exc
    return load_pinned_policy(raw, protected_digest=protected_digest)

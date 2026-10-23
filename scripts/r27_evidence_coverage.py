"""Fail-closed selection from trusted, candidate-bound gate proofs.

The caller must authenticate each proof and provide the complete required-gate
dependency closure. This helper neither authenticates CI nor issues receipts.
"""
from dataclasses import dataclass
import re

SHA = re.compile(r"[0-9a-f]{40}\Z")
DIGEST = re.compile(r"[0-9a-f]{64}\Z")


@dataclass(frozen=True)
class GateProof:
    gate: str
    candidate: str
    policy_digest: str
    artifact_digest: str | None = None
    authenticated: bool = False  # advisory only; never an authority boundary


@dataclass(frozen=True)
class Decision:
    state: str
    code: str
    required: tuple
    reusable: tuple
    pending: tuple
    failed: tuple
    action: str

    def as_dict(self):
        return dict(state=self.state, code=self.code, required=list(self.required),
                    reusable=list(self.reusable), pending=list(self.pending),
                    failed=list(self.failed), action=self.action)


def plan(required, proofs, *, stage, candidate, policy_digest,
         artifact_digest=None, failed=(), trust_available=True):
    """Return minimal missing gates; fail closed on stale or untrusted proofs."""
    if stage not in ("pr", "integration", "release"):
        raise ValueError("EVIDENCE_STAGE: unsupported stage")
    if not isinstance(candidate, str) or not SHA.fullmatch(candidate):
        raise ValueError("EVIDENCE_TARGET: invalid candidate")
    if not isinstance(policy_digest, str) or not DIGEST.fullmatch(policy_digest):
        raise ValueError("EVIDENCE_POLICY: invalid policy digest")
    if artifact_digest is not None and (
            not isinstance(artifact_digest, str) or not DIGEST.fullmatch(artifact_digest)):
        raise ValueError("EVIDENCE_ARTIFACT: invalid artifact digest")
    if stage == "release" and artifact_digest is None:
        raise ValueError("EVIDENCE_ARTIFACT: release artifact digest required")
    if not isinstance(required, (tuple, list, set, frozenset)) or not isinstance(
            proofs, (tuple, list)) or not isinstance(failed, (tuple, list, set, frozenset)):
        raise ValueError("EVIDENCE_INPUT: invalid collection")
    for value in (*required, *failed):
        if not isinstance(value, str) or not value or len(value) > 128:
            raise ValueError("EVIDENCE_INPUT: invalid gate")
    required_set, failed_set = set(required), set(failed)
    if not failed_set <= required_set:
        raise ValueError("EVIDENCE_INPUT: unknown failed gate")
    if stage == "release" and "release/smoke" not in required_set:
        raise ValueError("EVIDENCE_SMOKE: release smoke gate is mandatory")
    for proof in proofs:
        if not isinstance(proof, GateProof) or not isinstance(proof.gate, str):
            raise ValueError("EVIDENCE_INPUT: invalid proof")
        if proof.gate not in required_set:
            raise ValueError("EVIDENCE_INPUT: proof for unknown gate")
    ordered = tuple(sorted(required_set))
    # This helper is intentionally incapable of granting a trusted PASS.
    # A caller-controlled authenticated flag is not a verifier signature.
    # Only the independent canonical verifier may authorize qualification.
    if not trust_available:
        return Decision("BLOCKED", "EVIDENCE_TRUST", ordered, (), ordered, (),
                        "Restore trusted verifier and rerun qualification")
    reusable_set = set()
    for proof in proofs:
        if (proof.authenticated is True and proof.candidate == candidate
                and proof.policy_digest == policy_digest):
            if proof.gate == "release/smoke" and proof.artifact_digest != artifact_digest:
                continue
            reusable_set.add(proof.gate)
    reusable_set -= failed_set
    pending_set = required_set - reusable_set - failed_set
    reusable = tuple(sorted(reusable_set))
    failed_sorted = tuple(sorted(failed_set))
    pending = tuple(sorted(pending_set))
    if failed_set:
        return Decision("ACTION_REQUIRED", "EVIDENCE_GATE_FAILED", ordered,
                        reusable, pending, failed_sorted,
                        "Fix failed gates; rerun only affected requirements")
    if pending_set:
        return Decision("WAITING", "EVIDENCE_GATES_PENDING", ordered,
                        reusable, pending, (), "Automatically schedule pending gates")
    return Decision("WAITING", "EVIDENCE_VERIFIER_REQUIRED", ordered, reusable,
                    (), (), "Submit complete candidate-bound proofs to the independent trusted verifier")

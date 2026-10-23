"""Conservative evidence coverage planner; not an authorization owner."""
from dataclasses import dataclass


@dataclass(frozen=True)
class Decision:
    state: str
    code: str
    summary: str
    reusable: tuple
    pending: tuple
    action: str

    def as_dict(self):
        return dict(state=self.state, code=self.code, summary=self.summary,
                    reusable=list(self.reusable), pending=list(self.pending), action=self.action)


def plan(required, covered, *, stage, exact_candidate, trusted=True, failed=()):
    """Required is the trusted planner's complete dependency closure.

    Covered is authenticated by the trusted verifier before calling this function.
    This helper does not infer coverage or issue PASS receipts.
    """
    if stage not in ("pr", "integration", "release"):
        raise ValueError("EVIDENCE_STAGE: unsupported qualification stage")
    if not isinstance(exact_candidate, str) or len(exact_candidate) != 40 or any(
            c not in "0123456789abcdef" for c in exact_candidate):
        raise ValueError("EVIDENCE_TARGET: invalid candidate SHA")
    for name, items in (("required", required), ("covered", covered), ("failed", failed)):
        if not isinstance(items, (list, tuple, set, frozenset)) or any(
                not isinstance(item, str) or not item or len(item) > 128 for item in items):
            raise ValueError("EVIDENCE_INPUT: invalid " + name)
    required_set, covered_set, failed_set = set(required), set(covered), set(failed)
    if not covered_set <= required_set or not failed_set <= required_set:
        raise ValueError("EVIDENCE_INPUT: unknown gate in coverage")
    if stage == "release" and "release/smoke" not in required_set:
        raise ValueError("EVIDENCE_SMOKE: mandatory final artifact smoke gate missing")
    reusable = tuple(sorted(covered_set - failed_set))
    pending = tuple(sorted(required_set - covered_set - failed_set))
    if not trusted:
        return Decision("BLOCKED", "EVIDENCE_TRUST", "Trusted qualification source unavailable",
                        (), tuple(sorted(required_set)), "Restore trusted verifier; do not issue PASS")
    if failed_set:
        return Decision("ACTION_REQUIRED", "EVIDENCE_GATE_FAILED",
                        "Required gates failed: " + ", ".join(sorted(failed_set)),
                        reusable, tuple(sorted(failed_set | set(pending))),
                        "Fix failing gates; rerun only invalidated requirements")
    if pending:
        return Decision("WAITING", "EVIDENCE_GATES_PENDING",
                        "Automatically schedule missing gates: " + ", ".join(pending),
                        reusable, pending, "No developer action while gates run")
    return Decision("PASS", "EVIDENCE_COVERAGE_COMPLETE",
                    "All required gates have authenticated coverage",
                    reusable, (), "Trusted issuer may record qualified result")

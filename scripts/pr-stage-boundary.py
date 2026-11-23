#!/usr/bin/env python3
"""Keep executable native PR gates selected and defer only cross-repo debt.

The trusted PR planner still discovers every required release gate.  Only the
PR execution stage is changed: cross-repo gates are recorded, never reported
as passed, and must be fulfilled by develop/release acceptance.
"""
import argparse
import json
from pathlib import Path

CROSS_REPO_STATIC = "integration/static/public-contracts"
CORELESS_LIFECYCLE_GATES = {
    "integration/docker/coreless-lifecycle",
    "integration/podman/coreless-lifecycle",
}
# Release-only STATIC gates under the atomic tier (e.g.
# atomic/static/shell-ux, atomic/static/ci-fast-check) run their executable
# check inside the PR itself. The stage boundary must classify them as
# PR-executed static gates: they are selected, never deferred. Only the exact
# gate ids listed here are PR-executed; every other atomic/static-shaped name
# stays unclassified so failure injection keeps rejecting invented gates.
# This changes nothing for native/runtime acceptance or for the deferred
# cross-repo static integration.
ATOMIC_STATIC_PR_EXECUTED_GATES = (
    "atomic/static/shell-ux",
    "atomic/static/ci-fast-check",
)


def stage(plan):
    if not isinstance(plan, dict) or plan.get("schema") != "baseharbor.pr-gate-plan/v1":
        raise ValueError("invalid trusted PR plan")
    if "deferred_integration_gates" in plan:
        raise ValueError("PR plan already staged")
    gates = plan.get("selected_gates")
    checks = plan.get("required_validation_checks")
    if not isinstance(gates, list) or not all(isinstance(x, str) for x in gates):
        raise ValueError("invalid selected gate list")
    if len(gates) != len(set(gates)):
        raise ValueError("duplicate selected gates")
    if not isinstance(checks, list) or not checks or not all(
        isinstance(x, dict) and isinstance(x.get("id"), str) and x["id"] for x in checks
    ):
        raise ValueError("no independently executable PR validation checks")
    for gate in gates:
        parts = gate.split("/")
        if gate not in (CROSS_REPO_STATIC, *CORELESS_LIFECYCLE_GATES,
                        *ATOMIC_STATIC_PR_EXECUTED_GATES) and not (
            len(parts) == 3 and parts[0] == "atomic" and parts[1] in ("docker", "podman")
        ):
            raise ValueError("unclassified gate cannot be deferred: " + gate)
    deps = plan.get("required_consumer_dependencies")
    if not isinstance(deps, list) or any(x not in ("core", "demo", "console", "connector") for x in deps):
        raise ValueError("invalid consumer dependency graph")
    # Atomic runtime and Coreless gates have real PR executors. Deferring them
    # would conceal successful native evidence and contradict the final verifier.
    # Only the cross-repository static integration remains deferred.
    selected = [gate for gate in gates if gate != CROSS_REPO_STATIC]
    deferred = [gate for gate in gates if gate == CROSS_REPO_STATIC]
    output = dict(plan)
    output["deferred_integration_gates"] = deferred
    output["deferred_consumer_dependencies"] = list(deps)
    output["selected_gates"] = selected
    output["pr_qualification_scope"] = "self-contained-v1"
    # Never overwrite unresolved_coverage or claim that deferred gates passed.
    return output


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--plan", type=Path, required=True)
    args = parser.parse_args()
    original = json.loads(args.plan.read_text())
    result = stage(original)
    args.plan.write_text(json.dumps(result, sort_keys=True) + "\n")
    print("PR · self-contained checks; deferred integration: " +
          (", ".join(result["deferred_integration_gates"]) or "none"))


if __name__ == "__main__":
    main()

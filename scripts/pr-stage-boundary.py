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
# Release-only STATIC gate under the atomic tier whose executable check runs at
# release/qualify time, NOT inside the PR. The stage boundary classifies it as a
# DEFERRED release-only gate (like CROSS_REPO_STATIC): it is selected, never
# emitted as PR-executed, and pulled out of the PR-executed list so failure
# injection keeps rejecting invented gates. Every other atomic/static-shaped
# name stays unclassified. No PR-executed class exists for it and none is
# created: its only executor is the release/qualify native runtime
# (pre-release.yml:663 static_gates, see the comment in
# pr-required-final.py for the matching deferred allow-list).
ATOMIC_STATIC_DEFERRED_RELEASE_ONLY = ("atomic/static/mcp",)
# Release-only STATIC gates under the atomic tier (e.g.
# atomic/static/shell-ux, atomic/static/ci-fast-check) run their executable
# check inside the PR itself. The stage boundary must classify them as
# PR-executed static gates: they are selected, never deferred. Only the exact
# gate ids listed here are PR-executed; every other atomic/static-shaped name
# stays unclassified so failure injection keeps rejecting invented gates.
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
                        *ATOMIC_STATIC_PR_EXECUTED_GATES,
                        *ATOMIC_STATIC_DEFERRED_RELEASE_ONLY) and not (
            len(parts) == 3 and parts[0] == "atomic" and parts[1] in ("docker", "podman")
        ):
            raise ValueError("unclassified gate cannot be deferred: " + gate)
    deps = plan.get("required_consumer_dependencies")
    if not isinstance(deps, list) or any(x not in ("core", "demo", "console", "connector") for x in deps):
        raise ValueError("invalid consumer dependency graph")
    # Atomic runtime and Coreless gates have real PR executors. Deferring them
    # would conceal successful native evidence and contradict the final verifier.
    # The release-only atomic/static gates (mcp) and the cross-repository static
    # integration are deferred: recorded, never reported as passed, and fulfilled
    # by develop/release acceptance. mcp has no PR-executed class; its only
    # executor is the release/qualify native runtime
    # (pre-release.yml:663 -> pre-release-runtime-suite.yml:267,
    # GATE=mcp runs tests/mcp/run.sh), so it stays out of the PR-executed
    # list exactly like the cross-repo gate below.
    deferred = [
        gate for gate in gates
        if gate == CROSS_REPO_STATIC or gate in ATOMIC_STATIC_DEFERRED_RELEASE_ONLY
    ]
    selected = [gate for gate in gates if gate not in deferred]
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

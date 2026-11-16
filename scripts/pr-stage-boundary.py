#!/usr/bin/env python3
"""Keep supported Coreless PR integrations selected and defer remaining debt.

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
        if gate not in (CROSS_REPO_STATIC, *CORELESS_LIFECYCLE_GATES) and not (
            len(parts) == 3 and parts[0] == "atomic" and parts[1] in ("docker", "podman")
        ):
            raise ValueError("unclassified gate cannot be deferred: " + gate)
    deps = plan.get("required_consumer_dependencies")
    if not isinstance(deps, list) or any(x not in ("core", "demo", "console", "connector") for x in deps):
        raise ValueError("invalid consumer dependency graph")
    # The coreless lifecycle gates run against the exact Core candidate through
    # the PR's selected integration executor. Keep only those executable gates
    # selected; other integration work remains deferred to develop/release.
    selected = [gate for gate in gates if gate in CORELESS_LIFECYCLE_GATES]
    deferred = [gate for gate in gates if gate not in CORELESS_LIFECYCLE_GATES]
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

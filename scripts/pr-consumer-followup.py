#!/usr/bin/env python3
"""Show PR follow-up dependencies without turning open integration into passing evidence."""
import argparse
import json
from pathlib import Path

REPOS = {
    "demo": "mcpdev80/baseharbor-demo",
    "console": "mcpdev80/baseharbor-console",
    "connector": "mcpdev80/baseharbor-node-connector",
}


def notice(plan):
    if not isinstance(plan, dict) or plan.get("schema") != "baseharbor.pr-gate-plan/v1":
        raise ValueError("missing trusted PR plan")
    dependencies = plan.get("deferred_consumer_dependencies")
    gates = plan.get("deferred_integration_gates")
    if (not isinstance(dependencies, list) or
            any(role not in {"core", *REPOS} for role in dependencies) or
            len(dependencies) != len(set(dependencies)) or
            not isinstance(gates, list) or
            any(not isinstance(gate, str) for gate in gates)):
        raise ValueError("untrusted or incomplete integration dependencies")
    affected = [role for role in REPOS if role in dependencies]
    if affected:
        names = ", ".join(REPOS[role] for role in affected)
        message = ("Consumer follow-up to review: " + names +
                   ". Check the relevant contracts, update consumers when needed, "
                   "and qualify compatible revisions during integration/release. "
                   "This is not a PR merge blocker or evidence that integration passed.")
    elif gates:
        message = ("Deferred integration gates need follow-up: " + ", ".join(gates) +
                   ". No consumer is identified by the trusted plan.")
    else:
        message = "No cross-repository follow-up identified by the trusted PR plan."
    return message, affected, gates


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--plan", required=True, type=Path)
    parser.add_argument("--summary", type=Path)
    args = parser.parse_args()
    try:
        message, affected, gates = notice(json.loads(args.plan.read_text()))
    except (ValueError, OSError, json.JSONDecodeError) as exc:
        message = "Consumer follow-up unresolved: inspect the PR dependency plan (" + str(exc) + ")."
        affected, gates = [], []
    print("::notice title=Consumer follow-up::" + message.replace("\n", " "))
    if args.summary:
        with args.summary.open("a") as out:
            out.write("\n### Consumer follow-up (non-blocking)\n\n")
            out.write(message + "\n")
            if affected:
                out.write("\n| Repository | Follow-up |\n| --- | --- |\n")
                for role in affected:
                    out.write(f"| [{REPOS[role]}](https://github.com/{REPOS[role]}) | Review impact and update if necessary |\n")
            if gates:
                out.write("\nDeferred integration: " + ", ".join(gates) + "\n")
            out.write("\nPR checks do not certify cross-repository compatibility.\n")


if __name__ == "__main__":
    main()

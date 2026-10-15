#!/usr/bin/env python3
"""Strict dependency classification from the trusted release-gate inventory."""
import argparse
import json
from pathlib import Path


def dependency_mode(plan, requirements):
    selected = plan.get("selected_gates")
    if not isinstance(selected, list) or not selected:
        raise ValueError("missing selected gates")
    catalog = {g["id"]: g for g in requirements["gates"]
               if "v0.4.26" in g.get("required_for", [])}
    union = set()
    for gate in selected:
        if gate not in catalog:
            raise ValueError("unknown required gate: " + str(gate))
        deps = catalog[gate].get("dependencies")
        if not isinstance(deps, list) or "core" not in deps:
            raise ValueError("invalid declared gate dependencies")
        union.update(deps)
    if not union <= {"core", "demo", "console", "connector"}:
        raise ValueError("unknown consumer dependencies")
    return {"dependencies": sorted(union),
            "mode": ("ecosystem" if union & {"console", "connector"}
                     else "demo" if "demo" in union else "core")}


def main():
    p = argparse.ArgumentParser()
    p.add_argument("--plan", type=Path, required=True)
    p.add_argument("--requirements", type=Path, required=True)
    args = p.parse_args()
    print(json.dumps(dependency_mode(json.loads(args.plan.read_text()),
                                     json.loads(args.requirements.read_text()))))


if __name__ == "__main__":
    main()

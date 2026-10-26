#!/usr/bin/env python3
"""Bounded selected PR runtime gate dispatch; never suppresses required coverage."""
import argparse
import json
from pathlib import Path


def select(plan, requirements):
    selected = plan.get("selected_gates")
    if not isinstance(selected, list) or len(selected) != len(set(selected)):
        raise ValueError("invalid selected gate set")
    catalog = {g["id"]:g for g in requirements["gates"] if "v0.4.26" in g.get("required_for", [])}
    if not set(selected) <= set(catalog):
        raise ValueError("unknown selected release gate")
    supported = []
    for gate in selected:
        parts = gate.split("/")
        if len(parts) == 3 and parts[0] == "atomic" and parts[1] in ("docker", "podman"):
            supported.append({"id":gate,"runtime":parts[1],"gate":parts[2]})
    # Schedule ALL supported gates. Workflow matrix max-parallel caps concurrency,
    # not coverage. Unsupported selected gates remain blocking in final acceptance.
    included = sorted(supported, key=lambda item: item["id"])
    return {"include": included or [{"id":"not-required","runtime":"static","gate":"not-required"}],
            "required": bool(supported), "unsupported": sorted(set(selected) - {x["id"] for x in supported} - {"integration/static/public-contracts"})}


def main():
    p = argparse.ArgumentParser()
    p.add_argument("--plan", type=Path, required=True)
    p.add_argument("--requirements", type=Path, required=True)
    args = p.parse_args()
    print(json.dumps(select(json.loads(args.plan.read_text()),
                            json.loads(args.requirements.read_text())),
                     separators=(",",":")))


if __name__ == "__main__":
    main()

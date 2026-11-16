#!/usr/bin/env python3
"""Validate declarative PR source-to-gate coverage against the release catalog."""
from __future__ import annotations

import argparse
import json
from pathlib import Path
import re
import sys

ROOT = Path(__file__).resolve().parent
SCHEMA = "baseharbor.pr-gate-coverage/v1"
SURFACE_RE = re.compile(r"^[a-z][a-z0-9_.-]*$")


def validate(coverage: dict, requirements: dict) -> list[str]:
    errors: list[str] = []
    if coverage.get("schema") != SCHEMA:
        errors.append(f"coverage schema must be {SCHEMA}")
    release = coverage.get("release")
    if not isinstance(release, str) or release not in requirements.get("releases", []):
        errors.append(f"coverage release {release!r} is not in release requirements")

    catalog = requirements.get("gates", [])
    gate_ids = {row.get("id") for row in catalog if isinstance(row, dict)}
    sources = coverage.get("sources")
    gates = coverage.get("gates")
    if not isinstance(sources, list) or not isinstance(gates, list):
        return errors + ["coverage sources and gates must be arrays"]

    source_surfaces: set[str] = set()
    source_paths: set[str] = set()
    for index, source in enumerate(sources):
        if not isinstance(source, dict):
            errors.append(f"sources[{index}] must be an object")
            continue
        path = source.get("path")
        if not isinstance(path, str) or not path or path.startswith("/") or ".." in Path(path).parts:
            errors.append(f"sources[{index}].path must be a normalized repository-relative path")
        elif path in source_paths:
            errors.append(f"duplicate source path: {path}")
        else:
            source_paths.add(path)
        surfaces = source.get("surfaces")
        if not isinstance(surfaces, list) or not surfaces:
            errors.append(f"sources[{index}].surfaces must be a non-empty array")
            continue
        for surface in surfaces:
            if not isinstance(surface, str) or not SURFACE_RE.fullmatch(surface):
                errors.append(f"sources[{index}] has invalid surface {surface!r}")
            elif surface in source_surfaces:
                # Multiple source paths may own the same surface.
                continue
            else:
                source_surfaces.add(surface)

    seen_gates: set[str] = set()
    covered_surfaces: set[str] = set()
    for index, gate in enumerate(gates):
        if not isinstance(gate, dict):
            errors.append(f"gates[{index}] must be an object")
            continue
        gate_id = gate.get("id")
        if not isinstance(gate_id, str) or gate_id not in gate_ids:
            errors.append(f"gates[{index}] references unknown release gate {gate_id!r}")
            continue
        if gate_id in seen_gates:
            errors.append(f"duplicate gate coverage: {gate_id}")
        seen_gates.add(gate_id)
        parts = gate_id.split("/")
        if len(parts) == 3 and parts[0] == "atomic":
            expected_runtime = parts[1]
            if expected_runtime not in {"docker", "podman"}:
                errors.append(f"gate {gate_id} has unsupported runtime {expected_runtime!r}")
        covers = gate.get("covers")
        if not isinstance(covers, list) or not covers:
            errors.append(f"gates[{index}].covers must be a non-empty array")
            continue
        for surface in covers:
            if not isinstance(surface, str) or not SURFACE_RE.fullmatch(surface):
                errors.append(f"gate {gate_id} has invalid surface {surface!r}")
            elif surface not in source_surfaces:
                errors.append(f"gate {gate_id} covers undeclared source surface {surface!r}")
            else:
                covered_surfaces.add(surface)

    for surface in sorted(source_surfaces - covered_surfaces):
        errors.append(f"source surface {surface!r} has no release gate owner")
    return errors


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--coverage", type=Path, default=ROOT / "pr-gate-coverage.json")
    parser.add_argument("--requirements", type=Path, default=ROOT / "release-requirements.json")
    args = parser.parse_args()
    try:
        coverage = json.loads(args.coverage.read_text())
        requirements = json.loads(args.requirements.read_text())
    except (OSError, json.JSONDecodeError) as exc:
        print(f"invalid coverage catalog input: {exc}", file=sys.stderr)
        return 2
    errors = validate(coverage, requirements)
    if errors:
        print("\n".join(errors), file=sys.stderr)
        return 1
    print("PR gate coverage catalog is valid")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

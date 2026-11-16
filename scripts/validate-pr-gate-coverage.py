#!/usr/bin/env python3
"""Validate and resolve declarative PR source-to-gate coverage contracts."""
from __future__ import annotations

import argparse
import json
from pathlib import Path
import re
import sys

ROOT = Path(__file__).resolve().parent
SCHEMA = "baseharbor.pr-gate-coverage/v1"
SURFACE_RE = re.compile(r"^[a-z][a-z0-9_.-]*$")


def resolve_covered_gates(changed_paths: list[str], coverage: dict) -> tuple[list[str], list[str]]:
    """Resolve changed repository paths to owning runtime gates.

    Paths inherit coverage from their declared package/component root. Any path
    without a declared owner is returned as unresolved for the caller's
    conservative fallback; it is never silently treated as covered.
    """
    packages = coverage.get("packages", [])
    gates = coverage.get("gates", [])
    selected_surfaces: set[str] = set()
    unresolved: list[str] = []
    for changed in changed_paths:
        owners = [
            row for row in packages
            if isinstance(row, dict)
            and isinstance(row.get("path"), str)
            and (changed == row["path"] or changed.startswith(row["path"].rstrip("/") + "/"))
        ]
        if not owners:
            unresolved.append(changed)
            continue
        # Nested package roots are handled by the most specific owner. Multiple
        # equally specific owners intentionally union their surface contracts.
        longest = max(len(row["path"].rstrip("/")) for row in owners)
        for row in owners:
            if len(row["path"].rstrip("/")) == longest:
                selected_surfaces.update(row.get("surfaces", []))
    selected_gates = sorted({
        row["id"] for row in gates if isinstance(row, dict)
        and selected_surfaces.intersection(row.get("covers", []))
        and isinstance(row.get("id"), str)
    })
    return selected_gates, unresolved


def validate(coverage: dict, requirements: dict) -> list[str]:
    errors: list[str] = []
    if coverage.get("schema") != SCHEMA:
        errors.append(f"coverage schema must be {SCHEMA}")
    release = coverage.get("release")
    if not isinstance(release, str) or release not in requirements.get("releases", []):
        errors.append(f"coverage release {release!r} is not in release requirements")

    catalog_ids = {
        row.get("id") for row in requirements.get("gates", []) if isinstance(row, dict)
    }
    surfaces = coverage.get("surfaces")
    packages = coverage.get("packages")
    gates = coverage.get("gates")
    if not isinstance(surfaces, list) or not isinstance(packages, list) or not isinstance(gates, list):
        return errors + ["coverage surfaces, packages, and gates must be arrays"]

    declared_surfaces: set[str] = set()
    for index, surface in enumerate(surfaces):
        if not isinstance(surface, str) or not SURFACE_RE.fullmatch(surface):
            errors.append(f"surfaces[{index}] has invalid identifier {surface!r}")
        elif surface in declared_surfaces:
            errors.append(f"duplicate surface: {surface}")
        else:
            declared_surfaces.add(surface)

    package_paths: set[str] = set()
    package_surfaces: set[str] = set()
    for index, package in enumerate(packages):
        if not isinstance(package, dict):
            errors.append(f"packages[{index}] must be an object")
            continue
        path = package.get("path")
        if (not isinstance(path, str) or not path or path.startswith("/")
                or "\\" in path or ".." in Path(path).parts
                or path.rstrip("/") != path):
            errors.append(f"packages[{index}].path must be a normalized repository-relative directory")
        elif path in package_paths:
            errors.append(f"duplicate package coverage: {path}")
        else:
            package_paths.add(path)
        owners = package.get("surfaces")
        if not isinstance(owners, list) or not owners:
            errors.append(f"packages[{index}].surfaces must be a non-empty array")
            continue
        for surface in owners:
            if surface not in declared_surfaces:
                errors.append(f"package {path!r} references undeclared surface {surface!r}")
            else:
                package_surfaces.add(surface)

    seen_gates: set[str] = set()
    gate_surfaces: set[str] = set()
    for index, gate in enumerate(gates):
        if not isinstance(gate, dict):
            errors.append(f"gates[{index}] must be an object")
            continue
        gate_id = gate.get("id")
        if not isinstance(gate_id, str) or gate_id not in catalog_ids:
            errors.append(f"gates[{index}] references unknown release gate {gate_id!r}")
            continue
        if gate_id in seen_gates:
            errors.append(f"duplicate gate coverage: {gate_id}")
        seen_gates.add(gate_id)
        parts = gate_id.split("/")
        if len(parts) != 3 or parts[0] != "atomic" or parts[1] not in {"docker", "podman"}:
            errors.append(f"gate {gate_id} must be an atomic Docker or Podman gate")
        covers = gate.get("covers")
        if not isinstance(covers, list) or not covers:
            errors.append(f"gates[{index}].covers must be a non-empty array")
            continue
        for surface in covers:
            if surface not in declared_surfaces:
                errors.append(f"gate {gate_id} covers undeclared surface {surface!r}")
            else:
                gate_surfaces.add(surface)

    for surface in sorted(package_surfaces - gate_surfaces):
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
        print("\\n".join(errors), file=sys.stderr)
        return 1
    print("PR gate coverage catalog is valid")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

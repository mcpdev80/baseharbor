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
    components = coverage.get("components", [])
    gates = coverage.get("gates", [])
    unresolved: list[str] = []
    selected_gates: set[str] = set()
    for changed in changed_paths:
        owners = [row for row in components if isinstance(row, dict)
                  and row.get("path") == changed]
        if not owners:
            owners = [row for row in packages if isinstance(row, dict)
                      and isinstance(row.get("path"), str)
                      and (changed == row["path"] or changed.startswith(row["path"].rstrip("/") + "/"))]
        if not owners:
            unresolved.append(changed)
            continue
        longest = max(len(row["path"].rstrip("/")) for row in owners)
        source_surfaces = set().union(*(set(row.get("surfaces", [])) for row in owners
                                        if len(row["path"].rstrip("/")) == longest))
        matching = {row["id"] for row in gates if isinstance(row, dict)
                    and row.get("selection_scope", "focused") == "focused"
                    and source_surfaces.intersection(row.get("covers", []))
                    and isinstance(row.get("id"), str)}
        # Shared/runtime-neutral source changes run one representative runtime
        # gate (Docker). A Podman-specific source has no Docker sibling in its
        # own contract and therefore keeps its exact Podman gate. Resolve each
        # source independently so a separate Podman-only source in the same
        # change set is not erased by a shared Docker source.
        runtime_specific = any(
            row.get("runtime_specific") is True and
            len(row["path"].rstrip("/")) == longest
            for row in owners)
        for gate_id in list(matching):
            parts = gate_id.split("/")
            if not runtime_specific and len(parts) >= 3 and parts[1] == "podman":
                docker_sibling = "/".join((parts[0], "docker", *parts[2:]))
                if docker_sibling in matching:
                    matching.remove(gate_id)
        selected_gates.update(matching)
    selected_gates = sorted(selected_gates)
    return selected_gates, unresolved


def resolve_native_checks(changed_paths: list[str], coverage: dict) -> list[str]:
    """Resolve source components to exact runtime-specific native checks."""
    components = {row.get("id"): row for row in coverage.get("components", [])
                  if isinstance(row, dict) and isinstance(row.get("id"), str)}
    changed_ids = {component_id for component_id, component in components.items()
                   if component.get("path") in changed_paths}
    selected: set[str] = set()
    checks = [row for row in coverage.get("native_checks", [])
              if isinstance(row, dict)
              and row.get("selection_scope", "focused") == "focused"
              and isinstance(row.get("id"), str)]
    # Resolve per changed component. A shared component uses Docker as its
    # representative when both runtime variants cover that same component;
    # a separate Podman-only component in the same PR still selects Podman.
    for component_id in changed_ids:
        matching = {check["id"] for check in checks
                    if component_id in check.get("source_components", [])}
        for check_id in list(matching):
            parts = check_id.split("/")
            if len(parts) == 3 and parts[0] == "native" and parts[1] == "podman":
                if f"native/docker/{parts[2]}" in matching:
                    matching.remove(check_id)
        selected.update(matching)
    return sorted(selected)


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
    packages = coverage.get("packages", [])
    components = coverage.get("components")
    gates = coverage.get("gates")
    native_checks = coverage.get("native_checks")
    if (not isinstance(surfaces, list) or not isinstance(packages, list)
            or not isinstance(components, list) or not isinstance(gates, list)
            or not isinstance(native_checks, list)):
        return errors + ["coverage surfaces, packages, components, gates, and native_checks must be arrays"]

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

    component_ids: set[str] = set()
    component_paths: set[str] = set()
    component_surfaces: set[str] = set()
    for index, component in enumerate(components):
        if not isinstance(component, dict):
            errors.append(f"components[{index}] must be an object")
            continue
        component_id, path = component.get("id"), component.get("path")
        if not isinstance(component_id, str) or not SURFACE_RE.fullmatch(component_id):
            errors.append(f"components[{index}].id is invalid")
        elif component_id in component_ids:
            errors.append(f"duplicate source component: {component_id}")
        else:
            component_ids.add(component_id)
        if (not isinstance(path, str) or not path or path.startswith("/")
                or "\\" in path or ".." in Path(path).parts):
            errors.append(f"components[{index}].path must be a normalized repository-relative file path")
        elif path in component_paths:
            errors.append(f"duplicate component path: {path}")
        else:
            component_paths.add(path)
        owners = component.get("surfaces")
        if not isinstance(owners, list) or not owners:
            errors.append(f"components[{index}].surfaces must be a non-empty array")
            continue
        for surface in owners:
            if surface not in declared_surfaces:
                errors.append(f"component {component_id!r} references undeclared surface {surface!r}")
            else:
                component_surfaces.add(surface)

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
        if gate.get("selection_scope") not in {"focused", "release-only"}:
            errors.append(f"gate {gate_id} has invalid selection_scope")
        if len(parts) >= 3 and parts[0] in {"atomic", "ha", "integration", "journey"}:
            if gate.get("runtime") != parts[1]:
                errors.append(f"gate {gate_id} runtime does not match its ID")
        if parts[0] == "journey" and gate.get("selection_scope") != "release-only":
            errors.append(f"journey gate {gate_id} must be release-only for PR selection")
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

    native_surfaces = {surface for check in native_checks if isinstance(check, dict)
                       for surface in check.get("covers", []) if isinstance(surface, str)}
    for surface in sorted(component_surfaces - gate_surfaces - native_surfaces):
        errors.append(f"source component surface {surface!r} has no release gate owner")
    required_ids = {row.get("id") for row in requirements.get("gates", [])
                    if isinstance(row, dict) and release in row.get("required_for", [])}
    missing = sorted(required_ids - seen_gates)
    extra = sorted(seen_gates - required_ids)
    if missing:
        errors.append("coverage catalog omits active release gates: " + ", ".join(missing))
    if extra:
        errors.append("coverage catalog includes gates outside active release catalog: " + ", ".join(extra))

    seen_native: set[str] = set()
    for index, check in enumerate(native_checks):
        if not isinstance(check, dict):
            errors.append(f"native_checks[{index}] must be an object")
            continue
        check_id = check.get("id")
        runtime = check.get("runtime")
        if not isinstance(check_id, str) or not check_id.startswith(f"native/{runtime}/"):
            errors.append(f"native_checks[{index}] ID must include its runtime")
        if runtime not in {"docker", "podman"}:
            errors.append(f"native_checks[{index}] has unsupported runtime {runtime!r}")
        if check_id in seen_native:
            errors.append(f"duplicate native check: {check_id}")
        seen_native.add(check_id)
        if check.get("selection_scope") not in {"focused", "release-only"}:
            errors.append(f"native check {check_id} has invalid selection_scope")
        covers = check.get("covers")
        if not isinstance(covers, list) or not covers:
            errors.append(f"native check {check_id} covers must be non-empty")
        elif any(surface not in declared_surfaces for surface in covers):
            errors.append(f"native check {check_id} covers an undeclared surface")
        owners = check.get("source_components")
        if not isinstance(owners, list) or not owners:
            errors.append(f"native check {check_id} source_components must be non-empty")
        elif any(owner not in component_ids for owner in owners):
            errors.append(f"native check {check_id} references an unknown source component")
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

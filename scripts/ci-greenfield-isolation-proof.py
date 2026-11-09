#!/usr/bin/env python3
"""Fail-closed verification of the native 0427-133 Greenfield acceptance.

Each runtime job must produce Go test -json events plus a .sha sidecar written
from git rev-parse HEAD in the same job. No historical or skipped result counts.
This verifier does not substitute for the actual native test implementation.
"""
from __future__ import annotations

import argparse
import json
from pathlib import Path
import re
import sys

TEST = "TestGreenfieldIsolatedCoreNativeInCI"
PACKAGE = "github.com/mcpdev80/baseharbor/cmd/baha"
SHA_RE = re.compile(r"^[0-9a-f]{40}$")


def verify_runtime(report: Path, sha_file: Path, expected_sha: str, runtime: str) -> None:
    if not report.is_file() or not sha_file.is_file():
        raise ValueError(f"{runtime}: required report or SHA witness missing")
    actual_sha = sha_file.read_text(encoding="utf-8").strip()
    if actual_sha != expected_sha:
        raise ValueError(f"{runtime}: candidate SHA mismatch")
    events = []
    for number, line in enumerate(report.read_text(encoding="utf-8").splitlines(), 1):
        if not line.strip():
            continue
        try:
            event = json.loads(line)
        except json.JSONDecodeError as exc:
            raise ValueError(f"{runtime}: invalid Go JSON at line {number}") from exc
        if not isinstance(event, dict):
            raise ValueError(f"{runtime}: event {number} is not an object")
        events.append(event)
    if not events:
        raise ValueError(f"{runtime}: empty Go test report")
    tests = [e for e in events if e.get("Package") == PACKAGE and e.get("Test") == TEST]
    actions = [e.get("Action") for e in tests]
    if actions.count("pass") != 1 or "fail" in actions or "skip" in actions:
        raise ValueError(f"{runtime}: {TEST} requires one real PASS, no SKIP/FAIL")
    if any(e.get("Action") == "fail" for e in events):
        raise ValueError(f"{runtime}: Go test report contains a failure")
    package_passes = [e for e in events if e.get("Package") == PACKAGE and "Test" not in e and e.get("Action") == "pass"]
    if len(package_passes) != 1:
        raise ValueError(f"{runtime}: package teardown did not PASS exactly once")
    print(f"PASS: {runtime} isolated Greenfield acceptance on {expected_sha}")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--sha", required=True, help="exact integration commit SHA")
    parser.add_argument("--docker", required=True, type=Path, help="Docker go test -json report")
    parser.add_argument("--podman", required=True, type=Path, help="Podman go test -json report")
    args = parser.parse_args()
    if not SHA_RE.fullmatch(args.sha):
        parser.error("--sha must be a 40-digit lowercase commit SHA")
    try:
        for runtime in ("docker", "podman"):
            report = getattr(args, runtime)
            verify_runtime(report, Path(str(report) + ".sha"), args.sha, runtime)
    except (ValueError, OSError) as exc:
        print(f"REJECT: {exc}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

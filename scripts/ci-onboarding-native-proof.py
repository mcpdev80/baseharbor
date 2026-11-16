#!/usr/bin/env python3
"""Reject missing, skipped or incomplete workstation remote-native acceptance."""
import argparse
import json
from pathlib import Path

REQUIRED = ("CoreReady", "Invitation", "WorkstationImport", "RemoteDeployment", "RemoteDestroy")


def verify(events, test):
    status = {}
    for event in events:
        if not isinstance(event, dict):
            raise ValueError("invalid Go test event")
        name = event.get("Test")
        action = event.get("Action")
        if name == test or (isinstance(name, str) and name.startswith(test + "/")):
            if action in ("pass", "fail", "skip"):
                status[name] = action
        elif action == "fail" and not name:
            raise ValueError("Go package failed")
    expected = [test] + [test + "/" + item for item in REQUIRED]
    bad = [name for name in expected if status.get(name) != "pass"]
    if bad:
        raise ValueError("missing/skipped/failed native onboarding proof: " + ", ".join(bad))
    if any(value != "pass" for value in status.values()):
        raise ValueError("native onboarding subtest failed or was skipped")
    return len(status)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--input", type=Path, required=True)
    parser.add_argument("--test", required=True)
    args = parser.parse_args()
    events = [json.loads(line) for line in args.input.read_text().splitlines() if line.strip()]
    count = verify(events, args.test)
    print(f"PASS: {args.test}, {count} explicit successful native test events")


if __name__ == "__main__":
    try:
        main()
    except (ValueError, OSError) as exc:
        raise SystemExit(str(exc))

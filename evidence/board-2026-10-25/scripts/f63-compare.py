#!/usr/bin/env python3
"""F63-R2 baseline honesty comparison: NEW=0 required, refuses incomplete runs."""
import json
import sys


def failures(path):
    names = set()
    saw_package_result = False
    package_failed = False
    with open(path, encoding="utf-8") as handle:
        for line in handle:
            line = line.strip()
            if not line or not line.startswith("{"):
                continue
            event = json.loads(line)
            action = event.get("Action")
            if action in ("pass", "fail") and not event.get("Test"):
                saw_package_result = True
                package_failed = package_failed or action == "fail"
            if event.get("Test") and action == "fail":
                names.add(event["Test"])
    if not saw_package_result:
        raise SystemExit(f"incomplete run (no package result events): {path}")
    return names, package_failed


def main():
    if len(sys.argv) != 3:
        raise SystemExit("usage: f63-compare.py BASE.jsonl HEAD.jsonl")
    base, base_failed = failures(sys.argv[1])
    head, head_failed = failures(sys.argv[2])
    new = sorted(head - base)
    removed = sorted(base - head)
    report = {
        "base_failed_tests": len(base),
        "head_failed_tests": len(head),
        "NEW": len(new),
        "REMOVED": len(removed),
        "new_names": new,
        "removed_names": removed,
        "base_package_fail": base_failed,
        "head_package_fail": head_failed,
    }
    json.dump(report, sys.stdout, indent=2)
    print()
    return 1 if new else 0


if __name__ == "__main__":
    sys.exit(main())

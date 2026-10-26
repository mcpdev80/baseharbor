#!/usr/bin/env python3
import re
import sys
from pathlib import Path

try:
    import yaml
except ImportError:
    print("PyYAML is required to match known failures", file=sys.stderr)
    sys.exit(2)

ROOT = Path(__file__).resolve().parents[1]
REGISTRY = ROOT / ".github" / "known-failures.yaml"

def main() -> int:
    text = sys.stdin.read()
    data = yaml.safe_load(REGISTRY.read_text(encoding="utf-8")) or {}
    matches = []
    for item in data.get("known_failures", []):
        for signature in item.get("signatures", []):
            try:
                matched = re.search(signature, text, re.MULTILINE) is not None
            except re.error:
                matched = signature in text
            if matched:
                matches.append((item.get("id", "unknown"), signature, item.get("status", "unknown")))
                break

    if not matches:
        return 1

    for failure_id, signature, status in matches:
        print(f"{failure_id}\t{status}\t{signature}")
    return 0

if __name__ == "__main__":
    raise SystemExit(main())

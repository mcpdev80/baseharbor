#!/usr/bin/env bash
set -euo pipefail

# Run the existing real Docker/Podman installation-destroy retry acceptance.
# This must not silently SKIP or substitute source/fixture-only coverage.
test_name=TestInstallationDestroyCurrentChildrenInterruptedVolumeAndRetry
export BASEHARBOR_INSTALLATION_DESTROY_ACCEPTANCE=1
report="$(mktemp "${RUNNER_TEMP:-/tmp}/baseharbor-installation-destroy.XXXXXX.jsonl")"
trap 'rm -f "$report"' EXIT

go test -p 1 ./cmd/baha -run "^${test_name}$" -count=1 -timeout 16m -json | tee "$report"

python3 - "$report" "$test_name" <<'PY'
import json
import pathlib
import sys

events = [
    json.loads(line)
    for line in pathlib.Path(sys.argv[1]).read_text().splitlines()
    if line.startswith("{")
]
actions = [event.get("Action") for event in events if event.get("Test") == sys.argv[2]]
if actions.count("pass") != 1 or "skip" in actions or "fail" in actions:
    raise SystemExit("REJECT: required real installation destroy retry test did not PASS exactly once")
if any(event.get("Action") == "fail" for event in events):
    raise SystemExit("REJECT: native installation destroy package failed")
print("PASS: native installation destroy current-scope, partial failure, retry and foreign preservation")
PY

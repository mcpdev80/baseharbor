#!/usr/bin/env bash
set -euo pipefail

# Contract-level Node/HTTPS/remote CLI safety acceptance.
# This does not substitute for the separate live Connector + deployment E2E.
expected=(
  TestConnectorEnrollmentScopeUsesCoreOwnershipAndTypedAccess
  TestNodeListStateTableDrivesProductionRedemptionPath
  TestNodeEnrollmentHTTPSOneUseAndForeignScope
  TestRemoteUpRealCLIPathTakesConnectorRouteNeverLocalCore
  TestRemoteUpRealCLIPathBrokerRequiredFailsClosedBeforeMutation
)
pattern="$(IFS='|'; printf '%s' "${expected[*]}")"
report="$(mktemp "${RUNNER_TEMP:-/tmp}/baseharbor-node-contract.XXXXXX.jsonl")"
trap 'rm -f "$report"' EXIT

go test -p 1 ./cmd/baha -run "^(${pattern})$" -count=1 -timeout 8m -json | tee "$report"

python3 - "$report" "${expected[@]}" <<'PY'
import json
import pathlib
import sys

events = [
    json.loads(line)
    for line in pathlib.Path(sys.argv[1]).read_text().splitlines()
    if line.startswith("{")
]
for name in sys.argv[2:]:
    actions = [event.get("Action") for event in events if event.get("Test") == name]
    if "pass" not in actions or "skip" in actions or "fail" in actions:
        raise SystemExit(f"REJECT: missing mandatory PASS or SKIP/FAIL for {name}")
if any(event.get("Action") == "fail" for event in events):
    raise SystemExit("REJECT: node contract package failed")
print("PASS: mandatory Node/HTTPS/remote CLI contract tests")
PY

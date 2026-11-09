#!/usr/bin/env bash
# Ephemeral PostgreSQL fixture for PR go-unit-tests only.
set -euo pipefail

name="baha-pr-unit-db-${GITHUB_RUN_ID:-local}-$$"
cleanup() {
  docker rm -f "$name" >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker run --detach --rm --name "$name" \
  --publish 127.0.0.1:5433:5432 \
  --env POSTGRES_DB=baseharbor_test \
  --env POSTGRES_USER=postgres \
  --env POSTGRES_PASSWORD=postgres \
  postgres:18-alpine >/dev/null

ready=false
for _ in $(seq 1 30); do
  if docker exec "$name" pg_isready -U postgres -d baseharbor_test >/dev/null 2>&1; then
    ready=true
    break
  fi
  sleep 2
done
if [ "$ready" != true ]; then
  docker logs "$name" >&2 || true
  echo 'PR Go unit test database did not become ready' >&2
  exit 1
fi

export BASEHARBOR_TEST_DATABASE_URL="postgres://postgres:postgres@127.0.0.1:5433/baseharbor_test?sslmode=disable"
mode="${1:-all}"
case "$mode" in
  all)
    go test ./... -count=1
    ;;
  cli-0|cli-1|cli-2|packages)
    python3 scripts/pr-go-unit-shard.py --mode "$mode"
    exit 0
    ;;
  owner)
    ;;
  *)
    echo "Unsupported Go unit shard: $mode" >&2
    exit 2
    ;;
esac

# Keep the first-owner onboarding and node authorization contract mandatory.
# Go exits successfully if -run matches nothing, so assert exact PASS events.
owner_node_report="$(mktemp "${RUNNER_TEMP:-/tmp}/baseharbor-owner-node.XXXXXX.jsonl")"
trap 'rm -f "$owner_node_report"; cleanup' EXIT
go test -p 1 ./cmd/baha ./internal/onboardingwizard ./internal/targetenrollment ./internal/database ./internal/identityprovider \
  -run '^(TestInitialTenantDefaultAndCustomOptions|TestCoreConnectorOIDCUsesConfiguredOperatorRealm|TestNodeAuthorityDefaultsAndCustom|TestNodePublicEndpointFailClosed|TestNodeAuthorityRejectsIndependentCustomCA|TestFirstOwnerWizardDefaultCustomAndCancel|TestOwnerContextRequiresVerifiedAuthorizedScope|TestBootstrapInitialOperatorTenantIntegration|TestManagedOperatorCanonicalBaseURL)$' \
  -count=1 -json > "$owner_node_report"
python3 - "$owner_node_report" <<'PY'
import json
import pathlib
import sys

required = {
    "TestInitialTenantDefaultAndCustomOptions",
    "TestCoreConnectorOIDCUsesConfiguredOperatorRealm",
    "TestNodeAuthorityDefaultsAndCustom",
    "TestNodePublicEndpointFailClosed",
    "TestNodeAuthorityRejectsIndependentCustomCA",
    "TestFirstOwnerWizardDefaultCustomAndCancel",
    "TestOwnerContextRequiresVerifiedAuthorizedScope",
    "TestBootstrapInitialOperatorTenantIntegration",
    "TestManagedOperatorCanonicalBaseURL",
}
events = [
    json.loads(line)
    for line in pathlib.Path(sys.argv[1]).read_text().splitlines()
    if line.strip()
]
passed = {event.get("Test") for event in events if event.get("Action") == "pass"}
skipped = {event.get("Test") for event in events if event.get("Action") == "skip"}
failed = {event.get("Test") for event in events if event.get("Action") == "fail"}
missing = required - passed
if missing or (required & skipped) or (required & failed):
    raise SystemExit(
        "Mandatory owner/tenant/Keycloak/node test did not PASS: "
        + ", ".join(sorted(missing | (required & skipped) | (required & failed)))
    )
print(f"Verified {len(required)} required owner/tenant/Keycloak/node tests: PASS")
PY

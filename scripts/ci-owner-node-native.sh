#!/usr/bin/env bash
set -euo pipefail
test "$(git rev-parse HEAD)" = "$EXPECTED_HEAD"
case "$BASEHARBOR_TEST_RUNTIME" in
  docker|podman) ;;
  *) echo 'REJECT: unsupported runtime' >&2; exit 2 ;;
esac
if [ "$BASEHARBOR_TEST_RUNTIME" = podman ]; then
  docker context use default
  export DOCKER_CONTEXT=
  export DOCKER_HOST=unix:///var/run/docker.sock
fi

require_pass() {
  local pkg="$1" name="$2" duration="$3" report
  report="$(mktemp "$RUNNER_TEMP/baseharbor-native.XXXXXX.jsonl")"
  go test -p 1 "$pkg" -run "^${name}$" -count=1 -timeout "$duration" -json | tee "$report"
  python3 - "$report" "$name" <<'PY'
import json, pathlib, sys
events = [json.loads(line) for line in pathlib.Path(sys.argv[1]).read_text().splitlines() if line.startswith('{')]
name = sys.argv[2]
if not any(e.get('Test') == name and e.get('Action') == 'pass' for e in events):
    raise SystemExit(f'REJECT: missing native PASS: {name}')
if any(e.get('Action') in ('skip', 'fail') for e in events):
    raise SystemExit(f'REJECT: native test skipped or failed: {name}')
PY
  rm -f "$report"
}
bash scripts/test-ci-impact-selectors.sh
require_pass ./internal/identityprovider TestKeycloakSingleNativeSQLInCI 16m
bash scripts/ci-runtime-reset.sh "$BASEHARBOR_TEST_RUNTIME"
require_pass ./cmd/baha TestControlPlaneSingleDefaultNativeRuntimeInCI 20m
# These are production-CLI/HTTP contract regressions, not live remote E2E.
# They must run in both native jobs and may neither skip nor silently pass.
require_pass ./cmd/baha TestConnectorEnrollmentScopeUsesCoreOwnershipAndTypedAccess 3m
require_pass ./cmd/baha TestNodeListStateTableDrivesProductionRedemptionPath 3m
require_pass ./cmd/baha TestRemoteUpRealCLIPathTakesConnectorRouteNeverLocalCore 3m
require_pass ./cmd/baha TestRemoteUpRealCLIPathBrokerRequiredFailsClosedBeforeMutation 3m

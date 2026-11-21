#!/usr/bin/env bash
set -euo pipefail

gate_id="${1:?native gate id required}"
runtime="${BASEHARBOR_TEST_RUNTIME:?runtime required}"
report="$(mktemp "${RUNNER_TEMP:-/tmp}/baseharbor-native-pr-gate.XXXXXX.jsonl")"
trap 'rm -f "$report"' EXIT

run_exact_test() {
  local test_name="$1" timeout="$2" package="${3:-./cmd/baha}"
  go test -p 1 "$package" -run "^${test_name}$" -count=1 -timeout "$timeout" -json | tee "$report"
  python3 - "$report" "$test_name" <<'PY'
import json
import pathlib
import sys

events = [json.loads(line) for line in pathlib.Path(sys.argv[1]).read_text().splitlines()
          if line.startswith('{')]
test = sys.argv[2]
actions = [event.get('Action') for event in events if event.get('Test') == test]
if actions.count('pass') != 1 or 'skip' in actions or 'fail' in actions:
    raise SystemExit(f'REJECT: {test} did not PASS exactly once')
if any(event.get('Action') == 'fail' for event in events):
    raise SystemExit(f'REJECT: native package failed while running {test}')
print(f'PASS: {test}')
PY
}

case "$gate_id" in
  native/docker/installation-destroy|native/podman/installation-destroy)
    export BASEHARBOR_INSTALLATION_DESTROY_ACCEPTANCE=1
    run_exact_test TestInstallationDestroyCurrentChildrenInterruptedVolumeAndRetry 16m
    ;;
  native/docker/second-target-conflict|native/podman/second-target-conflict)
    export BASEHARBOR_CI_RUNTIME_INTEGRATION=1
    export BASEHARBOR_SECOND_TARGET_NATIVE_ACCEPTANCE=1
    run_exact_test TestSecondSameHostCoreNativeConflictInCI 12m
    ;;
  native/docker/greenfield|native/podman/greenfield)
    export BASEHARBOR_GREENFIELD_NATIVE_ACCEPTANCE=1
    run_exact_test TestGreenfieldIsolatedCoreNativeInCI 20m
    ;;
  native/docker/onboarding|native/podman/onboarding)
    export BASEHARBOR_CI_RUNTIME_INTEGRATION=1
    export BASEHARBOR_GREENFIELD_NATIVE_ACCEPTANCE=1
    export BASEHARBOR_ONBOARDING_NATIVE_ACCEPTANCE=1
    binary="${RUNNER_TEMP:-/tmp}/baha-onboarding-native"
    go build -o "$binary" ./cmd/baha
    export BASEHARBOR_CI_BAHA_BINARY="$binary"
    go test -p 1 ./cmd/baha -run '^(TestGreenfieldIsolatedCoreNativeInCI|TestWorkstationOnboardingRemoteNativeInCI)$' \
      -count=1 -timeout 30m -json | tee "$report"
    python3 - "$report" <<'PY'
import json
import pathlib
import sys

expected = {'TestGreenfieldIsolatedCoreNativeInCI', 'TestWorkstationOnboardingRemoteNativeInCI'}
events = [json.loads(line) for line in pathlib.Path(sys.argv[1]).read_text().splitlines()
          if line.startswith('{')]
actions = {name: [event.get('Action') for event in events if event.get('Test') == name]
           for name in expected}
for name, result in actions.items():
    if result.count('pass') != 1 or 'skip' in result or 'fail' in result:
        raise SystemExit(f'REJECT: {name} did not PASS exactly once')
if any(event.get('Action') == 'fail' for event in events):
    raise SystemExit('REJECT: native onboarding package failed')
print('PASS: greenfield baseline and remote workstation onboarding')
PY
    python3 scripts/ci-onboarding-native-proof.py --input "$report" \
      --test TestWorkstationOnboardingRemoteNativeInCI
    ;;
  native/docker/perses-runtime-smoke)
    export BASEHARBOR_PERSES_RUNTIME_SMOKE=1
    run_exact_test TestPersesRuntimeSmokeDockerIsolated 5m ./internal/application
    ;;
  native/docker/prometheus-alert-rules)
    export BASEHARBOR_PROMETHEUS_HA_ACCEPTANCE=1
    run_exact_test TestPrometheusHARuntimeFailoverAcceptanceInCI 12m ./internal/metrics
    ;;
  *)
    echo "REJECT: unsupported native PR gate $gate_id" >&2
    exit 2
    ;;
esac

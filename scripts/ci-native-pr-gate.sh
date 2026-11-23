#!/usr/bin/env bash
set -euo pipefail

gate_id="${1:?native gate id required}"
runtime="${BASEHARBOR_TEST_RUNTIME:?runtime required}"
report="$(mktemp "${RUNNER_TEMP:-/tmp}/baseharbor-native-pr-gate.XXXXXX.jsonl")"
trap 'rm -f "$report"' EXIT

prepare_exact_runtime_image() {
  local source_sha image build_dir
  source_sha="$(git rev-parse HEAD)"
  [[ "$source_sha" =~ ^[0-9a-f]{40}$ ]]
  image="localhost/baseharbor-runtime:ci-native-${runtime}-${source_sha}"
  build_dir="${RUNNER_TEMP:-/tmp}/baseharbor-native-runtime-${runtime}-${source_sha}"
  mkdir -p "$build_dir"
  CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w -X main.commit=${source_sha} -X main.date=unknown" \
    -o "$build_dir/baha" ./cmd/baha
  "$build_dir/baha" version -o json | python3 -c '
import json, sys
version = json.load(sys.stdin)
if version.get("commit") != sys.argv[1]:
    raise SystemExit("native runtime binary does not match the checked-out source SHA")
' "$source_sha"
  cp deploy/control-plane/Dockerfile.binary "$build_dir/Dockerfile"
  "$runtime" build --pull --tag "$image" "$build_dir"
  "$runtime" image inspect "$image" >/dev/null
  "$runtime" run --rm "$image" version -o json | python3 -c '
import json, sys
version = json.load(sys.stdin)
if version.get("commit") != sys.argv[1]:
    raise SystemExit("native runtime image does not contain the checked-out source SHA")
' "$source_sha"
  export BASEHARBOR_RUNTIME_IMAGE="$image"
  printf 'Using exact native runtime image %s\n' "$image"
}

run_exact_test() {
  local test_name="$1" timeout="$2" package="${3:-./cmd/baha}" env_assignment="${4:-}"
  if [[ -n "$env_assignment" ]]; then
    env "$env_assignment" go test -p 1 "$package" -run "^${test_name}$" -count=1 -timeout "$timeout" -json | tee "$report"
  else
    go test -p 1 "$package" -run "^${test_name}$" -count=1 -timeout "$timeout" -json | tee "$report"
  fi
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
  native/docker/onboarding|native/podman/onboarding|native/podman/workstation-onboarding|native/docker/guided|native/podman/guided)
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
  native/docker/data-capabilities|native/podman/data-capabilities)
    prepare_exact_runtime_image
    run_exact_test TestApplicationCapabilityLifecycleExistingCore 20m ./cmd/baha \
      BASEHARBOR_CORELESS_ACCEPTANCE=1
    run_exact_test TestApplicationCapabilityLifecycleFootprintShowDigestConsent 20m ./cmd/baha \
      BASEHARBOR_CORELESS_ACCEPTANCE=1
    ;;
  native/docker/coreless-lifecycle|native/podman/coreless-lifecycle)
    prepare_exact_runtime_image
    run_exact_test TestSelectiveBackendCleanupPreservesRuntimeBaseline 30m ./cmd/baha \
      BASEHARBOR_BACKEND_LIFECYCLE_ACCEPTANCE=1
    run_exact_test TestSelectiveApplicationBackendRuntimeAddRemoveReAdd 30m ./cmd/baha \
      BASEHARBOR_BACKEND_LIFECYCLE_ACCEPTANCE=1
    run_exact_test TestApplicationCapabilityLifecycleExistingCore 30m ./cmd/baha \
      BASEHARBOR_CORELESS_ACCEPTANCE=1
    run_exact_test TestApplicationCapabilityLifecycleFootprintShowDigestConsent 30m ./cmd/baha \
      BASEHARBOR_CORELESS_ACCEPTANCE=1
    run_exact_test TestSelectiveHTTPSRuntimeAddRemoveReAdd 30m ./cmd/baha \
      BASEHARBOR_HTTPS_LIFECYCLE_ACCEPTANCE=1
    run_exact_test TestManagedRegisteredOTLPNativeConsumerHandoff 30m ./cmd/baha \
      BASEHARBOR_OTLP_HANDOFF_ACCEPTANCE=1
    ;;
  native/docker/environment-destroy|native/podman/environment-destroy)
    run_exact_test TestEnvironmentDestroyNativeLifecycle 30m ./cmd/baha \
      BASEHARBOR_ENV_DESTROY_ACCEPTANCE=1
    ;;
  native/podman/service-stop)
    run_exact_test TestSelectedStopAcceptsPinnedContainerName 5m ./internal/runtime
    ;;
  *)
    echo "REJECT: unsupported native PR gate $gate_id" >&2
    exit 2
    ;;
esac

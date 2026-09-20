#!/usr/bin/env bash
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

head_sha="$(git rev-parse HEAD)"
short_sha="$(git rev-parse --short=12 HEAD)"
binary="/tmp/baha-v049-${short_sha}"
runtime_image="baseharbor-runtime:v049-security-${short_sha}"
docs_venv="/tmp/baseharbor-v049-docs-${short_sha}"
security_workdir=""

printf 'BaseHarbor v0.4.9 validation\n'
printf 'head: %s\n' "$head_sha"
printf 'runtime image: %s\n\n' "$runtime_image"

cleanup() {
  set +e
  if [[ -n "$security_workdir" && -d "$security_workdir" ]]; then
    export BASEHARBOR_STATE_DIR="$security_workdir/platform-state"
    cd "$security_workdir" >/dev/null 2>&1 || true
    "$binary" app destroy security-runtime --yes >/dev/null 2>&1 || true
    "$binary" destroy --yes >/dev/null 2>&1 || true
    rm -rf "$security_workdir"
  fi
  docker image rm "$runtime_image" >/dev/null 2>&1 || true
  rm -f "$binary"
  rm -rf "$docs_venv"
}
trap cleanup EXIT

verify_container() {
  local project="$1"
  local service="$2"
  local id user privileged readonly cap_add cap_drop security_opt

  id="$(docker ps -q \
    --filter "label=com.docker.compose.project=$project" \
    --filter "label=com.docker.compose.service=$service")"

  if [[ -z "$id" || "$id" == *$'\n'* ]]; then
    printf 'FAIL: expected exactly one running container for %s/%s, got %q\n' "$project" "$service" "$id" >&2
    return 1
  fi

  user="$(docker inspect "$id" --format '{{.Config.User}}')"
  privileged="$(docker inspect "$id" --format '{{.HostConfig.Privileged}}')"
  readonly="$(docker inspect "$id" --format '{{.HostConfig.ReadonlyRootfs}}')"
  cap_add="$(docker inspect "$id" --format '{{json .HostConfig.CapAdd}}')"
  cap_drop="$(docker inspect "$id" --format '{{json .HostConfig.CapDrop}}')"
  security_opt="$(docker inspect "$id" --format '{{json .HostConfig.SecurityOpt}}')"

  printf '  %-34s user=%-14s privileged=%-5s readonly=%-5s\n' "$project/$service" "$user" "$privileged" "$readonly"

  if [[ -z "$user" || "$user" == "0" || "$user" == "root" || "$user" == 0:* || "$user" == root:* ]]; then
    printf 'FAIL: %s/%s runs as root or has no explicit runtime user (%q)\n' "$project" "$service" "$user" >&2
    return 1
  fi
  [[ "$privileged" == "false" ]] || {
    printf 'FAIL: %s/%s is privileged\n' "$project" "$service" >&2
    return 1
  }
  [[ "$readonly" == "true" ]] || {
    printf 'FAIL: %s/%s root filesystem is writable\n' "$project" "$service" >&2
    return 1
  }
  [[ "$cap_add" == "null" || "$cap_add" == "[]" ]] || {
    printf 'FAIL: %s/%s adds capabilities: %s\n' "$project" "$service" "$cap_add" >&2
    return 1
  }
  [[ "$cap_drop" == *'"ALL"'* ]] || {
    printf 'FAIL: %s/%s does not drop ALL capabilities: %s\n' "$project" "$service" "$cap_drop" >&2
    return 1
  }
  [[ "$security_opt" == *'no-new-privileges'* ]] || {
    printf 'FAIL: %s/%s lacks no-new-privileges: %s\n' "$project" "$service" "$security_opt" >&2
    return 1
  }
}

printf '[1/13] repository whitespace\n'
git diff --check

printf '[2/13] gofmt\n'
unformatted="$(gofmt -l .)"
if [[ -n "$unformatted" ]]; then
  printf 'gofmt required for:\n%s\n' "$unformatted" >&2
  exit 1
fi

printf '[3/13] Go tests\n'
go test ./...

printf '[4/13] provider conformance/fault injection\n'
go test ./internal/providerconformance ./internal/logs -count=1

printf '[5/13] vet\n'
go vet ./...

printf '[6/13] build CLI and exact-head runtime image\n'
go build -o "$binary" ./cmd/baha
docker build --pull -t "$runtime_image" -f deploy/control-plane/Dockerfile .

printf '[7/13] BaseHarbor image arbitrary-UID compatibility\n'
docker run --rm \
  --user 12345:0 \
  --entrypoint /bin/sh \
  "$runtime_image" \
  -ec 'test "$(id -u)" = 12345; touch /var/lib/baseharbor/runtime-operations/arbitrary-uid-probe; test -f /var/lib/baseharbor/runtime-operations/arbitrary-uid-probe'

printf '[8/13] real PostgreSQL/Valkey runtime security acceptance\n'
BASEHARBOR_RUNTIME_SECURITY_ACCEPTANCE=1 \
  go test ./internal/application -run '^TestMultiInstanceComposeLifecycleInCI$' -count=1 -v

printf '[9/13] real provider acceptances: S3, OTLP, metrics, exposure, logs\n'
BASEHARBOR_OBJECT_STORAGE_INTEGRATION=1 \
  go test ./internal/objectstorage -run '^TestManagedSeaweedFSRunsUnprivileged$' -count=1 -v

BASEHARBOR_OTLP_INTEGRATION=1 \
  go test ./internal/telemetry -run '^TestManagedCollectorRealOTLPExport$' -count=1 -v

BASEHARBOR_METRICS_INTEGRATION=1 \
  go test ./internal/metrics -run '^TestManagedPrometheusScrapesTwoIsolatedApplications$' -count=1 -v

BASEHARBOR_EXPOSURE_ACCEPTANCE=true \
  go test ./cmd/baha -run '^TestManagedHTTPExposure' -count=1 -v

BASEHARBOR_LOGS_INTEGRATION=1 \
  go test ./internal/logs -run '^TestManagedLokiIngestsRealComposeWorkloadLogs$' -count=1 -v

printf '[10/13] real control-plane PostgreSQL/OpenBao security and restart acceptance\n'
BASEHARBOR_RUNTIME_RESTART_ACCEPTANCE=true \
  go test ./cmd/baha -run '^TestExistingControlPlaneRestartRequiresAndUsesRecoveryFile$' -count=1 -v

printf '[11/13] real relay security and directed connectivity acceptance\n'
BASEHARBOR_RUNTIME_IMAGE="$runtime_image" \
BASEHARBOR_CONNECTIVITY_ACCEPTANCE=true \
  go test ./cmd/baha -run '^TestDirectedCrossApplicationConnectivityInCI$' -count=1 -v

printf '[12/13] broker/executor/SeaweedFS live security fixture\n'
security_workdir="$(mktemp -d)"
export BASEHARBOR_STATE_DIR="$security_workdir/platform-state"
export BASEHARBOR_RUNTIME_IMAGE="$runtime_image"
export BASEHARBOR_LOGS_ENABLED=false

postgres_port="$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()')"
openbao_port="$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()')"

cd "$security_workdir"
"$binary" up --control-plane-only --yes --postgres-port "$postgres_port" --openbao-port "$openbao_port"
"$binary" openbao bootstrap --recovery-file "$security_workdir/openbao-recovery.json"

mkdir -p "$security_workdir/security-runtime"
cd "$security_workdir/security-runtime"
cat > baseharbor.yaml <<'EOF'
version: 1
app:
  name: security-runtime
  environment: dev
workload:
  compose: compose.yaml
  services:
    - api
runtime:
  permissions:
    - capability: object-storage.s3/v1
      services:
        - api
      operations:
        - runtime.create
        - runtime.get
        - runtime.delete
EOF

cat > compose.yaml <<'EOF'
services:
  api:
    image: curlimages/curl:8.16.0
    entrypoint: ["sh", "-c"]
    command: ["sleep 3600"]
EOF

"$binary" app apply

printf 'Managed container security state:\n'
verify_container "baseharbor" "postgres"
verify_container "baseharbor" "openbao"
verify_container "baseharbor-object-storage" "seaweedfs"
verify_container "baseharbor-runtime-executor" "executor"
verify_container "baseharbor-broker-security-runtime-dev" "broker"

"$binary" app destroy --yes
cd "$security_workdir"
"$binary" destroy --yes
rm -rf "$security_workdir"
security_workdir=""

printf '[13/13] strict EN/DE docs\n'
cd "$(git rev-parse --show-toplevel)"
python3 -m venv "$docs_venv"
"$docs_venv/bin/python" -m pip install --disable-pip-version-check --quiet --requirement requirements-docs.txt
"$docs_venv/bin/mkdocs" build --strict --config-file mkdocs.yml
"$docs_venv/bin/mkdocs" build --strict --config-file mkdocs.de.yml

printf '\nPASS: BaseHarbor v0.4.9 validation succeeded on %s\n' "$head_sha"

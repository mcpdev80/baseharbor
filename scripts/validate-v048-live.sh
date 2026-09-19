#!/usr/bin/env bash
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

head_sha="$(git rev-parse HEAD)"
short_sha="$(git rev-parse --short=12 HEAD)"
runtime_image="baseharbor-runtime:v048-live-${short_sha}"
binary="/tmp/baha-v048-${short_sha}"

printf 'BaseHarbor v0.4.8 live validation\n'
printf 'head: %s\n' "$head_sha"
printf 'runtime image: %s\n\n' "$runtime_image"

command -v go >/dev/null
command -v docker >/dev/null
docker compose version >/dev/null

cleanup() {
  docker image rm "$runtime_image" >/dev/null 2>&1 || true
  rm -f "$binary"
}
trap cleanup EXIT

printf '[1/9] repository whitespace\n'
git diff --check

printf '[2/9] gofmt\n'
unformatted="$(gofmt -l .)"
if [[ -n "$unformatted" ]]; then
  printf 'gofmt required for:\n%s\n' "$unformatted" >&2
  exit 1
fi

printf '[3/9] unit/integration tests without opt-in acceptances\n'
go test ./...

printf '[4/9] vet\n'
go vet ./...

printf '[5/9] build CLI\n'
go build -o "$binary" ./cmd/baha

printf '[6/9] build exact-head BaseHarbor Runtime image\n'
docker build --pull -t "$runtime_image" -f deploy/control-plane/Dockerfile .

printf '[7/9] real provider acceptances\n'
BASEHARBOR_METRICS_INTEGRATION=1 \
  go test ./internal/metrics -run '^TestManagedPrometheusScrapesTwoIsolatedApplications$' -count=1 -v

BASEHARBOR_EXPOSURE_ACCEPTANCE=true \
  go test ./cmd/baha -run '^TestManagedHTTPExposure' -count=1 -v

printf '[8/9] real directed cross-application connectivity acceptance\n'
BASEHARBOR_RUNTIME_IMAGE="$runtime_image" \
BASEHARBOR_CONNECTIVITY_ACCEPTANCE=true \
  go test ./cmd/baha -run '^TestDirectedCrossApplicationConnectivityInCI$' -count=1 -v

printf '[9/9] real control-plane restart/unseal acceptance\n'
BASEHARBOR_RUNTIME_RESTART_ACCEPTANCE=true \
  go test ./cmd/baha -run '^TestExistingControlPlaneRestartRequiresAndUsesRecoveryFile$' -count=1 -v

if command -v mkdocs >/dev/null 2>&1; then
  printf '[extra] strict EN/DE documentation build\n'
  mkdocs build --strict --config-file mkdocs.yml
  mkdocs build --strict --config-file mkdocs.de.yml
else
  printf '[extra] mkdocs not installed; strict documentation build left for final release gate\n'
fi

printf '\nPASS: BaseHarbor v0.4.8 live validation succeeded on %s\n' "$head_sha"

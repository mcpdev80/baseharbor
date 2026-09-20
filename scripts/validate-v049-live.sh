#!/usr/bin/env bash
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

head_sha="$(git rev-parse HEAD)"
short_sha="$(git rev-parse --short=12 HEAD)"
binary="/tmp/baha-v049-${short_sha}"
docs_venv="/tmp/baseharbor-v049-docs-${short_sha}"

printf 'BaseHarbor v0.4.9 validation\n'
printf 'head: %s\n\n' "$head_sha"

cleanup() {
  rm -f "$binary"
  rm -rf "$docs_venv"
}
trap cleanup EXIT

printf '[1/8] repository whitespace\n'
git diff --check

printf '[2/8] gofmt\n'
unformatted="$(gofmt -l .)"
if [[ -n "$unformatted" ]]; then
  printf 'gofmt required for:\n%s\n' "$unformatted" >&2
  exit 1
fi

printf '[3/8] Go tests\n'
go test ./...

printf '[4/8] provider conformance/fault injection\n'
go test ./internal/providerconformance ./internal/logs -count=1

printf '[5/8] vet\n'
go vet ./...

printf '[6/8] build CLI\n'
go build -o "$binary" ./cmd/baha

printf '[7/8] strict EN/DE docs\n'
python3 -m venv "$docs_venv"
"$docs_venv/bin/python" -m pip install --disable-pip-version-check --quiet --requirement requirements-docs.txt
"$docs_venv/bin/mkdocs" build --strict --config-file mkdocs.yml
"$docs_venv/bin/mkdocs" build --strict --config-file mkdocs.de.yml

printf '[8/8] real Loki/Alloy acceptance when Compose is available\n'
if command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
  BASEHARBOR_LOGS_INTEGRATION=1 go test ./internal/logs -run '^TestManagedLokiIngestsRealComposeWorkloadLogs$' -count=1 -v
else
  printf 'SKIP: no usable Docker Compose daemon; run this exact-head acceptance on a Compose host before tagging.\n'
fi

printf '\nPASS: BaseHarbor v0.4.9 validation succeeded on %s\n' "$head_sha"

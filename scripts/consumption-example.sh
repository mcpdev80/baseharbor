#!/usr/bin/env bash
set -euo pipefail

: "${BASEHARBOR_VERSION:?Set the published BaseHarbor version to consume}"
: "${APP_ENVIRONMENT:=dev}"
if [[ ! "$BASEHARBOR_VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+([.-][A-Za-z0-9.-]+)?$ ]]; then
  echo "BASEHARBOR_VERSION must be a published semantic version" >&2
  exit 2
fi

baha version

# The runner owns this disposable Core for the duration of one job. Keep every
# command in this process so Core and application state do not cross runners.
baha --no-input up --control-plane-only --yes
cleanup() {
  baha --no-input down --yes >/dev/null 2>&1 || true
}
trap cleanup EXIT

baha --no-input app inspect . --plain > inspection.txt
baha --no-input app inspect . -o json > inspection.json
python3 -c 'import json, pathlib; json.loads(pathlib.Path("inspection.json").read_text())'

baha --no-input app plan -o json > plan.json
python3 -c 'import json, pathlib; json.loads(pathlib.Path("plan.json").read_text())'

# APP_API_KEY is optional. When used, define it as a masked CI variable. The
# secret value travels on stdin and is never expanded into a shell command.
if [[ -n "${APP_API_KEY:-}" ]]; then
  printf '%s' "$APP_API_KEY" | baha --no-input app secret set API_KEY --stdin
fi

baha --no-input app preflight
timeout 300 baha --no-input up --environment "$APP_ENVIRONMENT" --yes
timeout 120 baha --no-input app status -o json > status.json
python3 -c 'import json, pathlib; json.loads(pathlib.Path("status.json").read_text())'
baha --no-input app env --format json > environment.json
python3 -c 'import json, pathlib; json.loads(pathlib.Path("environment.json").read_text())'

# This optional read is valid when the checked-out app declares a PostgreSQL
# connection. Keep its metadata out of the job log.
if [[ "${APP_CHECK_POSTGRES_CREDS:-false}" == "true" ]]; then
  baha --no-input app creds postgres -o json > postgres-connection.json
  python3 -c 'import json, pathlib; json.loads(pathlib.Path("postgres-connection.json").read_text())'
fi

#!/usr/bin/env bash
set -euo pipefail

runtime="${BASEHARBOR_TEST_RUNTIME:-docker}"
baha="${BAHA:-baha}"
target="${BASEHARBOR_TARGET:-v0420-${runtime}-smoke}"
app="${BASEHARBOR_SMOKE_APP:-runtime-smoke}"
workdir="${BASEHARBOR_SMOKE_WORKDIR:-$(mktemp -d)}"
recovery="$workdir/openbao-recovery.json"

cleanup_workdir=0
if [ -z "${BASEHARBOR_SMOKE_WORKDIR:-}" ]; then
  cleanup_workdir=1
fi

cleanup() {
  set +e
  "$baha" app destroy "$app" --yes >/dev/null 2>&1 || true
  "$baha" destroy --yes >/dev/null 2>&1 || true
  if [ "$cleanup_workdir" = "1" ]; then
    rm -rf "$workdir"
  fi
}
trap cleanup EXIT

mkdir -p "$workdir"
cd "$workdir"

"$baha" target create "$target" --runtime-provider "$runtime" --access "$target" --access-provider local --reference local --default
"$baha" up --control-plane-only --yes
"$baha" openbao bootstrap --recovery-file "$recovery"
"$baha" openbao status | grep -q "AppRole login succeeded"

"$baha" app create "$app" --sql --cache

"$baha" app plan "$app" | tee plan.txt
grep -q 'ensure database.sql:default' plan.txt
grep -q 'verify database.sql:default' plan.txt
grep -q 'ensure cache.key-value:default' plan.txt
grep -q 'verify cache.key-value:default' plan.txt

"$baha" app preflight "$app"
"$baha" app apply "$app"

"$baha" app status "$app" --verbose | tee status.txt
grep -q 'authenticated SELECT 1 succeeded' status.txt
grep -q 'authenticated PING returned PONG' status.txt

"$baha" app doctor "$app" | tee doctor.txt
grep -q '^READY$' doctor.txt

"$baha" app down "$app"
"$baha" app up "$app"
"$baha" app doctor "$app" | tee resume-doctor.txt
grep -q '^READY$' resume-doctor.txt

"$baha" app destroy "$app" --yes
trap - EXIT
"$baha" destroy --yes >/dev/null 2>&1 || true
if [ "$cleanup_workdir" = "1" ]; then
  rm -rf "$workdir"
fi

echo "v0.4.20 runtime smoke: PASS ($runtime)"

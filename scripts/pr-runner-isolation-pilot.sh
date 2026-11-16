#!/usr/bin/env bash
# Disposable, non-privileged Docker/Podman isolation smoke test; never a release gate.
set -euo pipefail
runtime="${1:?docker or podman}"
case "$runtime" in docker|podman) ;; *) exit 2 ;; esac
: "${RUNNER_TEMP:?hosted scratch directory required}"
: "${GITHUB_RUN_ID:?Actions run identity required}"
: "${GITHUB_RUN_ATTEMPT:?Actions attempt identity required}"
case "$GITHUB_RUN_ID:$GITHUB_RUN_ATTEMPT" in *[!0-9:]*|'') exit 2 ;; esac
lane="bh-${runtime}-${GITHUB_RUN_ID}-${GITHUB_RUN_ATTEMPT}-${GITHUB_JOB:-pilot}"
lane="${lane//_/-}"
test "${#lane}" -le 100
workspace="$(mktemp -d "$RUNNER_TEMP/${lane}.XXXXXX")"
container="${lane}-container"
network="${lane}-net"
volume="${lane}-vol"
cleanup_status=0
cleanup() {
  local original=$?
  trap - EXIT HUP INT TERM
  "$runtime" rm -f "$container" >/dev/null 2>&1 || :
  "$runtime" volume rm -f "$volume" >/dev/null 2>&1 || :
  "$runtime" network rm "$network" >/dev/null 2>&1 || :
  if "$runtime" container inspect "$container" >/dev/null 2>&1 ||
     "$runtime" volume inspect "$volume" >/dev/null 2>&1 ||
     "$runtime" network inspect "$network" >/dev/null 2>&1; then
    echo "FAIL: own runtime resources survived cleanup" >&2
    cleanup_status=1
  fi
  rm -rf -- "$workspace"
  if test -e "$workspace"; then cleanup_status=1; fi
  echo "cleanup runtime=$runtime lane=$lane status=$cleanup_status"
  if test "$cleanup_status" -ne 0; then exit 1; fi
  exit "$original"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
trap 'exit 129' HUP
umask 077
mkdir -p "$workspace"/{target,provider,secrets,recovery,artifacts}
"$runtime" network create "$network" >/dev/null
"$runtime" volume create "$volume" >/dev/null
# No host ports and no privileged flags. Isolated network and volume are owned
# by this unique Actions run/attempt/job lane, never a shared global reset.
"$runtime" run --pull=always --rm --name "$container" --network "$network" \
  --mount "type=volume,src=$volume,dst=/isolated" \
  --security-opt no-new-privileges --cap-drop ALL \
  docker.io/library/alpine:3.21 /bin/sh -c 'printf ok > /isolated/probe && test -f /isolated/probe'
echo "pilot runtime=$runtime isolated lane=$lane"
case "${2:-success}" in
  success) ;;
  failure) exit 42 ;;
  cancellation) kill -TERM "$$"; exit 143 ;;
  *) exit 2 ;;
esac

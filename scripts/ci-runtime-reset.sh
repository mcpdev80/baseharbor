#!/usr/bin/env bash
set -euo pipefail

engine="${1:-docker}"
command -v "$engine" >/dev/null 2>&1 || exit 0

remove_containers() {
  local id project name
  while read -r id; do
    [ -n "$id" ] || continue
    project="$("$engine" container inspect --format '{{ index .Config.Labels "com.docker.compose.project" }}' "$id" 2>/dev/null || true)"
    if [ -z "$project" ] || [ "$project" = "<no value>" ]; then
      project="$("$engine" container inspect --format '{{ index .Config.Labels "io.podman.compose.project" }}' "$id" 2>/dev/null || true)"
    fi
    name="$("$engine" container inspect --format '{{ .Name }}' "$id" 2>/dev/null | sed 's#^/##' || true)"
    case "$project:$name" in
      baseharbor*:*|*:baseharbor-*)
        "$engine" container stop -t 2 "$id" >/dev/null 2>&1 || true
        "$engine" container rm "$id" >/dev/null 2>&1 || "$engine" container rm -f "$id" >/dev/null 2>&1 || true
        ;;
    esac
  done < <("$engine" container ls -aq 2>/dev/null || true)
}

remove_networks() {
  local id project name
  while read -r id; do
    [ -n "$id" ] || continue
    project="$("$engine" network inspect --format '{{ index .Labels "com.docker.compose.project" }}' "$id" 2>/dev/null || true)"
    if [ -z "$project" ] || [ "$project" = "<no value>" ]; then
      project="$("$engine" network inspect --format '{{ index .Labels "io.podman.compose.project" }}' "$id" 2>/dev/null || true)"
    fi
    name="$("$engine" network inspect --format '{{ .Name }}' "$id" 2>/dev/null || true)"
    case "$project:$name" in
      baseharbor*:*|*:baseharbor-*) "$engine" network rm "$id" >/dev/null 2>&1 || true ;;
    esac
  done < <("$engine" network ls -q 2>/dev/null || true)
}

remove_volumes() {
  local id project name
  while read -r id; do
    [ -n "$id" ] || continue
    project="$("$engine" volume inspect --format '{{ index .Labels "com.docker.compose.project" }}' "$id" 2>/dev/null || true)"
    if [ -z "$project" ] || [ "$project" = "<no value>" ]; then
      project="$("$engine" volume inspect --format '{{ index .Labels "io.podman.compose.project" }}' "$id" 2>/dev/null || true)"
    fi
    name="$("$engine" volume inspect --format '{{ .Name }}' "$id" 2>/dev/null || true)"
    case "$project:$name" in
      baseharbor*:*|*:baseharbor-*) "$engine" volume rm -f "$id" >/dev/null 2>&1 || true ;;
    esac
  done < <("$engine" volume ls -q 2>/dev/null || true)
}

remove_containers
remove_networks
remove_volumes

rm -rf /tmp/baseharbor-* /tmp/baha /tmp/mailflow /tmp/baseharbor-demo 2>/dev/null || true

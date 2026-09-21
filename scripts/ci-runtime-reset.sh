#!/usr/bin/env bash
set -euo pipefail

engine="${1:-docker}"
command -v "$engine" >/dev/null 2>&1 || exit 0

remove_containers() {
  local id project
  while read -r id; do
    [ -n "$id" ] || continue
    project="$("$engine" container inspect --format '{{ index .Config.Labels "com.docker.compose.project" }}' "$id" 2>/dev/null || true)"
    case "$project" in
      baseharbor*) "$engine" container rm -f "$id" >/dev/null 2>&1 || true ;;
    esac
  done < <("$engine" container ls -aq --filter label=com.docker.compose.project 2>/dev/null || true)
}

remove_networks() {
  local id project
  while read -r id; do
    [ -n "$id" ] || continue
    project="$("$engine" network inspect --format '{{ index .Labels "com.docker.compose.project" }}' "$id" 2>/dev/null || true)"
    case "$project" in
      baseharbor*) "$engine" network rm "$id" >/dev/null 2>&1 || true ;;
    esac
  done < <("$engine" network ls -q --filter label=com.docker.compose.project 2>/dev/null || true)
}

remove_volumes() {
  local id project
  while read -r id; do
    [ -n "$id" ] || continue
    project="$("$engine" volume inspect --format '{{ index .Labels "com.docker.compose.project" }}' "$id" 2>/dev/null || true)"
    case "$project" in
      baseharbor*) "$engine" volume rm -f "$id" >/dev/null 2>&1 || true ;;
    esac
  done < <("$engine" volume ls -q --filter label=com.docker.compose.project 2>/dev/null || true)
}

remove_containers
remove_networks
remove_volumes

rm -rf /tmp/baseharbor-* /tmp/baha /tmp/mailflow /tmp/baseharbor-demo 2>/dev/null || true

#!/usr/bin/env bash
set -euo pipefail

engine="${1:-docker}"
command -v "$engine" >/dev/null 2>&1 || exit 0

is_baseharbor_resource() {
  local project="${1:-}" name="${2:-}"
  case "$project:$name" in
    baseharbor*:*|*:baseharbor-*) return 0 ;;
    *) return 1 ;;
  esac
}

remove_containers() {
  local line name docker_project podman_project project
  local -a ids=() targets=()

  mapfile -t ids < <("$engine" container ls -aq 2>/dev/null || true)
  [ "${#ids[@]}" -gt 0 ] || return 0

  while IFS= read -r line; do
    [ -n "$line" ] || continue
    IFS='|' read -r name docker_project podman_project <<< "$line"
    name="${name#/}"
    project="$docker_project"
    if [ -z "$project" ] || [ "$project" = "<no value>" ]; then
      project="$podman_project"
    fi
    if is_baseharbor_resource "$project" "$name"; then
      targets+=("$name")
    fi
  done < <(
    "$engine" container inspect --format '{{.Name}}|{{ index .Config.Labels "com.docker.compose.project" }}|{{ index .Config.Labels "io.podman.compose.project" }}' "${ids[@]}" 2>/dev/null || true
  )

  [ "${#targets[@]}" -gt 0 ] || return 0
  "$engine" container stop -t 2 "${targets[@]}" >/dev/null 2>&1 || true
  "$engine" container rm "${targets[@]}" >/dev/null 2>&1 || "$engine" container rm -f "${targets[@]}" >/dev/null 2>&1 || true
}

remove_networks() {
  local line name docker_project podman_project project
  local -a ids=() targets=()

  mapfile -t ids < <("$engine" network ls -q 2>/dev/null || true)
  [ "${#ids[@]}" -gt 0 ] || return 0

  while IFS= read -r line; do
    [ -n "$line" ] || continue
    IFS='|' read -r name docker_project podman_project <<< "$line"
    project="$docker_project"
    if [ -z "$project" ] || [ "$project" = "<no value>" ]; then
      project="$podman_project"
    fi
    if is_baseharbor_resource "$project" "$name"; then
      targets+=("$name")
    fi
  done < <(
    "$engine" network inspect --format '{{.Name}}|{{ index .Labels "com.docker.compose.project" }}|{{ index .Labels "io.podman.compose.project" }}' "${ids[@]}" 2>/dev/null || true
  )

  [ "${#targets[@]}" -gt 0 ] || return 0
  "$engine" network rm "${targets[@]}" >/dev/null 2>&1 || true
}

remove_volumes() {
  local line name docker_project podman_project project
  local -a ids=() targets=()

  mapfile -t ids < <("$engine" volume ls -q 2>/dev/null || true)
  [ "${#ids[@]}" -gt 0 ] || return 0

  while IFS= read -r line; do
    [ -n "$line" ] || continue
    IFS='|' read -r name docker_project podman_project <<< "$line"
    project="$docker_project"
    if [ -z "$project" ] || [ "$project" = "<no value>" ]; then
      project="$podman_project"
    fi
    if is_baseharbor_resource "$project" "$name"; then
      targets+=("$name")
    fi
  done < <(
    "$engine" volume inspect --format '{{.Name}}|{{ index .Labels "com.docker.compose.project" }}|{{ index .Labels "io.podman.compose.project" }}' "${ids[@]}" 2>/dev/null || true
  )

  [ "${#targets[@]}" -gt 0 ] || return 0
  "$engine" volume rm -f "${targets[@]}" >/dev/null 2>&1 || true
}

remove_containers
remove_networks
remove_volumes

rm -rf /tmp/baseharbor-* /tmp/baha /tmp/mailflow /tmp/baseharbor-demo 2>/dev/null || true

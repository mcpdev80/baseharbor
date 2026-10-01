#!/usr/bin/env bash
set -euo pipefail

engine="${1:-docker}"
scope="${2:-}"

matches_scope() {
  local project="${1:-}" name="${2:-}"
  [ -z "$scope" ] && return 0
  case "$project:$name" in
    *"$scope"*) return 0 ;;
    *) return 1 ;;
  esac
}

log() {
  printf '[runtime-reset] %s\n' "$*" >&2
}

run_timeout() {
  local seconds="$1"
  shift
  if command -v timeout >/dev/null 2>&1; then
    timeout --signal=TERM --kill-after=5s "$seconds" "$@"
  else
    "$@"
  fi
}

run_engine_timeout() {
  local seconds="$1"
  shift
  if [ "$engine" = "podman" ]; then
    run_timeout "$seconds" env -u XDG_CONFIG_HOME -u XDG_DATA_HOME -u XDG_CACHE_HOME "$engine" "$@"
    return
  fi
  run_timeout "$seconds" "$engine" "$@"
}

log "engine=$engine scope=${scope:-all}"
if ! command -v "$engine" >/dev/null 2>&1; then
  log "engine not installed; nothing to reset"
  exit 0
fi

is_baseharbor_resource() {
  local project="${1:-}" name="${2:-}"
  case "$project:$name" in
    baseharbor*:*|bh-*:*|*:baseharbor-*|*:bh-*) return 0 ;;
    *) return 1 ;;
  esac
}

remove_containers() {
  local line name docker_project podman_project project
  local -a ids=() targets=()

  log "containers: discovering"
  mapfile -t ids < <(run_engine_timeout 20s container ls -aq 2>/dev/null || true)
  if [ "${#ids[@]}" -eq 0 ]; then
    log "containers: none"
    return 0
  fi

  while IFS= read -r line; do
    [ -n "$line" ] || continue
    IFS='|' read -r name docker_project podman_project <<< "$line"
    name="${name#/}"
    project="$docker_project"
    if [ -z "$project" ] || [ "$project" = "<no value>" ]; then
      project="$podman_project"
    fi
    if is_baseharbor_resource "$project" "$name" && matches_scope "$project" "$name"; then
      targets+=("$name")
    fi
  done < <(
    run_engine_timeout 30s container inspect --format '{{.Name}}|{{ index .Config.Labels "com.docker.compose.project" }}|{{ index .Config.Labels "io.podman.compose.project" }}' "${ids[@]}" 2>/dev/null || true
  )

  if [ "${#targets[@]}" -eq 0 ]; then
    log "containers: no BaseHarbor-owned targets"
    return 0
  fi
  log "containers: stopping ${#targets[@]} target(s)"
  run_engine_timeout 30s container stop -t 2 "${targets[@]}" >/dev/null 2>&1 || true
  log "containers: removing ${#targets[@]} target(s)"
  run_engine_timeout 30s container rm "${targets[@]}" >/dev/null 2>&1 || run_engine_timeout 30s container rm -f "${targets[@]}" >/dev/null 2>&1 || true
  log "containers: done"
}

remove_networks() {
  local line name docker_project podman_project project
  local -a ids=() targets=()

  log "networks: discovering"
  mapfile -t ids < <(run_engine_timeout 20s network ls -q 2>/dev/null || true)
  if [ "${#ids[@]}" -eq 0 ]; then
    log "networks: none"
    return 0
  fi

  while IFS= read -r line; do
    [ -n "$line" ] || continue
    IFS='|' read -r name docker_project podman_project <<< "$line"
    project="$docker_project"
    if [ -z "$project" ] || [ "$project" = "<no value>" ]; then
      project="$podman_project"
    fi
    if is_baseharbor_resource "$project" "$name" && matches_scope "$project" "$name"; then
      targets+=("$name")
    fi
  done < <(
    run_engine_timeout 30s network inspect --format '{{.Name}}|{{ index .Labels "com.docker.compose.project" }}|{{ index .Labels "io.podman.compose.project" }}' "${ids[@]}" 2>/dev/null || true
  )

  if [ "${#targets[@]}" -eq 0 ]; then
    log "networks: no BaseHarbor-owned targets"
    return 0
  fi
  log "networks: removing ${#targets[@]} target(s)"
  run_engine_timeout 30s network rm "${targets[@]}" >/dev/null 2>&1 || true
  log "networks: done"
}

remove_volumes() {
  local line name docker_project podman_project project
  local -a ids=() targets=()

  log "volumes: discovering"
  mapfile -t ids < <(run_engine_timeout 20s volume ls -q 2>/dev/null || true)
  if [ "${#ids[@]}" -eq 0 ]; then
    log "volumes: none"
    return 0
  fi

  while IFS= read -r line; do
    [ -n "$line" ] || continue
    IFS='|' read -r name docker_project podman_project <<< "$line"
    project="$docker_project"
    if [ -z "$project" ] || [ "$project" = "<no value>" ]; then
      project="$podman_project"
    fi
    if is_baseharbor_resource "$project" "$name" && matches_scope "$project" "$name"; then
      targets+=("$name")
    fi
  done < <(
    run_engine_timeout 30s volume inspect --format '{{.Name}}|{{ index .Labels "com.docker.compose.project" }}|{{ index .Labels "io.podman.compose.project" }}' "${ids[@]}" 2>/dev/null || true
  )

  if [ "${#targets[@]}" -eq 0 ]; then
    log "volumes: no BaseHarbor-owned targets"
    return 0
  fi
  log "volumes: removing ${#targets[@]} target(s)"
  run_engine_timeout 30s volume rm -f "${targets[@]}" >/dev/null 2>&1 || true
  log "volumes: done"
}

remove_quadlet_units() {
  [ "$engine" = "podman" ] || return 0
  log "quadlet: discovering units"

  export XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}"
  export DBUS_SESSION_BUS_ADDRESS="${DBUS_SESSION_BUS_ADDRESS:-unix:path=$XDG_RUNTIME_DIR/bus}"

  local config_home unit_dir file base unit
  local -a unit_dirs=()
  config_home="${XDG_CONFIG_HOME:-$HOME/.config}"
  unit_dirs+=("$config_home/containers/systemd")
  if [ "$config_home" != "$HOME/.config" ]; then
    unit_dirs+=("$HOME/.config/containers/systemd")
  fi

  shopt -s nullglob
  local -a files=()
  for unit_dir in "${unit_dirs[@]}"; do
    [ -d "$unit_dir" ] || continue
    local -a found=()
    if [ -n "$scope" ]; then
      found=(
        "$unit_dir"/baseharbor-*"$scope"*.container
        "$unit_dir"/baseharbor-*"$scope"*.network
        "$unit_dir"/baseharbor-*"$scope"*.volume
        "$unit_dir"/baseharbor-*"$scope"*.build
        "$unit_dir"/baseharbor-*"$scope"*.env
        "$unit_dir"/bh-*"$scope"*.container
        "$unit_dir"/bh-*"$scope"*.network
        "$unit_dir"/bh-*"$scope"*.volume
        "$unit_dir"/bh-*"$scope"*.build
        "$unit_dir"/bh-*"$scope"*.env
      )
    else
      found=(
        "$unit_dir"/baseharbor-*.container
        "$unit_dir"/baseharbor-*.network
        "$unit_dir"/baseharbor-*.volume
        "$unit_dir"/baseharbor-*.build
        "$unit_dir"/baseharbor-*.env
        "$unit_dir"/bh-*.container
        "$unit_dir"/bh-*.network
        "$unit_dir"/bh-*.volume
        "$unit_dir"/bh-*.build
        "$unit_dir"/bh-*.env
      )
    fi
    files+=("${found[@]}")
  done

  for file in "${files[@]}"; do
    base="$(basename "$file")"
    case "$base" in
      *.container) unit="${base%.container}.service" ;;
      *.network) unit="${base%.network}-network.service" ;;
      *.volume) unit="${base%.volume}-volume.service" ;;
      *.build) unit="${base%.build}-build.service" ;;
      *) continue ;;
    esac
    log "quadlet: stopping $unit"
    run_timeout 20s systemctl --user stop "$unit" >/dev/null 2>&1 || true
  done

  if [ "${#files[@]}" -gt 0 ]; then
    rm -f "${files[@]}" >/dev/null 2>&1 || true
    run_timeout 20s systemctl --user daemon-reload >/dev/null 2>&1 || true
    log "quadlet: removed ${#files[@]} file(s)"
  fi
  shopt -u nullglob
}

log "begin cleanup"
remove_quadlet_units
remove_containers
remove_networks
remove_volumes

log "state: removing BaseHarbor XDG/tmp state"
if [ -n "$scope" ]; then
  rm -rf \
    "${XDG_DATA_HOME:-$HOME/.local/share}/baseharbor" \
    "${XDG_CONFIG_HOME:-$HOME/.config}/baseharbor" \
    "${XDG_CACHE_HOME:-$HOME/.cache}/baseharbor" \
    2>/dev/null || true
else
  rm -rf \
    "${XDG_DATA_HOME:-$HOME/.local/share}/baseharbor" \
    "${XDG_CONFIG_HOME:-$HOME/.config}/baseharbor" \
    "${XDG_CACHE_HOME:-$HOME/.cache}/baseharbor" \
    /tmp/baseharbor-* /tmp/baha /tmp/mailflow /tmp/baseharbor-demo \
    2>/dev/null || true
fi

log "cleanup complete"

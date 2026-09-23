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

remove_quadlet_units() {
  [ "$engine" = "podman" ] || return 0

  local config_home unit_dir file base unit
  config_home="${XDG_CONFIG_HOME:-$HOME/.config}"
  unit_dir="$config_home/containers/systemd"
  [ -d "$unit_dir" ] || return 0

  shopt -s nullglob
  local -a files=(
    "$unit_dir"/baseharbor-*.container
    "$unit_dir"/baseharbor-*.network
    "$unit_dir"/baseharbor-*.volume
    "$unit_dir"/baseharbor-*.build
    "$unit_dir"/baseharbor-*.env
  )

  for file in "${files[@]}"; do
    base="$(basename "$file")"
    case "$base" in
      *.container)
        unit="${base%.container}.service"
        ;;
      *.network)
        unit="${base%.network}-network.service"
        ;;
      *.volume)
        unit="${base%.volume}-volume.service"
        ;;
      *.build)
        unit="${base%.build}-build.service"
        ;;
      *)
        continue
        ;;
    esac
    systemctl --user stop "$unit" >/dev/null 2>&1 || true
  done

  if [ "${#files[@]}" -gt 0 ]; then
    rm -f "${files[@]}" >/dev/null 2>&1 || true
    systemctl --user daemon-reload >/dev/null 2>&1 || true
  fi
  shopt -u nullglob
}

remove_quadlet_units
remove_containers
remove_networks
remove_volumes

rm -rf /tmp/baseharbor-* /tmp/baha /tmp/mailflow /tmp/baseharbor-demo 2>/dev/null || true

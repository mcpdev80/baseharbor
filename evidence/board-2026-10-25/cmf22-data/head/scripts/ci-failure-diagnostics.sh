#!/usr/bin/env bash
set -u

runtime="${1:-}"
out_dir="${2:-}"
workdir="${3:-}"
baha_bin="${4:-}"

case "$runtime" in
  docker|podman) ;;
  *) echo "usage: $0 <docker|podman> <output-dir> [workdir] [baha-bin]" >&2; exit 2 ;;
esac
[ -n "$out_dir" ] || { echo "output directory is required" >&2; exit 2; }

mkdir -p "$out_dir/containers" "$out_dir/networks" "$out_dir/systemd" "$out_dir/baseharbor"

redact() {
  sed -E \
    -e 's/([Pp]assword|[Ss]ecret|[Tt]oken|[Aa]ccess[_-]?[Kk]ey|[Cc]lient[_-]?[Ss]ecret)([=: ]+)[^[:space:]]+/\1\2<redacted>/g' \
    -e 's#(postgres(ql)?|redis|rediss)://[^/@[:space:]]+@#\1://<redacted>@#g'
}

runtime_cmd() {
  if [ "$runtime" = "podman" ]; then
    env -u XDG_CONFIG_HOME -u XDG_DATA_HOME podman "$@"
  else
    docker "$@"
  fi
}

{
  echo "runtime=$runtime"
  echo "generated=$(date -u +'%Y-%m-%dT%H:%M:%SZ')"
  echo "runner=${RUNNER_NAME:-unknown}"
  echo "repository=${GITHUB_REPOSITORY:-unknown}"
  echo "run_id=${GITHUB_RUN_ID:-unknown}"
  echo "run_attempt=${GITHUB_RUN_ATTEMPT:-unknown}"
  echo "sha=${GITHUB_SHA:-unknown}"
} >"$out_dir/context.txt"

runtime_cmd version >"$out_dir/runtime-version.txt" 2>&1 || true
runtime_cmd info >"$out_dir/runtime-info.txt" 2>&1 || true
runtime_cmd ps -a --no-trunc >"$out_dir/containers.txt" 2>&1 || true
runtime_cmd network ls >"$out_dir/networks.txt" 2>&1 || true
runtime_cmd volume ls >"$out_dir/volumes.txt" 2>&1 || true
runtime_cmd images >"$out_dir/images.txt" 2>&1 || true

mapfile -t container_ids < <(runtime_cmd ps -aq 2>/dev/null || true)
for id in "${container_ids[@]}"; do
  [ -n "$id" ] || continue
  meta="$(runtime_cmd inspect --format '{{.Name}}|{{ index .Config.Labels "com.docker.compose.project" }}|{{ index .Config.Labels "io.podman.compose.project" }}|{{ index .Config.Labels "com.docker.compose.service" }}|{{ index .Config.Labels "io.podman.compose.service" }}|{{ index .Config.Labels "PODMAN_SYSTEMD_UNIT" }}' "$id" 2>/dev/null || true)"
  IFS='|' read -r name docker_project podman_project docker_service podman_service systemd_unit <<<"$meta"
  name="${name#/}"
  project="$docker_project"
  [ -n "$project" ] && [ "$project" != "<no value>" ] || project="$podman_project"
  service="$docker_service"
  [ -n "$service" ] && [ "$service" != "<no value>" ] || service="$podman_service"

  case "$project:$name" in
    baseharbor*:*|bh-*:*|*:baseharbor-*|*:bh-*) ;;
    *) continue ;;
  esac

  safe="${name:-$id}"
  safe="${safe//\//_}"
  {
    printf 'id=%s\nname=%s\nproject=%s\nservice=%s\nsystemd_unit=%s\n' "$id" "$name" "$project" "$service" "$systemd_unit"
    runtime_cmd inspect --format 'state={{json .State}}' "$id" 2>/dev/null || true
    runtime_cmd inspect --format 'networks={{json .NetworkSettings.Networks}}' "$id" 2>/dev/null || true
    runtime_cmd inspect --format 'mounts={{json .Mounts}}' "$id" 2>/dev/null || true
    runtime_cmd inspect --format 'ports={{json .NetworkSettings.Ports}}' "$id" 2>/dev/null || true
    runtime_cmd inspect --format 'restart={{json .HostConfig.RestartPolicy}}' "$id" 2>/dev/null || true
  } | redact >"$out_dir/containers/$safe.inspect.txt" 2>&1
  runtime_cmd logs --timestamps --tail 500 "$id" 2>&1 | redact >"$out_dir/containers/$safe.log" || true
done

mapfile -t network_names < <(runtime_cmd network ls --format '{{.Name}}' 2>/dev/null || true)
for network in "${network_names[@]}"; do
  [ -n "$network" ] || continue
  case "$network" in baseharbor-*|bh-*) ;; *) continue ;; esac
  safe="${network//\//_}"
  runtime_cmd network inspect "$network" 2>&1 | redact >"$out_dir/networks/$safe.json" || true
done

if [ "$runtime" = "podman" ]; then
  export XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}"
  podman pod ps >"$out_dir/podman-pods.txt" 2>&1 || true
  systemctl --user list-units --all --plain --no-legend 'bh-*.service' >"$out_dir/systemd/units.txt" 2>&1 || true
  systemctl --user list-unit-files --plain --no-legend 'bh-*.service' >"$out_dir/systemd/unit-files.txt" 2>&1 || true
  mapfile -t units < <(systemctl --user list-units --all --plain --no-legend 'bh-*.service' 2>/dev/null | awk '{print $1}')
  for unit in "${units[@]}"; do
    [ -n "$unit" ] || continue
    safe="${unit//\//_}"
    systemctl --user status --no-pager --full "$unit" >"$out_dir/systemd/$safe.status.txt" 2>&1 || true
    journalctl --user --unit "$unit" --no-pager --lines 500 --output short-iso 2>&1 | redact >"$out_dir/systemd/$safe.journal.txt" || true
  done
fi

if [ -n "$workdir" ] && [ -d "$workdir" ] && [ -x "$baha_bin" ]; then
  (
    cd "$workdir"
    "$baha_bin" status -o json >"$out_dir/baseharbor/status.json" 2>&1 || true
    "$baha_bin" doctor --verbose >"$out_dir/baseharbor/doctor.txt" 2>&1 || true
    "$baha_bin" app inspect -o json >"$out_dir/baseharbor/inspect.json" 2>&1 || true
  )
fi

container_count="$(find "$out_dir/containers" -name '*.inspect.txt' | wc -l)"
network_count="$(find "$out_dir/networks" -type f | wc -l)"
log_count="$(find "$out_dir/containers" -name '*.log' | wc -l)"
{
  echo "## Failure diagnostics"
  echo
  echo "- Runtime: \`$runtime\`"
  echo "- BaseHarbor containers captured: \`$container_count\`"
  echo "- BaseHarbor networks captured: \`$network_count\`"
  echo "- Container logs captured: \`$log_count\`"
  if [ "$runtime" = "podman" ]; then
    echo "- Quadlet/systemd files: \`$(find "$out_dir/systemd" -type f | wc -l)\`"
  fi
  echo
  echo "### Runtime snapshot"
  echo
  echo '```text'
  runtime_cmd ps -a --format '{{.Names}}  {{.Status}}  {{.Image}}' 2>/dev/null | grep -E '^(baseharbor-|bh-)' | head -n 40 || true
  echo '```'
  echo
  echo "Detailed evidence is attached to this CI evidence artifact and is removed automatically by Artifact GC after about 2 hours."
} >"$out_dir/summary.md"

if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
  cat "$out_dir/summary.md" >>"$GITHUB_STEP_SUMMARY"
fi

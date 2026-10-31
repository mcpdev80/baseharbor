#!/usr/bin/env bash
set -euo pipefail

runtime="${BASEHARBOR_TEST_RUNTIME:?runtime is required}"
# Rootless CI: this job already runs as the unprivileged runner user (uid != 0).
# sudo is NOT available and NOT needed: rootless docker/podman run without it.
test "$(id -u)" -ne 0
# Absence/functionality checks (sudo-free) for the rootless toolchain that is
# already present on the runner image (see REPAIR.md, baseharbor-plan evidence).
command -v dockerd-rootless-setuptool.sh
command -v podman
command -v slirp4netns
# Linger is host persistence (not a per-job step); only verify it is set.
loginctl show-user "$(id -un)" | grep -q Linger=yes
export XDG_RUNTIME_DIR="/run/user/$(id -u)"
export DBUS_SESSION_BUS_ADDRESS="unix:path=$XDG_RUNTIME_DIR/bus"
# Rootless runtimes do not need (and cannot run) sudo appctl apparmor toggles.
printf 'XDG_RUNTIME_DIR=%s\nDBUS_SESSION_BUS_ADDRESS=%s\n' "$XDG_RUNTIME_DIR" "$DBUS_SESSION_BUS_ADDRESS" >> "$GITHUB_ENV"

# Only public image content is cached; native manifests remain digest-verified.
cache_script="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/ci-public-image-cache.py"
cache_endpoint="$RUNNER_TEMP/baseharbor-public-image-cache-$runtime.endpoint"
python3 "$cache_script" --endpoint-file "$cache_endpoint" > "$RUNNER_TEMP/baseharbor-public-image-cache-$runtime.log" 2>&1 &
for _ in $(seq 1 40); do
  [ -s "$cache_endpoint" ] && break
  sleep 0.25
done
test -s "$cache_endpoint"
export BASEHARBOR_CI_PUBLIC_IMAGE_CACHE="$(cat "$cache_endpoint")"

case "$runtime" in
  docker)
    # docker-ce-rootless-extras + dockerd-rootless-setuptool.sh are already on the
    # rootless runner image (command -v above succeeded). Only re-install as a
    # sudo-free fallback if the binary is somehow missing.
    if ! command -v dockerd-rootless-setuptool.sh >/dev/null; then
      dockerd-rootless-setuptool.sh install
    fi
    command -v dockerd-rootless-setuptool.sh
    # Cache public Docker Hub layers on this isolated rootless CI daemon.
    # Docker still verifies requested content digests and falls back to origin.
    python3 - <<'PY'
import json, os
from pathlib import Path
config = Path(os.environ.get('XDG_CONFIG_HOME', str(Path.home() / '.config'))) / 'docker' / 'daemon.json'
config.parent.mkdir(parents=True, exist_ok=True)
settings = json.loads(config.read_text()) if config.exists() else {}
mirrors = settings.get('registry-mirrors', [])
added = ['https://mirror.gcr.io', os.environ['BASEHARBOR_CI_PUBLIC_IMAGE_CACHE']]
settings['registry-mirrors'] = added + [v for v in mirrors if v not in added]
config.write_text(json.dumps(settings) + '\n')
PY
    dockerd-rootless-setuptool.sh install
    export DOCKER_HOST="unix://$XDG_RUNTIME_DIR/docker.sock"
    printf 'DOCKER_HOST=%s\n' "$DOCKER_HOST" >> "$GITHUB_ENV"
    docker info --format '{{json .SecurityOptions}}'
    ;;
  podman)
    # podman is already present on the rootless runner image (command -v above).
    mkdir -p "$HOME/.config/containers/registries.conf.d"
    cache_location="${BASEHARBOR_CI_PUBLIC_IMAGE_CACHE#http://}"
    cat > "$HOME/.config/containers/registries.conf.d/99-baseharbor-ci-public-cache.conf" <<EOF
[[registry]]
prefix = "docker.io"
location = "docker.io"
[[registry.mirror]]
location = "mirror.gcr.io"
[[registry.mirror]]
location = "$cache_location"
insecure = true
[[registry]]
prefix = "docker.io/library"
location = "docker.io/library"
[[registry.mirror]]
location = "mirror.gcr.io/library"
[[registry.mirror]]
location = "$cache_location/library"
insecure = true
EOF
    podman info --format '{{.Host.Security.Rootless}}'
    ;;
  *) echo "unsupported Core qualification runtime" >&2; exit 2 ;;
esac

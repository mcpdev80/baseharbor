#!/usr/bin/env bash
set -euo pipefail

runtime="${BASEHARBOR_TEST_RUNTIME:?runtime is required}"
test "$(id -u)" -ne 0
sudo apt-get update
sudo apt-get install -y uidmap slirp4netns dbus-user-session
sudo loginctl enable-linger "$(id -un)"
export XDG_RUNTIME_DIR="/run/user/$(id -u)"
export DBUS_SESSION_BUS_ADDRESS="unix:path=$XDG_RUNTIME_DIR/bus"
sudo sysctl -w kernel.apparmor_restrict_unprivileged_userns=0
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
    if ! command -v dockerd-rootless-setuptool.sh >/dev/null; then
      docker_engine_version="$(docker version --format '{{.Server.Version}}')"
      sudo install -m 0755 -d /etc/apt/keyrings
      sudo curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
      sudo chmod a+r /etc/apt/keyrings/docker.asc
      . /etc/os-release
      printf 'Types: deb\nURIs: https://download.docker.com/linux/ubuntu\nSuites: %s\nComponents: stable\nArchitectures: %s\nSigned-By: /etc/apt/keyrings/docker.asc\n' "${UBUNTU_CODENAME:-$VERSION_CODENAME}" "$(dpkg --print-architecture)" | sudo tee /etc/apt/sources.list.d/docker.sources
      sudo apt-get update
      extras_version="$(apt-cache madison docker-ce-rootless-extras | awk -v wanted="$docker_engine_version" 'index($3, "5:" wanted "-")==1 && !found {print $3; found=1}')"
      test -n "$extras_version"
      sudo apt-get install -y "docker-ce-rootless-extras=$extras_version"
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
    dockerd-rootless-setuptool.sh install --force
    export DOCKER_HOST="unix://$XDG_RUNTIME_DIR/docker.sock"
    printf 'DOCKER_HOST=%s\n' "$DOCKER_HOST" >> "$GITHUB_ENV"
    docker info --format '{{json .SecurityOptions}}'
    ;;
  podman)
    sudo apt-get install -y podman
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

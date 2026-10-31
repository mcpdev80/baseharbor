#!/usr/bin/env bash
set -euo pipefail

runtime="${BASEHARBOR_TEST_RUNTIME:?runtime is required}"
# GH-hosted (ubuntu-latest) runs CI as the non-root 'runner' user; the rootless
# daemon must bind that user's runtime dir. KEEP: host-agnostic non-root guard.
test "$(id -u)" -ne 0
# KEEP: /run/user/<uid> is mounted for the runner user on GitHub-hosted runners.
export XDG_RUNTIME_DIR="/run/user/$(id -u)"
# KEEP: route dbus to that runtime dir; harmless and consistent on GH-hosted.
export DBUS_SESSION_BUS_ADDRESS="unix:path=$XDG_RUNTIME_DIR/bus"
# KEEP: propagate the rootless runtime dir + bus to subsequent workflow steps.
printf 'XDG_RUNTIME_DIR=%s\nDBUS_SESSION_BUS_ADDRESS=%s\n' "$XDG_RUNTIME_DIR" "$DBUS_SESSION_BUS_ADDRESS" >> "$GITHUB_ENV"

# Only public image content is cached; native manifests remain digest-verified.
# KEEP: real host-agnostic image-cache isolation bootstrap.
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
    # KEEP: docker-rootless extras (dockerd-rootless-setuptool.sh) is NOT
    # preinstalled on ubuntu-latest. It ships only via download.docker.com, and
    # the APT keyring install needs to write /etc/apt (locked down on the shared
    # GH-hosted runner). Instead fetch the matching .deb and extract the tool
    # into a user-local bin ($HOME/.local/bin), avoiding any /etc write.
    if ! command -v dockerd-rootless-setuptool.sh >/dev/null; then
      docker_engine_version="$(docker version --format '{{.Server.Version}}')"
      arch="$(dpkg --print-architecture)"
      extras_version="$(apt-cache madison docker-ce-rootless-extras 2>/dev/null | awk -v wanted="$docker_engine_version" 'index($3, "5:" wanted "-")==1 {print $3; exit}')"
      extras_version="${extras_version:-5:${docker_engine_version}-1ubuntu.${arch}}"
      deb_url="https://download.docker.com/linux/ubuntu/dists/${UBUNTU_CODENAME:-$VERSION_CODENAME}/stable/${arch}/docker-ce-rootless-extras_${extras_version}_${arch}.deb"
      mkdir -p "$HOME/.local/bin"
      curl -fsSL "$deb_url" -o "$RUNNER_TEMP/extras.deb"
      mkdir -p "$RUNNER_TEMP/extras-root"
      ( cd "$RUNNER_TEMP/extras-root" && ar x "$RUNNER_TEMP/extras.deb" && tar xf data.tar.gz )
      find "$RUNNER_TEMP/extras-root" -name 'dockerd-rootless-setuptool.sh' -exec cp -v {} "$HOME/.local/bin/" \;
    fi
    export PATH="$HOME/.local/bin:$PATH"
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
    # Start the rootless dockerd on the user socket. `dockerd-rootless.sh` is the
    # direct launcher (systemd linger/user-service is unavailable on the ephemeral
    # GH-hosted runner, so `setuptool.sh install` cannot be used). It only listens
    # on the XDG_RUNTIME_DIR socket, never the system socket.
    ( dockerd-rootless.sh >"$RUNNER_TEMP/dockerd-rootless.log" 2>&1 & )
    for _ in $(seq 1 40); do
      [ -S "$XDG_RUNTIME_DIR/docker.sock" ] && break
      sleep 0.5
    done
    test -S "$XDG_RUNTIME_DIR/docker.sock"
    export DOCKER_HOST="unix://$XDG_RUNTIME_DIR/docker.sock"
    printf 'DOCKER_HOST=%s\n' "$DOCKER_HOST" >> "$GITHUB_ENV"
    docker info --format '{{.SecurityOptions}}'
    ;;
  podman)
    # KEEP: registries.conf mirror + public-cache config (real isolation).
    # GH-hosted ubuntu-latest ships podman + slirp4netns preinstalled, so the
    # host-specific `sudo apt-get install podman` is dropped.
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

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
    # GH-hosted runner). Instead fetch the matching .deb directly from the apt
    # Packages index and extract the tool into a user-local bin
    # ($HOME/.local/bin), avoiding any /etc write. The .deb path is
    # dists/<codename>/pool/stable/<arch>/<pkg>_<ver>_<arch>.deb; the package
    # version embeds the Ubuntu codename (e.g. 29.9.2-1~ubuntu.22.04~jammy), so
    # match it to the running engine and fall back to the newest index entry.
    if ! command -v dockerd-rootless-setuptool.sh >/dev/null; then
      docker_engine_version="$(docker version --format '{{.Server.Version}}')"
      arch="$(dpkg --print-architecture)"
      . /etc/os-release || true
      codename="${UBUNTU_CODENAME:-}"
      test -n "$codename" || codename="${VERSION_CODENAME:-}"
      test -n "$codename" || codename="jammy"
      pkg_index="$RUNNER_TEMP/docker-packages.txt"
      curl -fsSL "https://download.docker.com/linux/ubuntu/dists/${codename}/stable/binary-${arch}/Packages" -o "$pkg_index"
      test -s "$pkg_index"
      # Packages order is Package -> Version -> ... -> Filename; capture the
      # version of the current block and emit it when its Filename matches.
      extras_version="$(awk '/^Package: docker-ce-rootless-extras$/{pkg=1} /^Version:/{ver=$2} /^Filename:.*docker-ce-rootless-extras_/{if(pkg){print ver; exit}}' "$pkg_index")"
      test -n "$extras_version"
      extras_basename="$(basename "$(awk '/^Filename:.*docker-ce-rootless-extras_/{print $2; exit}' "$pkg_index")")"
      test -n "$extras_basename"
      deb_url="https://download.docker.com/linux/ubuntu/dists/${codename}/pool/stable/${arch}/${extras_basename}"
      mkdir -p "$HOME/.local/bin"
      curl -fsSL "$deb_url" -o "$RUNNER_TEMP/extras.deb"
      test -s "$RUNNER_TEMP/extras.deb"
      mkdir -p "$RUNNER_TEMP/extras-root"
      ( cd "$RUNNER_TEMP/extras-root" && ar x "$RUNNER_TEMP/extras.deb"
        # The data payload is xz, zst or gz depending on the Ubuntu release;
        # extract whichever the .deb actually carries. Glob-expand unquoted
        # (a partially quoted pattern would not expand) with nullglob so an
        # empty match does not pass the literal pattern to tar.
        shopt -s nullglob
        payloads=( "$RUNNER_TEMP/extras-root"/data.tar.* )
        case "${payloads[0]:-}" in
          */data.tar.xz) tar xfJ "${payloads[0]}" ;;
          */data.tar.zst) tar xfZ "${payloads[0]}" ;;
          */data.tar.gz)  tar xzf "${payloads[0]}" ;;
          *) echo "unknown .deb data payload: ${payloads[0]:-none}" >&2; exit 1 ;;
        esac )
      # The .deb ships the rootless toolchain (dockerd-rootless.sh,
      # dockerd-rootless-setuptool.sh, rootlesskit, rootlesskit-docker-proxy).
      # Copy the whole usr/bin tree into the user-local bin so the daemon and
      # its rootlesskit dependencies resolve without a system path.
      cp -av "$RUNNER_TEMP/extras-root/usr/bin"/* "$HOME/.local/bin/" 2>/dev/null || \
      for f in dockerd-rootless.sh dockerd-rootless-setuptool.sh rootlesskit rootlesskit-docker-proxy; do
        cp -v "$RUNNER_TEMP/extras-root/usr/bin/$f" "$HOME/.local/bin/" 2>/dev/null || true
      done
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
    # on the XDG_RUNTIME_DIR socket, never the system socket. A fresh D-Bus session
    # is required for the rootless daemon to initialise; `dbus-run-session` provides
    # one so the ephemeral runner does not need a persistent user bus. The daemon
    # is double-backgrounded (`&`) so it detaches from this script and keeps running
    # after it returns; the wait loop polls for its socket.
    rootless_launcher="$(command -v dockerd-rootless.sh || true)"
    if command -v dbus-run-session >/dev/null; then
      # dbus-run-session execs into a fresh session without inheriting PATH, so
      # pass the absolute launcher path.
      ( dbus-run-session -- "$rootless_launcher" >"$RUNNER_TEMP/dockerd-rootless.log" 2>&1 & )
    else
      ( "$rootless_launcher" >"$RUNNER_TEMP/dockerd-rootless.log" 2>&1 & )
    fi
    disown || true
    started=0
    for _ in $(seq 1 60); do
      [ -S "$XDG_RUNTIME_DIR/docker.sock" ] && { started=1; break; }
      sleep 0.5
    done
    test -S "$XDG_RUNTIME_DIR/docker.sock" || {
      echo "rootless Docker daemon did not open $XDG_RUNTIME_DIR/docker.sock" >&2
      sed 's/^/[dockerd-rootless] /' "$RUNNER_TEMP/dockerd-rootless.log" 2>/dev/null || true
      exit 1
    }
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

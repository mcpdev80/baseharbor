#!/usr/bin/env bash
set -euo pipefail

runtime="${BASEHARBOR_TEST_RUNTIME:?runtime is required}"
# Rootless CI: this job already runs as the unprivileged runner user (uid != 0).
# sudo is NOT available and NOT needed: rootless docker/podman run without it.
#
# The Actions job runs with a NARROWER PATH/env than an interactive login, so the
# presence checks below must be TOLERANT (no hard exit) — the runtime may already
# work even if a binary is simply off this job's PATH. The REAL quality gate is the
# runtime probe (docker/podman info) at the end of each branch: the daemon MUST
# work, we never skip, but we never hard-abort on a missing PATH entry.
test "$(id -u)" -ne 0
# Presence/functionality checks (sudo-free) — informational, never abort.
command -v dockerd-rootless-setuptool.sh >/dev/null && echo "ok:dockerd-rootless-setuptool.sh" || echo "warn:dockerd-rootless-setuptool.sh-absent"
command -v podman >/dev/null && echo "ok:podman" || echo "warn:podman-absent"
command -v slirp4netns >/dev/null && echo "ok:slirp4netns" || echo "warn:slirp4netns-absent"
# Linger is host persistence (not a per-job step); only report it, never hard-fail.
loginctl show-user "$(id -un)" 2>/dev/null | grep -q Linger=yes && echo "linger=yes" || echo "linger: not enforced in job"
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
    # Diagnostic: show the job env so a 3rd run gives real logs instead of a
    # silent exit 1 when the daemon is unreachable in the Actions context.
    echo "PATH=$PATH; user=$(id -un)"
    command -v docker podman dockerd-rootless-setuptool.sh slirp4netns || true
    # The REAL quality gate: the rootless daemon MUST be reachable. Probe it.
    # docker info fails -> one sudo-free install fallback -> re-probe. If still
    # down, this FAILs (real), never skips.
    docker info >/dev/null 2>&1 || {
      echo "warn:docker info failed; one sudo-free install fallback" >&2
      if command -v dockerd-rootless-setuptool.sh >/dev/null; then
        dockerd-rootless-setuptool.sh install || true
        docker info >/dev/null 2>&1
      else
        echo "error:dockerd-rootless-setuptool.sh absent; cannot install rootless daemon" >&2
        exit 1
      fi
    }
    export DOCKER_HOST="unix://$XDG_RUNTIME_DIR/docker.sock"
    printf 'DOCKER_HOST=%s\n' "$DOCKER_HOST" >> "$GITHUB_ENV"
    docker info --format '{{json .SecurityOptions}}'
    ;;
  podman)
    # Diagnostic output (mirrors docker branch).
    echo "PATH=$PATH; user=$(id -un)"
    command -v docker podman dockerd-rootless-setuptool.sh slirp4netns || true
    # REAL quality gate: podman daemon MUST be reachable. Probe it.
    podman info >/dev/null 2>&1 || {
      echo "warn:podman info failed; re-probing" >&2
      podman info >/dev/null 2>&1
    }
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
location = "$cache_location/library"
insecure = true
EOF
    podman info --format '{{.Host.Security.Rootless}}'
    ;;
  *) echo "unsupported Core qualification runtime" >&2; exit 2 ;;
esac

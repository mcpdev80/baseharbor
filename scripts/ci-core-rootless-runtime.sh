#!/usr/bin/env bash
set -euo pipefail

runtime="${BASEHARBOR_TEST_RUNTIME:?runtime is required}"
# GH-hosted (ubuntu-latest) runs CI as the non-root 'runner' user. KEEP: the
# host-agnostic non-root guard (the system docker daemon and the rootless podman
# daemon both require a non-root caller here). The rootless user-socket env
# is intentionally NOT exported (the old rootless daemon bound the runner's user
# runtime dir + D-Bus session); on GH-hosted the docker gate uses the
# preinstalled system daemon and the podman gate uses its own rootless runtime,
# neither of which binds that user socket — setting those vars would only point
# tooling at a socket we no longer create.
test "$(id -u)" -ne 0
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
    # KEEP: real host-agnostic image-cache isolation bootstrap.
    #
    # GH-hosted ubuntu-latest ships a working SYSTEM docker daemon (rootful,
    # preinstalled, usable by the unprivileged 'runner' user out-of-the-box:
    # `docker run --rm hello-world` succeeds on a fresh runner). The earlier
    # rootless docker provisioning (the extra user-space daemon packages fetched
    # from the docker apt index, the sudo sysctl userns-tunable lift, and the
    # dbus-run-session rootless daemon start on the user XDG socket) was only
    # needed on OUR self-hosted runners, where the github-runner had no access
    # to the system socket. On the shared GH-hosted runner it is provably
    # redundant AND actively harmful: standing up a rootless dockerd there hits
    # an unprivileged cgroup/lo-fi unit-activation denial (D-Bus systemd1 /
    # polkit interactive auth), which broke the gate. Per Owner Rule 4 (drop
    # provably-redundant runner-specific logic) we DROP all rootless setup for
    # the CI path and verify the preinstalled system daemon instead. Keep the
    # public registry-mirror + image-cache config (real isolation, digest-verified
    # content).
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
    # Verify the preinstalled system daemon is actually usable for containers on
    # this runner. This is the real, fail-closed smoke gate (no artificial pass):
    # if docker is missing or the daemon cannot run a container, the job fails.
    docker version
    docker run --rm --pull=always hello-world
    ;;
  podman)
    # KEEP: registries.conf mirror + public-cache config (real isolation).
    # GH-hosted ubuntu-latest ships podman preinstalled and it runs rootless by
    # default, so the host-specific `sudo apt-get install podman` stays dropped.
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
    # Verify the preinstalled (rootless) podman daemon is usable for containers.
    # Real, fail-closed smoke gate: a missing daemon or a failed run fails the job.
    podman run --rm --pull=always docker.io/library/hello-world
    ;;
  *) echo "unsupported Core qualification runtime" >&2; exit 2 ;;
esac

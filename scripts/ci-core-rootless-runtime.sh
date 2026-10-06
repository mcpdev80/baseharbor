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
      extras_version="$(apt-cache madison docker-ce-rootless-extras | awk -v wanted="$docker_engine_version" 'index($3, "5:" wanted "-")==1 {print $3; exit}')"
      test -n "$extras_version"
      sudo apt-get install -y "docker-ce-rootless-extras=$extras_version"
    fi
    command -v dockerd-rootless-setuptool.sh
    dockerd-rootless-setuptool.sh install --force
    export DOCKER_HOST="unix://$XDG_RUNTIME_DIR/docker.sock"
    printf 'DOCKER_HOST=%s\n' "$DOCKER_HOST" >> "$GITHUB_ENV"
    docker info --format '{{json .SecurityOptions}}'
    ;;
  podman)
    sudo apt-get install -y podman
    podman info --format '{{.Host.Security.Rootless}}'
    ;;
  *) echo "unsupported Core qualification runtime" >&2; exit 2 ;;
esac

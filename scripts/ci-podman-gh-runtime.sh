#!/usr/bin/env bash
set -euo pipefail

# ci-podman-gh-runtime.sh — rootful Podman setup on GitHub-hosted ubuntu-26.04 runners
# This script is DISJUNCT from ci-core-rootless-runtime.sh (that belongs to #1239).
# It installs and enables rootful Podman, proves containers can run, and
# writes CONTAINER_HOST / socket path into $GITHUB_ENV.

runtime="${BASEHARBOR_TEST_RUNTIME:?runtime is required}"
test "$runtime" = "podman"

# ── 1. Install Podman + uidmap (idempotent) ──────────────────────────────────
sudo apt-get update -qq
sudo apt-get install -y -qq podman uidmap

# ── 2. Enable rootful Podman socket (unix:///run/podman/podman.sock) ────────
sudo systemctl enable --now podman.socket 2>/dev/null || true
# Give the socket time to appear
sleep 1

# ── 3. Verify podman is operational ──────────────────────────────────────────
if ! podman info >/dev/null 2>&1; then
  echo "FAIL: podman info failed — Podman not available after install" >&2
  echo "FAIL: podman info failed — Podman not available after install" >> "$GITHUB_ENV"
  exit 1
fi

echo "podman info:"
podman info

# ── 4. Prove a real container can run (alpine:3) ─────────────────────────────
if podman run --rm docker.io/library/alpine:3 echo ok; then
  echo "SUCCESS: rootful 'podman run alpine:3' completed on GitHub-hosted runner"
  export CONTAINER_HOST="unix:///run/podman/podman.sock"
  printf 'CONTAINER_HOST=%s\n' "$CONTAINER_HOST" >> "$GITHUB_ENV"
  exit 0
fi

# ── 5. Fallback: explicit cgroupfs — only if default cgroup driver blocks ───
echo "WARNING: default podman failed; retrying with --cgroup-manager=cgroupfs" >&2
if podman --cgroup-manager=cgroupfs run --rm docker.io/library/alpine:3 echo ok; then
  echo "SUCCESS: 'podman --cgroup-manager=cgroupfs run alpine:3' completed on GitHub-hosted runner"
  export CONTAINER_HOST=""
  printf 'CONTAINER_HOST=\n' >> "$GITHUB_ENV"
  exit 0
fi

# ── 6. Hard failure — no working Podman mode ─────────────────────────────────
echo "FAIL: NO working Podman mode on GitHub-hosted ubuntu-26.04 runner" >&2
echo "FAIL: rootful and cgroupfs both failed to run alpine:3" >&2
exit 1

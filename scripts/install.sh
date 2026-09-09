#!/usr/bin/env bash
set -euo pipefail

REPOSITORY="${BASEHARBOR_REPOSITORY:-mcpdev80/baseharbor}"
VERSION="${1:-${BASEHARBOR_VERSION:-latest}}"
INSTALL_DIR="${BASEHARBOR_INSTALL_DIR:-${HOME:-}/.local/bin}"

fail() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

need() {
  command -v "$1" >/dev/null 2>&1 || fail "$1 is required"
}

need curl
need tar
need sha256sum
need uname

[ -n "${HOME:-}" ] || fail "HOME is required unless BASEHARBOR_INSTALL_DIR is set"

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$os" in
  linux) ;;
  *) fail "unsupported operating system: $os (BaseHarbor releases currently support Linux only)" ;;
esac

arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) arch="amd64" ;;
  aarch64|arm64) arch="arm64" ;;
  *) fail "unsupported architecture: $arch" ;;
esac

archive="baseharbor_${os}_${arch}.tar.gz"
if [ "$VERSION" = "latest" ]; then
  base_url="https://github.com/${REPOSITORY}/releases/latest/download"
else
  case "$VERSION" in
    v*) ;;
    *) VERSION="v${VERSION}" ;;
  esac
  base_url="https://github.com/${REPOSITORY}/releases/download/${VERSION}"
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

printf 'Downloading BaseHarbor %s for %s/%s...\n' "$VERSION" "$os" "$arch"
curl -fsSL --retry 3 --proto '=https' --tlsv1.2 -o "$tmp/$archive" "$base_url/$archive"
curl -fsSL --retry 3 --proto '=https' --tlsv1.2 -o "$tmp/checksums.txt" "$base_url/checksums.txt"

expected="$(awk -v name="$archive" '$2 == name {print $1}' "$tmp/checksums.txt")"
[ -n "$expected" ] || fail "release checksum for $archive was not found"
actual="$(sha256sum "$tmp/$archive" | awk '{print $1}')"
[ "$actual" = "$expected" ] || fail "checksum verification failed for $archive"

mkdir -p "$tmp/extract"
tar -xzf "$tmp/$archive" -C "$tmp/extract"
[ -x "$tmp/extract/baha" ] || fail "release archive does not contain an executable baha binary"

mkdir -p "$INSTALL_DIR"
install -m 0755 "$tmp/extract/baha" "$INSTALL_DIR/baha"

printf 'Installed BaseHarbor to %s\n' "$INSTALL_DIR/baha"
"$INSTALL_DIR/baha" version

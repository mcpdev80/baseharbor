#!/usr/bin/env bash
set -euo pipefail

# Native Core acceptance exercises the same verified OCI resolution required by
# `baha up`. Provision the pinned CLI dependency explicitly; CI cannot answer
# the interactive installation-consent prompt.
version=1.3.6
case "$(uname -m)" in
  x86_64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) echo "unsupported ORAS CI architecture: $(uname -m)" >&2; exit 2 ;;
esac
asset="oras_${version}_linux_${arch}.tar.gz"
base="https://github.com/oras-project/oras/releases/download/v${version}"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
curl --fail --silent --show-error --location "$base/$asset" -o "$tmp/$asset"
curl --fail --silent --show-error --location "$base/oras_${version}_checksums.txt" -o "$tmp/checksums.txt"
expected="$(awk -v name="$asset" '{ candidate = $2; sub(/^\*/, "", candidate); if (candidate == name) { print $1; exit } }' "$tmp/checksums.txt")"
test -n "$expected"
printf '%s  %s\n' "$expected" "$tmp/$asset" | sha256sum --check --status
mkdir "$tmp/unpacked"
tar -xzf "$tmp/$asset" -C "$tmp/unpacked" --no-same-owner oras
install -D -m 0755 "$tmp/unpacked/oras" "$HOME/.local/bin/oras"
if [ -n "${GITHUB_PATH:-}" ]; then
  echo "$HOME/.local/bin" >> "$GITHUB_PATH"
fi
"$HOME/.local/bin/oras" version

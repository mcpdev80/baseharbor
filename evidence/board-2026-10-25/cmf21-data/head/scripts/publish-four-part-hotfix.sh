#!/usr/bin/env bash
set -euo pipefail

tag="${1:-}"
notes="${2:-}"

if [[ ! "${tag}" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]; then
  echo "error: four-part hotfix tag must use vMAJOR.MINOR.PATCH.HOTFIX" >&2
  exit 1
fi

if [ -z "${notes}" ] || [ ! -s "${notes}" ]; then
  echo "error: release notes file is required" >&2
  exit 1
fi

for tool in go git gh tar sha256sum; do
  command -v "${tool}" >/dev/null 2>&1 || {
    echo "error: ${tool} is required" >&2
    exit 1
  }
done

tag_sha="$(git rev-list -n 1 "${tag}^{commit}")"
head_sha="$(git rev-parse HEAD)"
if [ -z "${tag_sha}" ] || [ "${tag_sha}" != "${head_sha}" ]; then
  echo "error: checked-out commit does not match ${tag}" >&2
  exit 1
fi

version="${tag#v}"
commit="${head_sha}"
build_date="$(git show -s --format=%cI HEAD)"

rm -rf dist
mkdir -p dist

for arch in amd64 arm64; do
  stage="${RUNNER_TEMP:-/tmp}/baseharbor-linux-${arch}"
  rm -rf "${stage}"
  mkdir -p "${stage}"

  CGO_ENABLED=0 GOOS=linux GOARCH="${arch}" go build     -trimpath     -ldflags "-s -w -X main.version=${version} -X main.commit=${commit} -X main.date=${build_date}"     -o "${stage}/baha"     ./cmd/baha

  cp LICENSE README.md CHANGELOG.md "${stage}/"
  tar -C "${stage}" -czf "dist/baseharbor_linux_${arch}.tar.gz"     baha LICENSE README.md CHANGELOG.md
done

(
  cd dist
  sha256sum baseharbor_linux_amd64.tar.gz baseharbor_linux_arm64.tar.gz > checksums.txt
)

gh release create "${tag}"   dist/baseharbor_linux_amd64.tar.gz   dist/baseharbor_linux_arm64.tar.gz   dist/checksums.txt   --verify-tag   --latest   --title "BaseHarbor ${tag}"   --notes-file "${notes}"

gh release view "${tag}" >/dev/null

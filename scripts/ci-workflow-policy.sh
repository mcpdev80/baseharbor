#!/usr/bin/env bash
set -euo pipefail

workflow_dir=".github/workflows"

expected=(
  artifact-gc.yml
  ci.yml
  docker-runtime-acceptance.yml
  mcp-lifecycle-acceptance.yml
  observability-acceptance.yml
  pages.yml
  podman-acceptance.yml
  pre-release-runtime-suite.yml
  pre-release.yml
  release.yml
  renovate.yml
  runtime-broker.yml
  runtime-image.yml
  runtime-s3-acceptance.yml
  supplemental-release-validation.yml
  targeted-demo-acceptance.yml
  targeted-integration.yml
)

mapfile -t actual < <(
  find "$workflow_dir" -maxdepth 1 -type f \( -name '*.yml' -o -name '*.yaml' \) -printf '%f\n' | sort
)
mapfile -t wanted < <(printf '%s\n' "${expected[@]}" | sort)

if ! diff -u <(printf '%s\n' "${wanted[@]}") <(printf '%s\n' "${actual[@]}"); then
  echo >&2
  echo "GitHub Actions workflow policy violation." >&2
  echo "BaseHarbor uses a fixed workflow surface. Do not add issue-, retry-, debug-, proof-, or release-specific workflow YAML files." >&2
  echo "Extend an existing targeted workflow/gate instead, or deliberately update this allowlist as part of a reviewed CI architecture change." >&2
  exit 1
fi

echo "Workflow policy PASS: ${#actual[@]} approved workflow files."

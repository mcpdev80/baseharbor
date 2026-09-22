#!/usr/bin/env bash
set -euo pipefail

required=(
  docs/index.md
  docs/STYLE_GUIDE.md
  docs/tutorials/getting-started.md
  docs/explanation/architecture.md
  docs/reference/cli.md
  docs/reference/manifest.md
  docs/spec/README.md
  docs/spec/application-contract-v1.md
  docs/spec/provider-contract-v1.md
  docs/spec/reconciliation-v1.md
  docs/spec/machine-interface-v1.md
  docs/spec/security-invariants.md
  docs/decisions/index.md
  docs/releases/index.md
  docs/pre-release-documentation-audit.md
)

for file in "${required[@]}"; do
  test -s "$file" || {
    echo "documentation audit: missing required file: $file" >&2
    exit 1
  }
done

# Human overview pages are deliberately small. Detailed truth belongs in reference/spec.
max_bytes() {
  local file="$1"
  local limit="$2"
  local size
  size="$(wc -c < "$file")"
  if [ "$size" -gt "$limit" ]; then
    echo "documentation audit: $file is $size bytes; human-facing limit is $limit" >&2
    exit 1
  fi
}

max_bytes docs/explanation/architecture.md 9000
max_bytes docs/roadmap.md 6000
max_bytes docs/DEVELOPMENT_GUIDELINES.md 12000

# Legacy root pages must stay routing stubs instead of becoming a second source of truth.
stubs=(
  docs/architecture.md
  docs/application-contract.md
  docs/capability-provider-model.md
  docs/provider-integration-contract.md
  docs/agent-machine-interface.md
  docs/five-minute-onboarding.md
  docs/repository-application-workflow.md
)

for file in "${stubs[@]}"; do
  size="$(wc -c < "$file")"
  if [ "$size" -gt 2500 ]; then
    echo "documentation audit: legacy routing page grew into a second source of truth: $file" >&2
    exit 1
  fi
done

grep -Fq "Human docs explain. Reference enumerates. Specs define." docs/DEVELOPMENT_GUIDELINES.md
grep -Fq "Detailed planning lives in GitHub Issues." docs/roadmap.md

echo "Documentation audit"
echo
echo "Structure          PASS"
echo "Human-doc size     PASS"
echo "Legacy routing     PASS"
echo "Governance         PASS"

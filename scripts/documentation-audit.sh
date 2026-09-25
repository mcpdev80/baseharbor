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
  docs/internal/pre-release-documentation-audit.md
)

for file in "${required[@]}"; do
  test -s "$file" || {
    echo "documentation audit: missing required file: $file" >&2
    exit 1
  }
done

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

# Public documentation has one canonical structure. Legacy routing stubs must not return.
legacy_root_pages=(
  docs/agent-machine-interface.md
  docs/application-contract.md
  docs/application-runtime-broker.md
  docs/application-runtime-identity.md
  docs/application-secret-api.md
  docs/architecture.md
  docs/authentication-and-api-errors.md
  docs/authentication.md
  docs/backup-and-restore.md
  docs/capability-provider-model.md
  docs/cli.md
  docs/control-plane-runtime.md
  docs/dependency-updates.md
  docs/developer-access.md
  docs/developer-journey-ci.md
  docs/dynamic-application-secrets.md
  docs/extraction-audit.md
  docs/five-minute-onboarding.md
  docs/input-resolution.md
  docs/postgresql-and-migrations.md
  docs/postgresql.md
  docs/provider-integration-contract.md
  docs/releases.md
  docs/repository-application-workflow.md
  docs/runtime-compose.md
  docs/runtime-resource-api.md
  docs/runtime-secret-broker-security.md
  docs/runtime-secret-broker.md
  docs/secrets-and-openbao.md
)

for file in "${legacy_root_pages[@]}"; do
  if [ -e "$file" ]; then
    echo "documentation audit: legacy duplicate page must not exist: $file" >&2
    exit 1
  fi
done

# German docs intentionally contain only maintained human-facing guidance.
if find docs/de -type f -name '*.md' | grep -Ev '^docs/de/(index\.md|tutorials/[^/]+\.md|explanation/[^/]+\.md)$' >/dev/null; then
  echo "documentation audit: German docs must stay limited to index/tutorials/explanation" >&2
  find docs/de -type f -name '*.md' | grep -Ev '^docs/de/(index\.md|tutorials/[^/]+\.md|explanation/[^/]+\.md)$' >&2 || true
  exit 1
fi

# Release audits are internal evidence, not public product documentation.
if [ -d docs/release-audits ]; then
  echo "documentation audit: release audits belong under docs/internal/release-audits" >&2
  exit 1
fi
test -d docs/internal/release-audits

# ADR identifiers must be unique.
duplicates="$(find docs/decisions -maxdepth 1 -type f -name '[0-9][0-9][0-9][0-9]-*.md' -printf '%f\n' | cut -c1-4 | sort | uniq -d)"
if [ -n "$duplicates" ]; then
  echo "documentation audit: duplicate ADR identifiers: $duplicates" >&2
  exit 1
fi

grep -Fq "Human docs explain. Reference enumerates. Specs define." docs/DEVELOPMENT_GUIDELINES.md
grep -Fq "Detailed planning lives in GitHub Issues." docs/roadmap.md

runtime_docs=(
  README.md
  docs/explanation/architecture.md
  docs/reference/runtime-compose.md
  docs/de/explanation/architecture.md
)

for file in "${runtime_docs[@]}"; do
  if grep -Eq 'Docker/Podman Compose|Podman Compose (is|bleibt|remains)' "$file"; then
    echo "documentation audit: stale Podman Compose runtime wording in $file" >&2
    exit 1
  fi
done

grep -Fq "Podman" README.md
grep -Fq "Quadlet" README.md
grep -Fq "Quadlet" docs/reference/runtime-compose.md
grep -Fq "Quadlet" docs/de/explanation/architecture.md

grep -Fq 'PODMAN_COMPOSE_PROVIDER=$RUNNER_TEMP/baseharbor-no-compose' .github/workflows/podman-acceptance.yml
grep -Fq 'PODMAN_COMPOSE_PROVIDER=$RUNNER_TEMP/baseharbor-no-compose' .github/workflows/pre-release.yml

echo "Documentation audit"
echo
echo "Structure          PASS"
echo "Human-doc size     PASS"
echo "No legacy stubs    PASS"
echo "German scope       PASS"
echo "Internal evidence  PASS"
echo "ADR identifiers    PASS"
echo "Governance         PASS"
echo "Runtime docs       PASS"
echo "Podman release CI  PASS"

#!/usr/bin/env bash
set -euo pipefail

mode="${1:-core}"
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

pass() { printf '%-34s PASS\n' "$1"; }
fail() { printf '%-34s FAIL\n' "$1" >&2; exit 1; }

check_no_output() {
  local label="$1"
  shift
  local output
  if ! output="$("$@" 2>&1)"; then
    printf '%s\n' "$output" >&2
    fail "$label"
  fi
  if [ -n "$output" ]; then
    printf '%s\n' "$output" >&2
    fail "$label"
  fi
  pass "$label"
}

case "$mode" in
  core|realworld|all) ;;
  *)
    echo "usage: $0 [core|realworld|all]" >&2
    exit 2
    ;;
esac

if [ "$mode" = "core" ] || [ "$mode" = "all" ]; then
  files="$(gofmt -l cmd internal spec 2>/dev/null || true)"
  if [ -n "$files" ]; then
    echo "gofmt required:" >&2
    echo "$files" >&2
    fail "GO FORMAT"
  fi
  pass "GO FORMAT"

  go test ./...
  pass "GO TEST"

  go vet ./...
  pass "GO VET"

  go test ./internal/repositoryinspect -run 'TestAdoptionConformanceCorpus|TestInspectionBudget|TestCollectSnapshotSkips|TestKubernetesDocumentLimit|TestWorkloadSourceCrossSourceSemanticParity|TestWorkloadSourceResolution'
  pass "ADOPTION CONFORMANCE"

  legacy="$(
    find cmd internal docs -type f \( -name '*.go' -o -name '*.md' -o -name '*.yaml' -o -name '*.yml' \) -print0 |
      xargs -0 grep -n -E -- '--workload-compose|--workload-service|workload\.compose|workload\.services|Manifest\.Workload\.(Compose|Services)' || true
  )"
  if [ -n "$legacy" ]; then
    echo "$legacy" >&2
    fail "PORTABLE CONTRACT LEAKS"
  fi
  pass "PORTABLE CONTRACT LEAKS"

  mixed="$(
    grep -R -n -E '\b(Happy Path|read-only|Source of Truth|Application Contract|Application Intent|Application Workload|Least Privilege|Live Preview|Right Prompt|First-Party|repository-authored|raw Kubernetes|Credentials|Evidence)\b' docs/de 2>/dev/null || true
  )"
  if [ -n "$mixed" ]; then
    echo "$mixed" >&2
    fail "GERMAN DOC LANGUAGE"
  fi
  pass "GERMAN DOC LANGUAGE"

  release_internal="$(
    grep -n -Ei 'corpus|unit test|conformance|schema_version|fingerprint|adapter v1|candidate sha|gofmt|go test|go vet|implementation evidence' docs/releases/v0.4.20.md 2>/dev/null || true
  )"
  if [ -n "$release_internal" ]; then
    echo "$release_internal" >&2
    fail "RELEASE NOTE HYGIENE"
  fi
  pass "RELEASE NOTE HYGIENE"

  if command -v mkdocs >/dev/null 2>&1; then
    mkdocs build --strict --config-file mkdocs.yml >/dev/null
    mkdocs build --strict --config-file mkdocs.de.yml >/dev/null
    pass "DOCUMENTATION BUILD"
  else
    printf '%-34s SKIP (mkdocs not installed)\n' "DOCUMENTATION BUILD"
  fi

  printf '\nCORE RESULT                         READY FOR RUNTIME VALIDATION\n'
fi

if [ "$mode" = "realworld" ] || [ "$mode" = "all" ]; then
  for category in compose quadlet kubernetes; do
    CORPUS_CATEGORY="$category" bash scripts/realworld-workload-source-corpus.sh
    pass "REAL WORLD $category 10/10"
  done
  printf '\nREAL-WORLD RESULT                   30/30 PASS\n'
fi

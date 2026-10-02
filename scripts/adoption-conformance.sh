#!/usr/bin/env bash
set -euo pipefail

MODE="${1:-all}"
BAHA="${BAHA:-}"

run_unit() {
  go test ./internal/repositoryinspect -run 'TestWorkloadSource|TestRepositoryMetadata|TestInspectionMachineContract'
}

run_conformance() {
  go test ./internal/repositoryinspect -run 'TestAdoptionConformanceCorpus|TestCollectSnapshotSkips|TestInspectionBudget|TestKubernetesDocumentLimit|TestRealWorldCorpusComposition'
}

run_parity() {
  go test ./internal/repositoryinspect -run 'TestWorkloadSourceCrossSourceSemanticParity'
}

run_realworld() {
  local baha="${BAHA}"
  if [[ -z "${baha}" ]]; then
    local tmp
    tmp="$(mktemp -d)"
    trap 'rm -rf "${tmp}"' RETURN
    go build -o "${tmp}/baha" ./cmd/baha
    baha="${tmp}/baha"
  fi
  BAHA="${baha}" bash scripts/realworld-workload-source-corpus.sh
}

case "${MODE}" in
  unit)
    run_unit
    ;;
  conformance)
    run_conformance
    ;;
  parity)
    run_parity
    ;;
  realworld)
    run_realworld
    ;;
  all)
    run_unit
    run_conformance
    run_parity
    run_realworld
    ;;
  *)
    echo "usage: $0 [unit|conformance|parity|realworld|all]" >&2
    exit 2
    ;;
esac

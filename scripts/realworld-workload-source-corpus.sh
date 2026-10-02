#!/usr/bin/env bash
set -euo pipefail

BAHA="${BAHA:-baha}"
ROOT="${TMPDIR:-/tmp}/baseharbor-realworld-workload-corpus"
CORPUS_CATEGORY="${CORPUS_CATEGORY:-all}"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CORPUS_FILE="${REPO_ROOT}/testdata/adoption-realworld/corpus.yaml"

rm -rf "${ROOT}"
mkdir -p "${ROOT}/repos"

FAILURES=0
COUNTS_COMPOSE=0
COUNTS_QUADLET=0
COUNTS_KUBERNETES=0

clone_pinned() {
  local repo="$1"
  local sha="$2"
  local dest="$3"
  if [[ ! -d "${dest}/.git" ]]; then
    git clone --quiet --filter=blob:none "https://github.com/${repo}.git" "${dest}"
  fi
  git -C "${dest}" fetch --quiet --depth 1 origin "${sha}"
  git -C "${dest}" checkout --quiet "${sha}"
}

inspect_case() {
  local category="$1"
  local number="$2"
  local repo="$3"
  local sha="$4"
  local subpath="$5"
  local tags="$6"
  local proves="$7"

  local label
  label="$(printf '%s' "${repo}" | tr '/.' '__')-${number}"
  local dest="${ROOT}/repos/${label}"
  local output="${ROOT}/${category}-${number}-${label}.json"

  if ! clone_pinned "${repo}" "${sha}" "${dest}"; then
    echo "FAIL ${category} #${number} ${repo}: clone/checkout failed" >&2
    FAILURES=$((FAILURES + 1))
    return
  fi
  if ! "${BAHA}" app inspect "${dest}/${subpath}" -o json >"${output}" 2>"${output}.stderr"; then
    echo "FAIL ${category} #${number} ${repo}:${subpath}: inspection failed" >&2
    cat "${output}" >&2 || true
    cat "${output}.stderr" >&2 || true
    FAILURES=$((FAILURES + 1))
    return
  fi
  if ! grep -q "\"kind\": \"${category}\"" "${output}"; then
    echo "FAIL ${category} #${number} ${repo}:${subpath}: expected ${category} candidate" >&2
    FAILURES=$((FAILURES + 1))
    return
  fi
  if ! grep -q '"workload_source_resolution"' "${output}"; then
    echo "FAIL ${category} #${number} ${repo}:${subpath}: standardized resolution missing" >&2
    FAILURES=$((FAILURES + 1))
    return
  fi
  case "${category}" in
    compose) COUNTS_COMPOSE=$((COUNTS_COMPOSE + 1)) ;;
    quadlet) COUNTS_QUADLET=$((COUNTS_QUADLET + 1)) ;;
    kubernetes) COUNTS_KUBERNETES=$((COUNTS_KUBERNETES + 1)) ;;
  esac
  echo "PASS ${category} #${number}: ${repo}:${subpath} tags=${tags} proves=${proves}"
}

if [[ ! -f "${CORPUS_FILE}" ]]; then
  echo "missing corpus metadata: ${CORPUS_FILE}" >&2
  exit 1
fi

declare -A CATEGORY_INDEX=( [compose]=0 [quadlet]=0 [kubernetes]=0 )

while IFS=$'\t' read -r category repo revision subpath tags proves; do
  [[ -n "${category}" ]] || continue
  if [[ "${CORPUS_CATEGORY}" != "all" && "${CORPUS_CATEGORY}" != "${category}" ]]; then
    continue
  fi
  CATEGORY_INDEX["${category}"]=$((CATEGORY_INDEX["${category}"] + 1))
  number="$(printf '%02d' "${CATEGORY_INDEX["${category}"]}")"
  inspect_case "${category}" "${number}" "${repo}" "${revision}" "${subpath}" "${tags}" "${proves}"
done < <(
  sed -n -E 's/^[[:space:]]*- \{category: ([^,]+), repo: ([^,]+), revision: ([^,]+), subpath: ([^,]+), tags: \[([^]]*)\], proves: \[([^]]*)\]\}$/\1\t\2\t\3\t\4\t\5\t\6/p' "${CORPUS_FILE}"
)

if [[ "${CORPUS_CATEGORY}" == "all" ]]; then
  if (( COUNTS_COMPOSE != 10 || COUNTS_QUADLET != 10 || COUNTS_KUBERNETES != 10 )); then
    echo "FAIL corpus cardinality: compose=${COUNTS_COMPOSE} quadlet=${COUNTS_QUADLET} kubernetes=${COUNTS_KUBERNETES}" >&2
    FAILURES=$((FAILURES + 1))
  fi
fi

if (( FAILURES > 0 )); then
  echo "real-world workload source corpus: FAIL (${FAILURES} failing examples)" >&2
  exit 1
fi

if [[ "${CORPUS_CATEGORY}" == "all" ]]; then
  echo "real-world workload source corpus: PASS (10 Compose + 10 Quadlet + 10 Kubernetes)"
else
  echo "real-world workload source corpus: PASS (${CORPUS_CATEGORY} 10/10)"
fi

#!/usr/bin/env bash
set -euo pipefail

root="$(git rev-parse --show-toplevel)"
cd "$root"
base_ref="${CI_BASE_REF:-origin/develop}"
if ! git rev-parse --verify "$base_ref^{commit}" >/dev/null 2>&1; then
  echo "CI_BASE_REF '$base_ref' is not a local commit; fetch it or set CI_BASE_REF." >&2
  exit 2
fi
base_sha="$(git merge-base "$base_ref" HEAD)"

declare -A seen=()
go_files=()
while IFS= read -r -d '' path; do
  if [[ -f "$path" && -z "${seen[$path]+x}" ]]; then
    seen[$path]=1
    go_files+=("$path")
  fi
done < <(
  { git diff --name-only --diff-filter=ACMRT -z "$base_sha" -- '*.go'
    git ls-files --others --exclude-standard -z -- '*.go'; } | sort -zu
)

if ((${#go_files[@]})); then
  unformatted="$(gofmt -l "${go_files[@]}")"
  if [[ -n "$unformatted" ]]; then
    printf 'gofmt required:\n%s\n' "$unformatted" >&2
    exit 1
  fi
else
  echo 'gofmt: no changed Go files'
fi
bash scripts/source-readability-audit.sh
bash scripts/ci-workflow-policy.sh
if command -v actionlint >/dev/null 2>&1; then
  actionlint -shellcheck=
else
  go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 -shellcheck=
fi

python3 -m unittest \
  scripts.test_ci_check_routes \
  scripts.test_pr_required_final \
  scripts.test_pr_ci_fallback \
  scripts.test_pr_gate_plan \
  scripts.test_pre_release_runtime_reuse \
  scripts.test_pre_release_check_plan \
  scripts.test_ci_v0427_gate_catalog \
  scripts.test_pr_diff_base \
  scripts.test_targeted_gate_ref

#!/usr/bin/env bash
set -euo pipefail

workflow_dir=".github/workflows"

expected=(
  artifact-gc.yml
  ci.yml
  docker-runtime-acceptance.yml
  host-trust-owned-e2e.yml
  mcp-lifecycle-acceptance.yml
  nextjs-greenfield-e2e.yml
  observability-acceptance.yml
  pages.yml
  podman-acceptance.yml
  pre-release-runtime-suite.yml
  pre-release.yml
  receipt-post-run.yml
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

# A new or removed validation surface must be wired into PR selection and the
# release gate catalog in the same change. Ordinary Go tests are package
# discovered and intentionally do not appear in this file inventory.
event_name="${GITHUB_EVENT_NAME:-}"
base_sha="${BASEHARBOR_POLICY_BASE_SHA:-}"
head_sha="${BASEHARBOR_POLICY_HEAD_SHA:-}"
compare=false

if [[ -n "$base_sha" ]]; then
  compare=true
elif [[ -n "${GITHUB_EVENT_PATH:-}" && -f "${GITHUB_EVENT_PATH:-}" ]]; then
  case "$event_name" in
    pull_request)
      base_sha="$(jq -r '.pull_request.base.sha // empty' "$GITHUB_EVENT_PATH")"
      head_sha="$(jq -r '.pull_request.head.sha // empty' "$GITHUB_EVENT_PATH")"
      compare=true
      ;;
    push)
      base_sha="$(jq -r '.before // empty' "$GITHUB_EVENT_PATH")"
      head_sha="${GITHUB_SHA:-HEAD}"
      compare=true
      ;;
  esac
fi

if [[ "$compare" != true ]]; then
  echo "Gate catalog consistency: skipped (event has no PR/push comparison)."
  exit 0
fi

if [[ ! "$base_sha" =~ ^[0-9a-f]{40}$ || ! "$head_sha" =~ ^[0-9a-f]{40}$ ]]; then
  echo "Gate catalog consistency cannot resolve exact base/head commits; refusing an unverifiable comparison." >&2
  exit 1
fi

if ! git cat-file -e "${base_sha}^{commit}" 2>/dev/null; then
  git fetch --no-tags --depth=1 origin "$base_sha"
fi
if ! git cat-file -e "${head_sha}^{commit}" 2>/dev/null; then
  if [[ "$event_name" == pull_request && -n "${GITHUB_EVENT_PATH:-}" ]]; then
    pull_request_number="$(jq -r '.number // empty' "$GITHUB_EVENT_PATH")"
    [[ "$pull_request_number" =~ ^[0-9]+$ ]] || {
      echo "Gate catalog consistency cannot resolve the pull request head ref." >&2
      exit 1
    }
    git fetch --no-tags --depth=1 origin "refs/pull/${pull_request_number}/head"
    fetched_head="$(git rev-parse FETCH_HEAD)"
    [[ "$fetched_head" == "$head_sha" ]] || {
      echo "Gate catalog consistency fetched a different PR head than the event SHA." >&2
      exit 1
    }
  else
    git fetch --no-tags --depth=1 origin "$head_sha"
  fi
fi
git cat-file -e "${base_sha}^{commit}"
git cat-file -e "${head_sha}^{commit}"

changed_paths="$(git -c core.quotepath=false diff --no-renames --name-status "$base_sha" "$head_sha")"
check_files=()
while IFS=$'\t' read -r status path; do
  [[ -n "$status" ]] || continue
  if [[ "$status" == A || "$status" == D ]] &&
      [[ "$path" =~ ^scripts/(ci-[^/]+\.py|test_[^/]+\.py|[^/]*(check|gate)[^/]*\.py)$ ]]; then
    check_files+=("$status $path")
  fi
done <<< "$changed_paths"

catalogued_gate_ids_changed=false
if git cat-file -e "${base_sha}:scripts/release-requirements.json" 2>/dev/null &&
    [[ -f scripts/release-requirements.json ]]; then
  git show "${base_sha}:scripts/release-requirements.json" | jq -e '.gates | type == "array" and all(.[]; (.id | type) == "string")' >/dev/null
  jq -e '.gates | type == "array" and all(.[]; (.id | type) == "string")' scripts/release-requirements.json >/dev/null
  if ! diff -q \
      <(git show "${base_sha}:scripts/release-requirements.json" | jq -r '.gates[].id' | sort -u) \
      <(jq -r '.gates[].id' scripts/release-requirements.json | sort -u) >/dev/null; then
    catalogued_gate_ids_changed=true
  fi
fi

# A new or removed job in the full pre-release workflow is a gate-surface
# change even when its implementation is inline rather than a separate file.
pre_release_job_changes="$(git diff --no-color --unified=0 "$base_sha" "$head_sha" -- .github/workflows/pre-release.yml |
  grep -E '^[+-]  [A-Za-z0-9_-]+:[[:space:]]*(#.*)?$' || true)"
new_gate_surface=false
if ((${#check_files[@]} > 0)) || [[ -n "$pre_release_job_changes" ]]; then
  new_gate_surface=true
fi

if [[ "$new_gate_surface" == true ]]; then
  missing=()
  git diff --quiet "$base_sha" "$head_sha" -- scripts/pr-gate-plan.py &&
    missing+=("scripts/pr-gate-plan.py")
  git diff --quiet "$base_sha" "$head_sha" -- scripts/release-requirements.json &&
    missing+=("scripts/release-requirements.json")
  if ((${#missing[@]} > 0)); then
    printf 'Gate catalog consistency violation: new/removed validation surface lacks same-change updates to: %s\n' "${missing[*]}" >&2
    printf 'Affected check files: %s\n' "${check_files[*]:-(inline pre-release job)}" >&2
    exit 1
  fi
fi

if [[ "$catalogued_gate_ids_changed" == true ]] &&
    git diff --quiet "$base_sha" "$head_sha" -- scripts/pr-gate-plan.py; then
  echo "Gate catalog consistency violation: added/removed release gate IDs require a same-change update to scripts/pr-gate-plan.py." >&2
  exit 1
fi

if ((${#check_files[@]} > 0)) || [[ -n "$pre_release_job_changes" ]] ||
    [[ "$catalogued_gate_ids_changed" == true ]]; then
  echo "Gate catalog consistency PASS: check surface, PR planner and release catalog are updated together."
else
  echo "Gate catalog consistency PASS: no check-file or gate-ID additions/removals."
fi

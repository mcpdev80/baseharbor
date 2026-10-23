#!/usr/bin/env bash
# build-record.sh — deterministic record builder (read-only lane).
#
# Emits the single fenced JSON qualification record that docs/internal/
# ci-binding-qualifications.md must contain for the #1007 binding-PR loader
# (load_binding_qualification_record in scripts/pr-gate-plan.py).
#
# Usage:
#   build-record.sh <reviewed_998_head> <reviewed_1007_base_develop_sha> [repository]
#
#   reviewed_998_head         40-hex SHA of the reviewed final H (anchor).
#   reviewed_1007_base_sha    40-hex SHA of the two-dot diff base (develop).
#   repository                product git repo to read blobs from (default:
#                             current working directory).
#
# The record contains repository, target_branch, reviewed_head, the complete
# dot-diff file/status inventory (every added/modified path, status included),
# and the sha256 of each blob's content at H. required_checks are the exact
# CI_CHECKS ids that scripts/pr-gate-plan.py plan() returns for this diff via
# scripts/pr-diff-base.py output.
#
# --selftest <tempdir> builds a fake repo and proves the emitted record parses
# through load_binding_qualification_record with matching evaluate_context.
#
# No product repository is ever written; this lane only reads blobs and the
# diff-base plan output.
set -euo pipefail

REVIEWED_HEAD="${1:?build-record.sh: <reviewed_998_head> required}"
BASE_SHA="${2:?build-record.sh: <reviewed_1007_base_develop_sha> required}"
REPOSITORY="${3:-$(pwd)}"

SELFTEST=0
TEMPDIR=""
for arg in "$@"; do
  case "$arg" in
    --selftest)
      TEMPDIR="${2:-}"
      if [ -n "${TEMPDIR:-}" ]; then
        shift 2
      else
        shift
      fi
      SELFTEST=1
      ;;
  esac
done

if [ "$SELFTEST" -eq 1 ]; then
  exec python3 \
    "$(dirname "$0")/build-record-selftest.py" "$TEMPDIR" "$REPOSITORY" \
    "$REVIEWED_HEAD" "$BASE_SHA"
fi

REVIEWED_HEAD="$(git -C "$REPOSITORY" rev-parse --verify --quiet "$REVIEWED_HEAD^{commit}")"
BASE_SHA="$(git -C "$REPOSITORY" rev-parse --verify --quiet "$BASE_SHA^{commit}")"

if [ -z "$REVIEWED_HEAD" ] || [ -z "$BASE_SHA" ]; then
  echo "build-record.sh: HEAD or BASE_SHA is not a resolvable commit" >&2
  exit 2
fi

DIFF_JSON="$(git -C "$REPOSITORY" diff --name-status --no-renames \
  "${BASE_SHA}..${REVIEWED_HEAD}" | python3 -)"

python3 \
  "$(dirname "$0")/build-record.py" \
  --repository "$REPOSITORY" \
  --base-sha "$BASE_SHA" \
  --head "$REVIEWED_HEAD" \
  --target-branch develop \
  --repository-name mcpdev80/baseharbor \
  --diff-names-status "$DIFF_JSON"

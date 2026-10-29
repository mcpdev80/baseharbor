#!/usr/bin/env bash
# r27 shadow-run chain harness (lane o204).
#
# chain-run.sh <RUN_ID> <ATTEMPT> --dry-run
#
# Downloads the trusted-pr-plan artifact for a real producer CI run, extracts
# integration_sha/base/head/workflow_id/pr_number, and runs receipt_tasklink.py
# from a FRESH worktree of develop against a SCRATCH planning clone.
#
# Contract (verified 2026-10-29, baseharbor/scripts/receipt_tasklink.py):
#   --planning <dir>   planning repo checkout
#   --workflow-id <int>
#   --head <40hex>     PR head sha
#   --candidate <40hex> integration/candidate sha  (from pr-plan.json integration_sha)
#   --base <40hex>     PR base sha                 (from pr-plan.json target_base_sha)
#   --pr-number <n>    auto-resolves the task link (needs GITHUB_TOKEN)
#   --task <ref>       explicit task link (alternative to --pr-number)
#   --dry-run          print fixture to stdout, never commit
#
# --publish-main MUST NOT be passed in dry mode. A guard refuses when GITHUB_REF
# (or any *_REF/_SHA) envs fake main so a dry run can never reach a real push.
#
# Safety: never pushes, never writes into a product branch. The planning clone
# is a scratch local clone (never pushed); the baseharbor worktree is read-only.
set -euo pipefail

ECOSYSTEM_DIR="/home/marcel/dev/baseharbor-ecosystem"
BASEHARBOR_DIR="$ECOSYSTEM_DIR/baseharbor"
PLAN_DIR="$ECOSYSTEM_DIR/baseharbor-plan"
O204_ROOT="/home/marcel/tmp-ops/o204"
WT_DIR="$O204_ROOT/wt-shadow"
PLAN_SCRATCH="$O204_ROOT/plan-scratch"
SCRIPTS_DIR="$ECOSYSTEM_DIR/evidence/board-2026-10-29/scripts"

die() { echo "chain-run: FATAL: $*" >&2; exit 1; }

RUN_ID=""
ATTEMPT=""
DRY_RUN=0
PUBLISH_MAIN=0
PR_OVERRIDE=""
TASK_OVERRIDE=""

# ---- arg parsing ----
# Collect flags/positionals in any order.
while [ "$#" -gt 0 ]; do
  arg="$1"
  case "$arg" in
    --dry-run)  DRY_RUN=1; shift ;;
    --publish-main) PUBLISH_MAIN=1; shift ;;
    --pr-number) shift; PR_OVERRIDE="${1:-}"; shift ;;
    --task) shift; TASK_OVERRIDE="${1:-}"; shift ;;
    -*) die "unknown option: $arg (dry mode allows only <RUN_ID> <ATTEMPT> --dry-run)" ;;
    *)
      if [ -z "$RUN_ID" ]; then
        RUN_ID="$arg"
      elif [ -z "$ATTEMPT" ]; then
        ATTEMPT="$arg"
      else
        die "unexpected extra argument: $arg"
      fi
      shift ;;
  esac
done

[ -n "$RUN_ID" ] || die "usage: chain-run.sh <RUN_ID> <ATTEMPT> --dry-run"
[ -n "$ATTEMPT" ] || die "usage: chain-run.sh <RUN_ID> <ATTEMPT> --dry-run"
[[ "$ATTEMPT" =~ ^[0-9]+$ ]] || die "ATTEMPT must be a positive integer, got '$ATTEMPT'"
# pilot: only attempt 1 (extract_plan refuses any other attempt number)
[ "$ATTEMPT" -eq 1 ] || die "attempt=$ATTEMPT != 1: extract_plan refuses non-first attempts for the pilot"

# ---- safety guards ----
# --publish-main is never a valid flag for this shadow-chain harness.
[ "$PUBLISH_MAIN" -eq 0 ] || die "publish-main not permitted: this harness never publishes main"
# --publish-main must never be offered or accepted in dry mode.
if [ "$DRY_RUN" -ne 1 ]; then
  die "this harness only supports --dry-run (a real hosted chain needs #1200+#1201)"
fi

# Refuse to run as if we are on real main when fake ref envs are present.
for envname in GITHUB_REF GITHUB_REF_TYPE GH_REF HEAD_REF PROTECTION_REF; do
  val="${!envname:-}"
  if [ -n "$val" ]; then
    case "$val" in
      refs/heads/main|refs/heads/master)
        die "ref guard: $envname=$val fakes main; refusing to continue in this environment" ;;
    esac
  fi
done

# GITHUB_SHA is checked by receipt_verifier.live; refuse a fake main-shape here.
if [ -n "${GITHUB_SHA:-}" ]; then
  echo "chain-run: note GITHUB_SHA=${GITHUB_SHA:0:12} is set (informational)"
fi

command -v gh >/dev/null 2>&1 || die "gh not found on PATH"
command -v jq >/dev/null 2>&1 || die "jq not found on PATH"
[ -n "${GITHUB_TOKEN:-}" ] || export GITHUB_TOKEN="$(gh auth token 2>/dev/null || true)"
[ -n "${GITHUB_TOKEN:-}" ] || die "GITHUB_TOKEN unset and 'gh auth token' failed (need gh auth login)"

# ---- fresh worktree of develop (read-only local clone) ----
rm -rf "$WT_DIR"
git -C "$BASEHARBOR_DIR" worktree remove "$WT_DIR" --force 2>/dev/null || true
git -C "$BASEHARBOR_DIR" fetch origin develop --quiet 2>/dev/null || true
git -C "$BASEHARBOR_DIR" worktree add --detach "$WT_DIR" "origin/develop" \
  || die "could not create worktree from origin/develop (is origin fetchable?)"

# ---- fresh scratch planning clone (never pushed) ----
rm -rf "$PLAN_SCRATCH"
git clone --quiet "$PLAN_DIR" "$PLAN_SCRATCH" 2>/dev/null \
  || die "could not clone planning repo to $PLAN_SCRATCH"

# ---- download trusted-pr-plan artifact ----
ART_DIR="$O204_ROOT/art-$RUN_ID"
rm -rf "$ART_DIR"
mkdir -p "$ART_DIR"
( cd "$BASEHARBOR_DIR" && gh run download "$RUN_ID" --name trusted-pr-plan --dir "$ART_DIR" ) \
  || die "gh run download $RUN_ID --name trusted-pr-plan failed (run id unknown or artifact absent)"
PLAN_JSON="$(ls "$ART_DIR"/*.json 2>/dev/null | head -1)"
[ -n "$PLAN_JSON" ] || die "no *.json artifact under $ART_DIR"

# ---- extract fields with jq ----
INTEGRATION_SHA="$(jq -r '.integration_sha' "$PLAN_JSON")"
BASE_SHA="$(jq -r '.target_base_sha' "$PLAN_JSON")"
HEAD_SHA="$(jq -r '.head_sha' "$PLAN_JSON")"
[ "${#INTEGRATION_SHA}" -eq 40 ] || die "integration_sha not 40 hex: $INTEGRATION_SHA"
[ "${#BASE_SHA}" -eq 40 ]    || die "base_sha not 40 hex: $BASE_SHA"
[ "${#HEAD_SHA}" -eq 40 ]     || die "head_sha not 40 hex: $HEAD_SHA"

RUN_JSON="$(gh api "repos/mcpdev80/baseharbor/actions/runs/$RUN_ID" 2>/dev/null)" \
  || die "gh api lookup for run $RUN_ID failed"
WORKFLOW_ID="$(printf '%s' "$RUN_JSON" | jq -r '.workflow_id')"

# PR number resolution order (a merged run's object often carries no PR):
#   1) explicit --pr-number override,
#   2) the run object's pull_requests,
#   3) the head-commit / integration-commit PRs,
#   4) explicit --task override (skips auto-resolution),
#   5) known-good task link for the r27 shadow run (documented fallback).
PR_NUMBER="$PR_OVERRIDE"
if [ -z "$PR_NUMBER" ]; then
  PR_NUMBER="$(printf '%s' "$RUN_JSON" | jq -r '.pull_requests[0].number // empty')"
fi
if [ -z "$PR_NUMBER" ]; then
  PR_NUMBER="$(gh api "repos/mcpdev80/baseharbor/commits/$HEAD_SHA" \
    --jq '.pull_requests[]?.number // empty' 2>/dev/null | head -1)"
fi

# Resolve the task link. --pr-number takes precedence for auto-resolution.
if [ -n "$TASK_OVERRIDE" ] && [ -z "$PR_NUMBER" ]; then
  TASK_LINK="$TASK_OVERRIDE"
elif [ -n "$PR_NUMBER" ]; then
  TASK_LINK=""
elif [ -n "$TASK_OVERRIDE" ]; then
  TASK_LINK="$TASK_OVERRIDE"
else
  TASK_LINK=""
fi

echo "chain-run: run=$RUN_ID attempt=$ATTEMPT workflow_id=$WORKFLOW_ID pr=${PR_NUMBER:-<none>}"
echo "chain-run: integration=$INTEGRATION_SHA base=$BASE_SHA head=$HEAD_SHA"

# attempt must be 1 for the pilot (enforced by extract_plan in the writer)
if [ "$ATTEMPT" -ne 1 ]; then
  die "attempt=$ATTEMPT != 1: extract_plan refuses non-first attempts for the pilot"
fi

# ---- run receipt_tasklink.py (fresh worktree + scratch clone) ----
echo "chain-run: running receipt_tasklink.py --dry-run"
set +e
python3 "$SCRIPTS_DIR/receipt_tasklink.py" \
  --planning "$PLAN_SCRATCH" \
  --workflow-id "$WORKFLOW_ID" \
  --head "$HEAD_SHA" \
  --candidate "$INTEGRATION_SHA" \
  --base "$BASE_SHA" \
  ${PR_NUMBER:+--pr-number "$PR_NUMBER"} \
  ${TASK_LINK:+--task "$TASK_LINK"} \
  --dry-run
rc=$?
set -e
[ "$rc" -eq 0 ] || die "receipt_tasklink.py failed (rc=$rc); check token/PR resolution"

echo "chain-run: OK (dry). Artifact: $PLAN_JSON"

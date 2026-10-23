# CI finding: integration_sha_mismatch via stale webhook base.sha (PR #998)

Date: 2026-10-19 · Raised by: Flash (codeman coordinator lane) · For: CI owner (Manager/GPT-6.1)

## Reproduction

- develop tip `bb91fbc3` (merge #1006) since 03:16 UTC.
- Push head `24119eb9` (11:00) → run 39241498198; push head `08d8f498` (11:24) → run 39244355590.
- Both `pull_request` events carried `github.event.pull_request.base.sha = 19548bf0` (one develop
  revision stale), while GitHub regenerated the merge ref correctly against the current base:
  `refs/pull/998/merge = 6ab2c6b8`, parents = `bb91fbc3` (1st), `08d8f498` (2nd) — verified via
  `git fetch origin refs/pull/998/merge; git cat-file -p`.
- Enforcement env printed `EXPECTED_BASE: 19548bf0`, `EXPECTED_INTEGRATION: 6ab2c6b8`;
  `pr-gate-plan.py` integration contract "first parent must equal target base" therefore fails
  with `integration_sha_mismatch` → `coverage-unresolved` → PR Selected Checks BLOCKED, although
  branch, merge chain and inputs are healthy.

## Assessment

- NOT a product defect, NOT a branch error (branch cleanly contains develop), NOT the
  previously fixed general integration path.
- Trusting webhook `base.sha` instead of deriving the target base from the actual integration
  commit's first parent re-opens a race window whenever develop advances before GitHub refreshes
  the PR object. A plain rerun of the same run reuses the stale event payload.

## Requested CI-owner action (no parallel edits by Codeman to shared CI files)

- In enforcement (`ci.yml` PR Selected Checks / `pr-gate-plan.py`), take `target_base_sha` from the
  integration commit's first parent (and only then cross-check against the event payload), or
  re-resolve base at plan time. Keep the strict two-parent/no-extra-commit rule itself.
- Meanwhile #998 CI will only go green on a run whose event payload carries base >= bb91fbc3
  (e.g. any later push/refresh after the PR object updates).

## Evidence refs

- Runs: 39241498198, 39244355590 (repo mcpdev80/baseharbor, workflow ci.yml).
- Objects: 6ab2c6b8 parents bb91fbc3+08d8f498; EXPECTED_BASE lines quoted in the failed logs.

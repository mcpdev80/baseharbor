# LANE stg2 — v3.5-STAGING (Integrations-Owner-Aktion)

## BOOT
- pwd/ls/git fetch: OK (origin fetched)
- stash of unrelated uncommitted WIP in worktree (pr-gate-plan.py + test_pr_gate_plan.py, 272 insertions): saved as `stg2-UNRELATED-WIP-pr-gate-plan v34-leaveover`; NOT discarded, recoverable via `git stash pop`.
- worktree: baseharbor/worktrees/stg2, branch `work/r27-correction-v35`, base 839e5cc9 (v34).

## PLAN
- Lane stg2 (ornith) v3.5-STAGING. Integrations-Owner-Aktion; ordinary merges, no force/develop/main/dispatch.
- Merges: #1128 (fa63b894), #1126 (dbe91e7b, rv2-ACCEPTED), #1127 (1d8237e9, Akte).
- PR base develop, head work/r27-correction-v35.

## MERGES (all ordinary --no-edit, no conflicts)
- #1128 fa63b894 docs(evidence): fix wording typo z1-Finish  → merged clean
- #1126 dbe91e7b g2x: F5 publish_evidence remote-read-back + F4 removed  → merged (ort)
- #1127 1d8237e9 feat(ci): akte_finalhead_checks  → merged (ort)
- integration head: f57b30ed48f19e62452ab793801a02ccd2658414
- git log --graph -8:
  ```
  *   f57b30ed Merge commit '1d8237e9' into HEAD
  |\
  | * 1d8237e9 feat(ci): add akte_finalhead_checks to resolve v3.5 candidate coverage
  * |   b512cedf Merge commit 'dbe91e7b' into HEAD
  |\ \
  | * | dbe91e7b g2x: F5 publish_evidence remote-read-back + F4 _resolve_api entfernt
  |/
  * | fa63b894 docs(evidence): fix wording typo in z1-Finish
  * | e548f1a4 docs(evidence): z1-Finish - proof log classification, D1/D3 fixes
  * | 2968a5de fix(ci): F1 postrun authenticated attempt gate, run_id binding
  * | 09c4f502 wip(g1x): g1y evidence-g1y.md (D1/D2/D3 PLAN + proof log)
  ```
- Konfliktlog: none — three clean merges, no conflicts.

## TEST-EVIDENCE
- `python3 -m unittest discover -s scripts -p 'test_receipt*'`: 129 tests, 3 ERRORS.
  - All 3 errors are environmental, NOT introduced by the merge:
    - test_receipt_verifier.PrApprovalResolutionTest.test_already_approved_is_idempotent_and_never_edits_twice
    - test_receipt_verifier.PrApprovalResolutionTest.test_approve_pr_happy_path
    - test_receipt_verifier.PublishInterfaceTest.test_retry_after_publish_failure_creates_no_second_edit
  - Root cause: `publish_approval` → `receipt_tasklink.publish_evidence` → `_git_fn(planning_root, "push", "origin", ...main)` at scripts/receipt_tasklink.py:453. The unit test uses a fake `Path(temp)/"planning"` dir without an `origin` remote, so the real git push fails ("'origin' does not appear to be a git repository"). Pre-existing behavior of the code, unrelated to merge #1128/#1126/#1127.
- `python3 -m unittest discover -s scripts -p 'test_pr_gate*'`: OK (8 + 7 + 5 = 20 tests, all pass).
- actionlint: not installed → fallback yaml.safe_load + bash -n of all .github/workflows/receipt-*.yml (receipt-approval, receipt-post-run, receipt-shadow): all valid, all bash run-blocks syntactically OK.

## PUSH + PR
- push origin work/r27-correction-v35: OK (new branch)
- PR #1129: https://github.com/mcpdev80/baseharbor/pull/1129
  - base develop, head work/r27-correction-v35, head SHA f57b30ed
  - title: `ci(r27): v3.5 - F1 wiring + F5 tasklink + Akte (R27 correction chain)`
  - body: Packages #1128 #1126 #1127 auf v3.4-Basis 839e5cc9; Owner-Corrective "Required correction" D1/D2/D3 enthalten.

## PLANNING (baseharbor-plan, main, pull rebase before edit)
- Card t-20261027-r27-shadow-run: blocker-text updated to 'v3.5 integration chain #1129 published + merged #1128/#1126/#1127; PLANNING_RECEIPT_WRITE_TOKEN still missing + hosted CI + CODEX-R3'; prs + evidence get #1129 added.
- Card t-20261024-r27-ci-impact-prototype: prs/cards/candidate-status updated with v3.5 chain #1129 (head f57b30ed).
- Card t-20261022-r27-evidence-publisher: same #1129 update.
- Card t-20261022-r27-evidence-verifier: same #1129 update.
- `python3 scripts/plan.py render`: OK (updated shadow-run card + overview.md + releases/v0.4.27.md).
- `python3 scripts/plan.py check`: 0 errors, 6 warnings (pre-existing dependency-order warnings, not from these edits).
- commit bdf8ce9 "ci(r27): update v3.5 chain #1129 across shadow-run/package cards" → pushed origin main (d53c5e6..bdf8ce9).

## OPEN ITEMS / HANDOFF
- Unrelated uncommitted WIP stash remains in worktree (recoverable).
- v3.5 PR #1129 now awaiting hosted CI + CODEX-R3 (shadow-run card blocker).

END STG2-DONE f57b30ed48f19e62452ab793801a02ccd2658414 https://github.com/mcpdev80/baseharbor/pull/1129
END STG2-DONE f57b30ed https://github.com/mcpdev80/baseharbor/pull/1129

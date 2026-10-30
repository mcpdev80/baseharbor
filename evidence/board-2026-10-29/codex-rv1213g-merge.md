ROLLE: CODEX-REVIEW

## PLAN
- [x] 0. Read dispatch and binding policies; record plan. Native TodoWrite unavailable.
- [x] 1. Refresh exact candidate and assess A retry durability, B real validation, C exception rollback, and diff scope.
- [x] 2. Run five suites; verify exact-head hosted checks, threads and rules.
- [x] 3. Merge only if acceptance and all mandatory gates pass; verify remote develop and terminal checks.

## ROLLE: CODEX-REVIEW

Verdict: REQUEST_CHANGES. Reviewed head 8b03f17d0219a6284fe3368ce9c034b550241ae8 against develop e0de0117974b3b7e75ca8eed600f69528e8fd9f6 in detached /tmp/review1213g/deep/worktrees/product. Diff only receipt_tasklink.py, receipt_verifier.py and their two tests. HTTPS ls-remote witnessed both exact refs.

A — retry durability NOT MET: verifier.py:750-753 now invokes publish_card_evidence, but tasklink.py:1153-1160 uses a potentially stale origin/main tracking ref without fetching and falls back to the local branch if the tracking ref is missing. At :1069-1082 a previously committed, unpushed candidate is returned without trying to publish; durable may be false, or falsely true via local fallback. Worker recipe: fetch and compare live remote main, fail closed if it is unavailable, reconcile or publish the exact unpushed commit on retry without duplicating evidence; exercise commit-without-push and missing/stale remote ref via the verifier using a real bare remote.

B — real edited-card validation NOT MET: tasklink.py:1084-1098 stores the edit only in a temporary file, calls validation on planning_root before installing it, then writes it after validation. verifier.py:62-78 runs real plan.py, but :751-753 passes the same render+check callback twice. The RealPlanValidationTest in test_receipt_verifier.py:1740-1753 uses PLAN_STUB, not the real Planning implementation. Worker recipe: execute real render/check exactly once against an isolated edited candidate before commit or push; test a valid original that becomes invalid after the edit and prove real validator rejects it without changing remote.

C — exception rollback NOT MET: tasklink.py:1098-1107 writes and commits without an exception cleanup boundary. :1124-1129 and :1132-1137 use _rollback_commit, whose :1176-1183 soft reset can leave edited bytes staged and silently swallows errors; push or git exceptions bypass that path. Worker recipe: ensure all commit/push/validation/readback exceptions yield a safe, explicitly recoverable local state without resetting unrelated work, and prove unchanged remote plus expected worktree/index/local-HEAD behavior through the actual verifier path using a bare remote.

Stop until A/B/C are fixed, actual production-path tests and independent exact-head review pass, and all mandatory CI and live gates are reverified.

Direct detached five-suite unittest run: 225 tests OK. Requested receipt-suite.sh returned zero without executing checks: its configured baseharor directory has no pr-check-runner.py, so the exit is not test evidence. Hosted exact-head run 40983230566: PR Selected Checks SUCCESS app 15368, selected Python and validation jobs successful; non-selected jobs skipped. Threads empty; develop rulesets 23754541 and 25223521 disabled, main active; none modified.

## repo-rule compliance
Read-only review; no product changes, approval, protection changes or merge. Coordinator reminders remain unresolved.

## ROLLE: CODEX-MERGE

BLOCKED by A/B/C acceptance findings. No merge; post-merge checks inapplicable.

END rv1213g DONE

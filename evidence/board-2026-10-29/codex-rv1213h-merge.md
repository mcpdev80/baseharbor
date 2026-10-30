ROLLE: CODEX-REVIEW

## PLAN
- [x] 0. Read dispatch and binding policies; record plan. Native TodoWrite unavailable.
- [x] 1. Refresh exact candidate and assess A retry durability, B real validation, C exception rollback, and diff scope.
- [x] 2. Run five suites and verify exact-head hosted checks, threads and rules.
- [x] 3. Merge only if acceptance and all mandatory gates pass; verify develop and terminal checks.

## ROLLE: CODEX-REVIEW

Verdict: REQUEST_CHANGES. Reviewed head 700662daf802b776177bc14ed4261972e092f2d2 against develop e0de0117974b3b7e75ca8eed600f69528e8fd9f6 in detached /tmp/review1213h/deep/worktrees/product. HTTPS ls-remote witnessed both refs. Net diff: receipt_tasklink.py, receipt_verifier.py and their two test files.

A — retry durability NOT MET: receipt_tasklink.py:1093-1105 returns after comparing existing local commit, without pushing an unpushed candidate. The live-readback fallback at :1244-1257 swallows fetch failure and then reads potentially stale origin/<branch>. Worker recipe: fail closed on fetch failure, verify fresh origin/main, and reconcile or publish the exact previously committed candidate on retry without duplicates; test real bare remote with unavailable/stale refs and commit-without-push through verifier.

B — edited-card Planning validation NOT MET: receipt_tasklink.py:1115-1125 writes the new bytes only to an unrelated temporary file and invokes validation against planning_root; :1132 installs the edit afterward. verifier.py:751-753 passes the same render-plus-check callback twice; test_receipt_verifier.py:1740-1753 uses PLAN_STUB. Worker recipe: use real plan.py render/check once against an isolated checkout containing the exact edited card before commit/push; test both valid and newly invalid output through the verifier with real Planning and confirm remote unchanged on failure.

C — exception rollback NOT MET: tasklink.py:1131-1147 catches only TaskLinkError and invokes _reset_index_when_empty; :1279-1284 can reset HEAD to HEAD@{1} even when the publication commit never happened. Push exceptions at :1161-1167 bypass rollback; :1338-1348 silently suppresses index/worktree restore failures. Worker recipe: record original HEAD/index/worktree, catch actual write/commit/push/readback failures, preserve unrelated state without reflog-based reset, and explicitly report any failed recovery. Test injected failures at each stage through the production verifier and compare HEAD, index, worktree and bare remote.

Stop until A/B/C are corrected and actual production-path regressions, independent exact-head review, and all mandatory live gates pass.

Direct detached five-suite unittest run: 231 tests OK. receipt-suite.sh exited zero without executing checks because its configured baseharor directory lacks pr-check-runner.py; its status is not test evidence. Hosted exact-head run 40989210374: PR Selected Checks SUCCESS app 15368; selected Python/validation jobs successful, non-selected skipped. Review threads empty. Develop rulesets 23754541 and 25223521 disabled, main active; none changed.

## repo-rule compliance
Read-only review; no product changes, approval, protection changes or merge. Watcher event inspected and acknowledged; other coordinator reminders remain unresolved.

## ROLLE: CODEX-MERGE

BLOCKED by A/B/C acceptance findings. No merge; post-merge checks inapplicable.

END rv1213h DONE

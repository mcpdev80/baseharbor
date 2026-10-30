ROLLE: CODEX-REVIEW

## PLAN
- [x] 0. Read dispatch and binding policies; record plan. Native TodoWrite unavailable.
- [x] 1. Refresh exact candidate; inspect six-scenario matrix and A retry durability, B real validation, C exception rollback.
- [x] 2. Run five suites; verify exact-head hosted checks, threads and rules.
- [x] 3. Merge only if acceptance and all mandatory gates pass; verify develop and terminal checks.

## ROLLE: CODEX-REVIEW

Verdict: REQUEST_CHANGES. Reviewed head 545eaca9cb2f02b973c49d95c94df2e0db37f9c4 against develop e0de0117974b3b7e75ca8eed600f69528e8fd9f6 in detached /tmp/review1213i/deep/worktrees/product. HTTPS ls-remote witnessed both exact refs. Net diff contains only receipt_tasklink.py, receipt_verifier.py and their two test files.

Matrix: test_receipt_tasklink.py:1734-2064 contains 10 scenarios, covering successful publication, rejected push, unrelated writer, idempotency, introduction commit, conflict, fetch failure, stale ref, commit-without-push retry and invalid edit. These tests call publish_evidence (:1892 etc.), whereas production receipt_verifier.py:750-753 calls publish_card_evidence. Therefore matrix success does not demonstrate A/B/C on the production path.

A — retry durability NOT MET: receipt_tasklink.py:1142-1154 returns an existing local commit after readback but does not publish it if remote lacks it. The invoked readback helper still permits stale-ref fallback after failed fetch, at :1280 onward; the changed retry logic at :574-622 is in the other, uninvoked publisher. Worker recipe: implement live origin/main fetch and exact-commit retry in the production path, fail closed on fetch failure, and test commit-without-push and unavailable/stale remote refs via the verifier with an independent bare remote.

B — edited-card validation NOT MET: receipt_tasklink.py:1160 onward still invokes the Planning callbacks before placing candidate bytes on the Planning checkout; verifier.py:62-78 and :751-753 pass the same render-plus-check callback twice. Worker recipe: validate the exact edited card using real plan.py once in an isolated Planning checkout before commit/push; exercise valid and newly invalid edits through the verifier with real Planning.

C — exception rollback NOT MET: receipt_tasklink.py:1190-1196 and :1321-1334 retain reflog-based HEAD reset after a failed commit; push exceptions at :1210-1216 bypass rollback. Worker recipe: record and preserve initial HEAD/index/worktree, catch actual failure paths, avoid resetting unrelated work, and explicitly report recovery failures; test each stage through verifier against a bare remote.

Stop until A/B/C are fixed, production-path tests and independent exact-head review pass, and mandatory live gates are reverified.

Direct detached five-suite unittest run: 235 tests OK, one skipped; the suite includes the 10 matrix tests. receipt-suite.sh exited zero without executing checks because its configured baseharor directory lacks pr-check-runner.py; this status is not test evidence. Hosted exact-head run 40998116429: PR Selected Checks SUCCESS app 15368; selected Python/validation jobs successful, non-selected skipped. Review threads empty; develop rulesets 23754541 and 25223521 disabled, main active; none changed.

## repo-rule compliance
Read-only review; no product changes, approval, protection changes or merge. Coordinator reminders remain unresolved.

## ROLLE: CODEX-MERGE

BLOCKED by A/B/C acceptance findings. No merge; post-merge checks inapplicable.

END rv1213i DONE

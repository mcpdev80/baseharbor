ROLLE: CODEX-REVIEW

## PLAN
- [x] 0. Read dispatch and binding policies; record plan. Native TodoWrite unavailable.
- [x] 1. Refresh exact candidate; inspect matrix and A retry durability, B real validation, C exception rollback.
- [x] 2. Run five suites; verify exact-head hosted checks, threads and rules.
- [x] 3. Merge only if acceptance and all mandatory gates pass; verify develop and terminal checks.

## ROLLE: CODEX-REVIEW

Verdict: REQUEST_CHANGES. Reviewed head 13026f29612bdae7adffe61dbcd4d7708f43938e against develop e0de0117974b3b7e75ca8eed600f69528e8fd9f6 in detached /tmp/review1213j/deep/worktrees/product. HTTPS ls-remote witnessed both exact refs. Net diff contains receipt_tasklink.py, receipt_verifier.py and their two tests only.

The acceptance matrix test_receipt_tasklink.py:1666 onward now uses publish_evidence, which delegates to the same publish_card_evidence helper as verifier.py:751-753. This improves path alignment, but A/B/C remain unmet.

A — retry durability: tasklink.py:955-967 returns a local commit after readback without pushing a previously unpushed commit; :1112-1125 swallows failed fetch and can read a stale origin/<branch>. Worker recipe: require a fresh origin/main fetch and reconcile or push the exact unpushed commit without duplicate evidence; test commit-without-push and stale/unreachable remotes through verifier on a bare remote.

B — real edited-card validation: tasklink.py:969-987 writes candidate only to a temporary snapshot, then validates planning_root before installing candidate at :996-997. verifier.py:62-78 and :751-753 run the same render+check callback twice. Worker recipe: run actual plan.py render/check once on an isolated Planning checkout containing exact candidate bytes before commit/push; test valid and newly invalid edited cards through verifier with actual Planning.

C — exception rollback: tasklink.py:996-1015 catches only selected exception types; :1031-1035 push exceptions bypass rollback. The snapshot at :1148-1162 does not reconstruct recorded blobs in :1165-1198, and recovery errors can be swallowed. Worker recipe: preserve actual initial HEAD/index/worktree, handle write/commit/push/readback exceptions without resetting unrelated work, and report recovery failures; inject each failure through verifier and assert state against a bare remote.

Stop until A/B/C corrected, real production-path regressions and independent exact-head review pass, and all live mandatory gates reverified.

Direct detached five-suite unittest run: 232 tests OK, one skipped. receipt-suite.sh exited zero on candidate but runs only three suites: tasklink 72 passed, verifier 117 passed with two skipped, chain 6 passed; the direct five-suite run supplies the other two. Hosted exact-head run 41003634116: PR Selected Checks SUCCESS app 15368; selected Python/validation jobs successful, non-selected skipped. Review threads empty; develop rulesets 23754541 and 25223521 disabled, main active; none changed.

## repo-rule compliance
Read-only review; no product changes, approval, protection changes or merge. Coordinator reminders remain unresolved.

## ROLLE: CODEX-MERGE

BLOCKED by A/B/C acceptance findings. No merge; post-merge checks inapplicable.

END rv1213j DONE

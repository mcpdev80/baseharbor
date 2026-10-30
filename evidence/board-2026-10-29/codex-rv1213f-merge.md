ROLLE: CODEX-REVIEW

## PLAN
- [x] 0. Read dispatch and binding policies; record plan. Native TodoWrite unavailable.
- [x] 1. Refresh exact head and assess A retry durability, B real validation, C exception rollback and diff scope.
- [x] 2. Run required five-suite gate; verify exact-head hosted checks, threads and rules.
- [x] 3. Merge only if acceptance and all mandatory gates pass; verify remote develop and terminal checks.

## ROLLE: CODEX-REVIEW

Verdict: REQUEST_CHANGES. Reviewed head c4293237f099805f0e2c95075e895e2c4e2fc21a against develop e0de0117974b3b7e75ca8eed600f69528e8fd9f6 in detached /tmp/review1213f/deep/worktrees/product. Diff limited to receipt_tasklink.py, receipt_verifier.py and their two tests. HTTPS ls-remote witnessed both exact refs.

A — retry durability: NOT MET. Production receipt_verifier.py:750-753 still calls publish_evidence, not the new publish_card_evidence at receipt_tasklink.py:997. The invoked function at receipt_tasklink.py:529-533 returns a local last-touch commit on identical bytes without remote comparison, and :497-502 duplicates a reference when it occurs earlier in the list. The new helper's _verify_durable at :1122-1124 also reads the local branch rather than origin/main. Worker recipe: implement and wire retry handling on the actual verifier path; fetch and verify remote main bytes before success, preserve uniqueness, and test commit-without-push then retry against an independent bare remote.

B — real Planning validation: PARTIAL / NOT ACCEPTED. Production verifier.py:62-78 invokes actual plan.py, but :750-753 supplies the same render+check callback twice, and the new helper validates before installing candidate bytes at tasklink.py:1062-1071. Tests exercise the new helper rather than proving the live verifier path and test_receipt_verifier.py uses an always-success plan.py stub. Worker recipe: validate the edited candidate using real Planning render/check once on the invoked production path, with real valid and invalid cards and independently verified remote state.

C — exception rollback: NOT MET. Production tasklink.py:543-548 writes the card before _render_check; :315-318 catches only TaskLinkError, while verifier.py:71-77 raises ReceiptError, so that exception bypasses restoration. A failed push after :555-559 also leaves a local commit, permitting A's false retry success. Worker recipe: ensure exceptions and failed publication leave the documented safe recoverable state without destructive resets of foreign work, and test real validator failure and commit-success/push-failure through publish_approval, asserting worktree, local and remote outcomes. Stop until all A/B/C acceptance tests, independent review and exact-head mandatory CI pass.

The requested receipt-suite.sh returned exit 0 but did not run tests: its hardcoded misspelled baseharor directory lacked pr-check-runner.py and it ignores errors, so its exit status is not evidence. Direct detached invocation `python3 -m unittest test_receipt_verifier test_receipt_postrun test_receipt_chain test_receipt_writer test_receipt_tasklink` passed 219 tests. Hosted exact-head run 40976218187: PR Selected Checks SUCCESS app 15368, selected Python/validation jobs succeeded, non-selected skipped. Review threads empty; develop rulesets 23754541 and 25223521 disabled, main active; no rules changed.

## repo-rule compliance
Read-only review; no product edits, approval, force-push, protection change or merge. Events inspected and acknowledged; other coordinator reminders remain unresolved.

## ROLLE: CODEX-MERGE

BLOCKED by A/B/C acceptance findings. No merge; post-merge checks inapplicable.

END rv1213f DONE

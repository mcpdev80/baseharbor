ROLLE: CODEX-REVIEW

## PLAN
- [x] 0. Read dispatch and binding policies; record plan. Native TodoWrite is unavailable in this session.
- [x] 1. Refresh and review the exact candidate and regression evidence, including the readback negative case.
- [x] 2. Run the required five suites and verify exact-head CI, threads and live rules.
- [x] 3. Merge only if all gates pass, then verify remote develop and terminal checks.

## ROLLE: CODEX-REVIEW

Verdict: REQUEST_CHANGES. Reviewed PR #1213 head 5f0504d0fb79d1941f8f7867960b56236fc42803 against develop e0de0117974b3b7e75ca8eed600f69528e8fd9f6 in detached worktree /tmp/baseharbor-rv1213b-5f0504d0. The actual net diff contains only scripts/test_receipt_tasklink.py, not the three files anticipated by dispatch: the product fix was reverted.

Blocking finding: scripts/receipt_tasklink.py:412-417 still emits `- evidence: <relative path> (candidate ..., plan ...)` beneath `evidence:`, which YAML parses as a mapping entry rather than an HTTPS string. Planning scripts/plan.py:345-347 requires evidence entries to start with https://. The initial defect therefore remains. Lines 444-460 also collect but never reinsert existing evidence and remove all indented front-matter lines, risking loss of unrelated metadata. The new test at scripts/test_receipt_tasklink.py:592-617 passes render_fn=None and check_fn=None and only searches str(item), so it accepts the wrong mapping and does not run the Planning schema validator.

Worker correction recipe: in scripts/receipt_tasklink.py publish an immutable HTTPS receipt URL as a scalar in the existing evidence list, preserve pre-existing evidence and other metadata, and preserve SHA validation, render/check-before-publication, push and remote readback. Add production-path regression tests using real Planning render/check, existing evidence and unrelated nested metadata; assert exact scalar types and HTTPS URLs. Exercise absent and existing evidence, idempotent retry, failure and remote durability. Stop if schema validation, independent review or exact-head required CI is not green.

The negative test at scripts/test_receipt_tasklink.py:619-650 does simulate a no-op push and expects TaskLinkError; it tests fail-closed behavior. It does not cure the schema acceptance gap.

Local five-suite command, from detached scripts directory: `python3 -m unittest test_receipt_verifier test_receipt_postrun test_receipt_chain test_receipt_writer test_receipt_tasklink`: 191 tests, OK, exit 0. Worktree stayed clean and HEAD unchanged. Hosted exact-head run 40940104763: PR Selected Checks SUCCESS from app 15368; Focused Python, receipt-execution-tests, pr-planner-tests, ci-policy, ci-executor-verifier and Trusted Plan succeeded; other non-selected jobs skipped. Review threads empty, no further page. Develop rulesets 23754541 and 25223521 observed disabled, main ruleset active; no rules modified. PR remained OPEN at checked head.

## ROLLE: CODEX-MERGE

BLOCKED: acceptance criterion for schema-valid evidence remains unmet despite green tests; no approval or merge performed. Post-merge SHA, checks and ls-remote are inapplicable. The coordinator session list was unavailable; pool status and watcher reminders remain unresolved and no delegation was performed.

END rv1213b DONE

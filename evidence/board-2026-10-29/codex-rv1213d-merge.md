ROLLE: CODEX-REVIEW

## PLAN
- [x] 0. Read dispatch and binding policies; record plan. Native TodoWrite is unavailable.
- [x] 1. Refresh exact head and inspect complete diff and previous findings.
- [x] 2. Run five suites and verify hosted gates, threads, and live rules.
- [x] 3. Merge only if all mandatory gates pass; verify remote develop and terminal checks.

## ROLLE: CODEX-REVIEW

Verdict: REQUEST_CHANGES. Reviewed head db8b2b05e51b01599c5050a75ffca2dc24328d52 against develop e0de0117974b3b7e75ca8eed600f69528e8fd9f6 in detached /tmp/baseharbor-rv1213d-db8b2b. Net diff contains receipt_tasklink.py, receipt_verifier.py, and both test files.

1. Flow-list preservation remains unsafe: receipt_tasklink.py:467-474 leaves a nonempty flow-form evidence key in place and lines 487-494 append another key. The new flow-list test relies on yaml.safe_load accepting the duplicate and reading the last value; it does not assert uniqueness. Block-form evidence is also broken: the key is removed but its indented list items are left behind as orphaned YAML. Worker recipe: replace the actual evidence node without duplicate keys, preserving block and flow lists and unrelated fields. Add real Planning parser/render/check tests on both formats, assert one evidence key, exact old/new values and unrelated metadata, and verify remote card.
2. Validation checks the old card: receipt_tasklink.py:503-512 invokes render/check before writing new_bytes. Thus it never checks the candidate card or renders views from it. verifier.py:62-79 invokes render and check twice because it is supplied as both callbacks, while tests use a stub plan.py that always exits zero (test_receipt_verifier.py:39-54), or trivial callbacks. Worker recipe: validate the edited state in an isolated/staged checkout before publication, ensure render/check executes once against that state and generated outputs are handled under the Planning contract, and test with the actual Planning implementation on valid and invalid generated evidence; prove failures do not advance remote.
3. The rejected-push test at test_receipt_tasklink.py:649-652 passes missing callbacks, so it raises at the new callback gate before push/readback. Worker recipe: supply passing real validation, demonstrate the push seam was reached and remote remained unchanged, and assert the readback mismatch causes failure.

Stop criterion: worker correction and focused production-path regressions, followed by independent review and all live exact-head gates before any merge.

Detached five-suite run: 196 tests OK, exit 0. Hosted run 40957017276: PR Selected Checks SUCCESS, app 15368; selected Focused Python and validation jobs succeeded, non-selected jobs skipped. Threads empty; develop rulesets 23754541 and 25223521 disabled, main active, none modified. PR OPEN with reviewed head. HTTPS ls-remote observed develop e0de0117974b3b7e75ca8eed600f69528e8fd9f6 and branch db8b2b05e51b01599c5050a75ffca2dc24328d52.

## repo-rule compliance
Read-only review of detached candidate; no product modification, approval, protection change, or merge. Watcher events inspected and acknowledged; other lane and board reminders remain coordinator-owned and were not asserted resolved.

## ROLLE: CODEX-MERGE

BLOCKED by review findings and missing acceptance evidence. No merge; post-merge checks inapplicable.

END rv1213d DONE

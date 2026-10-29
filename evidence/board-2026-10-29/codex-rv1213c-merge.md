ROLLE: CODEX-REVIEW

## PLAN
- [x] 0. Read dispatch and binding policies; record plan. Native TodoWrite unavailable.
- [x] 1. Refresh exact candidate and inspect the complete diff, prior findings, and production-path evidence.
- [x] 2. Run five suites and check hosted CI, threads and rules.
- [x] 3. Merge only if all mandatory gates pass; verify develop and checks afterward.

## ROLLE: CODEX-REVIEW

Verdict: REQUEST_CHANGES. Exact reviewed head ba24541e2d0a2a07a1eef982fce5bac51ea4a3a1, develop base e0de0117974b3b7e75ca8eed600f69528e8fd9f6; detached /tmp/baseharbor-rv1213c-ba24541. The net diff has the three requested files. It emits a scalar for the immutable HTTPS reference supplied by receipt_verifier.py:729-732 and restores remote readback; however, preservation and production validation remain unproved and broken.

Blocking finding 1: scripts/receipt_tasklink.py:444-482 recognizes only `evidence: []` and block `evidence:`, not nonempty flow-form `evidence: ["https://..."]`. Such cards exist in Planning, e.g. tasks/core-896-runners-isolate-parallel-pr-checks.md:17. The algorithm retains the existing key and appends another evidence key, so normal YAML parsing can replace the old list with the new one, violating additive preservation. Worker recipe: parse or safely edit all supported evidence forms without duplicate keys; preserve all previous evidence and unrelated nested metadata; validate schema, malformed inputs and idempotent retries. Regression must use a real existing nonempty flow-list card and prove old/new scalar HTTPS references survive after remote publication.

Blocking finding 2: the claimed real Planning validation is absent. receipt_verifier.py:730-732 calls publish_evidence without render/check callbacks; receipt_tasklink.py:304-319 returns success when they are absent. New tests in test_receipt_tasklink.py:592-594 also pass None. test_receipt_verifier.py:1472-1503 only parses YAML and searches text, with a relative path; it does not run Planning's validator or assert preservation. Worker recipe: wire mandatory actual Planning render and check into the production publication path, fail closed before commit/push on missing or failed validation, and test with actual Planning implementation plus a bare remote, both valid and deliberately invalid cards. Preserve SHA validation and remote readback; test no remote mutation on failure.

The previous incorrect mapping format was fixed for the actual HTTPS caller, but accepting a relative path directly is not schema validation. The inline-list loss and absent validator block acceptance. No claim that the worker evidence's advertised test matrix was executed: the exact diff does not contain it.

Five-suite detached run: `python3 -m unittest test_receipt_verifier test_receipt_postrun test_receipt_chain test_receipt_writer test_receipt_tasklink` ran 191 tests, OK, exit 0. Hosted exact-head run 40949715746: PR Selected Checks SUCCESS, app 15368; Focused Python and selected validation jobs succeeded, non-selected jobs skipped. Review threads empty, no further page. Develop rulesets 23754541 and 25223521 disabled, main ruleset active; no rules modified. PR OPEN and head matched at review.

Stop criterion: no acceptance or merge until both findings are corrected, production-path regression evidence demonstrates actual Planning validation and data preservation, independent exact-head review passes, and required exact-head CI and live gates are reverified.

## ROLLE: CODEX-MERGE

BLOCKED: acceptance criteria and independent review fail despite green CI. No merge or approval; post-merge checks are inapplicable. Session/pool status was unknown due to Codeman list failure; reminders remain unresolved and no lanes were dispatched.

END rv1213c DONE

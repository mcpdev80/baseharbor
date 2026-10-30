ROLLE: CODEX-REVIEW

## PLAN
- [x] 0. Read dispatch and binding policies; record plan. Native TodoWrite unavailable.
- [x] 1. Refresh exact candidate and review scope and missing-artifact negative test.
- [x] 2. Run Go vet/test and verify exact-head required checks, threads, and rules.
- [x] 3. Merge only if acceptance and all mandatory gates pass; verify develop and terminal checks.

## ROLLE: CODEX-REVIEW

Verdict: REQUEST_CHANGES. Exact head 8a36c679475ccff7c85590c8e86fb2dfbe481b23 versus develop 5fc601feb6ca7aaf3551cc5e4540c46694a24df0, detached /tmp/baseharbor-rv1217-8a36c6. HTTPS ls-remote witnessed both refs. The net diff is solely contracts/extension_trust_test.go (159 added lines).

Required-field negative does not establish the requested missing-artifact constraint: contracts/extension_trust_test.go:44-52 omits the entire optional artifact object and also sets configuration_schema to an empty map. The schema at contracts/extension/v1/extension-descriptor.schema.json:127-132 does not require artifact at top level; when artifact is present its oci_reference is required. The test can pass solely because configuration_schema lacks its required dialect.

Worker recipe: in contracts/extension_trust_test.go, derive the case from the validated baseline, replace artifact with an empty object while retaining the valid configuration dialect, and assert schema rejection for the missing oci_reference; separately test that omitting optional artifact is accepted if that is the intended schema contract. Run `go test ./contracts ./internal/extension` and `go vet ./contracts ./internal/extension`, then independent exact-head review and mandatory hosted CI. Stop if the isolated required-field test fails or the contract is disputed.

Go test and vet passed for both packages using temporary GOCACHE directories; initial attempts with default cache failed because it was read-only. Hosted exact-head run 41017667953: PR Selected Checks SUCCESS app 15368; selected Public Contracts, source-contracts, Focused Python and other selected jobs successful, non-selected jobs skipped. Threads empty; develop rulesets 23754541 and 25223521 disabled, main active; none changed. Scope and Go checks pass, but the specified remaining test-quality acceptance criterion does not.

## repo-rule compliance
Read-only review; no product changes, approval, protection changes or merge. Coordinator reminders remain unresolved.

## ROLLE: CODEX-MERGE

BLOCKED by the specified test-quality finding. No merge; post-merge checks inapplicable.

END rv1217 DONE

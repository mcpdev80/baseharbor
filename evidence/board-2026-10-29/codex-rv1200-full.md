ROLLE: REVIEW
UTC: 2026-10-29 09:52 UTC
Candidate: 114d6e48e0396d4a92a97f4fe4dca9badc4b0a63
Base: 80d0312276aab8829deb80188aa9902ff0183b73 (develop)
Planning main: 2eff5d668e772a42c6eca0f6cfbb1ed4242a5b9f
VERDICT: REQUEST_CHANGES

## PLAN
- [x] 0. Record plan; native TodoWrite is unavailable in this session.
- [x] 1. Refresh candidate and task contract, inspect complete diff and PR body.
- [x] 2. Run focused suites and check self-pin positive and negative paths.
- [x] 3. Verify current CI and map acceptance criteria to evidence.
- [x] 4. Record SHA-specific verdict, findings and actionable recipe.

## Review evidence
Fetched product origin; remote develop and PR branch resolve to the stated base and candidate. Planning SSH fetch failed due to system SSH config permissions; GitHub API independently confirmed origin/main SHA above, matching the locally available task contract. GitHub API confirmed current PR head and develop SHA. Reviewed the entire two-file three-dot diff. The temporary detached candidate worktree was clean before and after the tests; existing unrelated product working-tree changes were untouched.

Acceptance mapping:
1. Approval loads intended trusted source and rejects wrong or absent pins: workflow loads `vars.BASEHARBOR_RECEIPT_TRUSTED_SOURCE`, checks lowercase 40-hex, then calls `pinned_source` which compares actual verifier and workflow blobs. Candidate tests exercise same-commit and unchanged-descendant positive cases, missing, malformed, nonexistent SHA, divergent verifier and divergent workflow negatives. A different SHA with identical relevant blobs is intentionally accepted; comparison is byte equality, not identity. Registration must select a reviewed integrated commit with the candidate bytes. The PR body currently contradicts this requirement (see finding).
2. Shadow static pin/interface correctness: `b53632aaaee6067397c27000c229f34736ec2155` is an ancestor of the base; its shadow workflow blob equals the candidate's shadow blob. The called workflow requires receipt, plan, candidate and publication, all four wired from approval outputs. Hosted execution is explicitly assigned to the separate shadow-run task and was not claimed here.
3. Focused tests and workflow validation: in detached candidate worktree, `cd scripts && python3 -m unittest test_receipt_verifier test_receipt_chain` exited 0. Actual final output: `Ran 122 tests in 10.394s` / `OK`. The run emitted ResourceWarning messages for unclosed fixture files in existing test_receipt_verifier.py lines 1383–1402; no test failed. GitHub Actions run 40848710241 identifies this exact head and completed successfully; CI actionlint, CI policy, focused Python, receipt-execution and verifier tests passed. `PR Selected Checks` passed for exact head with app ID 15368. Several path-selected jobs were skipped; they are not represented as passing.

## Finding and recipe
The live PR #1200 body says both pins are set to b53632aa. That commit has the old approval workflow bytes and therefore cannot serve as TRUSTED_SOURCE for this candidate: `pinned_source` will reject it. The prompt's description of a self-pin claim is also not present in the current PR body. This is operationally material because following the body at registration causes approval to fail closed.

Recipe for the owner: update the PR body to state that the shadow call uses b53632aa, while BASEHARBOR_RECEIPT_TRUSTED_SOURCE must be set at registration to a reviewed final integrated commit containing the exact approval-workflow and verifier bytes. Do not set the variable or activate the workflow as part of this review. Verify the corrected body against the final integrated commit; if product bytes change, rerun affected tests and obtain a new SHA-specific review. Stop if the intended integrated commit does not contain the reviewed bytes or if required CI is not green. The static code acceptance criteria otherwise have the evidence above; the misleading registration instruction remains unresolved.

## Limits
No hosted shadow execution, registration, merge, push or production activation was performed. The system's TodoWrite tool was unavailable. The initial board check reported unknown Codeman session state and pending events/reminders; these were inspected but not acknowledged as resolved because their underlying sessions and outstanding actions could not be verified in this bounded review.
END RV1200FULL

## RV1200FULLB PLAN
- [x] 0. Record plan in evidence; native TodoWrite is unavailable.
- [x] 1. Fetch and verify unchanged PR head.
- [x] 2. Read and hash current PR body; compare with the requested correction.
- [x] 3. Record complete-candidate verdict and remaining findings.

ROLLE: REVIEW
UTC: 2026-10-29 09:56 UTC
Head: 114d6e48e0396d4a92a97f4fe4dca9badc4b0a63
VERDICT: ACCEPT (complete candidate; registration instruction corrected).

Fetched origin and confirmed both the remote tracking PR branch and GitHub PR head match the reviewed candidate. The live develop branch remains 80d0312276aab8829deb80188aa9902ff0183b73; the PR API base.sha reports 7a4debccc4e2fa5e78287e9b9388b0cb521d66e2, its earlier base snapshot. Both produce the identical three-dot diff (git hash-object 8d5bc939371f2c0730cc547b1b5453ec642e9283). The original complete review and acceptance mapping remain applicable to unchanged candidate bytes; current integration and CI gates require their own live verification before any merge.

The PR body now states the static shadow ref b53632aa, the TRUSTED_SOURCE repository variable, and registration at the reviewed final integrated commit containing this candidate's exact workflow and verifier bytes, explicitly excluding b53632aa. The earlier finding is resolved; no remaining review findings. Registration and hosted shadow execution remain future work under their own task.

Git hash-object of the API body text, including its terminating newline as returned by gh --jq: 9849b55318a59d22bc8f5a24c2d26bd0af76d0fe. The text quoted below is copied directly from that same file and has the same hash.

```text
Repair approval trusted-source wiring per Codex rv1200 (F1) + rv1200b: the shadow call is pinned to b53632aa (static interface-correct reviewed ref), while TRUSTED_SOURCE is now read from repository variable BASEHARBOR_RECEIPT_TRUSTED_SOURCE with fail-closed guard (unset/malformed -> rejection). AT REGISTRATION the coordinator sets that variable to the reviewed final integrated commit containing the exact approval-workflow and verifier bytes of this candidate - NOT to b53632aa (whose approval bytes predate this change and would fail pinned_source). Same-commit/unchanged-descendant positive and wrong/absent/malformed/divergent negatives are covered by scripts/test_receipt_verifier.py regressions. Scoped to receipt-approval.yml + focused tests. Evidence: baseharbor-plan/evidence/r27-ci-replacement-2026-10-29/approval-pin.md + approval-pin-correction.md; verdicts codex-rv1200.md / codex-rv1200b-verdict.md (plan repo). Task t-20261029-r27-ci-approval-pin, lanes o201+f60.
```
END RV1200FULLB

ROLLE: REVIEW

## PLAN
- [x] 0. Read dispatch and policies; record plan. Native TodoWrite is unavailable; no native update is claimed.
- [ ] 1. Refresh candidate, base, task and effective rules; inspect full diff and preserved hardening.
- [ ] 2. Check local syntax/lint and current hosted evidence; map acceptance and registration behavior.
- [ ] 3. Record verdict, actionable recipe and limitations.

UTC: 2026-10-29T10:43:28Z
Reviewer: Codex, GPT-6, independent of worker; no Horde task ID exposed.
Head 3fc45250b1210dd05e3853491d0ea5198519598b; base main 7a417d378292724e1820c976b110fe3c107eba45; Planning origin/main 44058ee7f2e5595f4ff3824f3fcf42d6bbbcc8e7. PR remains OPEN at head at review end.
VERDICT: REQUEST_CHANGES; main registration is not approved or performed.

## Findings and actionable recipe

1. Dispatch requires the complete diff to consist only of the write-token env line(s); actual diff also changes scripts/test_receipt_postrun.py extensively (two files, 194 insertions/18 deletions overall), and adds seven explanatory workflow comment lines. The task card itself asks for focused test coverage, so the test-file scope is consistent with that broader card but conflicts with this narrower dispatch; do not silently decide the conflict. Coordinator/owner should clarify the permitted test changes and comments or have the worker revise the candidate to the authorized scope while retaining required focused coverage through an approved path. If test changes are authorized, independently review their expanded shell fixture behavior and run relevant positive/negative tests; stop if test scope or acceptance remains ambiguous.
2. PR Selected Checks is SKIPPED, not green, on exact head (Actions app 15368). The binding GitHub authority file requires this check succeed and prohibits substituting or bypassing required CI; Workflow policy success and local tests cannot satisfy that policy. Main ruleset 23754542 has no required-status-check rule, but technical absence does not override the binding policy gate. Coordinator must obtain an authoritative policy decision and a valid exact-candidate CI path before any proposed registration; no skip may be treated as success, and no protection changed. Stop if the required evidence is unavailable.

## Evidence and acceptance

Exact workflow diff adds only the PLANNING_RECEIPT_WRITE_TOKEN secret mapping and seven comments in the publish env; no other workflow lines changed. Therefore main-specific pin, attempt, PR fallback and GH_WORKFLOW_TOKEN lines remain byte-identical to main within that file. The env mapping supplies the writer token and does not itself print it; static inspection is not a proof of live secret protection. The test-file changes are substantive, not merely the workflow line. Focused tests in isolated detached candidate worktree: `python3 -m unittest test_receipt_postrun test_receipt_chain` exit 0, Ran 17 tests in 1.386s, OK. YAML `yaml.safe_load` parsed successfully. actionlint was unavailable locally; it was not run. Worktree status clean before and after.

Run 40858818994 attempt 1, pull_request exact head: Workflow policy succeeded; PR Selected Checks, focused Python and other selected PR jobs were skipped. Main ruleset 23754542 is active, no bypass, requires linear history, PR, squash-only, thread resolution and extra approval for unattributed changes; no required check is listed. ReviewThreads returned none and no next page. Ruleset technical requirements and binding policy are distinct; no claim is made that selected-check skip passes the latter.

If a separately authorized merge/registration happens, the default-branch workflow_run definition will include the mapped token for eligible successful producer completions; this does not establish that a particular run triggers or publishes successfully. Next evidence: real successful producer run after registration, inspect automatic writer run and durable task-linked receipt publication without logging secrets; #1203 remains REQUEST_CHANGES and cannot be assumed a ready producer. CODEMAN-START requires such observed proof and negative/retained checks for chain acceptance. No workflow dispatch, credential access, merge, approval or product edits performed.

Turn-start events/reminders inspected; Codeman visibility unknown and shared acknowledgement not advanced as unrelated issues cannot be verified in this review.

END RV1204

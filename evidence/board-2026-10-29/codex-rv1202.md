ROLLE: REVIEW

## PLAN
- [x] 0. Read dispatch and binding policies; record plan. Native TodoWrite is unavailable; no native update is claimed.
- [ ] 1. Refresh candidate and contract; inspect complete diff, workflow and pins.
- [ ] 2. Verify checkout behavior and run focused tests in isolated candidate worktree.
- [ ] 3. Map acceptance, check exact-head CI, record verdict and actionable recipe.

UTC: 2026-10-29T10:08:51Z
VERDICT: BLOCKED — dispatched candidate is no longer PR head.

Fresh authenticated product fetch and GitHub PR view agree: PR #1202 is OPEN at de93ff656ad5bf44a8f70c107a9c0764260123dd, not the dispatched 2656e69b7248f4578f64482405974f7503a979a8. Fetched develop remains a3750a8f6a26170fbe3995f0c480e5929948a276. The current gh pr checks report executed checks passing on the current PR, but these are not evidence of the dispatched SHA and have not been qualified as an acceptance of the new SHA. The named worker evidence path was not found at evidence/board-2026-10-29/rv1202-worker.md. The local task card currently names PR #1201 and an outstanding dependency; refreshed Planning contract and ownership require verification for any new review.

Recipe: coordinator supplies a SHA-specific review dispatch for the actual current PR head and the current worker evidence/contract; reviewer refreshes base and ownership, inspects the complete new diff and checkout semantics, runs the requested focused tests in an isolated worktree, verifies exact-head CI and maps acceptance. Stop again if head or base changes during review. No test or review verdict is asserted for either candidate; no merge, approval, push or product modification performed. The turn-start events/reminders were inspected; Codeman session status remains unknown, and shared acknowledgement was not advanced as resolution of other lanes cannot be verified here.

END RV1202

## RV1202B PLAN
- [x] 0. Read dispatch and policies; establish plan. Native TodoWrite is unavailable; no native update is claimed.
- [ ] 1. Refresh candidate, task and evidence; inspect complete diff and checkout semantics.
- [ ] 2. Execute focused tests on isolated candidate and check exact-head CI.
- [ ] 3. Map acceptance, record verdict and actionable recipe.

## RV1202B
ROLLE: REVIEW
UTC: 2026-10-29T10:11:11Z
Reviewer: Codex, GPT-6, current session; no Horde task ID exposed. Independent of worker implementation.
Head: de93ff656ad5bf44a8f70c107a9c0764260123dd; fetched develop: a3750a8f6a26170fbe3995f0c480e5929948a276; Planning origin/main: 335f8df0d083812c11dc67ada8f177c3df64fff4.
VERDICT: ACCEPT for the reviewed develop-side source change; this is not hosted execution, default-branch registration, task closure or merge authorization.

Complete develop...head diff changes only .github/workflows/receipt-post-run.yml and scripts/test_receipt_postrun.py: both immutable controller checkouts move from e13f157 to dd8fb61d; publication step adds GH_TOKEN from secrets.GITHUB_TOKEN while preserving PLANNING_RECEIPT_WRITE_TOKEN from its own secret. The writer's --product argument exists and is forwarded at dd8fb61d; the prior e13f157 writer lacks it. Comparing the three controller Python files between pins shows the writer change, not changes in tasklink or verifier. Both jobs execute scripts from the pinned checkout, so the pin selects that actual controller tree; it does not copy main source files into the develop branch.

Acceptance mapping: (1) focused tests: isolated detached head, `cd scripts && python3 -m unittest test_receipt_postrun test_receipt_chain` returned exit 0, Ran 19 tests in 3.283s, OK; worktree clean before and after. (2) diff scope: the two specified files only. (3) publication path: the existing separate Planning secret mapping is retained in checkout and writer step; the added GH_TOKEN supplies GitHub authentication; the changed step does not echo either token. This is static evidence, not a hosted secret-handling proof. (4) exact candidate PR is open at reviewed head. Task scope also calls for default-branch registration and dependency t-20261017-release-0-4-27-updates remains open; these are not certified complete by this review. The current Planning card lists #1201 rather than #1202 and has an unassigned handoff: coordinator should reconcile the registration handoff and ownership without claiming completion.

CI run 40851604490 is pull_request success on exact head; PR Selected Checks success from app 15368. Focused Python, receipt-execution, executor/verifier, planner, actionlint, policy and workflow policy checks pass; selected runtime, consumer and other jobs are skipped, not passed. No hosted receipt-post-run execution is asserted.

Checkout assessment: extract_plan checkout has no fetch-depth setting (default shallow); publish_receipt checkout explicitly uses fetch-depth: 0. At the exact pinned actions/checkout v7.0.1 source, git-source-provider.ts uses targeted getRefSpec for shallow fetch; ref-helper.ts returns the commit itself for an unqualified SHA. Thus resolution depends on the server allowing direct fetch of that reachable SHA, not on it being within one commit of develop. dd8fb61d is not an ancestor of develop (merge-base --is-ancestor exit 1). The worker's claim that a depth-1 branch fetch necessarily includes its parent and that PR CI executed both receipt-post-run jobs is unsupported; CI checks do not establish workflow_run execution. The full-history publication path also includes a targeted fetch fallback if its ref is missing. No current concrete checkout failure established; follow-up candidate after cutover: independently exercise both exact SHA checkout paths on hosted workflow and consider full-history hardening if its actual requirements warrant it. Stop if direct fetch fails or actual controller bytes differ. Source examined: https://github.com/actions/checkout/blob/3d3c42e5aac5ba805825da76410c181273ba90b1/src/git-source-provider.ts and https://github.com/actions/checkout/blob/3d3c42e5aac5ba805825da76410c181273ba90b1/src/ref-helper.ts.

Repo-rule compliance: review only; no product modification, push, approval or merge. Shared event and reminder observations inspected; Codeman session visibility remains unknown. Current PR head and local fetched develop remained unchanged at review end. Worker evidence at baseharbor-plan/evidence/r27-ci-replacement-2026-10-29/rv1202-worker.md was inspected as data; its ancestry/CI claims are not adopted.

END RV1202B

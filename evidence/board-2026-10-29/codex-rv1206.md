ROLLE: REVIEW

## PLAN
- [x] 0. Read dispatch and policies; record plan. Native TodoWrite unavailable; no native update claimed.
- [ ] 1. Refresh exact candidate and inspect complete diff, reconciliation and pin behavior.
- [ ] 2. Verify focused tests, CI and producer/shadow binding; record verdict and recipe.

UTC: 2026-10-29T13:32:39Z
Reviewer Codex GPT-6, independent of worker; Horde task ID not exposed. Head 3dfd98c0d1ee2241beefdba7b74ac230f38062b1, develop f95b3446ed9a007a31fcc692396d6dfaf3b38c36, main 7a417d378292724e1820c976b110fe3c107eba45. Authenticated HTTPS ls-remote (SSH failed on system config permissions):
`3dfd98c0d1ee2241beefdba7b74ac230f38062b1 refs/heads/work/r27-develop-main-reconcile`
VERDICT: REQUEST_CHANGES

## Blocking finding and recipe

`.github/workflows/receipt-post-run.yml` removes the publish step's `PLANNING_RECEIPT_WRITE_TOKEN` environment mapping; `scripts/test_receipt_postrun.py` changes its regression from assertIn to assertNotIn and asserts without evidence that in-process consumption supplies an absent environment variable. At `scripts/receipt_writer.py:108-110`, `_push_authed` explicitly reads that environment variable and rejects a real remote push when absent. Planning checkout receiving a secret in an earlier action does not export it to a later step. This reintroduces the publication failure that #1201 fixed. Worker must restore the publish-step secret mapping and positive exact-expression regression, preserve main's attempt/PR/token hardening, and test actual real-remote-path missing-token failure plus appropriately mocked success without exposing the secret. Re-run focused suites and exact-head required CI; stop if writer cannot obtain the token or tests permit its absence. Do not activate or register this candidate.

## Reconciliation and limitations

Complete diff is three files: workflow, receipt_writer.py and test_receipt_postrun.py. Candidate writer blob abf4d075c54371fde66dcb4468f55aa5dd512281 and tasklink blob 52f11644dc72d3d38a71973fa0a73f7091f63c5c match fetched main; candidate workflow blob 06ae97480253d12c5cce80cc3f9e7437aa84c9c0 also matches main, demonstrating the develop token loss rather than preservation. Main GH_WORKFLOW_TOKEN/attempt and pull_requests fallback are present, as is --product forwarding. The controller checkout pins are dd8fb61d in both jobs; approval retains variable-based TRUSTED_SOURCE and static shadow b53632aa. Integration identity remains extracted from trusted plan; complete chain binding was not proven here.

The dispatch's claimed rv1205 finding about a mutable PR head ref is not present in the recorded rv1205 verdict. In this candidate post-run workflow uses `github.event.workflow_run.head_sha` and plan facts, while checkouts pin controller SHA; BASE_REF is parsed for a PR number. No direct mutable-head checkout was identified in these inspected paths, so a specific one-line fix is not substantiated. Actual producer-run identity and merge-ref lifetime remain unverified as an end-to-end integration requirement; use authenticated run/PR identities and test closed/merged PR and changed head before cutover rather than inventing a fix.

Shadow workflow blob at b53632aa equals candidate shadow blob 5c0cbcdb62791d0b6d2c8ef674d948e5eaa8a152, while writer blobs differ: b53632aa 9a0bcf8a146b5726bb026507f0f56a0d2536e7a3 vs dd8fb61d abf4d075c54371fde66dcb4468f55aa5dd512281. Equality of shadow workflow does not show it needs writer CLI changes: inspect its actual verifier checkout and retained-proof interfaces before any pin change; hosted shadow chain remains unqualified.

Independent local command in detached candidate worktree: `python3 -m unittest test_receipt_postrun test_receipt_chain test_receipt_writer test_receipt_tasklink` exit 0, 73 tests in 5.874s, OK (ResourceWarning). Passing suite includes inverted token assertion, so does not disprove finding. CI run 40881268069 exact-head PR Selected Checks success app 15368; relevant validation jobs pass and skips remain skips. Candidate worktree clean; no product edits, push, merge or approval. Events/reminders inspected; Codeman session visibility unknown, other lanes not altered.

END rv1206 DONE

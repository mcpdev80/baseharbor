ROLLE: REVIEW

## PLAN
- [x] 0. Read dispatch and binding policies; record plan. Native TodoWrite unavailable; no native update claimed.
- [ ] 1. Refresh candidate and inspect complete promotion diff, lineage, individual blob provenance and prior examples.
- [ ] 2. Check current CI and rules, evaluate cutover impact, and record verdict and actionable recipe.

UTC: 2026-10-29T11:11:50Z
Reviewer: Codex, GPT-6, independent of implementation; no Horde ID exposed.
PR #1205 OPEN, head f95b3446ed9a007a31fcc692396d6dfaf3b38c36 equals fetched develop; main 7a417d378292724e1820c976b110fe3c107eba45. GitHub reports mergeable=false, DIRTY. VERDICT: REQUEST_CHANGES.

## Findings and actionable recipe

1. Promotion as a whole is not safe to approve. Direct main-to-head diff shows receipt-post-run.yml removes main's GH_WORKFLOW_TOKEN, authenticated attempt forwarding and pull_requests[0] fallback; scripts/receipt_writer.py removes --product CLI support and forwarding. This would overwrite reviewed main-side fixes if a resolution were chosen to make the merged tree equal develop. GitHub reports a conflict; there is currently no actual merged tree to validate. Coordinator should assign a worker to reconcile main and develop while preserving the registered main hardening and intended token mapping, obtain independent review of the exact integrated diff, and prove no unrelated main changes are lost. Stop if conflict resolution or retained main behavior is unverified. Existing controller pin dd8fb61d may execute a different writer than the root blob, but this does not justify deleting main's fixed source or workflow defenses.
2. No checks are reported on the head, including PR Selected Checks; exact-head check-runs API returned total_count 0. Prior develop-side PR checks and precedent cannot substitute for binding GitHub authority's required successful PR Selected Checks at the exact candidate or a demonstrably valid current integration. Even if such a path were offered, conflicting main integration and removed hardening invalidate a presumption of equivalence. No skipped/absent check is green. Owner/coordinator must establish an authorized exact-integration CI route without bypass, then recheck actual result.

## Diff and provenance inventory

Direct main-to-develop diff: six files, 242 insertions, 28 deletions. Blob hashes at f95b3446 and last develop-side changes:
- .github/workflows/receipt-approval.yml: 2a1e029927123bedacc6a6378df070440e35dc7d, a3750a8f (#1200; RV1200FULLB accepted source change).
- .github/workflows/receipt-post-run.yml: 40daeee6d9de15f3b6b0b233794b2e381ddb95ed, f95b3446 (#1202; RV1202B accepted develop-side diff). It differs adversely from later main-specific hardening as above; that review did not accept its use as a replacement for main.
- contracts/contracts_test.go: 6abebd8f642d27ebe0ce1b6f9a87af3861a1b3ca, 7a4debcc (#1199); product contract test, not CI workflow itself. Prior acceptance not independently established in this review.
- scripts/receipt_writer.py: 9a0bcf8a146b5726bb026507f0f56a0d2536e7a3, last develop change b53632aa (#1147); predates and removes main #1196's product-argument fix. Prior accepted provenance for this exact promotion effect not established.
- scripts/test_receipt_postrun.py: 9cc49aea1434371e9c2e2111d94f6521b974fd8d, f95b3446 (#1202, reviewed develop-side).
- scripts/test_receipt_verifier.py: 292f0a019202204e880b3d84627d821675465b53, a3750a8f (#1200, reviewed).

Develop-head tree 2a17260d7096faee1cc7b08c349cb8adbbd4dd18. Local repository is shallow: merge-base found none and merge-tree refused unrelated histories; this does not prove histories are truly unrelated. GitHub's live DIRTY result independently establishes actual integration conflict. A squash commit with main as parent cannot be assumed to have the develop tree; no merged tree exists here. Each blob exists at develop head, but that fact alone does not establish acceptance of its main-side deletion effects or every historical source's review.

Historical examples: #1159 commit 364db5d4 changed 45 files (+8974/-67), #1186 df451e0d one file (+13/-228), #1170 e13f1570 one file (+1/-1); their occurrence does not prove this six-file candidate is safe or waive current gates. The promotion changes no rulesets directly, but loss of attempt/identity and writer hardening could compromise the future chain that cutover-pr3 requires. It is therefore not possible to affirm it leaves cutover prerequisites intact. Cutover itself remains blocked pending accepted shadow/evidence and explicit decision.

No merge, approval, push, rule change or product modification. Events/reminders inspected; Codeman state unknown; other lanes and acknowledgement remain untouched.

END RV1205

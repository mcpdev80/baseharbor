ROLLE: REVIEW

## PLAN
- [x] 0. Read dispatch and binding policies; record plan. Native TodoWrite unavailable; no native update claimed.
- [ ] 1. Verify current direct comparison, PR diff and exact-head CI against policy.
- [ ] 2. Record verdict; merge only if all binding gates pass.

## REVIEW
ROLLE: REVIEW
UTC: 2026-10-29T17:47:26Z
Reviewer Codex GPT-6; no Horde ID exposed. PR #1212 OPEN/CLEAN head 864c9609f1acae9cb25b64d8c9b9fc86758d3a19, main 0b0659f2a0280a87ae677d10bc69df1368c8228e. HTTPS ls-remote agrees with head.
VERDICT: BLOCKED on mandatory CI; direct-tree scope clarification accepted.

Direct current main->candidate workflow diff is precisely persist-credentials false->true plus Planning write-secret token line; candidate workflow blob 1c092f74cf3b2824312e39037f6535527f9d4695 equals current develop workflow blob, main blob b3a6ee05bde28e025665dae22d7873e899158627. The prior three-dot diff includes older ancestry changes; the dispatch clarifies the intended two-line scope is the direct current-main comparison. This resolves that specific interpretation, not all qualification gates.

The requested governing source is `/home/marcel/.config/baseharbor-coordinator/GITHUB_AUTHORITY.md:14`: “`PR Selected Checks` must succeed and originate from GitHub Actions integration **15368**.” It permits a demonstrably valid current integration but provides no main-PR exemption and explicitly forbids bypass. Exact-head check-runs API: PR Selected Checks skipped app 15368; Workflow policy success app 15368. GitHub synthetic integration SHA 631f09d9c5cd890040a05aac11058f16e80d0815 has zero check-runs in current API observation. Historical main merges and absence of a GitHub required-check rule do not create successful evidence or amend this binding policy. No valid successful current-integration check was supplied. Separately, that same authority file excludes release promotion and only grants standing qualified develop merges; the dispatch's direction alone does not document a specific Owner amendment to that policy.

Recipe: obtain an authorized qualification path whose successful PR Selected Checks from app 15368 binds the actual current integration, and separately establish applicable main-registration authority; secure an independent final review and rerun live gates. If the CI design structurally skips main PRs, route that mismatch for an Owner/CI decision rather than silently counting the skip as green. Stop if candidate/base changes, CI is skipped or registration authority remains unestablished.

## MERGE
ROLLE: MERGE
BLOCKED: no merge or approval attempted; no post-merge main SHA/tree exists. No rule, variable, product or remote changes. Turn-start observations inspected; Codeman visibility unknown and unrelated shared state untouched.

END rv1212b DONE

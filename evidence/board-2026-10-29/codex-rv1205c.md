ROLLE: REVIEW

## PLAN
- [x] 0. Read dispatch and binding policies; record plan. Native TodoWrite unavailable; no native update claimed.
- [ ] 1. Verify exact-head checks, main ruleset, precedent and policy.
- [ ] 2. Record evidence, verdict and actionable remedy.

UTC: 2026-10-29T15:04:43Z
Reviewer Codex GPT-6; no Horde task ID exposed. Exact head eb2ea56c3f93b22ca308babdb00625f56c957cf9, HTTPS ls-remote: `eb2ea56c3f93b22ca308babdb00625f56c957cf9 refs/heads/work/r27-promote-ci-registration`.
VERDICT: REQUEST_CHANGES / BLOCKED on binding CI-policy and promotion-authorization mismatch; not a claim that the main ruleset requires a status check.

Requested API outputs:
- `gh api repos/mcpdev80/baseharbor/rulesets/23754542 --jq '[.rules[] | select(.type == "required_status_checks")]'` => `[]`. Full ruleset inspection: active, no bypass actors, linear history, squash-only PR and thread resolution; no required-status-check rule.
- `gh pr view 1198 ... --jq '{state,mergedAt,checks:[.statusCheckRollup[] | select(.name == "Workflow policy" or .name == "PR Selected Checks") | {name,conclusion,status}]}'` => `{"checks":[{"conclusion":"SUCCESS","name":"Workflow policy","status":"COMPLETED"},{"conclusion":"SKIPPED","name":"PR Selected Checks","status":"COMPLETED"}],"mergedAt":"2026-10-29T03:20:58Z","state":"MERGED"}`.
- Candidate check-runs API => `[{"app_id":15368,"conclusion":"skipped","head_sha":"eb2ea56c3f93b22ca308babdb00625f56c957cf9","name":"PR Selected Checks"},{"app_id":15368,"conclusion":"success","head_sha":"eb2ea56c3f93b22ca308babdb00625f56c957cf9","name":"Workflow policy"}]`.

The main ruleset does not require Workflow policy or PR Selected Checks; the former's success is an observed check, not a ruleset-required context. Prior #1198 merge demonstrates historical behavior, not an exception to binding coordinator policy. `/home/marcel/.config/baseharbor-coordinator/GITHUB_AUTHORITY.md:14` requires PR Selected Checks succeed from app 15368 for an exact candidate or demonstrably valid current integration and says never bypass it; lines 13-14 prohibit treating skipped evidence as success. That policy is the explicit rule evidence, not a fabricated main ruleset field. Source main ci.yml guards the selected final job on base_ref develop, making a main-targeted PR skip structurally. This is a CI design/policy mismatch requiring an authorized solution, not an implementation defect in this PR alone. The same policy line 27 forbids release promotion, and line 5 limits standing merge authority to qualified develop PRs; no promotion authority is established by technical mergeability or this review.

Recipe: coordinator/Owner should establish an explicitly authorized exact-integration qualification path meeting the binding check contract and separately authorize the main promotion if intended. Confirm actual successful check from app 15368 on a demonstrably valid current integration, preserve all main rules and resolve threads, obtain final SHA-specific review, then verify the actual merged tree if a promotion is separately authorized. Stop if required CI is skipped, integration provenance is uncertain or promotion authority absent. No merge, approval, push, variable or ruleset action performed. Turn-start events/reminders inspected; Codeman visibility unknown, unrelated shared state untouched.

END rv1205c DONE

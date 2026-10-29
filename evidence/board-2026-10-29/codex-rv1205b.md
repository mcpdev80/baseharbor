ROLLE: REVIEW

## PLAN
- [x] 0. Read dispatch and binding policies; record plan. Native TodoWrite unavailable; no native update claimed.
- [ ] 1. Refresh exact candidate, ancestry, tree and full promotion diff.
- [ ] 2. Verify exact-head checks and document review verdict and limitations.

UTC: 2026-10-29T15:02:09Z
Reviewer: Codex GPT-6, independent of worker; no Horde task ID exposed. PR #1205 OPEN/MERGEABLE/CLEAN at eb2ea56c3f93b22ca308babdb00625f56c957cf9; authenticated HTTPS ls-remote: `eb2ea56c3f93b22ca308babdb00625f56c957cf9 refs/heads/work/r27-promote-ci-registration`.
VERDICT: BLOCKED — required exact-candidate CI is skipped, not green.

Head parents: develop 1f94951d2dc014ff924f35777b08a96e75781c06 and main 7a417d378292724e1820c976b110fe3c107eba45; tree da634e7412404c4254feb357d79f2505e1dab22a equals fetched develop tree. Main is ancestor (exit 0). Main->head diff is five files, 268 insertions/16 deletions: approval workflow, post-run workflow, contracts vector test, post-run and verifier tests. Post-run workflow changes only by adding the Planning write-token mapping/comments; receipt_writer.py has no diff, so inspected GH_WORKFLOW_TOKEN, attempt forwarding, PR fallback, dd8fb61d pins and --product writer remain present. The previous reconciliation regression is resolved in this tree. This source observation does not establish the entire chain is operational.

Run 40897108411 attempt 1 completed overall success on exact head, but `PR Selected Checks` itself is SKIPPED from app 15368; Workflow policy passed and all other listed jobs skipped. Binding authority requires PR Selected Checks success on exact candidate or demonstrably valid current integration and forbids treating skipped CI as green. No such passing check was identified. Coordinator should establish an authorized exact-integration CI path and obtain a successful required check without bypass, then request re-review/recheck. Do not merge while it is skipped. This conclusion also does not grant promotion authorization: bootstrap policy expressly excludes release promotion and main publication absent separate authority.

If an independently authorized squash promotion later succeeds on unchanged base/head, equality of its resulting tree with da634e74 must be measured, not assumed; registering the source variable and observing real producer publication are separate steps. No merge, approval, push, variable or ruleset change performed. Turn-start events/reminders inspected; Codeman visibility unknown and shared other-lane state left untouched.

END rv1205B DONE

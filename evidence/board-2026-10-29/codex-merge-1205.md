ROLLE: MERGE

## PLAN
- [x] 0. Read dispatch and binding policies; record plan. Native TodoWrite unavailable; no native update claimed.
- [ ] 1. Verify live candidate, review, CI, threads and effective rules.
- [ ] 2. Merge only after binding gates are met; verify resulting tree and checks.

## BLOCKED
ROLLE: MERGE
UTC: 2026-10-29T15:06:53Z

Dispatch attributes this instruction to Marcel, 2026-10-29, Coord-Thread: “Danach #1205 aktualisieren und regulär reviewen und mergen; anschließend Registrierung und Kettenlauf durchführen.” It is recorded as supplied, not independently verified here. It does not expressly waive required CI. The binding policy `/home/marcel/.config/baseharbor-coordinator/GITHUB_AUTHORITY.md:13-14` requires successful PR Selected Checks from app 15368 for exact candidate or demonstrably valid current integration, forbids bypass, and does not authorize counting a skip as success. RV1205C independently remains REQUEST_CHANGES/BLOCKED on this mismatch. The dispatch's assertion that this policy applies only to develop is not an amendment to the binding file. Main ruleset has no required-check rule, but that does not make the Owner's separate CI gate pass; historical precedent is not a successful exact-integration check.

Live PR #1205 OPEN/MERGEABLE/CLEAN at eb2ea56c3f93b22ca308babdb00625f56c957cf9; HTTPS ls-remote agrees. Exact head check-runs: PR Selected Checks skipped app 15368, Workflow policy success app 15368. ReviewThreads empty/no next page. Ruleset 23754542 active, no bypass, squash-only/linear history/thread resolution/extra approval for unattributed changes; no required-status-check rule. No rule changed. No merge attempted, so no after SHA, merged tree or push-check result exists.

Coordinator/Owner should supply an authorized exact-integration qualification satisfying the stated CI policy and a final independent review, or an explicit authoritative policy amendment; recheck all live gates before action. No approval, merge, push, registration or rule change performed. Turn-start events/reminders inspected; Codeman visibility unknown, unrelated shared state left unchanged.

END m1205 BLOCKED

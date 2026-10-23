# O47-QPR — Selector Q-predecessor PR (qpr prompt)

ROLLE: WORKER — tiny two-file predecessor PR anchoring the two Selector-Q entries
on the trusted `develop` base. Follows `evidence/board-2026-10-22/prompts/o47-qpr.txt`
and `o47-followup.txt`; anchors on the ACCEPTED corrected source S39/S42 from
`evidence/board-2026-10-22/codex-verdict-s39b-s42b.md` (Round 2).

## PLAN

- [x] Read task contract (o47-qpr.txt), followup (o47-followup.txt), task note, selector recipe (codex-verdict-selector.md) and S39b/S42b verdict (Round 2).
- [x] Claim/ownership: task t-20261023-selector-q-predecessor already claimed by o47 (plan.py claim, commit 54757f3), pushed to origin/main; local main current.
- [x] Reality check remotes: plan main current; develop fetched 9f4916be; PR API bases and source merge-bases remain D0 2f005813.
- [x] Accepted corrected source anchors (Round 2): S39 = 75efeef89b61d848d5ea351877ba6423a31e58a6 (#1039 fix/v0426-stepcap-split), S42 = 0267dbdb71f63a2c77e193618e6fba4106e5cd5b (#1042 fix/atomic-bare-sha-dispatch). Both PRs OPEN, heads unchanged after re-query.
- [x] Source-only verification (Round 2): both corrected planner blobs equal accepted D0; full D0→A inventory = only M .github/workflows/pre-release-runtime-suite.yml; D0→B = M ci.yml + M pre-release-runtime-suite.yml + M targeted-demo-acceptance.yml + A scripts/targeted-gate-ref.py + A scripts/test_targeted_gate_ref.py.
- [x] Confirm develop planner (98a9229c) + planner-test (1a1eb3de) blobs equal D0 before Q.
- [x] Build ONE tiny two-file predecessor Q at 19c1746b from D0 2f005813:
      - scripts/pr-gate-plan.py:244 — after EDIT_ZERO_PREVIEW_REVIEWED_FILES add RUNTIME_STEP_CAP_REVIEWED_HEAD='<S39=75efeef>' and DISPATCH_GATE_REF_REVIEWED_HEAD='<S42=0267dbd>' with the two source-only inventories. Register both at pr-gate-plan.py:747 with CI_CHECKS + WORKFLOW_SYNTAX_CHECKS; preserve generic classifier/chain verifier/runner/final-check enforcement.
      - scripts/test_pr_gate_plan.py:1580 — immutable full-SHA + complete inventory assertions for both entries; both names in the real-Git ReviewedSourceQualificationTests cases requiring exactly the chosen validations.
- [x] Precheck python3 scripts/test_pr_gate_plan.py — all suites pass (93 + 8 + 7 + 5). Anchors + inventories verified against accepted S39/S42 via importlib load.
- [x] Local CI prechecks: bash scripts/ci-workflow-policy.sh PASS (19 approved files); /tmp/actionlint 1.7.12 on ci.yml + pre-release-runtime-suite.yml + targeted-demo-acceptance.yml PASS (exit 0).
- [ ] Independent CODEX-REVIEW of the complete exact Q diff (two-file, two real anchors) before pushing.
- [ ] Push PR, target develop, await green gh pr checks (app 15368), no merge.
- [ ] END O47-QPR-DONE with PR number + SHA.

## Verified inputs

| Input | Actual state (2026-10-23) |
| --- | --- |
| baseharbor-plan local main | 874a2e3, current with origin/main |
| baseharbor-plan claim | t-20261023-selector-q-predecessor owner o47 (commit 54757f3) |
| develop (Q base) | 2f005813dc6d7fa94de164388b2531d025af31bc; planner 98a9229c, planner-test 1a1eb3de equal D0 |
| Q candidate | 19c1746bb4e12186619b92aa940b4926089a9570; D0..Q name-status = M scripts/pr-gate-plan.py, M scripts/test_pr_gate_plan.py |
| Trust boundary | develop has neither RUNTIME_STEP_CAP nor DISPATCH_GATE_REF entries; ci.yml:442-446 trust consumption at D0 (F14's ci.yml:458 identifies the boundary) |
| S39 accepted | 75efeef89b61d848d5ea351877ba6423a31e58a6; full D0→A inventory = only M .github/workflows/pre-release-runtime-suite.yml |
| S42 accepted | 0267dbdb71f63a2c77e193618e6fba4106e5cd5b; full D0→B inventory = M ci.yml + M pre-release-runtime-suite.yml + M targeted-demo-acceptance.yml + A scripts/targeted-gate-ref.py + A scripts/test_targeted_gate_ref.py |
| Selector recipe anchors | RUNTIME_STEP_CAP_REVIEWED_FILES = {pre-release-runtime-suite.yml: modified}; DISPATCH_GATE_REF_REVIEWED_FILES = {ci.yml, pre-release-runtime-suite.yml, targeted-demo-acceptance.yml: modified; scripts/targeted-gate-ref.py, scripts/test_targeted_gate_ref.py: added} |
| CI checks (app 15368) | strict up-to-date PR Selected Checks, integration 15368; review/thread/linear-history; squash only |

## repo-rule compliance

- baseharbor-plan: planning-only; edit only this task's evidence note; publish to main via normal push; render + check after metadata edits.
- baseharbor product: two-file predecessor Q uses the existing qualification mechanism (ci-planner classification pr-gate-plan.py:563-564/1121-1123, no new self-entries needed); strict up-to-date PR Selected Checks (review/thread/linear-history, squash only, integration 15368). No ruleset change, no force-push, no self-merge.
- Ownership: #1039/#1042 retained by existing owners; this session is the bounded qualification-worker slice; no merge, no merge-authority action (CODEX MERGE does the merge).

## Handoff

- State: active. Owner: o47. Branch: (none yet; develop worktree for the Q PR). Q candidate: 19c1746b (two files, two accepted anchors). PR: to be opened, target develop. Evidence: this file + codex-verdict-s39b-s42b.md (accepted anchors) + codex-verdict-selector.md (recipe).
- Next trigger: independent CODEX-REVIEW of the exact Q diff; then push, await app-15368 green checks; merge performed by a separately assigned Maintainer (CODEX MERGE).

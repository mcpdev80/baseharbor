# L6 — Complete exact path mapping for receipt control-plane sources

Mission: `ops/missions/l6.txt` — Run 40394197806 `PR Selected Checks` BLOCKED
(coverage-unresolved). Make `scripts/pr-gate-plan.py` map every final receipt
control-plane path to the smallest matching EXISTING check; no new gates.

- Worktree: `/home/marcel/dev/baseharbor-ecosystem/worktrees/l6-ustrack`
- Branch: `l6/ustrack` @ base `34159b1f` (integrate: L4 allowlist sync)
- Push: NOT authorized by mission (KEIN push)

## PLAN

- [x] 1. pwd-PFLICHT + Realitätsabgleich (basics-check, events acked)
- [x] 2. Worktree `l6/ustrack` from `34159b1f`, diff inventory vs origin/develop
- [x] 3. Read classifier: PLANNING_SCRIPTS/WORKFLOWS, RECEIPT_PILOT_*, CI_CHECKS, WORKFLOW_SYNTAX_CHECKS, load_tests discovery
- [ ] 4. Map receipt python paths -> existing pr-planner-tests lane (extend RECEIPT_PILOT_SCRIPTS + load_tests)
- [ ] 5. Map scripts/ci-workflow-policy.sh + scripts/test_pr_gate_plan_receipt_paths.py -> CI_CHECKS (existing ci-policy neighbor pattern)
- [ ] 6. Map three receipt-*.yml -> WORKFLOW_SYNTAX_CHECKS + policy (precedent: PLANNING_WORKFLOWS branch), NOT native
- [ ] 7. Update mapping regressions in scripts/test_pr_gate_plan_receipt_paths.py
- [ ] 8. Local proof: plan() over diff origin/develop...HEAD -> resolved, unresolved empty, no new check/gate names
- [ ] 9. Existing suites green (test_pr_gate_plan.py w/ discovery, receipt suites, ci-policy)
- [ ] 10. Single commit `ci(plan): complete exact path mapping for receipt control-plane sources` (no push)

## Befunde (Klassifikation vor Fix, Run 40394197806)

| Path | Failed class | Fix class |
| --- | --- | --- |
| (table filled in step 8/10) | | |

## repo-rule compliance

(to be filled)

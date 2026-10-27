# L6 — Complete exact path mapping for receipt control-plane sources

Mission: `ops/missions/l6.txt` — Run 40394197806 `PR Selected Checks` BLOCKED
(coverage-unresolved). `scripts/pr-gate-plan.py` now maps every final receipt
control-plane path to the smallest matching EXISTING check; no new gates, no
new validation names, no runner-whitelist or evidence-machinery changes.

- Worktree: `/home/marcel/dev/baseharbor-ecosystem/worktrees/l6-ustrack`
- Branch: `l6/ustrack`, base `34159b1f` (integrate: L4 allowlist sync)
- Candidate commit: `83d53e86` "ci(plan): complete exact path mapping for receipt control-plane sources"
- Push: NOT performed (mission: KEIN push)

## PLAN

- [x] 1. pwd-PFLICHT + Realitätsabgleich (basics-check, events acked)
- [x] 2. Worktree `l6/ustrack` from `34159b1f`, diff inventory vs origin/develop
- [x] 3. Read classifier: PLANNING_SCRIPTS/WORKFLOWS, RECEIPT_PILOT_*, CI_CHECKS, WORKFLOW_SYNTAX_CHECKS, load_tests discovery
- [x] 4. Map receipt python paths -> existing pr-planner-tests lane (extend RECEIPT_PILOT_SCRIPTS + load_tests)
- [x] 5. Map scripts/ci-workflow-policy.sh + scripts/test_pr_gate_plan_receipt_paths.py -> CI_CHECKS (existing ci-policy neighbor pattern)
- [x] 6. Map three receipt-*.yml -> WORKFLOW_SYNTAX_CHECKS + CI_CHECKS policy class (PLANNING_WORKFLOWS precedent), NOT native
- [x] 7. Update mapping regressions in scripts/test_pr_gate_plan_receipt_paths.py
- [x] 8. Local proof: plan() over diff origin/develop...HEAD -> resolved, unresolved empty, no new check/gate names
- [x] 9. Existing suites green (planner lane w/ discovery, receipt suites, ci-policy, actionlint, focused job lines)
- [x] 10. Single commit `ci(plan): complete exact path mapping for receipt control-plane sources` (no push)

## Datei -> Klasse -> Check

| Path | Klasse (area) | Bestehender Check |
| --- | --- | --- |
| scripts/receipt_writer.py | receipt-pilot-script | pr-planner-tests (PR · Focused Python) |
| scripts/receipt_verifier.py | receipt-pilot-script | pr-planner-tests |
| scripts/test_receipt_chain.py | receipt-pilot-script | pr-planner-tests |
| scripts/test_receipt_writer.py | receipt-pilot-script | pr-planner-tests (neu in load_tests-Discovery) |
| scripts/test_receipt_verifier.py | receipt-pilot-script | pr-planner-tests (neu in load_tests-Discovery) |
| scripts/test_ci_workflow_policy_receipts.py | receipt-pilot-script | pr-planner-tests (neu in load_tests-Discovery) |
| scripts/ci-workflow-policy.sh | ci-policy-source (neu, exakte Pfadmenge) | CI_CHECKS: ci-policy + pr-planner-tests + ci-executor-verifier-tests |
| scripts/test_pr_gate_plan_receipt_paths.py | ci-policy-source | CI_CHECKS (suite läuft zusätzlich in der Planner-Lane) |
| .github/workflows/receipt-approval.yml | receipt-dormant-workflow (neu, exakte Pfadmenge) | WORKFLOW_SYNTAX_CHECKS (ci-actionlint) + CI_CHECKS (ci-policy) |
| .github/workflows/receipt-post-run.yml | receipt-dormant-workflow | ci-actionlint + ci-policy |
| .github/workflows/receipt-shadow.yml | receipt-dormant-workflow | ci-actionlint + ci-policy |
| scripts/pr-gate-plan.py, scripts/test_pr_gate_plan.py | ci-planner (unverändert) | CI_CHECKS |
| docs/(de/)development/minimal-ci-receipts.md | documentation (unverändert) | DOC_CHECKS |

Änderungsdateien im Kandidaten: `scripts/pr-gate-plan.py`,
`scripts/test_pr_gate_plan.py` (load_tests-Tuple), `scripts/test_pr_gate_plan_receipt_paths.py`
(Mapping-Regressionen; `test_receipt_writer.py` aus der Unmapped-Liste entfernt,
neue Klasse `ReceiptControlPlaneMappingTests`).

Präzedenz-Beleg (Mission Zeile 6): Die Skript-Logik stellte bisher JEDE
`.github/workflows/`-Änderung über den `runtime-workflow-unresolved`-Zweig unter
"native qualification". Der Präzedenz für aufgelöste yml-Mappings ohne natives
Gate ist der `PLANNING_WORKFLOWS`-Zweig (pages.yml -> require(CI_CHECKS +
WORKFLOW_SYNTAX_CHECKS + DOC_CHECKS), unresolved-frei) sowie die
reviewed-HEAD-Blöcke mit yml-Bestandteil (z. B. DISPATCH_GATE_REF,
ECOSYSTEM_RESOLVER: CI_CHECKS + WORKFLOW_SYNTAX_CHECKS). Das dormante Mapping
folgt exakt diesem Muster; die drei receipt-*.yml tragen job-level
never-true-Guards (test_ci_workflow_policy_receipts.py) und stehen in der
ci-workflow-policy.sh-Allowlist (Zeilen 18–20). Aktive yml-Änderungen bleiben
unresolved (Regression `test_active_workflow_change_stays_native_unresolved`).

## BELEG (lokal, Commit 83d53e86)

`evidence/board-2026-10-26/scripts/l6-plan-proof.py` ruft `planner.plan()` gegen
den von `scripts/pr-diff-base.py` aufgelösten Diff `origin/develop...HEAD`
(target 433a5702, head 83d53e86, synthetischer dangling Integrations-Commit,
niemals gepusht):

```
paths=15
coverage_status=resolved
unresolved=[]
selected_gates=[]
validation_ids=["ci-actionlint","ci-executor-verifier-tests","ci-policy","documentation-audit","mkdocs-de-strict","mkdocs-en-strict","pages-candidate-build","pr-planner-tests"]
L6-PROOF-OK
```

Alle 8 Validations-IDs sind bestehende, in `pr-check-runner.VALIDATIONS`
gelistete Namen; keine neuen Gate- oder Check-Namen.

Testläufe im Worktree (alle exit 0):
- `python3 scripts/test_pr_gate_plan.py` — 179 Tests inkl. Discovery von
  test_receipt_chain (6), test_receipt_writer (11), test_receipt_verifier (21),
  test_ci_workflow_policy_receipts (5) und den 10 Mapping-Regressionen
- `python3 scripts/test_pr_gate_plan_receipt_paths.py` — 10/10 OK
- `bash scripts/ci-workflow-policy.sh` — PASS, 22 approved workflow files
- `go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 -shellcheck=` — PASS
- `test_pr_diff_base`, `test_pr_required_final`, `test_pr_check_runner`,
  `test_pr_wizard_workflow_route`, `test_pr_runtime_request` — OK

## repo-rule compliance

- Nur bestehende Checks/Validations referenziert; keine neuen Gates, keine
  Runner-Whitelist-, ci.yml- oder Evidence-Maschinerie-Änderung (AGENTS
  "Evidence: automatisch und reproduzierbar"; baseharbor AGENTS "As little as
  possible").
- Keine Funktion/Trigger außerhalb des Mappings umgeschrieben: die Änderungen
  an `test_pr_gate_plan.py` beschränken sich auf den load_tests-Discovery-Tuple
  + Kommentar; an `test_pr_gate_plan_receipt_paths.py` auf die
  Mapping-Erwartungen.
- Exakte Pfadsets statt Präfix-Freisgmente, konsistent zum Fail-Closed-Stil des
  Planners; Removals/Renames der Sets bleiben über die statusbezweigten
  Prefix-Logiken bzw. den `else`-Zweig unresolved.
- KEIN push ausgeführt; Arbeit in isoliertem Worktree, ein Commit.

END L6-DONE

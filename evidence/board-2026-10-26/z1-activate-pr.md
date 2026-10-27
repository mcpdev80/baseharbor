# Z1 — Shadow-Chain Activation Step 1 (Owner-gated approval dispatch)

Mission: `ops/missions/z1.txt`. Role: Worker (Codeman). Branch: `z1/shadow-activate`.

## Ziel
Minimalen Aktivierungs-Aenderungen fuer PR3-Vorlauf 'Owner approves shadow chain':
nur `receipt-approval.yml` aktivieren (Owner-gated dispatch), Post-Run-Writer und
Shadow Final-Check im Schattenlauf DORMANT lassen.

## Write-Sets (disjunkt)
- `.github/workflows/receipt-approval.yml`
- `.github/workflows/receipt-post-run.yml`
- `.github/workflows/receipt-shadow.yml`
- `scripts/test_ci_workflow_policy_receipts.py`

## Commits (SHA)
- `8e4d1297ad6718d3c46474797ac10f40fdb52c60` — `ci(r27): activate approval consumption for Owner-gated shadow dispatch`
- `1458ef46522dfc5195e75938611f7d3b9c0dc4e9` — `ci(r27): extend receipt policy test to Owner-gated dispatch guard`

## PR
https://github.com/mcpdev80/baseharbor/pull/1095
base `develop`, Titel: `ci(r27): shadow chain activation step 1 - Owner-gated approval dispatch`.

## Diff (HEAD~2 .. HEAD)
```
 .github/workflows/receipt-approval.yml      |  9 +++----
 .github/workflows/receipt-post-run.yml      |  2 +-
 .github/workflows/receipt-shadow.yml        |  2 +-
 scripts/test_ci_workflow_policy_receipts.py | 28 +++++++++++++-----
 4 files changed, 29 insertions(+), 12 deletions(-)
```

### receipt-approval.yml (Zeile 56 -> 57)
Alt (never-true, lief auf diesem Repo nie):
```
    if: github.repository == 'mcpdev80/receipt-approval-dormant'
```
Neu (exakt nach Mission):
```
    if: github.event_name == 'workflow_dispatch' && github.actor == 'mcpdev80'
```
Kommentarblock ersetzt; keine anderen Triggers armiert, kein required check hinzugefuegt.

### receipt-post-run.yml + receipt-shadow.yml
Beide bleiben DORMANT (Guard unveraendert). Ergaenzte Kommentar-Notiz (warum dormant):
"only ... Owner-gated dispatch lives in this shadow run; the ... chain is replayed
manually and the producer is already proven, so it stays dormant here (no trigger
armed, no required check added)."

## repo-rule compliance
- BaseHarbor AGENTS.md / DEVELOPMENT_GUIDELINES: CI-/aenderungen nur ueber reviewed CI-Architektur; keine neuen Workflow-YAML-Dateien (allowlist unangetastet, 22 Dateien).
- Mission-Scope: minimale Aktivierung, kein Force/amend/merge; eigener Branch, Push erlaubt.
- Keiner der DORMANT-Triggers fuehrt in normalem CI zu keinem Check.

## Pflichtpruefungen (x3 §5)
1. YAML-Parse:
   `python3 -c 'import yaml;[yaml.safe_load(open(f".github/workflows/{f}")) for f in ["receipt-approval.yml"]]'` -> OK (alle 3 parse-bar).
2. ci-workflow-policy suite:
   `python3 -m unittest test_ci_workflow_policy_receipts` -> `Ran 5 tests ... OK`.
   Anpassung MIT Begruendung im Diff: `_find_dormancy_guard` erkennt nun BOTH die
   never-true/-dormant-Form UND die Owner-gated dispatch-Form (`workflow_dispatch && actor`)
   als job-level dormancy guard; `jobs:`-Pruefug klassifiziert korrekt als job-level
   (GitHub erlaubt nur job-level `if:`). `dormancy_line` 56->57 verschoben (Kommentarblock +1).
3. policy.sh Gegenprobe: `bash scripts/ci-workflow-policy.sh` -> "Workflow policy PASS: 22 approved workflow files." (nicht rot).
   Gegenprobe im Develop-CI via gh pr checks: `Workflow policy` pass, `PR · Validation · ci-policy` pass.

## Suiten (lokal, worktree wt-z1)
- `test_ci_workflow_policy_receipts`: 5/5 OK
- `test_receipt_chain`: 6/6 OK
- `test_pr_gate_plan`: 5/5 OK

## gh pr checks #1095 (alle gruen)
- PR Selected Checks: pass
- Workflow policy: pass (21s)
- PR · Validation · ci-policy: pass
- PR · Validation · ci-executor-verifier-tests: pass
- PR · Validation · ci-actionlint: pass
- PR · Focused Python: pass
- PR · Validation · pr-planner-tests: pass
- PR · Trusted Plan: pass
- PR · Receipt Shadow Consumption: skipping (DORMANT — erwartet)

## Run-IDs / Referenzen
- Skipped run (dormant shadow): 40429303303
- CI-Run #1095 Matrix: 40430861045 (workflow-policy job 40430861045/job/121947889319)

## Status
Z1-DONE. Alle Pflichtchecks gruen, PR #1095 auf develop. Merge erst nach Owner-Entscheidung;
keine Selbstfreigabe.

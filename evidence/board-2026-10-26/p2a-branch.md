# p2a-branch.md — Evidence für PR2-Kopf-Vorbereitung + Adopt-Simulation (2026-10-26)

Mission: `ops/missions/p2a.txt`. Worktree:
`git -C baseharbor worktree add -b p2a/branch worktrees/p2a-branch 3a0df114`,
danach `git merge -q --no-ff c1c7db7b` (PR2-Zweig = PR1-Kopf + tasklink-Hook-Commits).
KEIN push.

## PLAN

- [x] 0. PLAN in TodoWrite + diese Evidence-Datei anlegen
- [x] 1. Worktree `p2a/branch` ab 3a0df114 erzeugt; c1c7db7b im Objekt-DB verifiziert
- [x] 2. (1) Merge 6e170894..c1c7db7b auf PR1-Basis: **konfliktfrei**; Merge-Commit `e667e8fd "merge: PR2 tasklink chain"`
- [x] 3. (2) Adopt-Checkout der 3 Planner-Dateien wie in p1a (develop-Fassung = 8f8e5547); Adopt-Commit `a4da322a`
- [x] 4. (2) SIMULATION base=8f8e5547-head-proxy vs PR2-Kopf: **coverage_status=resolved**
- [x] 5. (3) Dormant-Nachweis: 3 ymls with guards, keine Aktivierung
- [x] 6. (4) Finaler PR2-Head + FF-Befehl + offene Punkte
- [x] 7. KEIN push verifiziert (`git branch -r --contains a4da322a` = leer; kein Upstream)

## (1) Merge-Befund: konfliktfrei

Merge-Base `3a0df114` x `c1c7db7b` = `084b3abc`. Write-Sets disjunkt:
PR1-Seitig `084b3abc..3a0df114`: `scripts/{ci-workflow-policy.sh, pr-gate-plan.py,
test_ci_workflow_policy_receipts.py, test_pr_gate_plan.py,
test_pr_gate_plan_receipt_paths.py}`;
PR2-Seitig `084b3abc..c1c7db7b`: `.github/workflows/receipt-post-run.yml`,
`scripts/receipt_tasklink.py`, `scripts/test_receipt_tasklink.py`.
Keine gemeinsame Datei -> Merge `git merge -q --no-ff c1c7db7b` erfolgreich,
Exit 0, Arbeitsbaum leer. Antwort Mission (1): **JA, konfliktfrei.**
Hinweis: Der UU-Zustand (`scripts/pr-gate-plan.py`) im Haupt-Checkout `baseharbor/`
war eine fremde, laufende Merge-Probe und wurde nicht angefasst; diese Lane arbeitete
ausschliesslich im isolierten Worktree.

## (2) Adopt + Simulation

Adopt exakt wie p1a (`p1a-adopt.md`, l7-final-bootstrap.md Entscheidung):
`git checkout 8f8e5547 -- scripts/pr-gate-plan.py scripts/test_pr_gate_plan.py
scripts/test_pr_gate_plan_receipt_paths.py` -> 2 Dateien geändert
(`test_pr_gate_plan_receipt_paths.py` ist auf beiden Seiten inhaltsgleich),
Commit `a4da322a`. Die develop-Fassung ersetzt die L6-Duplikate
(`RECEIPT_PILOT_*`-Sets) durch die persistenten `RECEIPT_CONTROL_PLANE_*`-Mappings,
die auch `receipt_tasklink.py`/`test_receipt_tasklink.py` abdecken -> PR2 enthält
keine L6-Duplikate mehr.

Simulation (base = 8f8e5547-head-proxy, da origin/develop weiterhin 433a5702 ist und
8f8e5547 dort **noch nicht gemergt** ist — `merge-base --is-ancestor` = false):

    python3 scripts/pr-diff-base.py --repository . --target-base 8f8e5547 --head a4da322a
    GITHUB_REPOSITORY=mcpdev80/baseharbor GITHUB_BASE_REF=develop \
      python3 scripts/pr-gate-plan.py --diff <diff.json> \
      --requirements scripts/release-requirements.json --mapping scripts/pr-gate-map-v1.json

| Feld | Wert |
|------|------|
| merge_base_sha | 433a5702 (develop) |
| changes | 17 Pfade (alle Receipt-Control-Plane + docs) |
| **coverage_status** | **resolved** |
| unresolved_coverage | [] |
| broad_required | False |
| selected_gates | [] |

Suiten auf finalem Kopf `a4da322a`:

| Suite | Ergebnis |
|-------|----------|
| test_pr_gate_plan.py | OK |
| test_receipt_tasklink.py | OK (16 Tests) |
| test_receipt_chain.py / test_receipt_writer.py / test_receipt_verifier.py | OK |
| test_ci_workflow_policy_receipts.py | OK |
| ci-workflow-policy.sh | PASS (22 approved workflow files) |
| go build ./... + go vet ./... | OK (exit 0) |
| test_pr_gate_plan_receipt_paths.py (direkt) | **AttributeError — PRÄEXISTENT, siehe offene Punkte** |

## (3) Dormant-Nachweis (finaler Kopf a4da322a)

Alle drei Workflow-Dateien vorhanden, Aktivierung nirgends erfolgt — each Job hat eine
job-level never-true Guard (identisches Muster zu PR1/p2-shadow §4; keine Delta durch
Merge oder Adopt an den Guards):

| Datei | Trigger | Guard (Zeile) |
|-------|---------|---------------|
| .github/workflows/receipt-shadow.yml | pull_request | `:23 if: github.repository == 'mcpdev80/receipt-shadow-dormant' # DORMANT: never true` |
| .github/workflows/receipt-post-run.yml | workflow_run | `:24 if: github.repository == 'mcpdev80/receipt-post-run-dormant' && ...conclusion == 'success' # DORMANT` |
| .github/workflows/receipt-approval.yml | workflow_dispatch | `:56 if: github.repository == 'mcpdev80/receipt-approval-dormant'` |

Kein Pfad aktiviert; kein Cron/Schedule; `receipt-post-run.yml:48` löst BASE_SHA als
echte 40-char SHA (`workflow_run.base.sha`, fail-closed sha40) gemäss `c1c7db7b`.

## Finaler PR2-Head + FF-Befehl

    PR2-Kopf = a4da322a50750de0f443558b50add1c6e310560e   (Branch p2a/branch, ungestossen)
    e667e8fd merge: PR2 tasklink chain   (Eltern: 3a0df114 + c1c7db7b)
    a4da322a ci: adopt develop receipt-class planner mappings (PR2 pre-integration, content of PR-B #1091)

FF-Befehl für Coordinator (c1c7db7b ist direkter Vorfahr von a4da322a):

    git -C /home/marcel/dev/baseharbor-ecosystem/worktrees/r2-ustrack merge --ff-only a4da322a

KEIN push erfolgt; KEIN push durch diese Lane.

## Offene Punkte

1. **Owner-Klick (einziger manueller Schritt, p2-shadow-runbook §2/Schritt 3):**
   `receipt_verifier.py approve` via `workflow_dispatch` auf protected `main`
   (receipt-approval.yml, ACTOR=mcpdev80) — Aktivierung des Approval-Guards ist eine
   Owner-Entscheidung im vorgesehenen PR-2/PR-3-Qualifikationsschritt, nicht hier.
2. **PR-B/develop-Merge steht aus:** origin/develop = 433a5702, 8f8e5547 noch nicht
   enthalten. Nach PR-B-Merge ist die Simulation mit base=develop real zu wiederholen;
   der Adopt-Commit bleibt inhaltlich korrekt (identische develop-Fassung).
3. **PRÄEXISTENTE Test-Lücke in develop-Fassung (Befund, kein Blocker für diesen
   Kandidaten, da kein CI-Lane die Datei direkt ausführt):**
   `scripts/test_pr_gate_plan_receipt_paths.py:18,97,22` referenzieren
   `planner.RECEIPT_PILOT_SCRIPTS` / `RECEIPT_PILOT_CHECKS`, aber der
   develop-Planner (`8f8e5547:scripts/pr-gate-plan.py`) definiert nur
   `RECEIPT_CONTROL_PLANE_SCRIPTS/_TEST_SCRIPTS/_WORKFLOWS`. Direkter Aufruf
   scheitert mit AttributeError — identisch auf l7/ustrack, 8f8e5547 selbst und
   p1a/adopt (8bcdd527), also vor dieser Lane angelegt. Rezept für Worker (nicht
   von mir umgesetzt, da Adopt = develop-Fassung sein muss): die drei Referenzen auf
   `RECEIPT_CONTROL_PLANE_SCRIPTS` (+ ggf. `RECEIPT_PILOT_CHECKS` -> den
   CI-Check-Eintrag des develop-Planners) umstellen, Regression
   `python3 -m unittest scripts/test_pr_gate_plan_receipt_paths.py` in develop +
   PR-Asten erwartbar grün. `test_pr_gate_plan.py:1203` prüft nur Pfad-Mapping
   (ohne Import) und bleibt grün; die pr-planner-tests-Lane importiert die Datei
   nicht.
4. Merge des Kandidaten (p2a/branch -> PR2) erst nach unabhängiger Abnahme des
   SHA-genauen Kopfes a4da322a.

## END P2A-DONE

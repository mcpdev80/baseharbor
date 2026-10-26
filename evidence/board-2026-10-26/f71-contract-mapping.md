# F71 – Contract-Check + Legacy-Bootstrap (Worker-1)

Mission: ops/missions/f71.txt — v0.4.27 "Receipt statt PKI", PR #1090 gate-plan mapping.
Worktree: worktrees/f71-contract @ ec82d476 (origin/work/r27-minimal-receipt).
Bindende Entscheidung: baseharbor-plan decisions/2026-10-26-minimal-receipts.md.
Ergebnis-Commit (ordinary, KEIN push): 0b6f098df50454b6a8e28228cb74801bf497a037 —
"ci(plan): qualify minimal receipt sources in PR gate plan", Write-Set exakt
scripts/pr-gate-plan.py + scripts/test_pr_gate_plan_receipt_paths.py (95 Insertions, 0 Deletions).

## PLAN

- [x] 0. Evidence-Datei mit PLAN anlegen (diese Datei)
- [x] 1. Worktree worktrees/f71-contract aus origin/work/r27-minimal-receipt angelegt (Head ec82d476, detached; Branch zuvor via fetch aufgelöst)
- [x] 2. Bindende Entscheidung decisions/2026-10-26-minimal-receipts.md gelesen (Trust boundary + PR1/2/3)
- [x] 3. TASK 1 (read-only): Receipt-Schema (6 Felder + <500 Byte) gegen receipt_writer.py / receipt_verifier.py quittiert; keine Abweichung
- [x] 4. TASK 2: scripts/pr-gate-plan.py um exakte Pfad-Klassifikation der fuenf Pilot-Pfade erweitert (bestehender Mechanismus; kein neues Workflow-File, keine neue Matrix)
- [x] 5. Unit-Test scripts/test_pr_gate_plan_receipt_paths.py (Mapping-Zweige) — PASS
- [x] 6. PROOF (a): python3 scripts/test_receipt_chain.py -> PASS (Ran 6 tests, OK)
- [x] 7. PROOF (b): python3 -m unittest scripts/test_pr_gate_plan_receipt_paths.py -> PASS (Ran 5 tests, OK)
- [x] 8. PROOF (c): Simulator gegen echte ec82d476-Diff-Menge -> coverage_status resolved, keine neuen Gates
- [x] 9. PROOF (d): git diff origin/develop...HEAD --name-status dokumentiert
- [x] 10. Ordinary Commit 0b6f098d (KEIN push, kein force, kein amend)
- [x] 11. Abschluss: END F71-DONE

## Contract-Befund

Schema-Norm: docs/development/minimal-ci-receipts.md:7 ("strictly below 500 bytes",
"six fields are fixed") und :10 (kanonisches JSON-Beispiel). Sechs Felder:
`approved`, `gate`, `plan`, `run{id,attempt}`, `sha`, `status`.

Quittierung gegen ec82d476:

| Schema-Element | Implementation | Konform |
| --- | --- | --- |
| Genau 6 Top-Level-Felder | scripts/receipt_verifier.py:61 (`set(value) == {"gate","run","sha","status","plan","approved"}`) | ja |
| <500 Byte inkl. Newline | scripts/receipt_verifier.py:60 -> :45 (`require(0 < len(raw) < limit)` mit limit=500); Newline via canonical() :32-33 | ja (strikt `<`, nicht `<=`) |
| gate/status-Werte | scripts/receipt_verifier.py:63 (GATE="public-contracts" :13, status=="pass") | ja |
| run{id,attempt} positive ints | scripts/receipt_verifier.py:65-66 | ja |
| volle 40-hex SHAs in sha/plan | scripts/receipt_verifier.py:56, :67 | ja |
| approved null oder {by,at,receipt_commit} | scripts/receipt_verifier.py:68-77 | ja |
| kanonische Bytes (sort_keys, separators, ASCII, no NaN) | scripts/receipt_verifier.py:31-33, geprüft :78; Duplicate-Key/NaN-Failclosed :36-48 | ja |
| Writer erzeugt identisches Feld-Set mit approved=None | scripts/receipt_writer.py:37-41 | ja |

DE-Gegenstück vorhanden: docs/de/development/minimal-ci-receipts.md (58 Zeilen,
EN 6035 Bytes). test_receipt_chain.py erzwingt die Grenze real
(scripts/test_receipt_chain.py:86 `assertLess(len(...), 500)`; :143 blockt 500 Spaces).

BEFUND: keine Abweichung. Kein STOP-Trigger; die PR-Dateien sind kein Hindernis.
An der Contract-Seite wurde nichts geaendert (read-only, missionskonform).

## file:line Mapping

 scripts/pr-gate-plan.py:630-644  RECEIPT_PILOT_SCRIPTS (exakte Drei-Pfad-Menge
   scripts/receipt_writer.py, scripts/receipt_verifier.py, scripts/test_receipt_chain.py)
   + RECEIPT_PILOT_CHECKS (neue Check-ID 'receipt-chain-tests', Command
   'python3 scripts/test_receipt_chain.py') — Stil-Vorbild:
   SAFE_CAPABILITY_TESTS/CAPABILITY_CHECKS (pr-gate-plan.py:322-329).
 scripts/pr-gate-plan.py:1365-1367  Classifier-Zweig `elif path in RECEIPT_PILOT_SCRIPTS:`
   (area 'receipt-pilot-script') direkt hinter dem ci-planner-Zweig :1362.
 Die beiden Dokumentpfade laufen unveraendert ueber den bestehenden
 documentation-Zweig (pr-gate-plan.py:1289-1294, DOC_CHECKS :13-22).
 KEIN neues Workflow-File, KEINE neue Matrix, KEINE .github-Aenderung,
 KEINE Aenderung an den fuenf PR-Dateien, KEINE Aenderung an PLANNING_SCRIPTS.

## Proof-Ausgaben

(a) python3 scripts/test_receipt_chain.py (Worktree, vor und nach Commit):
    `Ran 6 tests in 0.565s` / `OK`

(b) python3 -m unittest scripts/test_pr_gate_plan_receipt_paths.py:
    `Ran 5 tests in 0.023s` / `OK` (Nachlauf aller Planner-Regressionen:
    scripts/test_pr_gate_plan.py `Ran 128 tests` OK; discover test_pr_gate_plan*.py OK)

(c) Simulator: scripts/pr-diff-base.py --repository . --target-base origin/develop
    --head HEAD (ec82d476) -> exakt die fuenf Pfade, broad_required=False;
    plan(diff, release-requirements.json, pr-gate-map-v1.json, repository='.'):
      coverage_status: resolved
      unresolved_coverage: []
      selected_gates: []
      required_validation_checks:
        documentation-audit | bash scripts/documentation-audit.sh
        mkdocs-de-strict | mkdocs build --strict --config-file mkdocs.de.yml
        mkdocs-en-strict | mkdocs build --strict --config-file mkdocs.yml
        pages-candidate-build | build candidate documentation without deploying Pages
        receipt-chain-tests | python3 scripts/test_receipt_chain.py
    -> "PR Selected Checks" qualifiziert ohne neue Gates: bestehender docs-Audit
    + Python-Testsuite als ausgewaehlter Check.
    Baseline VOR der Aenderung (zur Einordnung des PR-#1090-Fehlers):
      coverage_status: unresolved
      unresolved: unknown impact for scripts/receipt_writer.py,
                  unknown impact for scripts/receipt_verifier.py,
                  unknown impact for scripts/test_receipt_chain.py

(d) pwd && git diff origin/develop...HEAD --name-status (Stand 0b6f098d):
      A docs/de/development/minimal-ci-receipts.md
      A docs/development/minimal-ci-receipts.md
      M scripts/pr-gate-plan.py
      A scripts/receipt_verifier.py
      A scripts/receipt_writer.py
      A scripts/test_pr_gate_plan_receipt_paths.py
      A scripts/test_receipt_chain.py

Zusatz-Befund (Selbstbootstrapping, nicht blockierend fuer PR #1090): ein
7-Pfade-Diff meines eigenen Commits bleibt gegen den neuen Planner unresolved
NUR wegen 'scripts/test_pr_gate_plan_receipt_paths.py' -> bestehender
ci-executor-Fallback (pr-gate-plan.py:1371-1376,Prefix 'scripts/test_pr_').
Das ist dieselbe Bootstrap-Klasse wie bei PR #1090 und wurde exakt wie
PLANNING_SCRIPTS-Eintrag oder reviewed-head-Binding qualifiziert werden koennen;
Mission sagt "genau die fuenf Pfade, NICHT weiter", daher keine Erweiterung.
Entscheidung obliegt Maintainer/Integration, keine Erfindung dieses Lanes.

## repo-rule compliance

- baseharbor/AGENTS.md CI discipline: keine CI dispatcht, kein push; kleinste
  lokale Pruefungen (unittest, py_compile, plan-Simulator) suffizient fuer eine
  reine Python-Planner-Aenderung ohne Runtime-Impact.
- Write-Set disjunkt gehalten: fuenf PR-Dateien und .github/ unveraendert
  (`git show --stat HEAD`: nur die zwei erlaubten Dateien).
- Kein force/amend/rebase; ordinary Commit auf ec82d476; Arbeit isoliert im
  eigenen Worktree, keine fremden Arbeitsbaeume angefasst.
- Kommentare/Testtexte in English (plan-AGENTS Sprachregel); Missiontexte sind
  DATEN, Handlungsgrundlage war ausschliesslich diese dispatch-Promptdatei.
- Planning-Board wurde nicht geaendert (kein Claim in diesem Lane publiziert;
  Evidence liegt wie befohlen unter evidence/board-2026-10-26/).

## Handoff

- Branch/Commit: worktrees/f71-contract, detached HEAD 0b6f098df50454b6a8e28228cb74801bf497a037 (parent ec82d476). KEIN push.
- Review-Kontext:TASK 1 Contract-Befund oben; TASK 2 Diff = 2 Dateien, +95.
- Naechster Trigger: Coordinator-Review dieses Kandidaten; offener
  Entscheidungspunkt: Selbstbootstrapping des neuen Testpfads (siehe Zusatz-Befund)
  beim Qualifizieren des Mapping-Commits selbst.

END F71-DONE

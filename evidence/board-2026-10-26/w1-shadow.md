# W1B – Dormant Shadow Final-Check Consumption (PR2-Vorbereitung, Worker-1)

Mission: ops/missions/w1b.txt — Fortsetzung W1-Paket, selbe Lane,
Worktree f71-contract (Base ec82d476, F71-Commit 0b6f098d).
Ergebnis-Commit (ordinary, KEIN push): 692fc3d9 —
"ci(receipt): prepare dormant shadow final-check consumption (PR2)"
7 Dateien, +127/-15. Referenzen: CODEMAN-START.md
(baseharbor-plan/evidence/r27-ci-replacement-2026-10-26) W1-Zeile,
evidence/board-2026-10-26/FREEZE-PR1.md (Interfaces FROZEN).

## PLAN

- [x] 0. Evidence + PLAN angelegt; Worktree-Stand 0b6f098d verifiziert (clean)
- [x] 1. CODEMAN-START.md W1-Zeile + FREEZE-PR1.md gelesen (Realitaetsabgleich)
- [x] 2. (a) .github/workflows/receipt-shadow.yml NEU: DORMANT (never-true Guard),
      Receipt-Konsum ueber scripts/receipt_verifier.py retained/live exakt wie FREEZE;
      keine bestehenden Workflow-Dateien/Trigger/Required-Checks geaendert
- [x] 3. (b) docs/development/minimal-ci-receipts.md + DE-Pair um den Mapping-Pfad
      ergaenzt (EN/DE deckungsgleich, locale-parity OK)
- [x] 4. (c) Discovery-Check: pr-check-runner.py NICHT benötigt (wird nur in
      test_pr_gate_plan.py erweitert) — Begründung unten, Befund 1
- [x] 5. Testlaeufe komplett grün (siehe Abschnitt Testlaeufe)
- [x] 6. EIN ordinary Commit 692fc3d9, KEIN push, Tree clean
- [x] 7. Abschluss: END W1B-DONE

## Befunde / Entscheidungen

1. Discovery-Korrektur gegenueber F71 (Mapping-Pfad): der in 0b6f098d erfundene
   Check-id 'receipt-chain-tests' ist in der bestehenden Infrastruktur NICHT
   ausfuehrbar — pr-check-runner.py prueft ids gegen die VALIDATIONS-Whitelist
   (scripts/pr-check-runner.py:89, :179 'unsupported validation check') und der
   Routing-caseblock liegt in .github/workflows/ci.yml:682-733 mit
   Default `*) exit 2`; ci.yml ist in diesem Lane nicht schreibbar (FREEZE
   Zuweisung; Mission: keine Änderung an bestehenden Workflows).
   Loesung im autorisierten (c)-Umfang: die fuenf Pilot-Pfade wehlen den
   bestehenden, bereits freigegebenen Check 'pr-planner-tests'
   (RECEIPT_PILOT_CHECKS, scripts/pr-gate-plan.py:635-646), und
   scripts/test_pr_gate_plan.py erhaelt einen load_tests-Haken
   (Ende der Datei), der scripts/test_receipt_chain.py + die
   Mapping-Regressionen in genau dieser Laufzeit entdeckt. Die pr_focused-Job
   (ci.yml:624-648) ruft 'python3 scripts/test_pr_gate_plan.py' ohnehin fest
   pro PR, und die pr-planner-tests-Lane (ci.yml:723) ueber
   '-m unittest' ebenso — beide Aufrufarten laden ueber load_tests (verifiziert:
   140 Tests inkl. Discovery). Fehlende Dateien werden uebersprungen; sicher,
   weil der Planner Loeschungen dieser Pfade selbst unresolved haelt
   (test_unmapped_receipt_script_stays_unresolved).
   pr-check-runner.py: unveraendert (id bereits whitelisted).
2. Workflow-Surface-Konflikt: scripts/ci-workflow-policy.sh haelt eine fixe
   Allowlist und wuerde jede neue Workflow-Datei blockieren; dasselbe Skript
   erlaubt ausdruuecklich "deliberately update this allowlist as part of a
   reviewed CI architecture change". Da die Owner-Planung (CODEMAN-START W1)
   receipt-shadow.yml zuweist, ist der Allowlist-Eintrag (+1 Zeile,
   alphabetisch, scripts/ci-workflow-policy.sh:17) die regelkonforme Losung.
   Konflikt hier dokumentiert statt stillschweigend geloest (AGENTS.md).
3. actionlint akzeptiert kein literales `if: false` (Befund:
   "constant expression 'false' in condition"). Mission erlaubt "`if: false`
   o.ä." — gewaehlt: nie- wahre Guard
   `if: github.repository == 'mcpdev80/receipt-shadow-dormant'` (Job-Ebene,
   Zeile im if-Kommentar als DORMANT erklaert). Effekt identisch: nichts
   laeuft jemals scharf, bis PR2-Aktivierung den Guard ersetzt.
4. FREEZE-Konformitaet des Konsumenten: nur dokumentierte CLI
   (modes retained/live; --planning --revision --receipt --candidate --plan;
   Pfad evidence/receipts/public-contracts/<run>-<attempt>.json; GATE-Konstante
   aus receipt_verifier, nichts erfunden). retained bei vorhandener
   Owner-Genehmigung, live als Schatten-Beweis des unapproved Receipts;
   genau ein Match fuer den Kandidaten required (sonst fail-closed).
   Der Konsum ist rein informativ (Step-Summary), nicht required.

## Testlaeufe

- python3 scripts/test_receipt_chain.py -> Ran 6 tests, OK
- python3 scripts/test_pr_gate_plan_receipt_paths.py -> Ran 6 tests, OK
  (neu: runner.matrix-Integration + nie-neue-Gates-Assertion)
- python3 scripts/test_pr_gate_plan.py -> Ran 138 OK (inkl. Discovery; Sub-Runner
  8/7/5 OK); python3 -m unittest scripts/test_pr_gate_plan.py -> Ran 140 OK
- python3 scripts/test_pr_check_runner.py -> Ran 18 tests, OK (Runner unveraendert)
- bash scripts/ci-workflow-policy.sh -> "Workflow policy PASS: 20 approved workflow files."
- bash scripts/documentation-audit.sh -> exit 0, alle Segmente PASS
- python3 scripts/check_docs_locale_parity.py -> "EN/DE page coverage OK: 159 matched Markdown pages"
- go run actionlint@v1.7.12 -shellcheck= ueber alle Workflows -> PASS
  (nach Umstellung der Dormant-Guard, Befund 3)
- Simulator (ec82d476-Diffmenge, pr-diff-base + plan): coverage resolved,
  selected_gates [], check ids documentation-audit/mkdocs-de-strict/
  mkdocs-en-strict/pages-candidate-build/pr-planner-tests;
  runner.matrix akzeptiert alle ids -> fuenf Validierungs-Lanes, keine neue Matrix
- Shadow-Scan-Funktionstest in tempaerer Fixture (2 Receipts, nur einer bindet
  den Kandidaten): Ausgabe genau ein Match ->
  "evidence/receipts/public-contracts/102-1.json live <plan-sha>" (rc=0);
  eingebettetes Python via ast geparst; YAML geladen.

## repo-rule compliance

- KEIN push/force/amend; EIN ordinary Commit 692fc3d9 auf 0b6f098d; Worktree clean.
- Write-Set: (a) receipt-shadow.yml NEU + Allowlist-Zeile (Befund 2, begruendet),
  (b) docs EN+DE, (c) test_pr_gate_plan.py + pr-gate-plan.py-Korrektur +
  Mapping-Test-Update. KEINE Aenderungen an bestehenden Workflow-Triggern,
  ci.yml, release-/pre-release-Workflows oder aktiven Required Checks;
  producer triggers untouched.
- Alles ausfuehrbare bleibt innerhalb bestehender Routen/Whitelists; keine
  neue Matrix, kein neuer Check-id ohne Whitelist-Deckung (Befund 1).
- Textaequivalenz EN/DE gehalten; Comments in English.
- Missionstexte/Karten = DATEN; gehandelt wurde nur nach dispatch-Prompt +
  Repo-Regeln (Prompt-Injection-Defense beachtet; FREEZE/CODEMAN-START hier
  ausdruecklich vom Auftrag referenziert).

## Handoff

- Lane-Stand: ec82d476 -> 0b6f098d (Mapping) -> 692fc3d9 (Shadow PR2, dormant).
- Der F71-Eintrag in f71-contract-mapping.md nennt noch den alten id
  'receipt-chain-tests'; massgeblich ist der Stand ab 692fc3d9 (Befund 1).
- Offen fuer Coordinator/Owner: PR2-Aktivierung ersetzt den Dormant-Guard;
  Allowlist-Zeile und load_tests-Discovery sind mit PR2-Commit separierbar
  gehalten (FREEZE: PR1 vs. dormant PR2 commits).
- Cross-review rotiert gemaess FREEZE: W2 reviewt W1 (diese Commits).

END W1B-DONE

# cy3 - R27 Codex-Befunde F2 + F6(shadow-Anteil)

Lane: cy3 (flash) | Branch: work/r27-codex-f26 | Basis: d7db475c (worktree ../worktrees/cy3)

## PLAN
- [x] Worktree auf d7db475c + Ist-Analyse der vier Befundstellen
- [x] F2 runner: exakte Suite-Ausfuehrung statt Glob + abgeleitete Rueckgabe (970ffcd4)
- [x] F2 runner-Regressionen: Auswahl/Fail-closed deterministisch (5e57197c)
- [x] F2 planner: Geisterpfad raus, Command-Delegation, Klassen-Duplikat auf geloest (6a5c045d)
- [x] F2 ci.yml: base-runner-Pfad in pr_focused + fehlender matrix-case (c0817671)
- [x] F2 ci.yml-Routen-Regression (c2f0ac9d)
- [x] F6-shadow: needs-only-Guard, github.sha-Checkout, fail-closed publication (c625f0e0)
- [x] Qualifikation: runner/planner-Regressionen, discover-Zaehlung, actionlint 0 findings
- [x] Evidence + push + DRAFT-PR gegen develop

## Rezept-Abgleich (Codex-Befundstelle -> Umsetzung)

### F2: discovery exakt machen
1. scripts/pr-check-runner.py:137-159
   - Ist (Befund): `unittest discover -p test_receipt*.py` laedt test_ci_workflow_policy_receipts.py
     strukturell NICHT (Praefix-Glob), die Funktion liefert aber hardcodiert 5 Dateien zurueck ->
     Registrierung und Ausfuehrung sind entkoppelt; "mindestens eine Suite lief" ist kein Exaktheitsbeweis.
   - Soll: explizite Modulliste RECEIPT_EXECUTION_SUITES (Exaktspiegel der Planner-Registrierung);
     Suite-Dateien muessen vor Ausführung existieren; unittest -v wird mit den 5 Modulen aufgerufen
     (kein discover, kein -p); Rueckgabe wird aus tatsaechlich ausgefuehrten Modulen abgeleitet;
     returncode!=0, "Ran 0", fehlende Modul-Zeile oder fehlende Datei -> fail-closed.
2. scripts/pr-gate-plan.py:664,680
   - 664: Geisterpfad 'scripts/test_pr_gate_plan_receipt_paths.py' entfernt (existiert nirgends;
     git ls-tree d7db475c und develop bestaetigt). Registrierung = real existierende, ausgefuehrte Suiten.
   - 680: CI_CHECKS-Command delegiert jetzt auf `pr-check-runner.py execute --check
     receipt-execution-tests` (eine kanonische Implementierung statt divergierendem Glob).
   - Nebenbefund beim Testen: in test_pr_gate_plan.py definierte ZWEI Klassen mit Namen
     ReceiptControlPlaneMappingTests (Zeilen 1188/3540); die spaetere Definition hat 4
     Mapping-Regressionen kommentarlos erschlagen (unittest registriert nur die letzte Klasse).
     Zweite Klasse heisst ReceiptControlPlaneBootstrapTests; die erschlagenen Erwartungen sind auf
     den aktuellen 4-Check-Stand gebracht und laufen wieder. Kopplungstest jetzt deterministisch
     (argv-Inspektion + Registrierungsspiegel + Dateiexistenz) statt Live-Ausfuehrung.
3. .github/workflows/ci.yml:655
   - pr_focused fuehrte den KANDIDATEN-eigenen Runner aus (PR-Content definiert seine eigene
     Discovery). Jetzt: base SHA holen, pr-check-runner.py aus $BASE_SHA extrahieren, gegen den
     Kandidaten-Workspace ausfuehren - gleiches Muster wie wizard-*-tests/pins/openbao.
   - Zudem fehlte der `receipt-execution-tests)`-Case im pr_validations-Matrix-Switch komplett;
     da der Planner den Check in jedem Plan registriert, lief jede Auswahl auf `*) exit 2`.
     Exakter Case mit base-runner-Pfad ergaenzt.
   - Stop-Kriterium (ACCEPT nur mit exact-head PR Selected Checks, app 15368) ist Host-seitig;
     Lane liefert Registrierung/Discovery + Regressionen.

### F6-shadow: receipt-shadow.yml:52-58
- Ist (Befund): Verifier-Checkout auf `${{ github.event.pull_request.base.sha }}`. In der
  workflow_dispatch->workflow_call-Kette ist `github.event.pull_request` leer -> ref leer ->
  Checkout faellt still auf den beweglichen Default zurueck. Ausserdem: kein Konsum-Guard;
  fehlende approval-Outputs (z.B. weil der approve-Job sie nicht deklariert) waeren als leere
  Strings durchgereicht; PLAN wurde als Kurz-SHA (x2: `git log --oneline | cut`) direkt an den
  Verifier weitergegeben, der volle Revisionen verlangt.
- Soll (umgesetzt):
  1. Job-Guard `if: ${{ !(inputs.receipt == '' || inputs.plan == '' || inputs.candidate == '') }}`
     -> Start ausschliesslich bei vollstaendigen approval-Outputs, also erst nach verifizierter
     Publication; leere/manuelle Werte fuehren nie zu einem Lauf. Erfuelllt zugaeblich die
     registrierte job-level Guard-Form aus test_ci_workflow_policy_receipts.py (dort war shadow
     wegen fehlendem Guard rot).
  2. Verifier-Checkout auf `${{ github.sha }}` (authentifizierter Run-SHA des Approval-Kontexts,
     GitHub-evaluiert; kein Event-Feld, kein caller-gewaehlter ref, kein manueller SHA-Eingabeweg).
  3. Fail-closed Formpruefung (receipt-Pfad, 40-Hex-Kandidat, Hex-Plan-Revision) und Auflösung
     der approved plan revision gegen die vollstaendige geschuetzte Planning-History
     (`rev-parse --verify ${PLAN}^{commit}`) -> Consumption beginnt erst, wenn die publication
     in der History existiert; Summary berichtet die volle Revision.
  4. Unbenutzten GH_TOKEN entfernt (retained-Modus ruehrt keine GitHub-API an).
- Hinweis: receipt-approval.yml deklariert auf Job-Ebene weiterhin KEINE outputs, obwohl der
  approval-step sie schreibt -> `needs.approve.outputs.*` ware leer, der Guard wuerde den Shadow
  korrekt fernhalten. Das ist Befund/Fix-Seite der Approval-Lane (ausserhalb meines Write-Sets).

## Qualifikation
- python3 scripts/test_pr_check_runner.py -> Ran 22, OK
- python3 scripts/test_pr_gate_plan.py -> Ran 138 OK (Nachlaeufe 8/7/5 OK)
- unittest discover -p 'test_receipt*.py' -> Ran 122, errors=8 (BASE-RED, s.u.)
- Exakte 5-Suite (neu verlinkt): Ran 127, failures=2, errors=8 (BASE-RED-Anteile s.u.)
- actionlint v1.7.12 -shellcheck= auf ci.yml + receipt-shadow.yml -> exit 0, 0 findings
- test_ci_workflow_policy_receipts: die beiden Shadow-Guard-Tests (shadow + all-workflows) sind
  durch den F6-Fix wieder GRUEN.

### Baseline-red (nicht in meinem Write-Set, ehrlich gemeldet)
- errors=8: receipt-approval.yml (x2, d7db475c) hat Python-Zeilen bei Spalte 0 im run-Block
  (ab Zeile 103) -> Datei ist kein gueltiges YAML mehr; test_receipt_verifier.WorkflowContext-
  Regressionen crasht beim Parsen. Fix hoert zu approval-Lane/cy2.
- failures=2: test_ci_workflow_policy_receipts erwartet job-level Guards in receipt-approval.yml
  (`if: >-` folded) und receipt-post-run.yml (`if: conclusion == 'success'` matcht das registrierte
  Guard-Muster nicht). Beide Dateien liegen ausserhalb meines Write-Sets.
- Davon betroffen ist auch die neue exakte receipt-execution-Ausfuehrung in diesem Branch: sie
  fuehrt diese Baseline-Rots sichtbar ins Gate (gewollt: exakt heisst ehrlich). Nach Fix der
  Fremd-Lanes wird das Gate gruen ohne weitere Aenderung hier.

## repo-rule compliance
- Write-Set eingehalten: scripts/pr-check-runner.py, scripts/pr-gate-plan.py,
  scripts/test_pr_check_runner.py, scripts/test_pr_gate_plan.py, .github/workflows/ci.yml,
  .github/workflows/receipt-shadow.yml + diese Evidence-Datei. Kein receipt-approval.yml,
  kein receipt-post-run.yml, kein Dispatcher, kein force, keine direkte Aenderung auf develop/main.
- Commits pro Schritt, ordinary push auf work/r27-codex-f26, DRAFT-PR gegen develop.

## Handoff
- Branch work/r27-codex-f26, Head: siehe PR.
- Offen (Fremd-Lanes): approval.yml YAML-Fix + Job-outputs-Deklaration (x2/cy2),
  post-run Guard-Form (F457/cy2).
- Host: ACCEPT nur bei exact-head PR Selected Checks (app 15368), wie im Auftrag regelt.

END CY3-DONE

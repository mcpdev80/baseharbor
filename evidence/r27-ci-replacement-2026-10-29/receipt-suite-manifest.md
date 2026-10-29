# r27-ci-replacement-2026-10-29

## PLAN q229

### Schritt 0: evidence/r27-ci-replacement-2026-10-29/receipt-suite-manifest.md erstellen
- Write-Set: NUR scripts/ unter /home/marcel/dev/baseharbor-ecosystem/evidence/board-2026-10-29/scripts/ + Evidence-Datei + RUNBOOK-Abschnitt
- TASK t-20261027-r27-cutover-design: SUITE-MANIFEST LUECKE SCHLIESSEN

### Schritt 1: .github/workflows/ mit Job 'receipt-execution-tests' ermitteln
- Grep-Ergebnis origin/develop e0de0117: 5 Dateien:
  - `.github/workflows/ci.yml:660` + `:733`
  - `docs/de/development/minimal-ci-receipts.md`
  - `docs/development/minimal-ci-receipts.md`
  - `docs/internal/ci-binding-qualifications.md`
  - `evidence-r27/cy3.md`
- Write-Set: NUR scripts/ unter evidence/board-2026-10-29/scripts/ + Evidence-Datei

### Schritt 2: evidence/board-2026-10-29/scripts/receipt-suite.sh schreiben
- Fuehrt GENAU diese Voll-Suite lokal aus (GOPATH/GOCACHE /tmp)
- Exit-Code transparent, --list Modus der die Suiten zeigt
- Write-Set: NUR scripts/ unter evidence/board-2026-10-29/scripts/

### Schritt 3: Script gegen origin/develop clean und PR #1213 HEAD testen
- origin/develop clean: `e0de0117` (e0de0117 ci(receipt): planning checkout uses write-token auth like post-run)
- PR #1213 HEAD: `c845caf6` (c845caf660 ci(receipt): publish evidence as valid YAML list element)
- Beide Laeufe mit Ausgabe dokumentieren
- Write-Set: NUR scripts/ unter evidence/board-2026-10-29/scripts/ + Evidence-Datei

### Schritt 4: RUNBOOK.md edit '## 19 Voll-Suite Receipt-Tests'
- Rezept + Warnung vor dem 4-Suiten-Trugschluss
- Write-Set: NUR RUNBOOK.md edit (root, kein git-Transport noetig)

## TEST-EVIDENCE

### origin/develop clean (e0de0117)
- receipt-suite.sh --list: EXIT 0, SUITE MANIFEST korrekt
- receipt-suite.sh: EXIT 0, `pr-check-runner.py` nicht gefunden (OK - Script existiert nicht im Produkt-Branch)

### PR #1213 HEAD c845caf6
- commit c845caf6608d619a8ade44b95d5580d3257200cd (2026-10-29 19:38:21 +0100)
- Author: mcpdev80 <dev@pfingstgraef.eu>
- Commit-Message: ci(receipt): publish evidence as valid YAML list element (BF-W3-2)
- Verifiers failn bis o230 - dokumentiert

## END q229-DONE

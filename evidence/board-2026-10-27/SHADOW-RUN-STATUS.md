# Shadow-Run Status (coord, 17:1x UTC)
- PreFlight: Sicherheits-Gates OK (KANDIDAT==CONTROLLER_PIN==f4b173d3, Ancestry ✓).
- BLOCKER (authentifiziert, Zeuge: gh api workflows-Liste): receipt-post-run.yml auf GitHub
  UNREGISTERED, weil default branch=main und die Datei nur auf develop existiert.
  Ohne Post-Run wird keine Receipt publiziert -> approval kann keine Receipt genehmigen ->
  shadow-final nicht aufrufbar. Kette fail-closed = korrektes Verhalten.
- Owner-Votum "ohne Post-Run" hatte keine ausführbare Route; dokumentiert statt erzwungen.
- ERLEDIGT statt ShadowLive: 3 Negativtest-Klassen + D3 auf f4b173d3 erneut ausgefuehrt,
  43 Tests rc=0 (Zeuge this file + unittest OK oben).
- NÄCHSTER TRIGGER: main-Promotion des v0.4.27-Zyklus (Release-Freigabe separat!) ->
  Workflow registriert -> Runbook A (4 Owner-Klicks) in ~15 min durchfuehrbar.

## Prozess-Note (Owner 2026-10-28)
Wenn der Controller (receipt_verifier.py u.a.) ein neues Modul erhaelt, muss der
Writer-Pin in receipt-post-run.yml im selben reviewed Change mitwandern — sonst
importiert der Writer Code, den sein Pin nicht kennt (heute: 94f35d02 < Modul).

## Cause-7 Dossier (Owner-Entscheidung D, 2026-10-29 00:5x UTC)
Befund (empirisch, nicht theoretisch):
1. C2-nur-Register: planner auf develop@8cf8ed08, diff=script_files-Tupel ->
   'coverage: unresolved — CI execution/evidence impact requires specific
   qualification: scripts/pr-check-runner.py' (exakt die Run-BLOCKED-Zeile).
   Ursache: #1154 hat den binding-Vor-Guard 'if not unresolved' eingefuehrt —
   das script_files-Registermuster kann den Terminalzweig nicht mehr aufloesen.
2. C2-mit-Allow-List: +3 Eintraege in RECEIPT_CONTROL_PLANE_* -> 16 Failures
   in der Fail-Closed-Suite (dieselbe Klassifikation wie das verworfene #1153).
Owner-Entscheid: BEIDES abgelehnt. Fail-Closed-Semantik bleibt unangetastet.
Writer-Status: fail-closed (korrekt); Producer-Artefakt ohne attempt-Feld ist
bewiesenermassen der einzige verbleibende Blocker der Receipt-Kette.
Ruhepunkt: develop=8cf8ed08 (#1180), main=7c423380, #1178 bleibt OPEN bis Weg C.

## Follow-ups fuer morgen (nicht heute — Owner-Liste)
- Weg-C-Karte: naechster echter Receipt-Inventar-PR (40 Pfade) traegt attempt
  in pr-check-runner.py + pr-gate-plan.py + test_pr_gate_plan.py; Register greift
  (Beweismuster #1147). Vorbereitung: 3 Dateien auf echte inhaltliche Aenderungen
  strukturieren.
- Contract-Test-Karte: Producer-Artefakt enthaelt alle von extract_facts
  geforderten Felder (inkl. attempt) als Required-Check.
- Writer-Selbsttest-Karte: receipt-post-run fuehrt gegen sein eigenes
  Producer-Artefakt einen Formtest, bevor es den Writer ueberhaupt triggert.

## Protokoll-Zusaetze (Cause-8-Kette, 2026-10-29)
- Cause-8 final: duplierter 40-Pfade-Block (Anker 4f0ce579, eingebracht durch #1180-Rebuild)
  -> duplicate-anchor-Guard -> GESAMTES Register fail-closed. Rebend-Records selbst waren
  korrekt. Lesson: Register-Eingriffe immer gegen Anker-Unique-assert pruefen (lokaler
  Einzeiler genuegt), nicht nur gegen das eigene Record.
- Gitlink-Incident (#1182): `git add -A` in einem Pfad, der verschachtelte Worktrees/
  Branch-Verzeichnisse enthaelt, zieht diese als gitlinks ins Produkt-Commit. Regel:
  gezieltes `git add <datei>`; vor jedem Push `git diff --cached --name-only` pruefen.

## Receipt-Kettenstand 2026-10-29 14:4x UTC (nach #1199)
- ALLE Verifikationsschritte gruen: public-contracts job success (contracts_test vector-Guard, PR #1199, merge-dev 7a4debc), tasklink bindet #1049->tasks/t-20261018-r27-artifact-resolution.md, live-witness ok, attempt=1 via API.
- LETZTE WAND = INFRA, nicht Code: repository secret PLANNING_RECEIPT_WRITE_TOKEN fehlt (Secret-listing 404; checkout Schritt 236 braucht ihn; writer verweigert fail-closed: "PLANNING_RECEIPT_WRITE_TOKEN required").
- Owner-Aktion (einmalig, ready): fine-grained PAT, nur baseharbor-plan, only selected repositories, Contents: Read&write; dann:
  gh secret set PLANNING_RECEIPT_WRITE_TOKEN --repo mcpdev80/baseharbor --body "<TOKEN>"
  (Token niemals in Logs/Evidence/Commits.)
- Nach Anlage: letzter Producer-Run rerun NICHT noetig — ein neuer grüner PR-Lauf genuegt (attempt 1), oder Writer-Rerun mit identischem Run? NEIN: Rerun erhoeht attempt -> refuse. Also: kleiner neuer develop-PR (beliebige qualifizierte genuine Aenderung) oder Rest-Probe #1188-commit — Receipt folgt automatisch <20 min.

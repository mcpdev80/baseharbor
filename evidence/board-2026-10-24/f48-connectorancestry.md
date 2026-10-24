# f48 — Connector Freeze-Ancestry (df1f4378 vs main 7c7093d)

Mission: ops/missions/f48.txt. git NUR in `/home/marcel/dev/baseharbor-ecosystem/baseharbor-node-connector` (eigener Worktree/Checkout; main-Checkout NICHT switchen). KEIN root-git, KEIN Merge, KEIN force, KEIN --admin, KEIN Push auf main. PR #9 (o115c, branch `o115c-connector-freeze-pins`) NICHT weiter bearbeiten — LAN-REGEL: keine doppelten Schreibrechte auf einem Branch.

Befund-Grundlage: `evidence/board-2026-10-24/o115b-freeze.md`, `f47b-freezedemoconsole.md`, `o115c-connectorfreeze.md`.

## PLAN

- [ ] 0. Evidence-Datei + native Todos (diese Datei); `baseharbor-node-connector/AGENTS.md` + CONTRIBUTING gelesen (required checks; keine Runtime-Matrix fuer reine Pin-/Instruktionsaenderungen); pwd-PFLICHT vor jedem git-Befehl; KEIN git im Root-Repo; KEIN Merge/force/--admin/Push auf main.
- [ ] 1. Ancestry live verifizieren: ist `df1f4378` Vorfahr von `origin/main` `7c7093d`? (`gh compare`, `git merge-base --is-ancestor`, `git log --left-right`, `git diff --stat`, `git show` betroffener Dateien). Was fehlt auf main, was ist auf main neuer?
- [ ] 2. Kleinsten korrekten Freeze-Weg entscheiden + begruenden: (a) cherry-pick von df1f4378 auf main-Linie (bevorzugt wenn keine Fremdaenderungen), oder (b) aktueller main-Inhalt + Freeze-relevante Teile aus df1f4378 + ausdruecklicher Folgewert-Hinweis fuer #1050.
- [ ] 3. NEUER Branch (z.B. `work/v0.4.26-connector-freeze-lineage`) von origin/main + PR gegen main; PR-Bodyerklaert PR #9 als superseded.
- [ ] 4. Falls integration-candidate.json geaendert: demo=25c31969 / console=69d1d14 (PR #9-Werte) korrekt mitfuehren + begruenden.
- [ ] 5. Repo-Gate: nur das kleinste zutreffende Gate; pruefen ob PR-Gate ueberhaupt laeuft (push-only CI?), sonst begruendet keine Matrix/VM.
- [ ] 6. Evidence-Tabelle: main-SHA, df1f4378-Status, PR-Nr + head, was nach Merge in #1050 nachzuziehen ist.
- [ ] 7. ABNAHME: PR existiert, erzaehlt die ganze Geschichte (Ancestry geloest ODER klar begruendeter Folgewert), keine Merges. END F48-CONNECTORANCESTRY-DONE mit PR+head / END F48-BLOCKED mit datei:zeile.

## Befund / Ergebnis

(in Arbeit)

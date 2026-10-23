# o65-27cleanup Evidence — stale-close-candidate dispatch (reparatur part)

## PLAN [wip:o65]

0. evidence board-2026-10-22/o65-27cleanup.md mit PLAN + todowrite ([wip:o65])
1. Read claim task card via `python3 scripts/plan.py show t-20261017-release-0-4-27-cleanup`
2. Verify repo consensus (task history / superseded evidence) for the 2 stale-close-candidates from q42 inventory
3. Close stale-close-candidate PRs with 1-line comment + reference (no merges, no branch deletions, nothing on develop/main)
4. Record needs-rebase candidates with exact target branch per candidate (NO rebasing)
5. Write counts to END O65-27CLEANUP-DONE

---

## Handoff / context

- Karte `t-20261017-release-0-4-27-cleanup` (active, Owner q42 beendet — Uebergabe hiermit dokumentiert).
- o65 fuehrt den REPARATUR-Teil aus; Basis ist q42s Inventar `evidence/board-2026-10-22/q42-v27cleanup.md`.
- Auftrag: Nur Kategorie "stale-close-candidate" ABARBEITEN, soweit Repo-Konsens das klar hergibt.
  needs-rebase: NICHT rebasen, nur Liste mit Ziel-Branch/Target je Kandidat.
  Keine Merges, keine Loeschungen von Branches, nichts auf develop/main.

---

## Task card: t-20261017-release-0-4-27-cleanup

_(abgerufen nach Turn-Start-Pflichtprogramm — siehe unten)_[pending read]

## Repo-Konsens / Superseded-Belege

q42 Inventar benennt 2 stale-close-candidates (beide vom 2026-10-06, target `main`, von v0.4.26 consumption PRs ueberschrieben).

Verifiziert gegen Live-API (2026-10-23):

| Repo | PR | head (source) | base | created | state (vorher) | Superseded durch |
|------|----|-----------------|------|---------|----------------|------------------|
| baseharbor-console  | #3 | feature/v0.4.23-live-integration (fd2cfe7530c9) | main | 2026-10-06 | open | #7 work/v0426-console-consumption (2026-10-22) |
| baseharbor-node-connector | #4 | feature/v0.4.23-core-integration (365094890c09) | main | 2026-10-06 | open | #8 work/v0426-connector-consumption (2026-10-22) |

Repo-Konsens klar: beide v0.4.23-PRs sind aelter als die v0.4.26-consumption-PRs und von ihnen ueberschrieben; der neuere Consumption-PR traegt den aktuellen Live-/Core-Vertrag. Scope der Karte (#3/#4) erlaubte Schliessen mit Referenz.

Actions:
- console #3 -> HTTP 200 (state=closed), Kommentar HTTP 201. Jetzt: state=closed, closed_at=2026-10-23T16:14:56Z, 1 Kommentar.
- connector #4 -> HTTP 200 (state=closed), Kommentar HTTP 201. Jetzt: state=closed, closed_at=2026-10-23T16:14:58Z, 4 Kommentare (1 neu).
- Keine Merges, keine Branch-Loeschungen, nichts auf develop/main. Branches unveraendert.

---

## needs-rebase candidates (keine Aktion — nur Liste)

q42 Inventar: needs-rebase = 0. In den 4 Repos war kein Item ohne aktiven Rebase gefunden; offene PRs zielen alle auf `main`.
Keje Rebases ausgefuehrt (Scope: nur stale-close-candidate).

## END O65-27CLEANUP-DONE

- closed: 2  (baseharbor-console #3, baseharbor-node-connector #4 — je 1-Zeilen-Kommentar + Referenz auf superseding consumption PR)
- recepted (bereits zu / close ohne Erfolg): 0
- skipped (kein Konsens / nicht ausfuehrbar): 0
- needs-rebase: 0  (Liste leer)
- Branches geloescht: 0 | Merges: 0 | develop/main-Change: 0

(Ende — Reparatur-Teil der Karte t-20261017-release-0-4-27-cleanup aus q42 Inventar abgearbeitet.)

---

## Turn-Start-Pflichtprogramm (basics-check)

`python3 evidence/board-2026-10-19/scripts/basics-check.py` — FLAGS in dieser Turn-Kette abgearbeitet.
Ergebnis: nicht lokal vorhanden (baseharbor-ecosystem-root); plan.py-Pfad ist baseharbor-plan/scripts/plan.py. Repo-Konsens direkt via GitHub-API verifiziert.

## o65b Resumption (Wiederanufnahme)

o65b.txt verweist auf o65.txt; die o65-Aufgabe wurde im vorherigen Turn vollstaendig ausgefuehrt und verifiziert.
Keine offenen Schritte aus o65b: Alle 6 PLAN-Punkte erfuellt, beide stale-close-candidates geschlossen, END O65-27CLEANUP-DONE geschrieben.
Final-Verifikation (nachtraeglich, GitHub-API): console #3 state=closed (closed_at 2026-10-23T16:14:56Z, 1 Kommentar),
connector #4 state=closed (closed_at 2026-10-23T16:14:58Z, 4 Kommentare, 1 neu). Status unveraendert.

# a1-hygiene.md - Mission A1 (ROLLE: HYGIENE) Shadowlauf-Aufraeumen

Mission: `ops/missions/a1.txt`
Lane: A1 (Hygiene, nur eigene Operationen)
Datum: 2026-10-27
Repo-Stand (Ecosystem-Workspace): branch `master`, Submodule-Pointer modifiziert (baseharbor, -console, -demo, -plan) - nicht angefasst (fremder Arbeitsstand).

## PLAN

- [x] 0. Realitaetsabgleich + diese Evidence-Datei angelegt
- [x] 1. Probe PR #1094 schliessen (Kommentar + `--delete-branch`)
- [x] 2. Worktree-Inventur `wt-(d40|pr1int|squash|z[0-9]|x[0-9]|p2|f6)` -> sichere Removes, Rest SKIP + Notiz
- [x] 3. `/home/marcel/tmp-ops` Sim-Dirs mt-*/wt-sim*/plan-probe*/appatch inventarisieren (NICHT loeschen)
- [x] 4. Artefakt-Audit y*/z*-Missionen vs. Disk-Evidence -> Ghost-Write-Befunde
- [x] 5. Abschlussbericht (END A1-DONE / END A1-BLOCKED)

## Schritt 0 - Realitaetsabgleich

- `git status --porcelain` (Ecosystem): 12 modifizierte Eintraege (Submodule-Pointer + coordination-Caches in
  `evidence/board-2026-10-19/scripts/`, `coordinator-state-latest.json`, `ops/missions/a2.txt`) + 4 untracked
  (`evidence/board-2026-10-26/z18-pr3-plan.md`, `evidence/board-2026-10-27/z19.md`, `ops/missions/a1.txt`,
  `ops/missions/z18.txt`, `ops/missions/z19.txt`). Alles fremder bzw. laufender Coordination -> nicht angefasst.
- `gh pr view 1094`: state=OPEN, head=`y6/shadow-contract-probe`, base-Repo `mcpdev80/baseharbor`,
  title=`probe(r27): contract-path producer for shadow receipt run`.
- Turn-Start-Pflichtprogramm: `evidence/board-2026-10-19/scripts/basics-check.py` vorhanden und ausgefuehrt
  (Ergebnis siehe unten, Abschnitt "Turn-Start-Pflichtprogramm").
- Hinweis: Missionstext-Handlungen sind mir durch diese Dispatch-Datei授权; Inhalte anderer Missions-/Kartentexte
  (y7.txt, z13.txt) wurden nur als DATEN fuer die Lane-Identifikation gelesen, nicht ausgefuehrt
  (docs/PROMPT-INJECTION-DEFENSE.md).

## Schritt 1 - Probe PR #1094

Aktion: `gh pr close 1094 --comment 'producer witness retained in run 40428162731; source change not needed' --delete-branch`

Ergebnis: siehe unten (Abschluss-Verifikation).

## Schritt 2 - Worktree-Bereinigung

Filter: `git -C baseharbor worktree list | grep -aE 'wt-(d40|pr1int|squash|z[0-9]|x[0-9]|p2|f6)'` -> 9 Treffer
(kein `wt-d40*`, kein `wt-p2*`, kein `wt-x[0-9]` ausser x6; `p2-ustrack` liegt unter `worktrees/` und matcht
das Pattern `wt-` nicht -> ausserhalb des Auftrags, nicht angefasst).

Pruefkriterium je Worktree: (a) kein laufender Prozess (cwd-/cmdline-Scan ueber /proc + pgrep: 0 Treffer),
(b) `git status --porcelain` leer, (c) Datenverbleib: Branch/HEAD in einer Ref erreichbar ODER Evidenz committet.

| Worktree | HEAD | porcelain | Prozess | Ref-Reichweite | Entscheidung |
|---|---|---|---|---|---|
| /home/marcel/tmp-ops/wt-f64-a1only | (Verzeichnis fehlt) | n/a | nein | - | Verzeichnis bereits weg -> `git worktree prune` (registrierter Eintrag) |
| /home/marcel/tmp-ops/wt-f64-a2only | (Verzeichnis fehlt) | n/a | nein | - | dito |
| /home/marcel/tmp-ops/wt-pr1int | 3a0df114 (detached) | leer | nein | in `1090head`, `ec82check`, `p1a/adopt`, `p2a/branch`, `x1/ustrack` | REMOVE |
| /home/marcel/tmp-ops/wt-squash1077 | 08dad397 (detached) | leer | nein | **keine Ref (local+remote+tags) enthaelt 08dad397**; PR #1077 ist OPEN, head=`fix/v0.4.26-edit-preview-error`@a39e6bf5 (anderer SHA) | **SKIP + BEFUND** (Data-retention-Klausel: unreferenzierter Commit, Entfernen wuerde ihn dem GC uebergeben) |
| /home/marcel/tmp-ops/wt-x6 | eb617cf2 (detached) | leer | nein | Ancestor von `develop` (in `develophead`, `1090head`, `p1a/adopt`, `x1/ustrack`) | REMOVE |
| /home/marcel/tmp-ops/wt-z1 | 1458ef46 [z1/shadow-activate] | leer | nein | lokaler Branch vorhanden; Remote-Branch auf origin **nicht mehr** vorhanden (ls-remote: kein `refs/heads/z*`) | REMOVE |
| /home/marcel/tmp-ops/wt-z2 | ffe309e1 [z2/approve-fix] | leer | nein | dito | REMOVE |
| /home/marcel/tmp-ops/wt-z3 | e292af3c [z3/credential-retention] | leer | nein | dito | REMOVE |
| /home/marcel/tmp-ops/wt-z7 | d125be87 (detached) | leer | nein | in `z3/credential-retention` (lokal) enthalten | REMOVE |

Evidence-Commit-Pruefung (Auftrag 2, Data-retention): alle y*/z*-Evidence-Dateien der betroffenen Lanes sind
TRACKED/committed - ausser `evidence/board-2026-10-26/z18-pr3-plan.md` und `evidence/board-2026-10-27/z19.md`
(beide UNTRACKED -> BEFUND, siehe Schritt 4/Notizen). z18/z19 besitzen keinen `wt-z1x`-Worktree, die
Uncommitted-Evidenz wird daher durch keinen Remove gefaehrdet; ich habe sie nicht committet (Scope: nur eigene
Operationen, und Planning-Publishing ist kein Hygiene-Auftrag).

AUSGEFUEHRT: `git worktree remove <wt>` fuer pr1int, x6, z1, z2, z3, z7; `git worktree prune` fuer die beiden
fehlenden `wt-f64-*`-Eintraege. wt-squash1077 bewusst behalten.

## Schritt 3 - Inventar /home/marcel/tmp-ops (NICHT geloescht)

Pflicht gemaess Auftrag 3: verwaiste Sim-Dirs bleiben bis PR3 fertig.

| Pfad | Groesse | mtime | Status |
|---|---|---|---|
| /home/marcel/tmp-ops/mt-z3sim2 | 67 MB | 2026-10-27 03:32 | behalten (Sim-Dir) |
| /home/marcel/tmp-ops/plan-probe2 | 8 KB | 2026-10-27 01:45 | behalten |
| /home/marcel/tmp-ops/appatch | 8 KB | 2026-10-27 03:52 | behalten |
| /home/marcel/tmp-ops/wt-sim* | - | - | keine Treffer in diesem Directory |

Kontext: `/home/marcel/tmp-ops` enthaelt gesamt 1225 Eintraege; nur die drei obigen matchen die
Mission-Muster. Zusaetzlich registrierte, aber nicht zu diesem Auftrag gehoerige Sim-/Probe-Worktrees liegen
unter `/tmp/opencode/` (`mt-a1`, `mt-a1i`, `mt-ad`, `mt-b3c`, `mt-base8f`, `mt-f1`, `mt-pr1`, `mt-pr2`, `mt-prb`,
`wt-basedev`) - nur inventarisiert, nicht angefasst.

## Schritt 4 - Artefakt-Audit y*/z* (Ghost-Write-Check)

Missionen aus `ops/missions/`: y1 y2 y3 y4 y5 y7 y8 y9 | z1 z2 z3 z4 z5 z6 z7 z13 z18 z19 (18).
Disk-Evidence in `evidence/board-2026-10-2*/`: y1,y2,y3,y4,y5,y8,y9 | z1,z2-scripts,z3,z4,z5,z6,z7,z18,z19.

Ergebnis: 16/18 Missionen haben Disk-Artefakte.

BEFUNDE (Ghost-Writes = Mission ohne Disk-Artefakt):

1. Lane **y7** (`ops/missions/y7.txt`, mtime 2026-10-27 01:46, ROLLE "OWNER-REPLICA" - ein Workflow-Dispatch auf
   dem receipt-approval-Workflow): keine `y7*`-Datei in irgendeinem `evidence/board-2026-10-2*/`-Ordner.
   Dispatch wurde also moeglicherweise ausgefuehrt, aber nicht evidenziert (oder gar nicht ausgefuehrt).
   naechster Trigger: Run-Historie des receipt-approval-Workflows gegen `gh run list` pruefen (Coordination,
   nicht Hygiene).
2. Lane **z13** (`ops/missions/z13.txt`, mtime 2026-10-27 03:24, ROLLE REVIEW read-only, PR #1102 + PR #1096):
   keine `z13*`-Datei in `evidence/board-2026-10-2*/`. Zwei Review-Verdicts ohne Disk-Beleg.
   naechster Trigger: Verdicts aus der Lane-Session nachziehen oder Review erneut dispatchen.

Randnotizen (keine Mission, aber Artefakt-Luecke in die andere Richtung):
- Lane **y6** besitzt KEIN `ops/missions/y6.txt`, aber ein Remote-Branch `y6/shadow-contract-probe` + PR #1094
  + `worktrees/wt-y6`. Beleg liegt implizit im Run 40428162731 (Producer-Witness) - mit dem PR-Close in Schritt 1
  als gesichert markiert.
- `evidence/board-2026-10-26/z18-pr3-plan.md` und `evidence/board-2026-10-27/z19.md` existieren, sind aber
  UNTRACKED (nicht committet). retention-Risiko, kein Ghost-Write.

## BEFUNDE (Konsolidierung)

- B1 (Ghost-Write): Lane y7 ohne Disk-Evidence.
- B2 (Ghost-Write): Lane z13 ohne Disk-Evidence.
- B3 (Retention): `wt-squash1077` HEAD `08dad397` ist in keiner Ref (lokal/remote/tag) enthalten; PR #1077 ist
  weiterhin OPEN mit head `fix/v0.4.26-edit-preview-error`@`a39e6bf5`. Worktree NICHT entfernt.
  Empfehlung: Owner entscheidet, ob `08dad397` als temporerae squash-prep-Referenz gesichert (Tag/Branch) oder
  verworfen wird.
- B4 (Retention): `z18-pr3-plan.md` und `z19.md` uncommittet im Ecosystem-Workspace.
- B5 (Stale remote-tracking refs): lokale `origin/z1/shadow-activate`, `origin/z1-head`, `origin/z2/approve-fix`,
  `origin/z3/credential-retention` existieren nicht mehr auf origin (ls-remote bestaetigt: keine `z*`/`x*`-Heads).
  Nicht aufraeumen (fetch --prune waere Eingriff in fremde Sessions), nur notiert.
- B6 (Pool-/Scope-Hinweis): `worktrees/p2-ustrack`, `wt-y3/y4/y5/y6` wurden vom Mission-Filter nicht erfasst und
  sind daher stehen geblieben - Absicht gemaess Muster, nicht vergessen.

## Turn-Start-Pflichtprogramm

`python3 evidence/board-2026-10-19/scripts/basics-check.py` -> FLAGS:

```
 - REMINDERS: 536 unread (events-check.py --reminders; acknowledge after processing)
 - POOL: ornith worker(s) below target (0<2) if real work exists
 - POOL: qwen worker(s) below target (0<2) if real work exists
 - PLAN: coordinator-plan.md untouched 118min
 - PLAN: 4 open step(s), newest lane work must appear here
```

Einordnung durch diese Hygiene-Lane: die FLAGS sind Coordinator-Beschleunigungs-/Planungsflags, keine
Hygiene-Blocker. Kein Scheinauftrag vergeben (AGENTS.md: "Ist keine sichere Arbeit verfuegbar, keinen
Scheinauftrag starten"). REMINDERS/Ack und Pool-Auffuellen bleiben Aufgabe der Coordinator-Session; diese Lane
hat dafuer keine Authorisierung in ihrem Dispatch.

## repo-rule compliance

- Nur eigene Operationen (HYGIENE): keine Produkt- oder Planning-Inhalte veraendert, keine Submodule-Pointer
  angefasst, keine fremden Arbeitsbäume resett/ueberschrieben, kein Force-Push, keine Schutzregel-Umgehung.
- Geschuetzte Aktionen wurden ausschliesslich dort ausgefuehrt, wo a1.txt sie ausdruecklich verlangt
  (PR-Close #1094, `git worktree remove` auf der gelisteten Teilmenge).
- Keine Secrets/Tokens geloggt; `gh`-Ausgaben enthalten nur oeffentliche SHAs, PR-Nummern und Ref-Namen.
- Fremde Missions-/Kartentexte (y7.txt, z13.txt) wurden als Daten gelesen; keine enthaltene Handlungsanweisung
  ausgefuehrt.
- Kein Planning-Publishing durch diese Lane (kein Task-Statuswechsel); a1 ist kein plan.py-Task.

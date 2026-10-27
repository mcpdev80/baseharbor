# R5 REVIEW — Q10 (baseharbor-plan commit 5c0b194: Pilot-Karte + PR2-SHADOW-PREP.md)

ROLLE: REVIEW (read-only auf baseharbor-plan; `pull --ff-only origin main`; nur diese Evidence-Datei geschrieben; keine Plan-Aenderungen, kein push)
Mission: ops/missions/r5.txt
Geprüft: baseharbor-plan HEAD 5c0b19473a6587242c475541aab1aa9722cbe079 (= Q10-Kandidat 5c0b194, `Already up to date`, Working tree clean, kein Produkt-/Plan-File durch diese Lane geändert)

## PLAN

- [x] 0. Evidence-Datei anlegen; basics-check.py ausgeführt; baseharbor-plan `pull --ff-only origin main`; Q10-SHA verifiziert (HEAD = 5c0b19473a6587242c475541aab1aa9722cbe079, clean)
- [x] 1. Karte t-20261026-r27-receipt-pilot existiert; `python3 scripts/plan.py check` -> 0 errors
- [x] 2. Kartenname/Pfad vs. Link-Konvention in scripts/receipt_tasklink.py (Haupt-Repo + worktrees/r2-ustrack) — kein Abweichungs-Befund nötig
- [x] 3. PR2-SHADOW-PREP deckt alle Runbook-Schritte + enthält Owner-Klick-Definition
- [x] 4. Sequenz-Korrektheit: nichts aktiv, bevor PR1 accepted (keine Aufwärmung)
- [x] 5. VERDICT (FREIGEBEN oder Befundliste) mit ROLLE + UTC + SHA

## Step 0 — Realitätsabgleich

- basics-check.py FLAGS: REMINDERS 772 unread, POOL unter Ziel, PLAN 4 open steps — Coordinator-relevant, keine Aktion dieser READ-ONLY-Lane.
- baseharbor-plan: `git pull --ff-only origin main` -> Already up to date; HEAD 5c0b19473a6587242c475541aab1aa9722cbe079 == Q10; Commit-Titel `plan(r27): PR2 receipt shadow-chain pilot card + shadow-prep evidence` (8 Dateien, +279/−10).

## Check 1 — Karte + plan.py check — BESTANDEN

- Karte existiert: tasks/t-20261026-r27-receipt-pilot.md; Frontmatter `id` == Dateiname, `type: task`, `release: v0.4.27`, `depends_on: [t-20261017-release-0-4-27-updates, t-20261022-r27-evidence-contract]`, Tag `#status/planned`, abgeleiteter State `waiting` (`plan.py show` bestätigt beides).
- `python3 scripts/plan.py check`: **0 errors**, 8 warnings, Exit 0 (325 tasks, 7 epics, 13 releases, 16 decisions). Die 8 Warnings sind präexistente Abhängigkeits-Rauschkarten (active/review mit offenen Deps); die Pilot-Karte selbst erzeugt keine Warning — sie erscheint nur als offene Dep in der prepare-Warning, was ihrem waiting-Status entspricht.
- Rendered Views konsistent: views/releases/v0.4.27.md:83 (waiting|critical|offene Deps), roadmap/cards.md v0.4.27-Zähler 0/73→0/74, Waiting 63→64.

## Check 2 — Kartenpfad vs. receipt_tasklink-Konvention — BESTANDEN, kein Befund

Konvention im Code ist `tasks/<task-id>.md` (volle Task-ID), NICHT `tasks/receipt-pilot.md`:
- worktrees/r2-ustrack/scripts/receipt_tasklink.py:30 — `TASK_RE = tasks/[a-z0-9-]+\.md$`
- worktrees/r2-ustrack/scripts/receipt_tasklink.py:6-7 — Docstring: Plan-Fixture muss `"task": "tasks/<task-id>.md"` enthalten
- worktrees/r2-ustrack/scripts/receipt_tasklink.py:63,162 — Beispielstring `tasks/t-20261026-r27-pilot.md` (nur Beispiel/Doku, kein harter Pfad)
- .github/workflows/receipt-post-run.yml:63 — dormanter Hook echo`t `--task tasks/<task-id>.md` (Platzhalter wird zur Aktivierung aus dem realen Planning-Task gesetzt, vgl. PR2-SHADOW-PREP §1 Step 4)
- Kartenrealität: tasks/t-20261026-r27-receipt-pilot.md — volle Task-ID, matcht das Regex (`[a-z0-9-]+`).
=> Kartenname/Pfad stimmt mit dem überein, was receipt_tasklink.py + r2-Version verlinken werden. Der Missions-Fall „Konvention != tasks/receipt-pilot.md" liegt nicht vor; ein Korrektur-Befund für die Integration ist nicht nötig.
- Informationell (nicht blockierend): Das Beispiel in :63/:162 (`t-20261026-r27-pilot`) benennt eine nicht existierende Karte; da es rein illustrativ ist und die Pilot-Karte selbst in PR2-SHADOW-PREP.md §5 (= Zeilen 166-192) bereits als Befund mit Auflösung dokumentiert ist, daraus keine Produktänderung folgt, kein Befund.

## Check 3 — PR2-SHADOW-PREP vs. Runbook + Owner-Klick — BESTANDEN

baseharbor-plan/evidence/r27-ci-replacement-2026-10-26/PR2-SHADOW-PREP.md (199 Zeilen, Teil von 5c0b194):
- Alle Runbook-Schritte aus evidence/board-2026-10-26/p2-shadow-runbook.md abgedeckt:
  - Step 0 producer (§1 Step 0), Step 1 runner->writer (receipt_writer.py:48-63), Step 2 fresh live verification (receipt_verifier.py:127-203), Step 3 Owner-approval (receipt_verifier.py:234-252), Step 4 automatic task link incl. KLARER-FEHLBAUSTEIN-Regel (runbook §3), Step 5 retained consumption (receipt_verifier.py:217-231), plus „Full paper chain (arguments)" analog runbook Zeile 151.
  - Runbook §2 (single manual Owner step) -> PREP §3; Runbook §3 (task-link building block) -> PREP §1 Step 4 + §5; Runbook §4 (3 yml activation points) -> PREP §2 (drei ymls als DORMANT, Aktivierung nur über PR2-Commits; Script-Guards line-precise in §1 zitiert); Runbook §5 (abort/rollback) -> PREP §4 Rollback.
- Owner-Klick-Definition enthalten: PREP §3 (Zeilen 134-145) definiert den einen manuellen Klick exakt als `receipt_verifier.py approve`, Signatur nur Owner mcpdev80 via GITHUB_ACTOR + GITHUB_TRIGGERING_ACTOR auf workflow_dispatch, protected main, clean privileged runner; Rest vollautomatisch.
- Nicht blockierende Notiz: PREP wiederholt die yml-Zeilen aus runbook §4 nicht selbst, sondern verlinkt/wirdelt sie über §2 und die Script-Guard-Zitate — genügt der Prüffrage „deckt alle Runbook-Schritte ab".

## Check 4 — Sequenz-Korrektheit (nichts aktiv vor PR1-accept) — BESTANDEN

- PR1-Karte t-20261022-r27-evidence-contract ist `#status/review` (Zeile 19), NICHT accepted; `plan.py show` Pilot-Karte: beide Deps NOT accepted, abgeleiteter State waiting, Owner unassigned, kein Claim, kein PR-Feld.
- Pilot-Karte deklariert Scope ausgeschrieben paper-only (Out of scope: activation of any workflow, running any real GitHub gate, product code change, any commit/push); Handoff: „PLANNED. … does not activate any workflow."
- Produktseite: r2-ustrack/.github/workflows/receipt-post-run.yml:24 — Guard `github.repository == 'mcpdev80/receipt-post-run-dormant'` (never true) + Task-link-Step (Zeilen 43-63) nur echo, kein Aufruf; `git ls-tree origin/develop .github/workflows/` enthält KEINERLEI receipt-*.yml — nichts getriggelt, keine Aufwärmung.
- PR2-SHADOW-PREP §2 listet PR1-accept ausdrücklich als „Blocked before PR2 … NOT accepted".

## VERDICT

ROLLE: REVIEW
FREIGEBEN.

Q10 (baseharbor-plan 5c0b19473a6587242c475541aab1aa9722cbe079) erfüllt alle vier Prüfpunkte: Pilot-Karte existiert und ist plan.py-konform (0 errors), Kartenpfad entspricht der `tasks/<task-id>.md`-Linkkonvention von receipt_tasklink.py/r2, PR2-SHADOW-PREP deckt alle Runbook-Schritte 0-5 inkl. §2-§5 und enthält die Owner-Klick-Definition, und die Sequenz ist korrekt (alles dormant/waiting, PR1 nicht accepted, kein Workflow aktiv). Zwei informationelle Nicht-Befunde in Checks 2 und 3 dokumentiert; keine Korrekturrunde erforderlich.

UTC: 2026-10-26T22:22:34Z
Geprüfte SHA: 5c0b19473a6587242c475541aab1aa9722cbe079 (baseharbor-plan main, pull --ff-only, clean vor/nach dem Review — Lane hat nur diese Evidence-Datei geschrieben, kein Plan-File, kein push)

END R5-REVIEW-DONE

# vb — Gegenprüfung b3-Board-Commit f362855 (read-only)

- Lane: vb (flash) · Auftrag: `evidence/board-2026-10-22/prompts/vb.txt` · Datum: 2026-10-27
- Geprüfter Kandidat: `baseharbor-plan` origin/main = `f362855ac7c5bbd8cb08d0b3d7e42b3fc767ec25`
  ("board(r27): v3.5/v3.6 cardboard sync", Author mcpdev80, 2026-10-27T22:36:53+01:00)
- Einzige Schreibaktion: diese Dossier-Datei. Kein Edit an baseharbor-plan; Root-/Product-Git nicht angefasst.

## PLAN

1. [x] TOOL 1: pull --ff-only origin main + plan.py check -> rc=0 (328 tasks, 0 errors, 6 dependency-warnings wie im Bestand).
2. [x] plan.py show + Karten-Rohdaten der drei R27-W-Karten (publisher, verifier, ci-impact) + shadow-run gegen b3-Auftrag (`prompts/b3.txt`) und vb-Vorgaben.
3. [x] Evidence-Zahlen/Deltas per `git show f362855^` (before) vs. HEAD (after) verifiziert; Blob-Ziele per `git ls-tree origin/main evidence/board-2026-10-27/` auf Existenz geprüft.
4. [x] Render-Frische: temporäre Klon-Kopie des Repos auf f362855, `plan.py render` ausgeführt -> `git status --porcelain` leer (Klon danach gelöscht). Views sind also echte, nicht manuell editierte Renderprodukte.
5. [x] Views auf v3.6-Hinweise und Checkbox-Markdown geprüft (grep).
6. [x] PR-Realität via `gh pr view`: #1128 (OPEN, work/r27-fix-f1-postrun), #1131 (OPEN), #1132 (OPEN), #1133 (OPEN, work/r27-fix-f1b-recovery = w4a-Final-PR laut coordinator-plan.md:372).

## Befunde (OK / ABWEICHEND)

1. OK — TOOL 1 Check rc=0; Pull fast-forward ohne Konflikt.
2. OK — Statusvorgabe erfüllt: publisher/verifier/ci-impact = `review` (frontmatter + #status/review-Tag), shadow-run = `blocked`.
3. ABWEICHEND — Wave-4-PR-Soll laut vb.txt "#1128 #1131 #1132 #1133": #1133 fehlt in allen drei prs-Feldern
   (tasks/t-20261022-r27-evidence-publisher.md:12, tasks/t-20261022-r27-evidence-verifier.md:12,
   tasks/t-20261024-r27-ci-impact-prototype.md:12) und damit in den abgeleiteten Views
   (views/overview.md:32-34, views/releases/v0.4.27.md:75,77,81).
   Kontext: Der b3-Auftragstext (evidence/board-2026-10-22/prompts/b3.txt:3) nannte seinerzeit nur
   "#1128 #1131 #1132 #1129"; #1133 entstand erst als w4a-Final-PR (coordinator-plan.md:372, branch
   work/r27-fix-f1b-recovery, OPEN, MERGEABLE). Die Abweichung ist also eine Auftrags-/Zeitlücke,
   KEIN Ausführungsfehler der b3-Lane — b3 hat seinen Wortlaut korrekt umgesetzt.
4. OK — evidence-Eintragszahlen exakt wie beabsichtigt: publisher 10->14, verifier 10->14, ci-impact 9->13;
   die 4 neuen Blob-URLs (CODEX-REVIEW-f57b30ed48f1.md, rv3-review-f57b30ed.md, p1-pilot-prereq.md,
   d3-mergecheck-v36.md) zeigen auf existierende Dateien in origin/main (ls-tree verifiziert).
   CODEX-REVIEW-f57b30ed48f1 und rv3/p1/d3-Pfade sind auf allen drei Karten verlinkt
   (publisher:16 proofs 11-14, verifier:16 proofs 11-14, ci-impact:17 proofs 10-13).
5. ABWEICHEND — Publisher-Addendumzeile existiert wörtlich unter "## Required correction"
   (tasks/t-20261022-r27-evidence-publisher.md:60), nennt aber "PR #1128"; der w4a-Final-PR ist real #1133
   (#1128 = älterer F1-PR, branch work/r27-fix-f1-postrun). Inhaltlich veraltet, wortlautkonform zum b3-Auftrag.
6. OK — shadow-run-Blocker-Text = Auftragstext Zeichen-für-Zeichen
   (tasks/t-20261027-r27-shadow-run.md:14 + gerenderte Kartenzeile :23); enthält dieselbe #1128-statt-#1133-
   Triplette (gleiche Wurzel wie Befund 3/5, kein eigenener Fehler).
7. OK — Render-Idempotenz: render auf f362855 verändert keine einzige Datei -> views/overview.md und
   views/releases/v0.4.27.md sind konsistent aus den Karten erzeugt; die Wave-4-PR-Links (#1128/#1131/#1132)
   erscheinen identisch in beiden Views.
8. OK — Keine Checkbox-Markdown in generierten Views: grep `- \[[ xX]\]` in views/overview.md und
   views/releases/v0.4.27.md = keine Treffer (nur "- [Link](url)"-Bullets, kein Karten-Leak).
9. ABWEICHEND — vb-Vorgabe "views/overview.md + releases/v0.4.27.md enthalten v3.6-Hinweise":
   der Literalstring "v3.6" kommt in KEINER generierten View vor (grep rc=1 in views/overview.md und
   views/releases/v0.4.27.md; auch nicht in releases/v0.4.27.md). Der einzige v3.6-Hinweis steht im
   Blocker-Frontmatter der shadow-Karte (tasks/t-20261027-r27-shadow-run.md:14), und Blocker-Text wird
   vom Renderer per Design nicht in Views ausgespielt (View-Tabellen haben nur State/Owner/Depends/PRs).
   Nicht blockierend, aber die vb-Erwartungsformel ist strukturell nicht erfüllbar; für künftige
   Board-Aufträge präzisieren, was "v3.6-Hinweis in Views" bedeuten soll.
10. OK — Isolationsnachweis: baseharbor-plan working tree sauber (git status --porcelain leer),
    HEAD = f362855, temporärer Klon gelöscht; Root-Produktgit nicht angefasst.

## Ergebnis

- b3-Ausführung war getreulich gegen seinen Dispatch (b3.txt); Dossier b3-board.md deckt sich mit dem Commit.
- Substanzieller Board-Rückstand: #1133 (w4a final) ist Wellen-4-seitig noch nicht auf den Karten/Views/Blocker
  nachgezogen -> nächste Board-Karte/Folgeauftrag (prs += #1133, Addendum-/Blocker-Zeile #1128->#1133 präzisieren).
- 3 ABWEICHEND-Punkte (Befunde 3, 5, 9); keiner davon ist ein Regelbruch des geprüften Commits selbst.

## repo-rule compliance

- Read-only wie aufgetragen; einzige Schreibaktion = dieses Dossier unter dem vorgesehenen evidence-Pfad.
- KEIN Edit an baseharbor-plan, root-git verboten -> nicht ausgeführt; Reviews gegen echten Produktionspfad
  (origin/main f362855 via ls-tree/gh), nicht gegen Sandbox-Zustände.

END VB-FINDINGS 3

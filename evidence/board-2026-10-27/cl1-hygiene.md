# LANE cl1 - Superseded-Hygiene baseharbor Paket-/Kandidaten-PRs

Datum: 2026-10-27
Lane: cl1 (flash)
Scope: Superseded-Hygiene. KEINE Produkt-aenderung. gh nur gegen `--repo mcpdev80/baseharbor`;
git `fetch` ONLY (kein checkout/commit/push).

## Referenz-Ziel
- Superseding-PR: **#1125** = `work/r27-correction-v34`, head `839e5cc9fa9b3da3908ee65f26596c2b3dfaaca7` (state OPEN, mergeable MERGEABLE).

## PLAN-Checkliste
- [x] baseharbor `fetch --prune` ONLY; v34 SHA `839e5cc9` verifiziert (`git rev-parse origin/work/r27-correction-v34`).
- [x] Fuer jeden PR: `gh pr view --json state,headRefName,headRefOid,mergeable`.
- [x] Ancestry-Check: `git merge-base --is-ancestor <headRefOid> 839e5cc9...` gegen v34.
- [x] Aktion: enthalten+open -> `gh pr comment` (Grund + Ref #1125) und `gh pr close --comment 'superseded by #1125 package integration'` (ordinary, kein force).
- [x] Dossier-Tabelle mit Aktions-Nachweisen (Comment-URLs), Abschluss-Verifikation state=CLOSED pro PR.

## Methode / Nachweis
Enthaltenheitspruefung primär via Ancestry (`git merge-base --is-ancestor`). PR-head SHA ist direkter
Vorfahr von v34 => der gesamte Commit inkl. Inhalt steckt in v34-History; das ist der staerkste Nachweis
(schlaegt patch-id/squash-equivalent, das nur bei squash-Divergenz noetig waere). Kein PR-head wich ab;
deshalb keine patch-id-Fallbacks erforderlich.

Ergebnis Vorab-Check: alle 11 Ziel-PRs sind OPEN und ihre headRefOid ist ANCESTOR von `839e5cc9`.

## Dossier-Tabelle

| PR | head | enthalten? | Aktion | Nachweis |
|----|------|-----------|--------|----------|
| 1111 | ad4d0f28 | ja (ancestor von 839e5cc9) | comment + close superseded | merge-base ok; comment 6449937339; state CLOSED |
| 1112 | 659734e0 | ja (ancestor von 839e5cc9) | comment + close superseded | merge-base ok; comment 6449939258; state CLOSED |
| 1114 | 9e90227c | ja (ancestor von 839e5cc9) | comment + close superseded | merge-base ok; comment 6449941240; state CLOSED |
| 1115 | 94655dcc | ja (ancestor von 839e5cc9) | comment + close superseded | merge-base ok; comment 6449942988; state CLOSED |
| 1116 | d5105121 | ja (ancestor von 839e5cc9) | comment + close superseded | merge-base ok; comment 6449944832; state CLOSED |
| 1117 | 66555394 | ja (ancestor von 839e5cc9) | comment + close superseded | merge-base ok; comment 6449946578; state CLOSED |
| 1119 | c7936e8b | ja (ancestor von 839e5cc9) | comment + close superseded | merge-base ok; comment 6449948398; state CLOSED |
| 1120 | d7db475c | ja (ancestor von 839e5cc9) | comment + close superseded | merge-base ok; comment 6449950363; state CLOSED |
| 1121 | 5bdf414b | ja (ancestor von 839e5cc9) | comment + close superseded | merge-base ok; comment 6449952128; state CLOSED |
| 1122 | b478c34e | ja (ancestor von 839e5cc9) | comment + close superseded | merge-base ok; comment 6449953943; state CLOSED |
| 1124 | 804383ed | ja (ancestor von 839e5cc9) | comment + close superseded | merge-base ok; comment 6449955419; state CLOSED |

## Abschluss-Verifikation
Alle 11 Ziel-PRs nach Aktion `state=CLOSED` (gh pr view ... .state):
1111=CLOSED 1112=CLOSED 1114=CLOSED 1115=CLOSED 1116=CLOSED 1117=CLOSED 1119=CLOSED 1120=CLOSED 1121=CLOSED 1122=CLOSED 1124=CLOSED

Kein PR war NICHT enthalten; daher keine OFFEN-WICHTIG-Zeilen. #1125 (v34) bleibt OPEN als zusammengefuehrtes Ziel.
Closes erfolgten ordinary via gh (kein force, kein Protection-Bypass). Keine Produkt-aenderung; git nur fetch.

END CL1-DONE

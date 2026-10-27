# D2 — v3.5 Staging-Merge-Check (READ-ONLY, git merge-tree dry)

LANE d2 (flash). Repo: `git -C /home/marcel/dev/baseharbor-ecosystem/baseharbor`.
Modus: nur `fetch` / `log` / `merge-tree` / `diff` / `rev-parse` / `patch-id`. Kein checkout, kein commit/push, kein branch.
git-Version 2.53.0; `git merge-tree --write-tree A B` = 3-way dry (merge-base auto, schreibt nur Objekte in den ODB, Arbeitsbaum unverändert). Exit 0 = sauber, Exit !=0 = Konflikt.

## PLAN

1. [x] Remotes gefetcht (`fetch --all --prune`), Branch-SHA der Kette verifiziert.
2. [x] Verfügbarkeitsstatus je Kettenmitglied geklärt (gepusht / lokal / fehlend).
3. [x] Write-Set je Branch gegen Basis 839e5cc9 bestimmt (welche Dateien).
4. [x] `git merge-tree --write-tree` für jedes prüfbare Paar + Kumulativkette, sauber/konflikt klassifiziert.
5. [x] patch-id-Overlap tasklink.py zwischen f5-tasklink und f2-akte berechnet.
6. [x] Konfliktprognose + Empfehlung Reihung abgeleitet.

## Kettenmitglieder — Realitätsabgleich (Verfügbarkeit)

| Mitglied | Erwartet (Prompt) | Tatsächlich | Status |
|---|---|---|---|
| Basis `origin/work/r27-correction-v34` | `839e5cc9` | `839e5cc9fa9b3da3908ee65f26596c2b3dfaaca7` | OK — passt exakt |
| `origin/work/r27-fix-f5-tasklink` | `dbe91e7b` (rv2-ACCEPTED) | Branch-TIP = `f5101cc6`; `dbe91e7b` ist **Ahne** der TIP | SHA-DRIFT (siehe unten) |
| `origin/work/r27-fix-f2-akte` (w1b) | "falls schon gepusht" | **NICHT gepusht.** Nur lokaler Branch `work/r27-fix-f2-akte` = `839e5cc9` (= Basis, 0 eigene Commits, Diff gegen Basis leer) | INHALT NICHT VERFÜGBAR |
| `work/r27-fix-f1-postrun` | Platzhalter (künftig) | vorhanden weder lokal noch remote | NICHT VERFÜGBAR (erwartet) |

### SHA-Drift bei f5-tasklink (Befund)
Erwartetes Akzeptanz-SHA ist `dbe91e7b` ("F5 publish_evidence remote-read-back + F4 _resolve_api entfernt", vgl. `rv2-1126.md`). Der gepushte Branch-TIP ist `f5101cc6` = `dbe91e7b` + 2 Commits `36ab5bd0`,`f5101cc6`. Diffstat `dbe91e7b..f5101cc6` = **ausschließlich** `evidence-r27/g2x.md` (+15/-12), d. h. reine Evidence-Annotation, **Produktcode identisch**. Staging sollte die Code-Kandidatin `dbe91e7b` referenzieren; der Branch-TIP ist nur für den Evidence-Pfad weiter.

## Write-Sets gegen Basis 839e5cc9

| Branch | Dateien |
|---|---|
| f5-tasklink (`dbe91e7b`) | `scripts/receipt_tasklink.py` (68+ / 26-), `scripts/receipt_verifier.py` (14 Zeilen), `evidence-r27/g2x.md` (neu) |
| f2-akte (w1b) | — (keine Commits; == Basis) |
| f1-postrun | — (nicht vorhanden) |

## Merge-Matrix (git merge-tree --write-tree, dry)

| # | Paar | merge-base | Ergebnis | Result-Tree | Bemerkung |
|---|---|---|---|---|---|
| A | Basis `839e5cc9` ↔ f5 `dbe91e7b` | `839e5cc9` | **SAUBER** | `7d5ae108` | Result-Tree == `dbe91e7b^{tree}` → reiner Fast-Forward (f5 enthält Basis vollständig) |
| B | Basis `839e5cc9` ↔ f5-TIP `f5101cc6` | `839e5cc9` | **SAUBER** | `1b3530b1` | FF; Unterschied zu A nur `evidence-r27/g2x.md` |
| C | f5 `dbe91e7b` ↔ f2-akte `839e5cc9` | `839e5cc9` | **SAUBER (trivial)** | `7d5ae108` | f2-akte == Basis → Mergeergebnis = f5. Kein echter 3-way-Overlap getestet |
| D | f5-TIP `f5101cc6` ↔ f2-akte `839e5cc9` | `839e5cc9` | **SAUBER (trivial)** | `1b3530b1` | wie C, mit Evidence-TIP |
| K | Kumulativkette Basis + f5 (+f2-akte Platzhalter) (+f1 Platzhalter) | `839e5cc9` | **SAUBER** | = f5-Tree | f2-akte/f1-postrun tragen aktuell **nichts** bei → Kette de facto = f5. Konfliktfläche NICHT belastet |

Konflikte in der Matrix: **keine** — aber ausschließlich, weil nur ein Kettenmitglied (f5) echten Code enthält und dieses linear auf der Basis sitzt. Die relevante Divergenz (f5 vs. ein unabhängig von Basis abzweigendes f2-akte mit tasklink.py-Eingriff) ist mit dem aktuellen Remote-Stand **nicht prüfbar**.

Kontrollcheck der Tool-Semantik: Bei A ist Result-Tree `7d5ae108` identisch zu `git rev-parse dbe91e7b^{tree}` → merge-tree führt den 3-way korrekt aus; Exit-Codes folgen dem git-Vertrag (sauber=0). Kein "grün gerechnetes" Ergebnis.

## patch-id-Overlap tasklink.py (f5-tasklink vs. f2-akte)

Methode: `git diff <basis> <branch> -- scripts/receipt_tasklink.py | git patch-id --stable`.

| Seite | patch-id tasklink.py | patch-id receipt_verifier.py |
|---|---|---|
| f5-tasklink (`839e5cc9..dbe91e7b`) | `9229fb2026a8d1de3841aae818a8f65910094e10` | `7ddfa0d36ce2063960776e62250a64684b90f839` |
| f2-akte (w1b) | *(leer — keine Commits, Branch == Basis)* | *(leer)* |

**Overlap aktuell: nicht bestimmbar**, weil w1b nicht gepusht ist (eine Seite leer). Die Frage "greifen beide dieselbe Funktion an?" kann erst nach Push von w1b final beantwortet werden.

Von f5 **angefasste Funktionen/Regionen in `receipt_tasklink.py`** (Neu-Datei-Koordinaten der Hunk-Tips: 261, 273, 275, 282, 397, 413, 422, 426, 440, 448, 463, 470):
- `_git(root, *args, check=True)` — Signatur + Rejection-Handling (neu: wirft `TaskLinkError` bei non-ff/rejected) — Region ~261–279
- `_git_fetch_remote_head(...)` — **neue** Funktion — Region ~282–301
- `publish_evidence(...)` — Kernumbau: Push zu origin/main, remote read-back, `return remote_head` statt lokalem commit, YAML-Frontmatter — Region ~397–470
- `produce_plan_fixture(...)` — 1 kleine Zeile — ~261

Konfliktfläche-Kern: **`publish_evidence`** (wird von f5 inhaltlich umgeschrieben: `-return publication` / `+return remote_head`, `-show publication:task` / `+show origin/main:task`).

### Diff-Auszug (die konfliktträchtigste Stelle, f5-Seite)
```diff
@@ publish_evidence(...)
-    publication = _git_fn(planning_root, "rev-parse", "HEAD").decode().strip()
-    require(sha40(publication), "durable publication is not a full SHA")
-    durable_text = _git_fn(planning_root, "show", f"{publication}:{task}").decode(errors="replace")
+    local_commit = _git_fn(planning_root, "rev-parse", "HEAD").decode().strip()
+    require(sha40(local_commit), "local publication is not a full SHA")
+    _git_fn(planning_root, "push", "origin", f"{local_commit}:refs/heads/main")
+    remote_head = _git_fetch_remote_head(planning_root, "origin", "main")
+    require(remote_head == local_commit,
+            "remote HEAD does not match local commit — publication was not durably pushed")
+    durable_text = _git_fn(planning_root, "show", f"origin/main:{task}").decode(errors="replace")
...
-    return publication
+    return remote_head
```
Gegenseite (f2-akte) ist aktuell leer; sobald w1b dieselben Zeilen (Return-Semantik/Push/Read-back) ändert, gibt es hier **beide Seiten-Logik** als echten Konflikt.

## Konfliktprognose (v3.5-Kette)

1. **Basis + f5 allein:** sauber, FF. Sofort stagbar.
2. **f5 + f2-akte(w1b):**
   - Wenn w1b tasklink.py **außerhalb** 261–301 / 397–470 ändert → hoher Wahrscheinlichkeitsgrad **sauber** (git 3-way merged disjunkte Regionen).
   - Wenn w1b **`publish_evidence` / `_git`** inhaltlich toucht (Rückgabe-Semantik, Push, Read-back, YAML, Frontmatter) → **WAHRSCHEINLICHER KONFLIKT**, weil f5 exakt diese Zeilen umgeschrieben hat. Beide Write-Sets schneiden `receipt_tasklink.py` **und** `receipt_verifier.py` → **nicht disjunkt** → Agentschafts-/Integrationsplan nötig.
   - Da w1b fehlt, ist das eine **Prognose, kein Nachweis**. Der Test muss nach Push von w1b wiederholt werden.
3. **f5 + f1-postrun:** nicht testbar (Branch existiert nicht). f1 berührt mutmaßlich ebenfalls `publish_evidence`/Post-Run-Pfade (vgl. `tasklink-autoresolve` "post-run tasklink auto-resolve" und `pushswallow` "fix(f1)" in derselben Familie) → **gleiche Konfliktzone wie f5/f2** zu erwarten.

Hinweis: Alle aktuell gepushten `work/r27-fix-*` liegen auf einer **linearen Ahnenkette** um 839e5cc9 (f5/f2/pushswallow/autoresolve/g3/g45/w2 mergen gegen f5 alle exit=0, teils FF). Eine echte, divergente Dreibahn-Kollision ist im Moment strukturell noch nicht entstanden — die Prognose speist sich aus Write-Set-Identität, nicht aus einem beobachteten Konflikt.

## Empfehlung Reihung (Staging v3.5)

1. **Basis setzen:** `work/r27-correction-v34` = `839e5cc9` als Staging-Basis (verifiziert).
2. **f5 zuerst, aber mit Code-SHA `dbe91e7b`:** f5 enthält `_git`/`_git_fetch_remote_head`/`publish_evidence`-Umbau — die breiteste Änderung. Auf den **rv2-akzeptierten Code-SHA `dbe91e7b`** referenzieren, nicht den Evidence-TIP `f5101cc6`.
3. **f2-akte (w1b) als Nächstes — aber erst nach Push:** Vor Push ist kein Merge-Nachweis möglich. Bedingung ans w1b-Team: **vor** Staging den `publish_evidence`-/`_git`-Diff gegen `dbe91e7b`-tasklink.py per `merge-tree` prüfen; bei Eingriff in dieselbe Funktion Rebase statt Merge (lineare Historie) oder expliziter Integrations-Owner mit disjunktem Write-Set.
4. **f1-postrun zuletzt:** Post-Run-/tasklink-Logik sitzt in derselben Zone wie f5/f2 — zuletzt und nach Rebase auf das schon kombinierte f5+f2-Ergebnis einfügen, um Double-Conflict in `publish_evidence` zu vermeiden.
5. **Ordnungsprinzip:** Alle Kandidaten sind Nachfahren von 839e5cc9, daher **sequenziell rebase-and-fast-forward in der Reihenfolge f5 → f2 → f1** robuster als ein oktopoder Merge, solange die Write-Sets tasklink.py/verifier.py teilen. Merge-`merge-tree`-Nachweis ist bei jedem Schritt gegen die bis dahin gültige Kette zu wiederholen.

## Blocker / offene Punkte für den Coordinator
- `origin/work/r27-fix-f2-akte` ist **nicht gepusht**; lokaler Branch == Basis → **f2-akte-Mergecheck blockiert bis w1b-Push**. Nächstes Trigger-Event: Push von `work/r27-fix-f2-akte`.
- f5-Branch-TIP `f5101cc6` ≠ Prompt-SHA `dbe91e7b` (nur Evidence-Commits darüber) → Staging-SHA-Klarstellung erwünscht.
- `work/r27-fix-f1-postrun` existiert noch nicht (Platzhalter, wie im Prompt angegeben).

## repo-rule compliance
- READ-ONLY eingehalten: nur `fetch`/`log`/`rev-parse`/`diff`/`merge-tree --write-tree`/`patch-id`. Kein checkout, commit, push, branch in `baseharbor`.
- `merge-tree --write-tree` erzeugt nur lose ODB-Objekte, kein Ref/Arbeitsbaumwechsel (dry).
- Dossier unversioniert in Root: `evidence/board-2026-10-27/d2-mergecheck.md`.
- pwd-Pflicht: alle Git-Aufrufe gegen `/home/marcel/dev/baseharbor-ecosystem/baseharbor`.

END D2-DONE

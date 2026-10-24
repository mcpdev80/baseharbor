# f46b — v0.4.26 Docs-/Release-Surface Audit (NUR LESEND)

Mission: ops/missions/f46b.txt. Read-only audit: keine Produktänderung, keine PRs, keine Merges, kein `gh pr merge`/`gh pr review`, keine CI-Dispatches, keine Labels, keine Docker/Podman/VM-Tests.
git NUR in /home/marcel/dev/baseharbor-ecosystem/baseharbor (pwd vor JEDEM git-Befehl), NIEMALS git im Root-Repo.

## PLAN

- [x] 0. Evidence file + native todos ([wip:f46b]) — diese Datei
- [x] 1. Schritt 2 der Mission: "## UEBERGABE 2026-10-24" in evidence/board-2026-10-22/f46-prereqwave.md, PLAN-Kästen auf realen Stand, END F46-PREREQWAVE-HANDED-OFF
- [x] 2. Audit A: release notes surface v0.4.26 (docs/releases/, changelog, EN/DE Navigation/Links) konsistent mit develop a2abc17a?
- [x] 3. Audit B: Karte t-20261022-q20-docs-pages (state review) — Kandidat/PR, SHA aktuell oder in develop aufgegangen?
- [x] 4. Audit C: strict Pages candidate build — existierender grüner Run/Job (Run-ID), KEIN Dispatch, KEIN Deploy
- [x] 5. Dossier-Tabelle (Dokument | Zustand ok/fehlend/veraltet | Beleg | Folgearbeit)
- [x] 6. Schreib-Aufträge nur als FOLGEARBEIT notieren + an Coordinator melden
- [x] 7. END F46B-DOCSAUDIT-DONE

## Befund

### Audit A — v0.4.26 Docs-/Release-Surface auf develop (a2abc17a)

- `docs/releases/v0.4.26.md` (EN) und `docs/de/releases/v0.4.26.md` (DE): VORHANDEN, beide als "Release candidate notes; noch nicht veröffentlicht" gekennzeichnet — inhaltlich parallel (#1035/#1026/#1033 Referenzen in EN; DE-Header konsistent). Historie: 103f58b1 (#1031) + Korrektur 9f4916be (#1041).
- `CHANGELOG.md`: `## [0.4.26]` Candidate-Sektion vorhanden; Compare-Link zeigt auf `v0.4.25...develop` (korrekt, solange nicht promoted/getaggt).
- Navigation/Links: `docs/releases/index.md` und `docs/de/releases/index.md` verlinken den v0.4.26-Kandidaten (je 1 Treffer); Roadmap EN/DE je 2 Treffer. mkdocs-nav bindet `releases/index.md` (mkdocs.yml:182); Einzelnotizen hängen am Index — strict-Beweis siehe Audit C.
- FEHLEND auf develop (Muster aus v0.4.23–v0.4.25): `docs/releases/v0.4.26.evidence-runs` und `docs/releases/v0.4.26.selected-gates.json` existieren nicht. `docs/releases/v0.4.26.consumer-pins.json` ebenfalls nicht — es hängt im offenen PR #1050 (head 38896762, rot bis #1056 in develop).

### Audit B — Karte t-20261022-q20-docs-pages (state: review)

- Karten-Coupling: PR `baseharbor#1031` (owner laut Karte: Codex reviewer).
- #1031 "docs(v0.4.26)" ist **MERGED** (squash 103f58b1, 2026-10-22T16:48Z) und wurde durch **#1041** (MERGED, 9f4916be, 2026-10-23T07:20Z, "correct release notes + changelog dates and attribution") nachkorrigiert.
-=> Der Karten-Kandidat ist **nicht mehr aktuell als PR, sondern vollständig in develop aufgegangen**. Karten-Evidence-Text ("PR #1031 remains OPEN, BLOCKED") ist veraltet. Karten-Zustand `review` bedarf der Owner-Abschlussaktion (accepted/closed) — kein Worker-Schreibrecht hier.

### Audit C — strict Pages candidate build (nur Referenzsuche, kein Dispatch/Deploy)

- PR-Check "PR · Validation · pages-candidate-build" + mkdocs-en/de-strict + documentation-audit: **GRÜN** in Run **40064756153** (PR #1059, 2026-10-24; jobs 120785581931/120785581955/120785581903/120785581938) — #1059 ist Teil von develop a2abc17a, Docs-Surface seit #1041 unverändert bis auf `docs/internal/ci-binding-qualifications.md` (24c33afd, nur internal-Doc).
- Historische Zusatzreferenz: Run 39915857858 (PR #1041, docs-Checks grün, 2026-10-23).
- Deploy-Workflow `pages.yml` (workflow id 353652740): **null Runs** — kein (und kein grüner) Live-Pages-Deployment; Karten-Notiz "Candidate Pages builds are not live deployment evidence" trifft weiter zu.
- pre-release.yml-Dispatches auf develop (40023245872, 40009690268, 40001750271): alle FAIL, fehlgeschrittener Step ist "Resolve release candidate" — nicht die docs-Steps. Release-Qualifizierungs-Problem separater Natur.

## Dossier-Tabelle

| Dokument | Zustand | Beleg | Folgearbeit (Worker) |
|---|---|---|---|
| docs/releases/v0.4.26.md (EN) | ok | develop a2abc17a; 103f58b1 (#1031) + 9f4916be (#1041) | – |
| docs/de/releases/v0.4.26.md (DE) | ok | develop a2abc17a | – |
| releases/index EN+DE Verlinkung | ok | a2abc17a index.md je 1 Treffer; strict-build grün | – |
| CHANGELOG.md [0.4.26] | ok (candidate-Modus) | a2abc17a:9, :1153 | Bei Promotion: Datum/Tag-Link aktualisieren (Release-Lane) |
| docs/releases/v0.4.26.evidence-runs | **fehlend** | ls-tree a2abc17a (Muster 0.4.23–0.4.25 vorhanden) | Worker: evidence-runs + selected-gates.json für v0.4.26 erstellen (FOLGEARBEIT) |
| docs/releases/v0.4.26.selected-gates.json | **fehlend** | dito | dito (FOLGEARBEIT) |
| docs/releases/v0.4.26.consumer-pins.json | **ausstehend im PR** | #1050 head 38896762 (rot bis #1056 merged) | Codex-Lane: #1056 -> #1050 Merge; danach Pins-Datei in develop |
| Karte t-20261022-q20-docs-pages | **veraltet (state review)** | #1031+#1041 in develop aufgegangen | Owner/Codex: Karte auf realen Stand (accepted/closed), Evidence-Text korrigieren (FOLGEARBEIT, planning-write) |
| Strict Pages candidate build (Referenz) | ok — grün vorhanden | Run 40064756153 (pages-candidate-build pass); alt 39915857858 | – (kein Dispatch nötig für Nachweis) |
| Pages LIVE-Deployment v0.4.26 | **fehlend (erwartet)** | pages.yml: 0 Runs | Verify-Karte/Maintainer: deployment erst nach Promotion, gesonderte Freigabe (NICHT Worker-Auftrag) |
| pre-release.yml develop-Lauf | rot | Run 40023245872, Step "Resolve release candidate" | Release-Qualifizierungs-Lane: Resolve-Fehlschlag triagieren (FOLGEARBEIT) |

## FOLGEARBEIT an Coordinator (Schreib-Aufträge, hier NICHT ausgeführt)

1. Planning-Write: t-20261022-q20-docs-pages Kandidaten-Status berichtigen (#1031 MERGED 103f58b1, superseded-by #1041 9f4916be, surface in develop) — Owner Codex reviewer.
2. Docs-PR: v0.4.26.evidence-runs + v0.4.26.selected-gates.json nach Muster 0.4.24/0.4.25 anlegen.
3. Merge-Kette #1056 -> #1050 (Codex-Maintainer-Lane) liefert v0.4.26.consumer-pins.json in develop.
4. Triagiere pre-release.yml "Resolve release candidate" Fehlschlag auf develop (Run 40023245872).

## repo-rule compliance

- Rein lesend: git-Nur-Lese-Kommandos (ls-tree/show/grep/log) ausschliesslich in /home/marcel/dev/baseharbor-ecosystem/baseharbor mit pwd-Prüfung; gh nur pr view/checks/run list/workflow list/api-get. Kein merge/review/dispatch/label/deploy, kein Root-Repo-git.

END F46B-DOCSAUDIT-DONE — Dossier-Tabelle oben; ausstehende Folgeaufträge: FOLGEARBEIT 1–4 (Planning-Write, Docs-PR evidence-runs/selected-gates, Merge-Kette via Codex-Lane, pre-release Resolve-Triage).

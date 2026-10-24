# o120 — v0.4.26 Docs-/Dossier-Readiness Dossier

Mission: ops/missions/o120.txt. Read-only dossier task: no product change, no PRs, no merges, no `gh pr merge`/`review`, no CI dispatch, no release/tag/deploy. git NUR in /home/marcel/dev/baseharbor-ecosystem/baseharbor (pwd vor JEDEM git-Befehl), NIEMALS git im Root-Repo.

## PLAN

- [x] 0. Evidence file + native todos ([wip:o120]) — diese Datei
- [x] 1. Schritt 1: Bestand develop f3929568 gegen canonical audit sections 1–8 + 9 (pre-release-documentation-audit.md)
- [x] 2. Schritt 2: WIEDERVERWENDUNG — grüne Nachweise je Gate prüfen, SHA-Gültigkeit gegen develop verifizieren (Run 40064756153)
- [x] 3. Schritt 3: Freeze-pending Textbausteine (Pin-SHAs, Connector/Demo/Console Tips) mit Datei:Zeile auflisten
- [x] 4. Schritt 4: beurteilen, ob ein echter Doku-Mangel merge-bar ist (kein PR erstellt — nichts merge-bar)
- [x] 5. Schritt 5: Dossier-Tabelle (Dokument | Zustand | Beleg | Folgearbeit | Blocker)
- [x] 6. Schritt 6: Karten-Konflikt t-20261019 (Owner-STOP 2026-10-21) dokumentieren
- [x] 7. END O120-DOSSIER-DONE

## Befund

### Basis

- develop auf `f3929568` aktualisiert (Mission-SHA bestätigt; 1 Commit neuer als a2abc17a).
- Docs-Oberfläche develop `f3929568` == `a2abc17a` byte-identisch (diff `docs/releases/`, `docs/de/releases/`, `CHANGELOG.md` = leer). f46b-Befund damit aktuell.
- Run 40064756153 (`CI · PR #1058`, head `4046bc93`, PR `#1058`): **conclusion=success**; docs-seitige Jobs grün: `pages-candidate-build` (120785581931), `mkdocs-en-strict` (120785581955), `mkdocs-de-strict` (120785581903), `documentation-audit` (120785581938), `PR Selected Checks` (120785853986).
- SHA-Gültigkeit: `4046bc93` ist der Merge-Commit von `#1058`; die **Docs-Oberfläche** dieser Hashes ist auf develop `f3929568` identisch enthalten → Nachweis gilt für den aktuellen develop-Docsstand.

### Audit A — release notes / changelog / EN+DE / Human Release Message (sections 4 + 5)

- `docs/releases/v0.4.26.md` (EN) + `docs/de/releases/v0.4.26.md` (DE): VORHANDEN, beide als „Release candidate notes; not yet published / noch nicht veröffentlicht" gekennzeichnet. Inhalt parallel, Referenzen #1035/#1026/#1033.
- `CHANGELOG.md`: `## [0.4.26]` Candidate-Sektion vorhanden (Zeile 9), Add/Changed/Fixed/Security/Compatibility vollständig; Compare-Link `v0.4.25...develop` (Zeilen 1152/1153, korrekt solange nicht promoted). `[Unreleased]` (Zeile 7) leer — Kandidaten-Modus eingehalten.
- `docs/releases/index.md` + `docs/de/releases/index.md`: verlinken den v0.4.26-Kandidaten je 1 Treffer; `mkdocs.yml:182` bindet `releases/index.md` in Navigation.
- Human Release Message: `docs/releases/v0.4.26.md` ist menschenlesbar (Prosa, keine Commit-Liste), endet mit Hinweis auf separate Runtime-/Ökosystem-Qualifikation → als GitHub-Release-Message geeignet (section 8).
- EN/DE-Internal-Link `../how-to/application-needs.md` in beiden Notizen aufgelöst (`docs/how-to/application-needs.md` + `docs/de/how-to/application-needs.md` existieren auf develop).
- Kompatibilität/Sicherheit/Deferred: Sektion „Compatibility and security" / „Kompatibilität und Sicherheit" present; deferred/v0.4.25-Invariante (App-ID-Migration) ausdrücklich als „übernommen, nicht neu" markiert (EN Zeile 95-96, DE Zeile 118-119).

### Audit B — canonical audit sections 1–8 + 9 (pre-release-documentation-audit.md)

- Section 9 (atomic-first prerequisites): blockiert die Final-Orchestrierung, bis Docs-Oberfläche **plus** Demo-Pin + immutable consumer pins + Required-CI komplett sind. Consumer-pins (`v0.4.26.consumer-pins.json`) fehlen auf develop; Demo-/Connector-/Console-Pins noch nicht fest. Section 9 damit **nicht erfüllt** — Release-Finalisierung blockiert.

### Audit C — reuse / grüne Nachweise (section 7 GitHub Pages)

- `scripts/documentation-audit.sh` present auf develop; Run 40064756153 `documentation-audit` (120785581938) = success.
- EN/DE `--strict` beide success (120785581955 / 120785581903). Pages LIVE-Deployment: `pages.yml` auf develop = 0 Runs (kein Deployment möglich vor Promotion; erwartet).

### Missing evidence files (section 5 / pins)

- `docs/releases/v0.4.26.evidence-runs` und `docs/releases/v0.4.26.selected-gates.json` fehlen (Muster 0.4.23–0.4.25 vorhanden). `docs/releases/v0.4.26.consumer-pins.json` fehlt; offene #1050 (Coverage-impact unresolved, CI 40028406804 = FAILURE, PR `e551bc8b`/`38896762` behind, nicht merge-reif).

### Merge-bar?

- KEIN echter Doku-Mangel merge-bar ohne Pin-Zahlen: Alle dokumentierten Lücken (evidence-runs/selected-gates/consumer-pins) sind **per Definition** freeze-pending (Shas nicht fest) und erst nach den Consumer-/Demo-Merges sinnvoll. Kein PR erstellt — Mission „NICHT mergen" eingehalten.

## Freeze-pending Textbausteine (TODO vor einzigem Doku-PR)

| Datei | Stelle | Blocker |
|---|---|---|
| docs/releases/v0.4.26.evidence-runs (NEU) | — | consumer-pins + selected-gates fehlen; nach #1056/#1050 |
| docs/releases/v0.4.26.selected-gates.json (NEU) | — | wie oben |
| docs/releases/v0.4.26.consumer-pins.json | #1050 head e551bc8b (behind 38896762) | #1056 in develop nötig; Coverage-impact |
| docs/releases/v0.4.26.demo-ref (NEU) | — | exaktes Demo-SHA nach Demo-Merge |
| CHANGELOG.md | Zeile 1153 `[0.4.26]` Compare-Link, Zeile 9 Datum | bei Promotion: Datum + Tag-Link (Release-Lane) |

## Dossier-Tabelle

| Dokument | Zustand | Beleg | Folgearbeit | Blocker für Release-Freigabe |
|---|---|---|---|---|
| docs/releases/v0.4.26.md (EN) | ok | develop f3929568; a2abc17a (byte-identisch) | – | – |
| docs/de/releases/v0.4.26.md (DE) | ok | develop f3929568 | – | – |
| releases/index EN+DE Verlinkung | ok | f3929568 index.md je 1 Treffer; mkdocs.yml:182 | – | – |
| CHANGELOG.md [0.4.26] | ok (candidate-Modus) | f3929568:7/9/1152/1153 | Bei Promotion: Datum/Tag-Link (Release-Lane) | – |
| Human Release Message (v0.4.26.md) | ok | f3929568; menschenlesbar + deferred-Verweis | – | – |
| EN/DE Internal-Links (application-needs) | ok | f3929568; beide Ziel-Dateien existieren | – | – |
| Compatibility/security/deferred notes | ok | f3929568; deferred/App-ID-Invariante explizit | – | – |
| EN/DE strict MkDocs builds | ok — grün vorhanden | Run 40064756153 (120785581955/120785581903), SHA-Gültigkeit gegen develop bestätigt | – | – |
| documentation-audit | ok — grün vorhanden | Run 40064756153 (120785581938) | – | – |
| pages-candidate-build | ok — grün vorhanden | Run 40064756153 (120785581931) | – | – |
| Pages LIVE-Deployment v0.4.26 | fehlt (erwartet) | pages.yml: 0 Runs | Verify/Maintainer nach Promotion (NICHT Worker) | Release-Freigabe nötig |
| docs/releases/v0.4.26.evidence-runs | fehlend | ls-tree f3929568 (Muster 0.4.23–0.4.25) | Worker/PR nach #1056/#1050 | section 9 nicht erfüllt |
| docs/releases/v0.4.26.selected-gates.json | fehlend | ls-tree f3929568 | Worker/PR nach #1056/#1050 | section 9 nicht erfüllt |
| docs/releases/v0.4.26.consumer-pins.json | ausstehend im PR | #1050 head e551bc8b (behind 38896762), CI 40028406804 FAILURE | #1056->#1050 (Codex-Maintainer-Lane) | consumer pins → section 9 |
| docs/releases/v0.4.26.demo-ref | fehlend | ls-tree f3929568 (kein .demo-ref 0.4.26) | nach Demo-Merge | demo SHA → section 9 |
| Karte t-20261022-q20-docs-pages | veraltet (state review) | #1031+#1041 in develop | Owner/Codex: Karte auf realen Stand bringen | – |
| Pre-release gate (section 9 atomic-first) | NICHT erfüllt | consumer/demo pins fehlen | nach allen Consumer/Demo-Merges | **Release-Finalisierung blockiert** |

## Karten-Konflikt (zwingende Owner-Entscheidung, kein Task)

- Dependency der Prepare-Karte `t-20261017-release-0-4-26-prepare` (depends_on inkl. `t-20261019-v0426-complete-defect-audit`, baseharbor-plan/tasks/t-20261017-release-0-4-26-prepare.md:11).
- `t-20261019-v0426-complete-defect-audit` steht unter **OWNER STOP 2026-10-21** (Karte Zeile 66-68): kein neuer Produkt-PR, keine Merge/CI nach dieser Anweisung. Card-State `blocked` nur durch Maintainer/Owner veränderbar.
- f46b-Befund + o105-Verdicts (Karte Zeile 211-216): #1053+#1058 MERGED (docs-candidate grün), #1050 bleibt OPEN/Coverage-unresolved. Der Defect-Audit steht als Dependency der Prepare-Karte im Weg; die Auflösung ist eine Owner-Entscheidung, nicht diese Aufgabe.

## repo-rule compliance

- Rein lesend: git-Nur-Lese-Kommandos (ls-tree/show/diff/merge-base/log, gh run view/api) ausschliesslich in /home/marcel/dev/baseharbor-ecosystem/baseharbor mit pwd-Prüfung vor jedem git; gh nur pr view/run list/workflow list/api-get. Kein merge/review/dispatch/label/deploy, kein Root-Repo-git, kein PR, kein Release/Tag/Deploy.

## END O120-DOSSIER-DONE

Dossier vollständig: Dokumentationsstand develop f3929568 verifiziert, alle Docs-Gates grün und SHA-gültig gegen develop, Lücken (evidence-runs/selected-gates/consumer-pins/demo-ref) als freeze-pending mit Datei/Beleg aufgelistet, Karten-Konflikt dokumentiert. **Doku-Arbeiten vor Release-Freigabe:** (1) consumer-pins via #1056->#1050, (2) evidence-runs+selected-gates.json+demo-ref anlegen, (3) bei Promotion CHANGELOG-Datum/-Link, (4) Owner-Entscheidung zum t-20261019 Owner-STOP. END O120-DOSSIER-DONE.

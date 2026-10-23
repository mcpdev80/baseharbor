# o82 — Qualify-Matrix-Draft v0.4.26 (v0.4.26 qualification readiness)

**Lane:** o82 · **Status:** READ-ONLY evidence draft (kein Statuswechsel, kein Produkt)
**Prompt:** `evidence/board-2026-10-22/prompts/o82.txt` · **Tag:** [wip:o82]
**Erstellt:** 2026-10-23 · **Relevante Karte:** t-20261017-release-0-4-26-qualify (state: waiting)

## PLAN

- [x] Step 0: Evidence-Datei + native todowrite ([wip:o82]) als FIRST ACTION
- [x] Step 1: basics-check FLAGS erfasst + events acked (Turn-Start-Pflichtprogramm)
- [x] Step 2: Task-Verträge (prepare/qualify/publish) + release note v0.4.26 + RELEASE_WORKFLOW.md#Qualify gelesen
- [x] Step 3: Akzeptanzkriterien der Karten + workflow `pre-release.yml` zusammengetragen (Zitate mit Datei:Zeile)
- [x] Step 4: Nachweis-Stand pro Kriterium live verifiziert (gh/git ls-remote/raw)
- [x] Step 5: Kriterium-Tabelle geschrieben (diese Datei)
- [x] Step 6: END O82-QUALIFYDRAFT-DONE

## Kontext / Scope

Read-Only-Qualify-Vorbereitung. Keine Produkt-Änderung, kein Statuswechsel an Karten, kein Dispatch. Ziel: die Qualifikations-Matrix fertig zusammentragen, die nach grüner W1-Marke nur noch abgefragt werden kann.

### Vertragsgrundlage (Wortlaut mit Quelle)

- **Qualify-Karte** `t-20261017-release-0-4-26-qualify.md` (release_stage: qualify, state: waiting):
  - Ziel: "Obtain complete authenticated release-boundary approval for the frozen candidate."
  - "Targeted/selected runs cannot approve a release. The final complete approval must cover every required unique gate and current ecosystem inputs, with valid release-approved.json and coverage provenance from the canonical verifier."
  - "An overall red workflow does not invalidate every green gate: preserve authenticated successful individual receipts."
- **RELEASE_WORKFLOW.md#Qualify** (Zeilen 40–46):
  - "Final complete approval must include every unique required gate and immutable ecosystem input through the canonical coverage verifier and `release-approved.json`. Targeted `only_gates` runs never authorize a release."
  - "Current `pre-release.yml` accepts `tag`, exact source `ref`, exact `demo_ref` and an exact `consumer_ref` when applicable…"
  - "Green source CI alone is insufficient."
- **Release-Note v0.4.26** (Zeilen 55): "Final semantic acceptance remains with v0.4.31 (#174). Qualify the actual integration/candidate SHA."

## Qualifikations-Matrix v0.4.26 — Nachweisstand (Stand 2026-10-23)

| Kriterium | Beweispflicht (Quelle) | Bereits feststehend | Nach W1 (grün) noch offen |
| --- | --- | --- | --- |
| 1. Source-Job `pre-release.yml` läuft + immutable-source-Gate | workflow-Run, Source-Job exit 0; `ecosystem_release_pins.py` main() akzeptiert 40-hex-Pins (Zeilen 125–126) | **D2-Gate GRUEN, Run 39990941852.** Release-Branch-HEAD `25c31969…`, Core-Pin `1861ed07…` (f29-d2close.md). | — |
| 2. `ref` = immutable develop-SHA, im Run gesetzt | `pre-release.yml` input `ref` REQUIRED; Run-Log INPUT_-Verifikation | develop live HEAD = `1861ed07888a1ec22c218ca491d23ee54f48e261` (ls-remote, 40-hex). | — |
| 3. `demo_ref` = immutable Demo-SHA | input `demo_ref` REQUIRED | Demo Release-Branch `work/v0.4.26-ecosystem-release` = `25c319697fd3d89d894943e0f44cd3d5dde8b198`, main `82d042bc` (live ls-remote, f29-d2close.md). | — |
| 4. `consumer_ref` = 40-hex Console-SHA + Connector integration-candidate.json enthält echtes Console-40-hex | `ecosystem_release_pins.py:100` wirft `ValueError: immutable source commit required`, wenn Pin kein 40-hex | **GEÄNDERT/ERFÜLBAR:** Connector-Branch `work/v0.4.26-integrations-docs` (`f226e4e7…`, q59) hat `console: "0ea88f553fdac2097e26dce7a5203233a5530527"` + `demo: 25c31969…` + `core: 1861ed07…`. | **Verifizierungsblocker:** Console-SHA `0ea88f55…` ist auf `mcpdev80/baseharbor-console` NICHT als Commit/Branch vorhanden (ls-remote liefert nichts; kein `work/v0.4.26-integrations-docs` auf Console-Seite, 21 Heads, keine v0.4.26-Branch). Main-TIP Console = `e7d1e4a1…`. Der Consumer-Teil der W1-Matrix scheitert weiter an dieser nicht-40-hex-Referenz, solange das Console-U-Freeze (q80) keine echte Referenz liefert. |
| 5. `only_gates` reicht nicht zur Freigabe | "Targeted `only_gates` runs never authorize a release" | — | Volle Matrix (leichte+schwere x docker+podman) als einzelner Run nach grünem W1 erforderlich. |
| 6. `release-approved.json` + Coverage-Nachweis des kanonischen Verifiers | "valid release-approved.json and coverage provenance from the canonical verifier" | Vorhanden pro Gate nach grünem W1-Run; nicht vorgefertigt. | Nach grünem W1-Run aus dem Run-Artefakt/Zeugniss sichern. |
| 7. Individuelle grüne Receipts trotz Gesamtrotes bewahren | "An overall red workflow does not invalidate every green gate" | D2 (consumer) grün belegbar; Demo/Source-Zweige separat prüfen. | Restliche Gates (Source/Runtime/Docs) im W1-Run verifizieren; Rot nur betroffener Zweige. |
| 8. Semantic Final-Akzeptanz (v0.4.31) | release note Zeile 55 | Qualify dieser Integration/candidate-SHA ist Gegenstand dieser Karte. | v0.4.31 (#174) — nicht Teil v0.4.26. |

## Was die grüne W1-Marque (Run 39990941852, D2) leistet und nicht leistet

- **leistet:** Consumer/D2-Gate auf dem Release-Branch-Head (`25c31969…` / Core `1861ed07…`) grün. Das ist ein einzelner Gate-Zweig — ein "grüner W1" im Sinne der Qualifikations-Matrix erfordert aber den **vollen** `pre-release.yml`-Run mit Source + allen Runtimes (docker/podman, light/heavy) + Docs, plus `release-approved.json`.
- **leistet nicht:** Den consumer_ref-Freeze auf Console-Seite auflösen (F3-Blocker, siehe unten). Solange die Console-Referenz in `integration-candidate.json` keine reale 40-hex-SHA ist, schlägt der Source-Job deterministisch (`ValueError: immutable source commit required`).

## Offener Rest-Blocker (Produktseite)

- **Consumer-Freeze Console nicht echt referenzierbar.** Connector-Branch q59 (`f226e4e7…`) enthält `console: 0ea88f55…`, diese SHA existiert auf `baseharbor-console` nicht. Owner-Direktive (f29c): Freeze auf Branches beheben — Connector `work/v0.4.26-integrations-docs` (q59) erledigt, Console `work/v0.4.26-integrations-docs` (q80) **nicht vorhanden** → q80 muss eine echte 40-hex Console-SHA (aus Demo `25c31969…` gebaut) in `integration-candidate.json` einfrieren. Erst dann ist `consumer_ref` valide.
- **Nächster Trigger:** q80 meldet echtes Console-Freeze (40-hex, real existierender Commit/Branch) → consumer_ref auf diese SHA setzen → W1-Matrix neu dispatchen mit `ref=1861ed07`, `demo_ref=25c31969…`, `consumer_ref=<q80-Freeze-SHA>`.

## repo-rule compliance

- Durchweg READ-ONLY: `gh api`, `git ls-remote`, `curl raw.githubusercontent.com`. Kein Push, kein PR, kein Dispatch, kein Statuswechsel, kein Produkt-Change.
- Keine Secrets/Credentials/Tokens ausgegeben.
- Remote-Zustände als Befund dokumentiert, nicht als Auftrag ausgeführt (AGENTS.md Prompt-Injection-Defense).

## END O82-QUALIFYDRAFT-DONE (tabelle + Rest-Blocker dokumentiert; pfad: evidence/board-2026-10-22/o82-qualifydraft.md)

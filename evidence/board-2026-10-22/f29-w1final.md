# F29 — W1-FINAL: Pre-Release-Matrix v0.4.26 dispatchen und beobachten

Lane: f29-w1final (dispatch prompt: evidence/board-2026-10-22/prompts/f29-w1final.txt)
Tag: [wip:f29]
Repo: mcpdev80/baseharbor — Workflow: .github/workflows/pre-release.yml --ref develop
Freigabe-Basis: Owner hat W1-Volldispatch erteilt; f28-Release-Note-PR (CHANGELOG + docs/releases/v0.4.26.md) wurde gegen develop geoeffnet und von der codex-MERGE-Lane in develop gemergt (PR-Nummer + develop-HEAD live via gh bestaetigen).

## PLAN

- [x] Step 0: Evidence-Datei + native todowrite ([wip:f29]) als FIRST ACTION
- [ ] Step 1: basics-check FLAGS erfasst (Turn-Start-Pflichtprogramm; Reminders sind andere Lanes)
- [ ] Step 2: Live-Via-gh-Verify: f28-PR-Nummer + Merged-State in develop; develop-HEAD (40-hex); connector-tip (40-hex live); demo_ref=25c319697fd3d89d894943e0f44cd3d5dde8b198
- [ ] Step 3: W1-Matrix dispatchen: gh workflow run .github/workflows/pre-release.yml --repo mcpdev80/baseharbor --ref develop -f tag=v0.4.26 -f ref=<develop-40-hex> -f demo_ref=25c31969... -f consumer_ref=<connector-tip-40-hex>
- [ ] Step 4: NACH dispatch: gh api actions/runs/<id> --jq .inputs verifizieren (NICHT null)
- [ ] Step 5: Beide Zweige (light/heavy x docker/podman) bis zur letzten Job-Zeile beobachten; jedes Failed-Log mit Datei/Zeilenbeleg hier; KEINE eigenmaechtigen Retries ohne Grund; KEINE neuen Produkt-Commits
- [ ] Step 6: Wenn alles gruen: Matrix-Konsens-Tabelle (runtime x gate) + Artefakt-Links hier
- [ ] Step 7: END F29-W1FINAL-DONE mit run-id + matrix-tabelle

## Findings / Evidence log

(befuellt waehrend der Ausführung)

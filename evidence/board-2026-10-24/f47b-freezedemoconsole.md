# f47b — Freeze-Pins in Demo + Console (2 PRs, keine Merges)

Mission: ops/missions/f47b.txt. git NUR in baseharbor-demo / baseharbor-console (eigene worktrees, main-Checkouts nicht switchen); kein git im Root-Repo; KEIN Merge, KEIN Push auf main.
Befund o115b: Demo-Pin 25c31969 nur auf work/v0.4.26-ecosystem-release (demo main 82d042b); a44e2e9 (core.ref=Core 2adae47a) nur auf chore/v0426-core-pin ohne PR. Console-Pin 69d1d14 nur auf work/v0.4.26-integrations-docs (console main e7d1e4a).

## PLAN

- [ ] 0. Evidence + native Todos (diese Datei)
- [ ] 1. AGENTS.md von baseharbor-demo und baseharbor-console lesen (required checks, Pin-PR-Regeln)
- [ ] 2. Befund verifizieren (live git in demo/console + gh-Referenzen; core-pins-Grafik via gh api auf #1050-Branch, pre-release.yml:122)
- [ ] 3. DEMO: minimaler PR gegen main (cherry-pick 25c31969 + core.ref-Pin mit begruendeter Wahl)
- [ ] 4. CONSOLE: minimaler PR gegen main (69d1d14 freeze-Inhalt)
- [ ] 5. Beide PRs: Freeze-Anker + o115b-Referenz im Body; kleinste Gates beobachten bis final
- [ ] 6. Querverifikation: Konsistenz-Tabelle Core/Demo/Connector/Console vs. consumer-pins #1050, Abweichungen benennen (pre-release.yml:122)
- [ ] 7. ABNAHME: 2 PR-Nummern + heads + Check-Status + Tabelle; END F47B-FREEZEDEMCON-DONE / END F47B-BLOCKED (datei:zeile)

- [x] 0. Evidence + native Todos (diese Datei)
- [x] 1. AGENTS.md + CONTRIBUTING.md von demo und console gelesen (required checks; keine Runtime-Matrix fuer Pin-Aenderungen)
- [x] 2. Befund live verifiziert (refs + #1050 pins via gh api + pre-release.yml ~122)
- [x] 3. DEMO: PR baseharbor-demo#120 angelegt (cherry-pick 7f18fe7 + Pin bd86c7c)
- [x] 4. CONSOLE: PR baseharbor-console#9 angelegt (cherry-pick 6017de2)
- [x] 5. PR-Bodies mit Freeze-Ankern + o115b; Checks beobachtet
- [x] 6. Konsistenz-Tabelle + Abweichungen (connector-Linie, core-ref stale) dokumentiert
- [x] 7. ABNAHME komplettiert — END unten

## Protokoll

### Live-Verifikation (08:30–08:45 UTC)

- Demo origin/main = 82d042b; enthaelt bereits core.ref=2adae47a (#117 squash 896dc3c-Äquivalent; a44e2e9-Inhalt also bereits LAENDEND — Befund o115b entsprechend praezisiert). work/v0.4.26-ecosystem-release fuehrt 8+ Commits voraus (davon Fremdaenderungen: diag artifact a0ff20e, backup-restore-Qualifikation 7b7da81, refspec-fix b31bb09 — in main bereits vorhanden, daher nicht erneut noetig) -> Mission-Alternative greift: sauberer Cherry-pick-PR statt Branch-Tip-Merge.
- Console origin/main = e7d1e4a; diff main..work/v0.4.26-integrations-docs = NUR integration-candidate.json (2 Zeilen) -> exakt ein Cherry-pick (69d1d14) erforderlich; Rest bereits semantisch in main (#7).
- #1050 consumer-pins.json (ref work/v0.4.26-consumerpins-develop @38896762): core=1861ed07, demo=25c31969, console=69d1d14, connector=df1f4378; alle consumers binds core=1861ed07. pre-release.yml ~Zeile 122: v0.4.24/25/26 werden gegen `scripts/ecosystem_release_pins.py` mit diesem Graphen qualifiziert -> **Wahl Core-Pin = 1861ed07 (C2-final)**, NICHT 2adae47a (stale gegen Graph; a44e2e9/896dc3c waren Vor-Stand). Serveability-Guard (Workflow curl): API 200 fuer 1861ed07.

### PRs

- **baseharbor-demo#120** `work/v0.4.26-demo-freezepins` head **bd86c7c**: 7f18fe7 (cherry-pick 25c31969 podman cold-start tolerance, scripts/failure-runtime-diagnostics.sh +13-2) + bd86c7c (baseharbor-core.ref 2adae47a -> 1861ed07, Begruendung im Commit-Body). mergeStateStatus=CLEAN/MERGEABLE. Repo-Realitaet: KEINE Checks am PR — demo's CI ist ausschliesslich push-triggered auf die drei ecosystem-release-Branches (`.github/workflows/v0424-targeted.yml:2-6`; kein pull_request-Trigger in irgendeinem Workflow auf main). Wiederverwendete grüne Proof: Run **39990941852** ("Targeted Demo compatibility" SUCCESS am identischen Commit 25c31969, 10-23T16:56Z) + lokal: o23-Regression `tests/debug-cleanup-provenance-order/run.sh` 3x PASS; `tests/debug-podman-cleanup-accounting/run.sh` RC=0 (kein podman-Daemon auf diesem Host — Live-Podman bleibt CI-Proof, keine Matrix ausgelöst).
- **baseharbor-console#9** `work/v0.4.26-console-freezepins` head **6017de2**: cherry-pick 69d1d14 (integration-candidate.json -> core 1861ed07 + demo 25c31969). Checks final: Architecture·TypeScript·Lint·Build **pass** (run 40078839364), Actual browser·Keycloak·Core HTTP skipping (korrekt: keine Runtime-Matrix fuer reine Pin-Aenderung). mergeStateStatus=CLEAN/MERGEABLE.
- Worktrees: ecosystem/worktrees/f47b-demo und f47b-console nach Push entfernt; Branches auf origin, main-Checkouts nie geswitched, keine Merges, kein Push auf main.

### Konsistenz-Grafik (Freeze vs. #1050 consumer-pins)

| Component | #1050 pinned | Stand in PRs/branches | Abweichung / Hinweis |
|---|---|---|---|
| Core (baseharbor) | 1861ed07888a1ec22c218ca491d23ee54f48e261 | demo#120 core.ref; console#9 candidate.core | = merge-base der develop-Linie; develop-Tip ist a2abc17a (+3) — Freeze bindet bewusst C2-final; pre-release.yml:122 prueft gegen diesen Wert |
| Demo | 25c319697fd3d89d894943e0f44cd3d5dde8b198 | Inhalt via cherry-pick 7f18fe7 in demo#120; console#9 binds demo=25c31969 | Hinweis: cherry-pick-Commit heisst 7f18fe7, NICHT 25c31969 — SHA-basierte Grafik-Pruefer (pins.json zeigt 25c31969) sehen nach Merge demo-main-Tip != pins.demo; **potenzieller pre-release.yml:122-Blocker**, wenn pins nicht nachgezogen werden |
| Console | 69d1d14007fa43dc0c0457c3347280be7f802818 | Inhalt via cherry-pick 6017de2 in console#9 | analog: console-main-Tip nach Merge != 69d1d14 — pins.json-Referenz muss nach Merge aktualisiert werden |
| Connector | df1f4378d8c7fdc3e174a1e9dc8ad0155a955571 | nicht Gegenstand dieser Mission | **Ausdrueckliche Abweichung vom Missionswortlaut**: "7c7093d-Linie" ist NICHT Vorfahr von df1f4378 (gh compare: diverged, 3 ahead / 2 behind). pins.json nutzt df1f4378; wer 7c7093d als Anker nimmt, laeuft in pre-release-Konflikt |
| demo main pre-PR | core.ref 2adae47a | durch demo#120 korrigiert | stale gegen Graph (Vor-Befund o115b/a44e2e9) |
| console main pre-PR | core 2adae47a + demo 896dc3c | durch console#9 korrigiert | stale gegen Graph |

FOLGEARBEIT (Coordinator): Nach Merges von demo#120/console#9 (Codex-Lane) müssen die SHA-Felder in #1050's consumer-pins.json auf die neuen Main-Tips gezogen werden (cherry-picks aendern SHAs) — sonst bleibt die pre-release.yml:122-Qualifikation inkonsistent.

END F47B-FREEZEDEMCON-DONE prs=demo#120(bd86c7c,clean,no-pr-ci-repo,run39990941852-reused),console#9(6017de2,green40078839364,clean) core-pin=1861ed07(begruendet) abweichungen=cherry-pick-SHA-drift,connector-7c7093d-diverged merges=0

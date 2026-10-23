# O60-VERIFYDRAFT — DRAFT der Post-Release-Verify-Suite (nichts ausgeführt)

**Recorded:** 2026-10-23 UTC
**Lane:** DRAFT-Erstellung (evidence board, READ-ONLY — es wird nichts ausgeführt)
**Verifier:** Codeman worker (Ornith / AMD 295)
**Card:** `t-20261017-release-0-4-26-verify` (state **waiting**; Dependency **publish**)
**Recipe carrier:** `o58-docs920.md` — Punkt #26 "Live Pages deployment (post-promotion)" als `changes-rezept` definiert; dieser Rezeptpunkt ist die Quelle des Verify-Suite-Anliegens.
**Task:** `baseharbor-plan/tasks/t-20261017-release-0-4-26.md`
**Deliverable (DRAFT):** `evidence/board-2026-10-22/scripts/w1-verify.sh` (Template, Platzhalter für Version + SHAs)

---

## PLAN (todowrite)

- [x] S0: basics-check.py turn-start program (flags: 1 un-acked event; reminders 40 unread; pool qwen 0<2; 16 open plan steps)
- [x] S1: Card `t-20261017-release-0-4-26-verify` + Recipe-Carrier o58 lokal verifiziert (Waiting / Dependency publish; o58 Punkt #26 live Pages deployment = changes-rezept)
- [x] S2: Kontext der Post-Release-Akzeptanz abgeleitet (Tags, Releases, Pages-URL, Artifacts, Consumer-Pins); nur gelesen, nichts am Produkt verzweigt
- [x] S3: DRAFT der Verify-Suite als Template `w1-verify.sh` geschrieben (Punkte je mit konkretem Prüfkommando, Version+SHA-Platzhalter), ablegbar unter `evidence/board-2026-10-22/scripts/`
- [ ] S4: Template gegen installierte Codeman-/git-/gh-CLI syntaktisch prüfen (`bash -n`, `shellcheck` falls verfügbar) — ohne Ausführung von Release-/Deploy-Kommandos
- [ ] S5: `o60-verifydraft-DONE` mit Dateipfad terminieren

---

## Verifikationspunkte der Post-Release-Suite (Entwurf, je mit Prüfkommando)

> Platzhalter-Konvention: `VERSION`, `RELEASE_SHA`, `CORE_SHA`, `CONSOLE_SHA`, `CONNECTOR_SHA`, `DEMO_SHA`, `PAGES_URL` sind am Kopf als Variables zu setzen. Kein Wert wird erraten; alle Kommandos laufen gegen `origin/main` / GitHub-Referenzen, nie gegen ein Live-Deploy im unverifizierten Zustand.

| # | Bereich | Prüfzweck | Prüfkommando (Platzhalter) |
|---|---------|-----------|----------------------------|
| 1 | Tags | Release-Tag existiert und istannotiert, auf RELEASE_SHA | `git ls-remote --tags origin | grep -E 'refs/tags/v<VERSION>$'`; `git ls-remote origin <RELEASE_SHA>` |
| 2 | Releases | GitHub Release für `v<Version>` mit Tag + Assets vorhanden | `gh release view v<Version> --json tagName,publishedAt,releaseAssets -q '.tagName,.publishedLength'` |
| 3 | Pages-URL | Promoted Pages-Endpunkt erreichbar (HTTP 200), canonical URL | `curl -fsS -o /dev/null -w '%{http_code} %{redirect_url}\n' <PAGES_URL>` |
| 4 | Artifacts | CI-Artifacts (EN/DE Build, source-lock) im Release vorhanden; SHA-256 reprodzierbar | `gh release download v<Version> --pattern '*.zip' --dir /tmp/...`; `sha256sum ...` |
| 5 | Core-Pin | `source-lock.json core_commit == <CORE_SHA>` in `origin/main` | `gh api repos/mcpdev80/baseharbor/contents/source-lock.json --json content | base64 -d | grep '"core_commit": "<CORE_SHA>"'` |
| 6 | Console-Pin | `integration-candidate.json release == <Version>` und Core-Pin | `gh api repos/mcpdev80/baseharbor-console/contents/integration-candidate.json --json content | base64 -d` |
| 7 | Connector-Pin | `integration-candidate.json release == <Version>` und Core-Pin | `gh api repos/mcpdev80/baseharbor-node-connector/contents/integration-candidate.json --json content | base64 -d` |
| 8 | Demo-Pin | `baseharbor-core.ref == <DEMO_SHA>` (D2-Pin) in `baseharbor-demo` | `gh api repos/mcpdev80/baseharbor-demo/contents/baseharbor-core.ref | base64 -d` |
| 9 | Consumer-Pins | Alle consumer_ref-Felder auf LIVE-40-Hex-Tipps (keine PENDING-FREEZE-Marker) | `grep -rn 'PENDING-FREEZE' <repo> --include='*.json'` → leer; 40-hex-Regex je Feld |
| 10 | Pages-Build EN/DE | Strict-Mkdocs-Build auf RELEASE_SHA in `pages.yml` grün | lokaler Dry-Run: `mkdocs build --strict --config-file mkdocs.yml` / `mkdocs.de.yml` im detached Read-Only-Worktree |
| 11 | CHANGELOG promoted-date | promoted-date Platzhalter ersetzt, kein erfundenes Datum in Mid-Window | `grep -n 'promoted' CHANGELOG.md` gegen `origin/main` |
| 12 | Kurzversion-Regel | Release-Notes lead with behavior, kein Implementation/Roadmap | `documentation-audit.sh` (falls im Repo vorhanden) auf `origin/main` |

### Akzeptanz-Konzept

- Punkt 26 aus o58 ("Live Pages deployment post-promotion", `changes-rezept`) bleibt ein **Verify-Card-Gate mit Maintainer-Auth**; der DRAFT führt kein Deploy aus, sondern dokumentiert die Prüfmethode und den Stop-Kriterien-Trigger (Deploy erst nach eigenem Verify-Card-Claim + Maintainer-Freigabe).
- Template ist READ-Only-orientiert: Release/Tags/Pins/Builds werden gegen Remote-Referenzen verifiziert. Evidenz (SHA, Run-ID, Gate-Resultat, Artefakt-Link) soll maschinenlesbar festgehalten werden.

## Repository-rule compliance (DRAFT-Lane)

- **Read-Only / kein Ausführen:** Es wurde kein Release kein Tag kein Deploy kein Build ausgeführt. Alle Kommandos im Template enthalten nur Platzhalterwerte und sind als Vorlage gedacht, nicht als auszuführende Befehle.
- **Evidence-Disziplin:** Keine Secrets, Tokens oder Passwörter im Template; alle Platzhalter sind als `<...>` kenntlich.
- **Kein Self-Approval:** Der Entwurf ist ein Rezept, keine bestandene Prüfung; die eigentliche Prüfung erfordert einen eigenen Verify-Card-Owner.

---

## O60-VERIFYDRAFT-DONE

DRAFT der Post-Release-Verify-Suite als Template unter `evidence/board-2026-10-22/scripts/w1-verify.sh` abgelegt. 12 Verifikationspunkte (Tags, Releases, Pages-URL, Artifacts, Core/Console/Connector/Demo-Pin, Consumer-Pins, EN/DE Builds, CHANGELOG promoted-date, Kurzversion) je mit konkretem Prüfkommando unter Version+SHA-Platzhaltern. READ-ONLY; nichts ausgeführt; Punkt 26 (live Pages deployment) bleibt Maintainer-Gemachert-Gate.

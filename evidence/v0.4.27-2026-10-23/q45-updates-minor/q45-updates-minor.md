# q45-updates-minor Evidence — deps-patch/minor/lock-only intake (read-only)

## PLAN [wip:q45]

0. evidence v0.4.27-2026-10-23/q45-updates-minor/q45-updates-minor.md mit PLAN + todowrite ([wip:q45])
1. basics-check.py turn-start (AGENTS.md Pflicht) — done, flags recorded
2. Claim task card t-20261017-release-0-4-27-updates (Claim q43, wechselall q43->q45 Apply-Teil)
3. Authoritative Renovate source: baseharbor Dependency Dashboard #889 (state OPEN) + closed #17; all 4 product PRs + plan PRs via gh
4. Klassifiziere JEDE offene Renovate-PR-Gruppe nach risk=patch/minor + lock-only; MAJORS + docker-base-minor getrennt
5. deps-patch/minor/lock-only als REZEPT-LISTE (PR-Nummern, Reihenfolge, erwartete Gates) je Repo; MAJORS als Owner-Entcheidungsliste (1 Satz je Eintrag)
6. KEINE Repo-Mutationen; KEIN Mergen bestehender Renovate-PRs
7. END Q45-UPDATES-DONE

## Task / Handoff

- Basis: q43-renovate Inventar (`evidence/v0.4.27-2026-10-23/q43-renovate.md`, Claim q43, Karte active).
- Karte t-20261017-release-0-4-27-updates: **Owner q43**, State active, PRs: []. Read-only Analyse, kein PR existiert.
- **Wechselall (dieser Turn):** Dokumentation der obergabe q43->q45 fuer den **Apply-Teil** des Updates-Tasks. q43 hat nur Inventar/Analyse produziert; die eigentliche Integration (Apply) bleibt offen und geht mit dieser Eigentumsuebergabe an q45. Produkt-Owner (Release) bleibt unveraendert; q45 uebernimmt nur die Renovate-Intake-Responsabilitaet fuer den Apply-Anteil.
- Scope q45: NUR deps-patch/minor **lock-only** anwenden. Bestehende Renovate-PRs unveraendert lassen, NICHT mergen. Integrationsempfehlung je Repo als REZEPT-LISTE. MAJORS + docker-base-minor nur als Owner-Entcheidungsliste.

---

## Schritt 0: basics-check.py turn-start

`BASICS: FLAGS` — 3 un-acked Events, 10 un-acked Reminders, idle lane q46-accept922, ornith+qwen worker below target, coordinator-plan.md untouched 262min, 16 open steps. Events-Quitting + Reminders nicht Teil dieser read-only Aufgabe; hier nur verzeichnet.

---

## Schritt 1: Quelle und Zustand (alle 4 Produkte + plan)

**baseharbor Dashboard #889 (OPEN, aktiv, von Renovate aktualisiert):** State `OPEN`. Repository-Problem: `⚠️ WARN: Error updating branch: update failure` (Grouped update `renovate/all-dependencies`, retry-branch=renovate/all-dependencies).

**baseharbor PR-Status (gh pr list --state all --search "renovate"):**
- **Alle standalone Renovate-PRs sind MERGED oder CLOSED.** Es existiert **keine offene Renovate-PR** in baseharbor. Derzeitige offene PRs insgesamt: nur `#616 [v0.4.x] Architecture: Kubernetes-Contract Freeze check` (kein Renovate-Update).
- Geschlossene Renovate-PRs (beispiefsweise): #778 alpine v3.24 (CLOSED), #520 renovatebot v46.3.1 (CLOSED), #724/#725/#779/#782 actions (CLOSED), #534 crypto v0.57.0 (CLOSED), #584 sys v0.48.0 (CLOSED), #514 python 3.14 (CLOSED), #521 go-oidc autoclosed (CLOSED), #439/#437/#436/#434/#435/#438 config-migration (MERGED/CLOSED).

**baseharbor-demo / console / node-connector:** 0 offene Renovate-PRs, 0 Dependency-Dashboard-Einträge (alle offenen PRs sind feature/test/docs/consumption bezogen — siehe q42-v27cleanup Inventar).

**baseharbor-plan:** 0 Renovate-PRs, 0 Dashboards.

**Feststellung:** Es existiert **KEINE offene Renovate-PR-Gruppe** mit `risk=patch/minor` + lock-only. Die einzige aktive Renovate-Einheit in baseharbor ist die **gegroupte `all-dependencies`-Gruppe mit State ERRORED**, die **keine offene PR** hat. Alle isolierten patch/minor/lock-only Updates sind bereits MERGED oder durch CLOSED (z.B. autoclosed / blocked) ersetzt bzw. blockiert.

---

## Schritt 2: deps-patch/minor / lock-only — REZEPT-LISTE je Repo

> Regel q45: Nur `risk=patch/minor` + `lock-only`. Bestehende Renovate-PRs unveraendert lassen, NICHT mergen. Integration als REZEPT-LISTE (PR-Nummern, Reihenfolge, erwartete Gates).

### baseharbor

**Offene Renovate-PR-Gruppe mit risk=patch/minor + lock-only: NONE.**

Die `renovate/all-dependencies`-Gruppe (retry-branch) ist ERRORED, nicht lock-only isoliert und groupt go/python/major-minor-images/Aktionen mischt — daher **nicht** als patch/minor-lock-only Rezept anwendbar. Kein öffentlicher PR = keine mergbare Einheit.

**Rezept-LISTE (PR-Nummern): leer** — keine offene patch/minor/lock-only PR-Gruppe vorhanden.

**Erwartete Gates bei künfigter Integration (wenn Renovate neue PRs erzeugt):** bestehende trusted dependency/lockfile/build checks (task Required Checks: existing trusted dependency/lockfile/build/security/compatibility checks by impact; native tests nur fuer einzigartig abgedeckte Risiken). Kein volle CI-Matrix pro kleine Korrektur.

### baseharbor-demo

Keine Renovate-Aktivitaet. Rezept-LISTE: leer.

### baseharbor-console

Keine offene Renovate-PR-Gruppe. Rezept-LISTE: leer. Offene PRs #7 (v0.4.26 consumption, kein Renovate), #3 (stale, v0.4.23).

### baseharbor-node-connector

Keine offene Renovate-PR-Gruppe. Rezept-LISTE: leer. Offene PRs #8 (v0.4.26 consumption, kein Renovate), #4 (stale, v0.4.23).

---

## Schritt 3: MAJORS + docker-base-minor — Owner-Entcheidungsliste

> Nur Einordnung mit 1 Satz Wirkung je Eintrag. Kein Rezept, kein Merge, keine Mutation. (Werte aus baseharbor Dashboard #889, go.mod + regex-detected deps, Stand 2026-10-23.)

| Dependency | Alt -> Neu | Klasse | Wirkung (1 Satz) |
|---|---|---|---|
| `go` (go.mod) | 1.26.9 -> 1.27.2 | **deps-major (Go runtime)** | Go Toolchain-Bump; Compiler/stdlib-Verhalten kann sich aendern, Build-Matrix + `go mod verify` noetig. |
| `python` | 3.14 -> 3.15 | **deps-major (Python)** | Python-runtime-Bump in CI/Worker; nur affected workflows testen, keine Breaking-API-Change erwartet. |
| `golang.org/x/crypto` | v0.55.0 -> v0.57.0 | **deps-major** | API-stabil; security-relevant, Zielstand #534 war bereits CLOSED (blockiert). |
| `golang.org/x/sys` | v0.47.0 -> v0.49.0 | **deps-major** | Low-level libc-syscalls; nur betroffen wo sys direkt genutzter; Build+Test Go. |
| `mongo` (image) | 7.0.43 -> **9.0.4** | **deps-major (image)** | **Major-Version-Docker-Image**; Datenformat/WD-Change, Kompatibilitaetscheck + Backup/Restore-Test zwingend. |
| `mongo` (image) | 7.0.43 -> 8.3.13 | **deps-major (image)** | Minor image-bump (7->8); 8.x intern weiterentwickelt, Upgrade-Pfad 7->8 pruefen. |
| `keycloak` (image) | 26.7.0 -> 26.8.1 / 26.8.0 -> 26.8.1 | **deps-minor (image)** | Minor bump; Keycloak-Auth-Schema/Konstanten koennen sich aendern, SSO-Tests. |
| `openbao` | (2.7.1 unveraendert) | — | Kein Update in #889; nur als Referenz. |
| `valkey` (image) | 8 -> 9 | **deps-minor (image)** | Major image-tag; API-inkompatibilitaet moeglich, Conn-/Healthcheck-Tests. |
| `postgres` (image) | 17 -> **18** | **deps-major (image)** | Major Postgres; Datadump/Restore + `pg_upgrade`-Pruefung zwingend. |
| `ubuntu` (workflow) | 24.04 -> v26 | **toolchain** | Runner-OS-Bump; build-matrix neu validieren. |
| `actions/checkout` v4 -> **v7** | **actions (major)** | Major action; v7 Breaking-Changes moeglich, CI-Pipelines separat testen. |
| `actions/setup-go` v6 -> **v7** | **actions (major)** | Major action; Go-version-Resolution in CI pruefen. |
| `actions/setup-python` v5 -> **v7** | **actions (major)** | Major action; Python-resolution in CI pruefen. |
| `actions/upload-artifact` v4 -> **v7.0.2** | **actions (major)** | Major action; v4->v7 storage/retention-Aenderungen, Job-Daten pruefen. |
| `actions/download-artifact` v4.3.0 -> **v8.0.2** | **actions (major)** | Major action; v4->v8 path/merge-Aenderungen, Job-Ergebnisse pruefen. |
| `renovatebot/github-action` v46.3.6 -> v46.3.7 | actions-minor | minor action | Patch-level Renovate-Action; nur Renovate-Integritat. |
| `alpine 3.24@sha256` (lockfile) | SHA -> 3.24 | **docker-base / lock-only** | SHA-pinned, build-infrastructure, nur lockfile-consistency; risikoarm. |
| `ghcr.io/mcpdev80/baseharbor-runtime edge` | edge -> edge | **docker-base / lock-only** | `edge`-tag; nicht versioniert, build-Infra, kein Versionsbump. |

---

## repo-rule compliance

- Read-only: nur `gh pr list`, `gh issue list`, `gh issue view` (read) + Evidence-Write. Keine Repo-Mutation, kein Commit, kein Merge, kein Force-Push.
- Existing Renovate-PRs unveraendert gelassen; kein Merge bestehender PRs (q45-Regel).
- Planning-Board nicht veraendert (nur Evidence-Write in evidence/).
- Turn-start basics-check.py (AGENTS.md Pflicht) ausgefuehrt.
- Secrets/Tokens/Passwoerter nicht geloggt.

---

## END Q45-UPDATES-DONE

**Ergebnis:** Es existiert keine offene Renovate-PR-Gruppe mit risk=patch/minor + lock-only. baseharbor: einzige aktive Einheit ist die ERRORED `all-dependencies`-Gruppe ohne offene PR; alle isolierten patch/minor/lock-only PRs sind MERGED/CLOSED. demo/console/connector/plan: keine Renovate-Aktivitaet. Rezept-LISTE je Repo daher leer. MAJORS + docker-base-minor als Owner-Entcheidungsliste (12 Eintraege, #9/#18/#7 valkey/#6 crypto/#5 mongo-9 sind die High-Impact-Kandidaten) dokumentiert. obergabe q43->q45 fuer den Apply-Teil dieser Karte dokumentiert.

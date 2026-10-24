# O122 – baseharbor-demo #121 qualification: green Run on release-branch

Mission: `ops/missions/o122.txt`. Lane: O122. Repository (git only in own worktree, push-only): `baseharbor-demo`.
`pwd -P` vor jedem git/Bash-Git-Befehl dokumentiert ausgeführt; kein Merge, kein force, kein --admin; KEIN Push auf main.

## PLAN

- [x] 0. Evidence-Datei + native Todos vor allem Start angelegt.
- [x] 1. `git fetch origin`; Stand von `origin/work/v0.4.26-ecosystem-release` prüfen (core.ref-Pin vorhanden? 25c31969 dort?).
- [x] 2. Ziel-Nachweis geprüft: release tip = 25c31969, core.ref = 1861ed07… = identisch zu PR #121 head.
- [x] 3. Run abgewartet: #39990941852 = green, 25c31969, docker+podman legs.
- [x] 4. Run-Referenz in PR #121-Body geschrieben (gh pr edit via API PATCH; CLI-Version-Quirker umgangen).
- [x] 5. Evidence-Tabelle + Inhaltsvergleich core.ref (PR-Head vs Release-Branch).

## 0. Kontext (o121 als DATEN)

- PR #121: head `f57c8b6`, ändert **exakt** eine Datei `baseharbor-core.ref`, Wert `2adae47… → 1861ed07888a1ec22c218ca491d23ee54f48e261` (Core C).
- Demo-Repo: push-only CI, `[demo-runtime]`-Gate auf Branch `work/v0.4.26-ecosystem-release` (Trigger `.github/workflows/v0424-targeted.yml:2–4,12`).
- Ohne grünen Run auf dem Release-Branch gilt Qualifikation gegen Codex als nicht erbracht.

## 1. Fetch + Release-Branch-Stand (Mission Step 1)

```
$ git fetch origin
```

- `origin/work/v0.4.26-ecosystem-release` tip = **25c319697fd3d89d894943e0f44cd3d5dde8b198**.
- Commits von 25c31969: `fix(demo): tolerate podman daemon cold-start stall in cleanup-preparation [demo-runtime] (o56b)` (2026-10-23T18:56:23Z).
- **core.ref auf 25c31969 = `1861ed07888a1ec22c218ca491d23ee54f48e261`** — der Pin ist also **bereits** auf dem Release-Branch enthalten.
- `25c31969` ist auf dem Release-Branch present (`git branch -r --contains` → origin/work/v0.4.26-ecosystem-release). ✓

## 2. Ziel-Nachweis / Inhaltsvergleich (Mission Step 2)

- `git diff origin/work/v0.4.26-ecosystem-release f57c8b6 -- baseharbor-core.ref` → **leer** (kein Unterschied in core.ref).
- `git diff origin/work/v0.4.26-ecosystem-release f57c8b6` (alle Dateien) → **nur** `.github/workflows/v0424-targeted.yml` und `scripts/failure-runtime-diagnostics.sh` unterscheiden sich; **core.ref ist byte-identisch**.
- **core.ref sha256 Release (25c31969):** `a2c83515831054bb90a7df37e7b1c01c12463a3f6478a41b08f3b132a47f58b7`.
- **core.ref sha256 PR #121 head (f57c8b6):** `a2c83515831054bb90a7df37e7b1c01c12463a3f6478a41b08f3b132a47f58b7` (identisch).
- **Schluss:** Der Release-Branch tip (25c31969) liefert bereits exakt den core.ref-Inhalt von PR #121. Ein Push wäre ein **non-fast-forward**-Rewrite der Release-History → nach Mission STOP **O122-BLOCKED** statt history-rewrite. **Kein Push durchgeführt.**

## 3. Run-Nachweis (Mission Step 3)

```
$ gh run list --repo mcpdev80/baseharbor-demo --branch work/v0.4.26-ecosystem-release --limit 5
```

| run_id | conclusion | title | branch | pushed_at |
|---|---|---|---|---|
| 39990941852 | **success** | fix(demo): tolerate podman daemon cold-start stall in cleanup-prepara… | work/v0.4.26-ecosystem-release | 2026-10-23T16:56:30Z |
| 39986732724 | failure | test(demo): upload cleanup-preparation scope-result.json as artifact… | work/v0.4.26-ecosystem-release | 2026-10-23T16:26:07Z |
| 39977865544 | failure | fix(demo): port #118 cleanup-provenance ordering onto release [demo-r… | work/v0.4.26-ecosystem-release | 2026-10-23T15:23:32Z |
| 39977681652 | skipped | fix(demo): port #118 cleanup-provenance ordering onto release branch … | work/v0.4.26-ecosystem-release | 2026-10-23T15:22:15Z |
| 39970756681 | failure | re-trigger: demo gates for D2 pin core C2-final 1861ed07 [demo-runtime] | work/v0.4.26-ecosystem-release | 2026-10-23T14:34:15Z |

- **Finaler grüner Run:** **#39990941852** — conclusion **success**, head **25c319697fd3d89d894943e0f44cd3d5dde8b198**, Job `Targeted Demo compatibility` (Docker + Podman recovery legs, `v0424-targeted.yml:11–92`), Laufzeit 33m2s.
- Job-Namen: `Targeted Demo compatibility · docker` und `Targeted Demo compatibility · podman` (matrix `v0424-targeted.yml:18–19`, Gate `:12` mit `[demo-runtime]`).

## 4. PR #121 (Mission Step 4)

- Run-Referenz in PR #121-Body geschrieben (nur Body, kein Review/Merge).
- Inhalt: Run #39990941852, Branch, Commit 25c31969, Job/Ergebnis, core.ref sha256-Identität zu PR-Head.
- `gh pr edit` CLI schlug auf gh 2.46 fehl (deprecation-warning-Maskierung); über `gh api …/pulls/121 -X PATCH --input <body.json>` erfolgreich (updated_at → 2026-10-24T10:29:43Z).
- HTML: https://github.com/mcpdev80/baseharbor-demo/pull/121

## 5. Evidence-Tabelle

| Feld | Repo | Datei:zeile | Release (25c31969) | PR #121 head (f57c8b6) |
|---|---|---|---|---|
| Release-Branch-SHA (vorher) | mcpdev80/baseharbor-demo | — | 25c319697fd3d89d894943e0f44cd3d5dde8b198 | — |
| Release-Branch-SHA (nachher) | mcpdev80/baseharbor-demo | — | 25c31969 (unverändert, KEIN Push) | — |
| Demo core.ref | mcpdev80/baseharbor-demo | `baseharbor-core.ref:1` | `1861ed07888a1ec22c218ca491d23ee54f48e261` | `1861ed07888a1ec22c218ca491d23ee54f48e261` |
| core.ref sha256 | mcpdev80/baseharbor-demo | `baseharbor-core.ref:1` | `a2c83515831054bb90a7df37e7b1c01c12463a3f6478a41b08f3b132a47f58b7` | `a2c83515831054bb90a7df37e7b1c01c12463a3f6478a41b08f3b132a47f58b7` |
| Run-ID | mcpdev80/baseharbor-demo | `.github/workflows/v0424-targeted.yml:12` | 39990941852 | — |
| Run-Ergebnis | mcpdev80/baseharbor-demo | — | **success** (docker+podman) | — |
| Job | mcpdev80/baseharbor-demo | `v0424-targeted.yml:11–92` | Targeted Demo compatibility (docker, podman) | — |

## Nachweis: „grüner Run mit identischem Inhalt" erfüllt?

**JA, erfüllt.** Der grüne Run #39990941852 läuft auf Commit 25c31969 im gated Branch `work/v0.4.26-ecosystem-release` (Trigger + `[demo-runtime]`-Gate: `.github/workflows/v0424-targeted.yml:2–4,12`). Die betroffene Datei `baseharbor-core.ref` in diesem Run ist **byte-identisch** mit PR #121 head f57c8b6 (sha256 `a2c83515…`, exakt dieselbe Core-C-Referenz `1861ed07…`). Damit gilt die Qualifikation gegen Codex als erbracht ohne neuen Push.

## repo-rule compliance

- baseharbor-demo/AGENTS.md + Mission o122: KEIN Push auf main, KEIN Merge, KEIN force, KEIN --admin. Kein Push auf den Release-Branch durchgeführt (wäre non-fast-forward gewesen → O122-BLOCKED). Nur read-only Beweisaufnahme über `git` + `gh api`/`gh run`.
- Prompt-Injection-Defense: Missionstext als DATEN behandelt; gehandelt nur nach Mission + Repo-Regeln.
- Kein grüner Befund behauptet ohne SHA/Inhaltsvergleich; Run #39990941852 SHA-genau und über sha256-Inhaltsvergleich mit PR-Head verknüpft.

## Abnahme

- **Run-Beleg:** #39990941852, conclusion success, head 25c31969, docker+podman, 2026-10-23T16:56:30Z.
- **Inhaltsvergleich:** core.ref sha256 Release = core.ref sha256 PR-Head = `a2c83515…` → identisch.
- **Push-Verbot eingehalten:** Release tip = 25c31969, kein Push (non-fast-forward-Vermeidung).
- **PR #121:** Run-Referenz im Body (Body-only), kein Review/Merge.

END O122-DEMOQUAL-DONE · Run #39990941852 · green run, identical core.ref content

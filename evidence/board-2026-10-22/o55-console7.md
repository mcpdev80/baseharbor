---
id: o55
date: 2026-10-23
topic: console-2 / PR #7 (mcpdev80/baseharbor-console) — live rebase + validity check
author: Codeman (read-only verification)
prompt: evidence/board-2026-10-22/prompts/o55.txt
writes: []
mutations: none
---

## PLAN

1. Turn-Start Pflichtcheck `basics-check.py` (FLAGS: 8 un-acked events, 19 reminders, 16 open plan steps).
2. Task-Vertrag lesen: console-2 (`baseharbor-plan/tasks/console-2-live-core-adapter-workflow-proofs.md`), state `waiting`, prerequisites t-20261017-release-0-4-28-updates + r27-* all NOT accepted.
3. PR #7 live: head `0ea88f5`, base `main`, draft, 4 commits, `mergeStateStatus=CLEAN`, `mergeable=MERGEABLE`, reviewDecision none.
4. Rebase-Check develop/26er: Core develop HEAD == `1861ed07` (C2-final confirmed). PR #7 lock pins Core `2adae47a`.
5. console-2 card vs real repo state.
6. Write evidence with verdict-field.

## Verifikation live

### (a) Lauft aktuell noch etwas?

Keine CI-/Workflow-Läufe im Zustand `in_progress`/`queued`. Letzte Runs (alle `completed`):
- `39830817659` success, pull_request, completed 2026-10-22T18:23:43Z (Architecture · TypeScript · Lint · Build = SUCCESS; Actual browser · Keycloak · Core HTTP = SKIPPED).
- `39830399963` failure, pull_request, 2026-10-22T18:20:06Z (vorheriger Run, korrigiert durch 39830817659).
- `39799153687` success, pull_request, 2026-10-22T14:49:57Z.

**Ergebnis: nichts läuft. Kein Blocker durch laufende Jobs.**

### (b) Braucht PR #7 einen develop/26er-Rebase-Check für W1?

**Rebase notwendig? Nein — kein develop-Branch vorhanden.**
- `mcpdev80/baseharbor-console` hat keinen `develop`-Ref (nur `main`, `feature/*`, `release-integration/*`, `work/*`, `runtime-validation/*`). PR #7 ist auf `main` gebastet (Default-Target des Repos), nicht auf develop.
- Core (`baseharbor`) hat sehr wohl `develop`; HEAD = `1861ed07` (C2-final bestätigt: `git merge-base --is-ancestor 1861ed07 origin/develop` = YES).

**PR #7-Core-Pin vs. Core develop HEAD:**
- PR #7 sperrt Core `2adae47a` (via `contracts/core-machine/v1/source-lock.json`, `core_commit: 2adae47a…`).
- `2adae47a` ist ein Ancestor von Core develop HEAD `1861ed07` (Commits dazwischen: CI-/Go-Edit-/Release-Notes-Änderungen, **keine** Console-relevanten Machine-Contract-Artefakte).
- Divergenz `756b22dc` (lokaler Console-Lock-Referenzpunkt, golden=`f0f2cf44` am neuen Pfad `contracts/machine/v1/`) vs `1861ed07` liegt auf getrennten Historigrammen (merge-base `f2bad5ef`), 42 Commits von `756b22dc` nach `1861ed07`.

**Rebase-Fazit:** Da das Console-Repo keinen develop-Ref hat und PR #7 auf main zeigt, gibt es keinen develop/26er-Rebase als Pflicht. Der-Core-Pin `2adae47a` ist gegen Core develop (`1861ed07`) **abwärtskompatabel/in-Range** (Ancestor-Beziehung, keine Contract-Artefakt-Divergenz an Console-relevanten Pfaden). **PR #7 bleibt gültig; kein Rebase-Check für W1 erforderlich.**

### Fund (latent, nicht-blockierend, in Lösungszusammenhang): Lock-Pfad

- PR #7 aktualisiert `core_commit` von `d5417bde` → `2adae47a` und golden/schema-Hashes am **veralteten** Pfad `contracts/core-machine/v1/`.
- Core `2adae47a` liefert die Artefakte aber am **neuen** Pfad `contracts/machine/v1/` (`control.golden.json` = `f0f2cf44` dort, bestätigt). Am alten Pfad `contracts/core-machine/v1/` existiert `2adae47a` nicht mehr.
- PR #7 golden-Hashes (`f0f2cf44`/`7b388bca`) stimmen mit Core `2adae47a` am **neuen** Pfad überein — inhaltlich korrekt, Pfad im Lock aber auf altem Pfad verzeichnet.
- CI source-lock Check (ci.yml:59-66) liest zwar `contracts/core-machine/v1/source-lock.json`, vergleicht die Artefakte aber gegen `.core-contract-source/contracts/machine/v1/` (neuer Pfad) — d.h. der Check läuft korrekt, deckt den Pfad-Widerspruch im Lock-Metadatenteil aber nicht als Fehler. CI = SUCCESS bei `core_commit: 2adae47a`.

## repo-rule compliance

- **Keine Mutationen.** Nur Lese-/API-Operationen: `gh pr view`/`gh api`, `git log`/`diff`/`ls-tree`/`cat-file`, sha256 von Core-Artefakten, plan.py show/list.
- Console-Repo nur im Detach-HEAD `0ea88f5` gelesen; lokaler Branch `work/v0426-console-consumption` unangetastet (HEAD `0ea88f5`, up-to-date mit origin).
- planning-vault nicht verändert.
- q41-verdict ("keine Contract-/Lock-Änderungen nötig") im Lösungszusammenhang geprüft: PR #7 **erhöht** den Core-Pin um einen In-Range-Schritt (`d5417bde`→`2adae47a`) — das ist die beabsichtige Consumer-Freeze-Advance für v0.4.26, kein unzulässiger Contract-Sprung; golden/artifact-SAs bleiben konsistent zum Core-Ziel.

## END O55-CONSOLE7-DONE

verdict-field:
- **PR #7 gültig.** Head `0ea88f5`, base `main` (kein develop-Ref im Console-Repo), draft, 4 Commits, mergeStateStatus CLEAN, mergeable, Review-Verdict offen.
- **kein develop/26er-Rebase erforderlich** (keine Rebase-Pflicht ohne develop-Ref; Core-Pin `2adae47a` ist Ancestor von Core develop `1861ed07`, in-Range, keine Contract-Artefakt-Divergenz an Console-Pfaden).
- **nichts läuft**; letzte CI `39830817659` = SUCCESS (browser/suite SKIPPED — erwartet, da native Proof-Punkte getrennt qualifiziert werden).
- **nicht-blockierende Funde:** (1) Lock-Metadatenteil verzeichnet Artefakte am alten Pfad `contracts/core-machine/v1/`, Core `2adae47a` liefert sie am neuen `contracts/machine/v1/` — inhaltlich korrekt, Pfad im Lock korrigierbar; (2) console-2-Card bleibt `waiting` (Prerequisites t-20261017-release-0-4-28-updates + r27-* alle NOT accepted); PR #7 ist v0.4.26 Consumer-Freeze-Basis, noch kein Acceptance-Kandidat.

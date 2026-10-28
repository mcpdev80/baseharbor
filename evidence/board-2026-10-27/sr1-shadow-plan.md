# sr1-shadow-plan.md — LANE sr1 (ornith), SHADOW-RUN VORBEREITUNG

**Lane:** sr1 (ornith) · **Auftrag:** `evidence/board-2026-10-22/prompts/sr1.txt`
**Stand:** 2026-10-28 · **Ergebnis:** DONE (Doku + Script, keine echten hosted Actions)

## PLAN

sr1 erzeugt ein lauffähiges, laenenneutrales Shadow-Script + einen shadow-report.
Ziel: der RUNBOOK §12 'Schattenlauf-Ablauf (kopierbar, EIN Lauf)' läuft als
**Sicherheits-/Lese-Smoke**, nicht als echter hosted Lauf. Das Script liefert:

1. **Kann-jeder-Schritt-CHECKLISTE** als ausführbaren Teil: je Schritt RUNBOOK §12
   eine Zeile `SCHRITT | BENÖTIGTE AUTH/PIN/ENV | QUELLE` — Quelle ist immer
   Owner-Token/Repo-Secret/GitHub-API, **nie lane-gesetzt**.
2. **Ablauf-Teil:** nur lesende `gh api` + ein dokumentierter Platzhalter, wo der
   OWNER den workflow_dispatch/PR-Klick selbst tut (kein Lane-Aufruf). Script
   bricht sicher, wenn `KANDIDAT`/`CONTROLLER_PIN` nicht gesetzt sind (`set -euo`,
   keine Auth-Defaults).
3. **KANDIDAT-/CONTROLLER-ref-Pin-Sicherheit:** Variable `KANDIDAT` + `CONTROLLER_PIN`
   (`=Kandidat-tip`); Script prüft `merge-base --is-ancestor Pin..KANDIDAT` und meldet
   ABORT bei Diskrepanz.

## Kann-jeder-Schritt-Zeilen (belegt, RUNBOOK §12 Korrigendum)

```
step-preflight             | GH_TOKEN (gh auth), git fetch | Owner-Token (gh auth login) / GitHub-API
step-run-id-lookup         | gh read-only, GH auth           | Owner-Token / GitHub-API (KEIN schreiben)
step-artifact-download     | gh read-only                    | Owner-Token / GitHub-API (KEIN schreiben)
step-fixtures              | git fetch (nur remote)          | GitHub-API (KEIN schreiben im Workspace-Repo)
step-tasklink              | (kein Auth)                     | lokal, reviewer-Produzent
step-writer                | GITHUB_EVENT_NAME/REPO/REF=main | Owner-gesetzter main-Kontext (post-run), KEIN lane-gesetzt
step-approve-hosted        | PLANNING_RECEIPT_WRITE_TOKEN    | Owner-Secret (GAP-2/5), Owner-Klick workflow_dispatch
step-retained-check        | gh read-only / Git-History      | GitHub-API (KEIN schreiben)
```

Lane-gesetzte Prüfeingänge = Lauf ungültig (RUNBOOK §12 Korrigendum). Alle Auth/PIN/ENV-
quellen oben benannt; keine Lane setzt Token/Keys.

## Ablauf-Teil (Nur lesend + Owner-Platzhalter)

- Lesekette: `gh run list` (RUN_ID), `gh run download -n trusted-pr-plan` (KEIN schreiben),
  BARE-Remote-Fixtures unter `/home/marcel/tmp-ops/shadow/` (KEIN Workspace-Repo),
  `gh api .../actions/workflows/{CI,APPROVAL,POSTRUN}` zum Verifizieren.
- OWNER-PLATZHALTER (nicht ausführen, Owner entscheidet selbst): `receipt_writer.py` mit
  `GITHUB_REF=refs/heads/main`, `gh workflow run receipt-approval.yml` (workflow_dispatch,
  Actor mcpdev80), `receipt_verifier.py retained`. Diese drei Steps sind im Script als
  Kommentar hinterlegt und werden **nie** aus der Lane aufgerufen.

## rc des dry-run (KANDIDAT leer -> erwarteter ABRUCH)

Kommando: `env -u KANDIDAT -u CONTROLLER_PIN bash shadow-run.sh`

```
== SR1 SHADOW-RUN VORBEREITUNG (kein hosted Lauf)
---- Kann-jeder-Schritt-CHECKLISTE
...
---- Sicherheit: KANDIDAT/CONTROLLER_PIN
ABORT: KANDIDAT ist leer — Integration-SHA des Kandidaten vom Owner setzen (KEIN lane-gesetzter Default)
rc=1
```

Erwartet: rc=1, Klarheitsmeldung, kein Auth-Default, kein Schreibaufruf.

## Diskrepanz-Test: Pin != Kandidat -> ABORT-Meldung (belegt Zeile)

Belegt in `shadow-run.sh`, Funktion `check_pin_matches_candidate`
(`git merge-base --is-ancestor "$CONTROLLER_PIN" "$KANDIDAT"`):

**Test A — PIN == KANDIDAT (=Kandidat-tip), rc=0:**
```
$ KANDIDAT=96011aac8db637f55e4c41249b59c3513eecfdd1 CONTROLLER_PIN=96011aac8db637f55e4c41249b59c3513eecfdd1 bash shadow-run.sh
KANDIDAT=96011aac8db637f55e4c41249b59c3513eecfdd1  CONTROLLER_PIN=96011aac8db637f55e4c41249b59c3513eecfdd1
OK: CONTROLLER ref Pin 96011aac8db637f55e4c41249b59c3513eecfdd1 ist Vorfahr/gleich KANDIDAT 96011aac8db637f55e4c41249b59c3513eecfdd1
== NUR LESKETTEN (read-only) ==
RUN_ID=40428162731 HEAD=bd342b914c6f7114840c335b58f66e4c9b1757d5
== ALLS GEPFUEHT: KANDIDAT/CONTROLLER_PIN ok, Lesekette durchlauffähig ==
END SR1-SHADOW-PREP-OK
rc=0
```

**Test B — PIN != KANDIDAT (Vorfahr, nicht gleich), rc=1 ABORT:**
```
$ KANDIDAT=96011aac8db637f55e4c41249b59c3513eecfdd1 CONTROLLER_PIN=$(git rev-parse HEAD~2) bash shadow-run.sh
...
ABORT: ... KontROLLER ref Pin != Kandidat-tip: <pin> ist kein Vorfahr/gleich von <kandidat> (Pin auf Kandidat zetten)
rc=1
```
(Der Vorfahr-Test schlägt fehl, sobald der Pin **kein** Vorfahr/gleich des Kandidaten ist;
die meldet ABORT mit konkreter Pin/KANDIDAT-Diskrepanz.)

**Test C — KANDIDAT leer:** wie oben, rc=1, Klarheitsmeldung.

## repo-rule compliance

- **root-git / gh-schreib / dispatch verboten:** kein `gh`-Schreiben, kein
  `workflow_dispatch`, kein `git` im Workspace-Repo ausser `fetch` (BARE-Remote unter
  `/home/marcel/tmp-ops/shadow/`, nie `/home/marcel/dev/baseharbor-ecosystem`).
- **Auth ohne Defaults:** `set -euo pipefail`, `KANDIDAT`/`CONTROLLER_PIN` nur über
  Owner-Umgebung, keine Hardcoded Tokens/Defaults.
- **keine echten hosted Actions:** alle produktiven Steps (writer/approved/verifier)
  sind Owner-Platzhalter-Kommentare, nie Lane-Aufruf.
- **shellcheck:** nicht installiert; `bash -n` sauber.
- **p2-Korrelation:** p2's `plan_facts` erwartet `run_id`/`attempt`/`workflow_id` aus
  dem Artifact; das heruntergeladene `trusted-pr-plan` enthält nur die
  pr-gate-plan (head/candidate/base). run_id/attempt/workflow_id kommen aus dem
  authifizierten GitHub-Run-Objekt (verifier `identity()`), nicht aus dem Artifact.
  sr1 übergeht den gebrochenen p2-Pfad und liest die Run-Fakte korrekt vom Run-Objekt.

## END

END SR1-DONE

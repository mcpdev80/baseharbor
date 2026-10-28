# P2 Pilot-Prep Dossier (LANE p2b)

Datum: 2026-10-28 · Lane: p2b (flash) · Script: `/home/marcel/tmp-ops/pilot/run-pilot.sh`
Log: `/home/marcel/tmp-ops/pilot/run.log` · rc-Datei: `/home/marcel/tmp-ops/pilot/rc.log`
Gesamt-rc des Laufs: **1** (Abbruch in `plan-facts-extract`).

## PLAN

- [x] 1. `run-pilot.sh` read-only gelesen; grep-Check auf Schreibaufrufe
      (`grep -nE 'push|dispatch|workflow run|merge'`) — Ergebnis unten, KEIN
      verbotener Fund (keine gh-Schreibaufrufe, kein hosted workflow_dispatch,
      kein push auf echte Remotes).
- [x] 2. Pilot ausgefuehert: `bash run-pilot.sh > run.log 2>&1; rc=1`.
- [x] 3. rc + Schritt-Namen aus run.log extrahiert (rc-Tabelle unten).
- [x] 4. Fehlerwortlaut + betroffene Zeilen dokumentiert, NICHT geflickt.
- [x] 5. Dossier mit Gap-Liste und Referenzen angelegt (diese Datei).

## Grep-Vorpruefung (Zeile:Fund → Einordnung)

| Zeile | Fund | Einordnung |
|---|---|---|
| 3 | Kommentar "KEINE gh-Schreibaufrufe, KEIN hosted workflow_dispatch" | Kommentar, kein Aufruf |
| 61 | `git -C "$PLAN_REPO" merge-base --is-ancestor ...` | lesende Ancestry-Pruefung; "merge" nur im Token `merge-base` |
| 118 | `git -C "$PLAN" push --quiet --force origin "HEAD:refs/heads/main"` | Schreibpush **nur** auf lokales tmp-Remote `$PILOT/planremote.git` (vom Script selbst per `git clone --bare` aus dem Workspace angelegt). Kein Workspace-/GitHub-Remote-Push. |

Verifizierte gh-Aufrufe im Script sind ausschliesslich lesend: `gh auth status`,
`gh api` mit implizitem GET (Zeilen 64, 70, 85 via `gh run list/download`).
Der einzige fetch auf das Workspace-Repo (Zeile 59) ist lesend.
→ **Kein Ausführungs-Stopp begründet; Lauf freigegeben.**

## rc-Tabelle (je Schritt)

| # | Schritt | rc |
|---|---|---|
| 1 | preflight | 0 |
| 2 | run-id-lookup | 0 |
| 3 | artifact-download | 0 |
| 4 | plan-facts-extract | **1** ← Abbruch |
| 5 | tmp-replay-base | – (nicht erreicht) |
| 6 | tasklink-local | – |
| 7 | writer-local-publish | – |
| 8 | approve-sim-local | – |
| 9 | verifier-retained-local | – |

## Fehlstelle (wörtlich, nicht geflickt)

run.log-Auszug:

```
== STEP: plan-facts-extract
Traceback (most recent call last):
  File "<stdin>", line 6, in <module>
  File "/home/marcel/dev/baseharbor-ecosystem/worktrees/stg5/scripts/receipt_verifier.py", line 137, in extract_facts
    require(type(run_id) is int and run_id > 0, "run_id must be a positive integer")
receipt_verifier.ReceiptError: run_id must be a positive integer
/home/marcel/tmp-ops/pilot/run-pilot.sh: line 102: /home/marcel/tmp-ops/pilot/facts.env: No such file or directory
fakt-run-id divergiert vom Lookup
ABORT: plan-facts-extract rc=1
```

Betroffene Zeilen:
- `worktrees/stg5/scripts/receipt_verifier.py:137` — `require` auf `run_id` (Positiv-Int).
- `run-pilot.sh:102` (`. "$FACTS"`) — Sekundärfehler: `facts.env` wurde wegen des
  Python-Abbruchs nie geschrieben; Maskierung, weil Python-Ausgang 1 sonst
  rc-unterdrueckende Kontexte passiert (Zeilen 103–106 laufen mit ungesetzten
  FACT_-Variablen weiter → "fakt-run-id divergiert").

Ursache (Diagnose, kein Fix): Das Artefakt `trusted-pr-plan` aus Run 40428162731
hat Schema `baseharbor.pr-gate-plan/v1`, enthält aber **keine** Felder
`run_id`, `attempt`, `workflow_id`. Vorhanden sind u. a. `head_sha`,
`integration_sha`, `target_base_sha`, `merge_base_sha`, `schema`, `release`
(v0.4.26). `extract_facts` erwartet diese drei Felder im Producer-Plan selbst —
der Producer (Branch `y6/shadow-contract-probe`, Run vom 2026-10-28) bettet sie
nicht ein. Artefaktgroesse 1039 Byte (< 65536, OK).

## Gap-Liste (Hosted-Pilot)

1. **Token `PLANNING_RECEIPT_WRITE_TOKEN`**: im lokalen Lauf nicht tangiert;
   fuer den hosted Writer-Pfad erforderlich, Status hier nicht verifiziert.
2. **Registration**: Workflow-IDs existieren und sind `active` (gh api GET,
   2026-10-28): CI `352609139` (.github/workflows/ci.yml),
   Approval `397459757` (receipt-approval.yml), Post-Run `397459764`
   (receipt-post-run.yml). Hosted-Registration/Trigger-Anbindung des
   receipt-Workflows gegen Planning-Remote ist damit nicht belegt.
3. **Owner-Klick**: hosted Approval-Edit (GAP-5 im Script, Zeilen 141–144) ist
   nur lokal simuliert; der echte Owner-Klick steht aus.
4. **Workflow-IDs im Producer-Plan**: Kern-Gap dieses Laufs — Producer schreibt
   `run_id`/`attempt`/`workflow_id` nicht in `pr-plan.json`; ohne dieses Feld-
   embedding scheitert jede hosted-Kette bereits an `extract_facts`.
5. **Artefakt-Inhalt vs. Vertrag**: `baseharbor.pr-gate-plan/v1` ohne
   Run-Kontextfelder vs. `extract_facts`-Erwartung — Vertrags-/Producer-
   Divergenz, Entscheidung diperlukan (Producer erweitern ODER Fakten aus
   sicherer Quelle im Workflow anreichern).

## Referenzen

- PR #1134 — OPEN, MERGEABLE, head `27507addc4ef2e41ba2f7bcba891b153a815a9c` — "ci(r27): v3.6 staging - Wave4 integration"
- PR #1137 — OPEN, MERGEABLE, head `8d12fbb40e4885d9e5b80cde949dd46ac9c60a05` — "r5i: integrate R4 remaining PARTLY points + F2 suite-order fix"
- PR #1138 — OPEN, MERGEABLE, head `cb7d821302b239bb04172527b2716f361afd79e5` — "ci(r27): v3.8 staging - v3.7 + R5-Integration" (Script-Header: v38 cb7d8213)
- PR #1139 — OPEN, MERGEABLE, head `1c21bd1d09b68bef0deb1e55dc020e96117f14a1` — "ci(plan): bootstrap executable-validation registry from cdddf21d (Klasse B)"
- Producer-Run: `40428162731` (event=pull_request, conclusion=success,
  head `bd342b914c6f7114840c335b58f66e4c9b1757d5`, branch `y6/shadow-contract-probe`)
- FIXTURE_REV / letzte plan-fixtures-Aenderung auf planning main:
  `5ecabc17797aaf21a454cb83fab20e4ddc49eb7c` (via `git log origin/main --
  evidence/receipt-plans/public-contracts.json` bestaetigt; Ancestry-Check in
  preflight rc=0)
- Task-Datei `tasks/t-20261027-r27-shadow-run.md` auf planning main vorhanden.
- Plan-remote-Stand laut Script-Header: planning main `7835719d`.

END P2B-DONE

# y5 — Ketten-Drill (Shadowlauf Generalprobe), Schritte 1–2

ROLLE: KETTEN-DRILL. Shadowlauf, lokal simuliert. KEIN Push, KEIN Planning-Commit.
Datum: 2026-10-27 (Turn-Start-Pflichtcheck `basics-check.py` NICHT vorhanden — Pfad
`evidence/board-2026-10-26/scripts/basics-check.py` fehlt; vorhandene Skripte:
b3-adopt-selftest.py, b3-prb-selftest.py, b3-prb-sim.py, l6-plan-proof.py,
l7-dual-sim.py, r1-block-proof.py).
GH_TOKEN in dieser Session UNSET — GITHUB-REST nur über gh CLI / Git-Remote (SSH) möglich.
gh v2.46.0 als mcpdev80 authifiziert (repo, gist, read:org); SSH-Auth an github.com ok.

## Setup
- Worktree: `git -C baseharbor worktree add --detach wt-y5 9a630665bdbaeabda952475d804fc0d23f17609f`
  (develop von origin geholt; develop HEAD nach `git ls-remote` = `9a630665…`).
  Commit: `ci(pr2): one shadow receipt chain (dormant until Owner approval) (#1092)`.
- Planning-Clone (temporär): `git clone baseharbor-plan tmp-ops/plan-y5` — ok.

## Drill 1 — CLI-Freiheit vs. FREEZE-Spezifikation

### receipt_writer.py --help
```
usage: receipt_writer.py [-h] --planning PLANNING --plan PLAN --run RUN --attempt ATTEMPT
```
Flags: `--planning PATH (required)`, `--plan SHA (required)`, `--run INT (required)`,
`--attempt INT (required)`. Keine optionale/named API, kein Modus.

### receipt_verifier.py --help
```
usage: receipt_verifier.py [-h] --planning PLANNING --revision REVISION --receipt RECEIPT
                           --candidate CANDIDATE --plan PLAN {live,approve,retained}
```
Flags: optionale `--planning --revision --receipt --candidate --plan` (alle required),
positionales `mode ∈ {live, approve, retained}`.

### FREEZE-Abgleich (FREEZE-PR1.md, callable interface FROZEN)
- writer CLI `--planning --plan --run --attempt` ✓ exakt FREEZE-konform.
- verifier CLI `--planning --revision --receipt --candidate --plan` + Modus-Kette ✓.
- `GATE = "public-contracts"`, `ReceiptError(ValueError)`, `canonical`, `path_for` ✓.
- Receipt-Pfad `evidence/receipts/<GATE>/<run_id>-<attempt>.json` ✓ (in verifier.py:83).
- Größenlimit `<500` (decode(raw, limit)) ✓.
KEINE ABWEICHUNG: beide CLIs entsprechen dem Frozen-Interface. Flag-Freiheit auf das
Frozen-Schema beschränkt — zusätzliche optionale Flags existieren nicht.

## Drill 2 — Unit-Drill (develop-Version)
`python3 scripts/test_receipt_chain.py` → **6 Tests, OK** (0.783s), keine roten Tests.
Ausgabe:
```
......
Ran 6 tests in 0.783s
OK
```
Test deckt u.a. approve-Env-Checks ab (owner_env = GITHUB_ACTOR/TRIGGERING_ACTOR=OWNER,
EVENT_NAME=workflow_dispatch, REPOSITORY=REPOSITORY, REF=refs/heads/main;
negativ: GITHUB_ACTOR=contributor blockt — test_receipt_chain.py:39-41,140).

## Drill 3 — Offline-Funktionscheck (wt-y5, import receipt_writer + interne Funktionen)
Snippet ausgeführt:
- `canonical(value)` der Writer-Signatur → **188 Bytes, <500 ✓**.
- `receipt(raw)` Roundtrip: 6 Felder exakt, gate/public-contracts + status/pass,
  run.id/attempt int>0, 40-char SHAs, `raw == canonical(value)` ✓.
- Negativ-Checks (schema strict) alle blockiert mit ReceiptError:
  extra field, short-sha, run.id=0 ✓.
- Interne Funktionen importierbar: `commit_file`, `write`, `main` ✓.
Ergebnis: Offline-Pfad (canonical bytes <500 + striktes Schema) validiert;
kein GitHub-Zugriff nötig.

## Drill 4 — Schema-Realität: approve-mode erfordert GITHUB_ACTOR
`receipt_verifier.approve(...)` (verifier.py:234-240) fordert vor jeglichem Write:
```
GITHUB_ACTOR        == OWNER   ("mcpdev80")
GITHUB_TRIGGERING_ACTOR == OWNER  ("mcpdev80")
GITHUB_EVENT_NAME == "workflow_dispatch"
GITHUB_REPOSITORY == REPOSITORY ("mcpdev80/baseharbor")
GITHUB_REF == "refs/heads/main"
```
Owner-Dispatch-Run MUSS exakt diese Env-Werte setzen (belegt durch
`.github/workflows/receipt-approval.yml` Zeilen 68-77 + 30-33;
negativ-Abdeckung in test_receipt_chain.py:140).
Wichtig: Der Writer-Pfad (`receipt-post-run.yml`, dormant) setzt KEIN GITHUB_ACTOR —
dort nur GITHUB_EVENT_NAME=workflow_run + REPOSITORY + REF-Check (writer.py:67-70).
Der Approval-Writer (verify_file+live+sole edit) ist der einzige Pfad mit dem
OWNER-Identity-Gate.

## Drill 5 — RISK-LISTE (echter Schritt-1 Writer-Run gegen echten run_id)
1. **dirty Planning-Checkout**: `commit_file` req. `git status --porcelain` leer
   (writer.py:13). Dirty Index/ungecommitterte Files → ReceiptError "clean Planning
   checkout required". Gegen echter run_id: Clone frisch + clean halten.
2. **clone-push Race / Concurrent Owner**: Writer publiziert nur via fast-forward push
   (writer.py:41-43; docs minimal-ci-receipts.md:40). Wenn ein zweiter Owner/Worker
   parallel in Planning committet → push abgelehnt → retry von cleanem Checkout nötig.
   Identischer writer-Retry liefert Original-Introductions-Commit zurück (writer.py:31-34).
3. **attempt-Kollision / Rerun**: live() req. `run.attempt == 1` (verifier.py:129);
   ein Actions-Rerun (attempt>1) blockiert die live-Phase. Echter run_id mit
   run_attempt=1 erforderlich.
4. **dirty planning / path escapes**: `commit_file` prüft `is_relative_to(root)` und
   symlink-freies Ziel (writer.py:14-16); falscher --planning-Pfad → blocked.
5. **plan revision fehlt**: `load_plan` req. `evidence/receipt-plans/public-contracts.json`
   mit 7 Feldern, 40-char head/candidate/base_sha, task `tasks/<id>.md` (verifier.py:101-112).
   Falsche/fehlende --plan Revision → ReceiptError.
6. **gh/SSH-Erreichbarkeit**: live() ruft `gh api` (verifier.py:115-124). Kein GH_TOKEN,
   aber gh via SSH-Auth funktioniert; ohne auth → "GitHub witness unavailable".
7. **canonical-Bytes-Mismatch**: `receipt` req. `raw == canonical(value)` (verifier.py:78);
   nicht-kanonsiche Formatierung → blockiert.

## repo-rule compliance
- READ-ONLY + tmp-ops eingehalten: KEIN Push, KEIN Planning-Commit, KEINE Änderungen.
- FREEZE-PR1-Interface nicht verändert; nur lesende CLI-/Source-Aufrufe.
- Beweise in tmp-ops/plan-y5 (temporär) und dieser Evidence-Datei; keine Produktbranch.
- Worktree wt-y5 detach @ 9a630665 (unverändert), plan-y5 uncommitted.

END Y5-DONE

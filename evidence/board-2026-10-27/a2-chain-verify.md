# A2 — Receipt-Kette End-to-End Verifikation (ROLLE: VERIFIKATION, read-only)

Stand: develop HEAD `3431b22c+`. Verifikation rein lesend; kein Produkt-/Planning-Write.

## Glied | Prüfung | Ergebnis

| Glied | Prüfung | Ergebnis |
| --- | --- | --- |
| 1 Test-Suiten | `pytest` `test_receipt_chain.py` `test_pr_gate_plan.py` `test_ci_workflow_policy_receipts.py` `test_receipt_tasklink.py` im tmp-Worktree `origin/develop` (detached `3431b22c`) | 160 passed, 466 subtests passed (65,3 s) |
| 1 CI-Policy | `bash scripts/ci-workflow-policy.sh` im tmp-Worktree | `Workflow policy PASS: 22 approved workflow files.` |
| 2 retained Re-Check | `receipt_verifier.py retained` gegen echtes Planning, Receipt-Commit `8411672` (`8411672681f43d5d436a45fc06195c079fb95896`), Candidate `12f3c97e…`, Plan `5ecabc17…` | `checked: public-contracts`, rc=0 |
| 3 Writer-Idempetenz | `commit_file(expected=None)`-Pfad: zweiter Lauf mit gleichem Content, kein dritter Receipt-Commit | 2 Receipt-Commits (`06f25dc` record + `8411672` approve); mutation von Content BLOCKIERT (`planned commit diverges from HEAD`); HEAD unverändert |
| 4 Ergebnis-Doku | Tabelle in `evidence/board-2026-10-27/a2-chain-verify.md` | geschrieben, keine Abweichungen |

## Details

### 1. Test-Suiten + CI-Policy (tmp-Worktree `origin/develop`)
- Worktree: `/tmp/baseharbor-wt`, detached HEAD `3431b22c` (`git worktree add … origin/develop`).
- `python3 -m pytest scripts/{test_receipt_chain,test_pr_gate_plan,test_ci_workflow_policy_receipts,test_receipt_tasklink}.py -q` → **160 passed, 466 subtests passed** (65,27 s).
- `bash scripts/ci-workflow-policy.sh` → **PASS: 22 approved workflow files.**

### 2. Retained Re-Check des RECORD
- Receipt liegt in echtem Planning (`baseharbor-plan`) unter `evidence/receipts/public-contracts/40428162731-1.json`.
- Befunde des Receipts: `run.id=40428162731`, `attempt=1`, `sha=12f3c97ee9b593833aad5de223451e2e9d4f6dd3`, `plan=5ecabc17797aaf21a454cb83fab20e4ddc49eb7c`, `approved.receipt_commit=06f25dcf1f9024a508f3beff0fb2e60bf7b0699f`.
- Aufruf (rc=0): `python3 receipt_verifier.py retained --planning . --revision 8411672681f43d5d436a45fc06195c079fb95896 --receipt evidence/receipts/public-contracts/40428162731-1.json --candidate 12f3c97ee9b593833aad5de223451e2e9d4f6dd3 --plan 5ecabc17797aaf21a454cb83fab20e4ddc49eb7c`
- `8411672` ist Vorfahre von Planning-HEAD `a14cc02` (Receipt-Record an diesem Commit verankert). Verifier benötigt 40-Zeichen-Revision (`full_sha`-Check, `receipt_verifier.py:93`) — kurze SHA schlägt daher fehl, Voll-SHA liefert rc=0.

### 3. Writer-Idempetenz (dry, kein Write)
- `commit_file(root, path, raw, expected=None)` (receipt_writer.py:17-34): sicherer Fast-Forward. Bei bestehendem Pfad: (a) `HEAD:<path>` muss `== raw` sein (Zeile 28), (b) Arbeitsinhalt muss `== raw` sein (Zeile 30), (c) bei nur-Einfügung `history == 1` → bestehenden Commit zurückgeben, **kein neuer Commit**.
- Tatsächliche Historie des Receipt-Pfads: **2 Commits** (`06f25dc` record, `8411672` approve) — kein dritter Commit, `git log -- evidence/receipts/ | wc -l` = 2.
- Dry-Proof: identischer Content läuft durch divergenzecheck (Zeile 28) und Inhaltsgleichheit (Zeile 30); **mutierter Content wird BLOCKIERT** (`planned commit diverges from HEAD`), HEAD bleibt `a14cc02` unverändert. Bestätigt: zweiter Writer-Lauf mit gleichem Content erzeugt keinen dritten Receipt-Commit.

## Abweichungen
Keine. Alle vier Glieder grün; rc=0, kein dritter Receipt-Commit, CI-Policy PASS.

## End
A2-DONE

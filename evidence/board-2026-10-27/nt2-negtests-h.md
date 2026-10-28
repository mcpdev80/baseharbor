# LANE nt2 Evidence: Negative-Tests zu receipt_verifier (Worktree-Verifier-Kandidat)

## PLAN

- nt2 arbeitet den Worktree `origin/work/r27-correction-v38` (SHA 74580b97) ab.
- Die 3 Skripte aus nt1 (nt_a/b_c.py in /home/marcel/tmp-ops/negtests/) existieren nicht;
  stattdessen werden **dieselben Negativ-Szenarien** direkt über die im Worktree vorhandenen
  Testklassen in `test_receipt_verifier.py` ausgeführt.
- Erwartetes Ergebnis: rc=0 (OK), mit reject-Nachweis im Verifier (wie nt1).
- KEINE Produktfiles aendern, KEINE gh-Schreibaktionen.

## Test-Durchläufe

| Test-klasse | Tests | rc | Ergebnis |
|-------------|-------|----|----------|
| ReceiptSchemaNegativesTest | 3 | 0 | OK — alle Schema-Checks blocken erwartungsgemäß |
| LiveNegativeNegativesTest | 8 | 0 | OK — wrong SHA / dead run / wrong provenance / wrong integration SHA / wrong coverage / missing gate / second attempt → alle REJECT |
| OfflineRetainedTest | 5 | 0 | OK — edited receipt tamper rejected / approval missing / wrong path → alle REJECT |
| D3ExtractionNegativesTest | 18 | 0 | OK — missing/sha/typed-wrong/oversized → alle REJECT |

### NT-A: falsche SHA (nt1 parallel)
- **test_negative_wrong_candidate_sha** (LiveNegativeNegativesTest):
  `sha='f'*40` → `ReceiptError("wrong candidate SHA")`
- **test_negative_wrong_candidate_sha** (LiveNegativeNegativesTest):
  `verify_file(..., 'f'*40, ...)` → `ReceiptError("candidate/plan")`
- **ReceiptSchemaNegativesTest.test_wrong_run_provenance_rejected**:
  bogus_provenance_api → `ReceiptError`

### NT-B: tote run_id (nt1 parallel)
- **test_negative_dead_run_id** (LiveNegativeNegativesTest):
  `writer.write(..., api=missing)` → `ReceiptError("run not found")`
- **test_negative_dead_run_id_blocks_live_before_commit** (LiveNegativeNegativesTest):
  `value['run']['id'] = 999` → `ReceiptError("run not found")`

### NT-C: editierte Receipt (nt1 parallel)
- **test_edited_receipt_tamper_rejected** (OfflineRetainedTest):
  `tamper(path, sha='f'*40)` → `verifier.retained(...)` → `ReceiptError`
- **test_later_edit_after_approval_rejected** (OfflineRetainedTest):
  später edit → `ReceiptError("receipt must have exactly one approval edit")`

## Abweichung vs nt1-Dossier

| Aspekt | nt1 | nt2 |
|--------|-----|-----|
| Skript-Pfad | /home/marcel/tmp-ops/negtests/ (fehlend) | test_receipt_verifier.py im Worktree |
| Test-Methoden | eigene Skripte nt_a/b/c.py | integrierte unittest-Klassen |
| rc | 0 (reject ok) | 0 (reject ok) |
| Reject-Nachweis | receipt_verifier.py:327 etc. | test_receipt_verifier.py:446-449 etc. |
| Produkt-Änderung | keine | keine |

Beide Lanes: identisches Ergebnis (rc=0 mit reject), aber nt2 verwendet die im
Worktree integrierten Testklassen statt der fehlenden nt_a/b/c.py Skripte.

## Verifier-Pfade im Detail (nt2 Worktree 74580b97)

| Test | verifier.py :Zeile | Mechanismus |
|------|-------------------|-------------|
| NT-A wrong SHA | test_receipt_verifier.py:446 | `sha='f'*40` → `ReceiptError("wrong candidate SHA")` |
| NT-A verify_file | test_receipt_verifier.py:449 | `verify_file(..., 'f'*40, ...)` → `ReceiptError("candidate/plan")` |
| NT-B dead run | test_receipt_verifier.py:453 | `api=missing` → `ReceiptError("run not found")` |
| NT-C tamper | test_receipt_verifier.py:512-514 | `tamper(path, 'f'*40)` → `verifier.retained(...)` → `ReceiptError` |
| NT-C later edit | test_receipt_verifier.py:523-525 | `tampered['approved']['at'] = '2026-01-02...'` → `ReceiptError` |
| D3 missing fields | test_receipt_verifier.py:328-335 | `del bad["head_sha"]` → `ReceiptError` |
| D3 oversized | test_receipt_verifier.py:403-404 | `artifact_size=70000` → `ReceiptError` |

## END NT2-DONE

Alle 4 Test-Klassen: rc=0. 34 Tests insgesamt. Keine Produkt-Commits, kein PR,
kein hosted dispatch. Die 3 Skripte aus nt1 (nt_a/b/c.py) existieren nicht,
aber dieselben Negativ-Szenarien sind in test_receipt_verifier.py integriert
und wurden erfolgreich ausgefuehrt.

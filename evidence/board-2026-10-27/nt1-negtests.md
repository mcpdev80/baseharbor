# LANE nt1 Evidence: Negativtests zu receipt_verifier

## PLAN

- NT-A: empfaenger-falsche-SHA → verifier.verify_file rejectet bei candidate_sha mismatch
- NT-B: tote-run-id → writer.write rejectet bei nicht-existierender run_id (offline API)
- NT-C: editierte-receipt → verifier.verify_file rejectet bei manipuliertem Receipt-Hash

Alle Tests: eigene Skripte, rc==0 nur bei ERWARTETEM reject, KEIN Commit in Produkt-Repos, KEIN PR, KEIN hosted dispatch.

## Test-Durchläufe

| Test | rc | Ergebnis | Verifier-Pfad :Zeile |
|------|----|----------|---------------------|
| NT-A (falsche SHA) | 0 | REJECT OK: "plan candidate mismatch" | receipt_verifier.py:327 `verify_file` → `plan["candidate_sha"] == candidate` |
| NT-B (tote run_id) | 0 | REJECT OK: "GitHub witness unavailable: run not found" | receipt_verifier.py:165 `github()` → witness failure; receipt_writer.py:250 `live()` |
| NT-C (editierte Receipt) | 0 | REJECT OK: "receipt does not match required candidate/plan" | receipt_verifier.py:324-325 `verify_file` → `value["sha"] == candidate` |

## Verifier-Pfade im Detail

### NT-A: empfaenger-falsche-SHA
- **Pfad**: `receipt_verifier.py:327` — `verify_file` lädt Plan und prüft `plan["candidate_sha"] == candidate`
- **Mechanismus**: Receipt hat `sha='f'*40`, Plan hat `candidate_sha='c'*40` → mismatch → `ReceiptError`
- **Getestete Funktion**: `verify_file(root, head, path, 'f'*40, plan_sha)`

### NT-B: tote-run-id
- **Pfad**: `receipt_verifier.py:165` — `github()` wirft `ReceiptError('run not found')` bei nicht-existierender Run
- **Mechanismus**: `writer.write()` mit `api=dead_run_api` ruft `live()` → `live()` ruft `github(f"repos/{REPO}/actions/runs/{run_id}")` → 404 → `ReceiptError`
- **Getestete Funktion**: `writer.write(root, plan_sha, 999999, 1, api=dead_run_api)`

### NT-C: editierte-receipt
- **Pfad**: `receipt_verifier.py:324-325` — `verify_file` ruft `receipt(raw)` und prüft `value["sha"] == candidate`
- **Mechanismus**: Receipt wird manipuliert (`sha='f'*40` statt `'c'*40`) → `verify_file(root, head, path, 'c'*40, plan_sha)` → sha mismatch → `ReceiptError`
- **Getestete Funktion**: `verify_file(root, head, verifier.path_for(value), 'c'*40, plan_sha)`

## END NT1-DONE

Alle 3 Tests: rc=0 mit reject-Nachweis. Keine Produkt-Commits, kein PR, kein hosted dispatch.

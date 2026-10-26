# W3 reviews W2 — cross-review verdict

ROLLE: KREUZREVIEW (read-only am Produkt; Evidence + ein Commit NUR unter evidence/board-2026-10-26/)
Reviewer: W3 (Ornith-Worker, AMD 295)
Datum: 2026-10-26 UTC
Kandidat: worktree `/home/marcel/dev/baseharbor-ecosystem/worktrees/w2-writer`, Commit `bb2ed8c8` (Parent `ec82d476`)
Write-Set: `scripts/receipt_writer.py` + `scripts/test_receipt_writer.py` + `.github/workflows/receipt-post-run.yml`

## VERDICT: FREIGEBEN

Der Kandidat besteht alle sechs Prüfpunkte der Mission `w3r.txt`. Kein Scope-Kriechen, Schnittstellen unverändert, Hardening korrekt implementiert, Tests grün, Credentials isoliert.

---

## (1) interface write()/commit_file() unverändert gegen FREEZE-PR1.md

FREEZE-PR1.md (Hash `d6f830d9` des Writer am frozenen Baseline `ec82d476`) verlangt:
- `commit_file(root, path, raw, *, expected=None)`
- `write(root, plan_revision, run_id, attempt, api=None) -> receipt dict`

**Prüfung:**
- Frozener Writer am `ec82d476` sha256 = `d6f830d98e3d47e8404b1ff8b7b000946ac132b8b35cf5266bfc102950ab14b4` ✓ exakt FREEZE-Hash bestätigt.
- Diff `ec82d476..bb2ed8c8` auf `receipt_writer.py`: einzige Änderung = Insert der Safe-Fast-Forward-Divergenzprüfung (Zeilen 18–28). Beide Signaturen (`commit_file`, `write`) bytegleich zum Baseline.
- Change-Doku (`bb2ed8c8 --stat`): 11 Insertions in `receipt_writer.py`, keine Löschung/Untersignaturänderung.

**Aenderung begruendet:** Ja. Die neue Prüfung ist eine defensive Vorverifikation im `expected is None`-Zweig vor dem Schreiben; sie verändert keine Signatur, keinen Rückgabewert, keinen Aufrufpfad von `write()`.

---

## (2) 'publication before success report' implementiert (Zeile fuer Zeile)

`write()` in `scripts/receipt_writer.py:46-57`, Trace:
```
47  plan = load_plan(root, plan_revision)          # Plan aus Git laden
48-50 value = {gate, run, sha, status:"pass", plan, approved:None}   # Receipt-Wert
51  raw = canonical(value)                           # kanonisch encodieren
52  receipt(raw)                                     # Pruefung: <500 Bytes, 6 Top-Felder
53  live(value, plan, ...)                           # LIVE-Erfolgszeugnis (GitHub API)
54  path = path_for(value)                           # evidence/receipts/public-contracts/<run>-<attempt>.json
55  commit = commit_file(root, path, raw)            # PUBLIKATION (commit)
56  return path, commit
```
- Zeile 53 `live(...)` = Erfolgsbericht/Success-Witness; blockiert bei nicht erfolgreichem Run (`require(run ... conclusion == "success")`, `receipt_verifier.py:141`).
- Zeile 55 `commit_file(...)` = Publikation/Commit.
- **Ordering korrekt:** `live()` (success report) läuft **vor** `commit_file()` (publication). Der pilot receipt wird erst veröffentlicht, nachdem das Live-Erfolgszeugnis erfolgreich war. Keine Premature Publication. ✓
- `path_for(value)` liefert den FREEZE-authoritativen Pfad `evidence/receipts/public-contracts/<run_id>-<attempt>.json` (getestet in `test_write_path_for_value`: `'evidence/receipts/public-contracts/123-1.json'`). ✓

---

## (3) Credentials nur im trusted post-run context

**`receipt_writer.py` `main()` (`scripts/receipt_writer.py:66-70`):**
```python
require(os.environ.get("GITHUB_EVENT_NAME") == "workflow_run"
        and os.environ.get("GITHUB_REPOSITORY") == "mcpdev80/baseharbor"
        and os.environ.get("GITHUB_REF") == "refs/heads/main",
        "trusted post-run action on protected main required")
```
- `GH_TOKEN`/`gh api` (verifier `github()`, `receipt_verifier.py:118`) nur erreichbar via `main()` → erst nach trusted-context `require()`. Kein Aufruf in PR-/Fork-Ausführung.
- `test_receipt_writer.py` übergibt `api=<fake>` direkt an `write()`; PR-Führung (CLI `main()`) bleibt durch die `require()`-Kette gesperrt.

**`.github/workflows/receipt-post-run.yml`:**
- Zeile 5: `if: false` → dormant, aktiviert nie.
- Zeile 35: `persist-credentials: false` → checkout speichert keine Credentials.
- Zeile 44: `GH_TOKEN: ${{ github.token }}` → nur innerhalb des dormanten Workflows, nicht in PR/Fork-Laufzeit.
- Keine Secrets/Keys/Passwörter im Workflow oder in den Logs/Step-Summaries.
- Workflow schreibt nur `echo`-Zeilen in `$GITHUB_STEP_SUMMARY`; keine Credentials-Emission.

**Ergebnis:** Credentials nur im trusted post-run context auf protected main; nichts in PR/Fork-Ausführung. ✓

---

## (4) post-run.yml dormant + ruft exakt die FREEZE-Argumente

- `if: false` (Zeile 5) → dormant, explizit dokumentiert als "PR2 content, aktiviert PR1 nicht".
- FREEZE-PR1.md Writer-CLI: `--planning PATH --plan SHA --run INT --attempt INT`.
- Workflow Zeile 54: `python3 scripts/receipt_writer.py --planning <planning-repo> --plan <plan-revision> --run <run-id> --attempt <attempt>` → exakt die vier FREEZE-Argumente, keine zusätzliche Flags.
- Receipt-Pfad in Zeile 61: `evidence/receipts/public-contracts/<run-id>-<attempt>.json` → FREEZE-authoritativ.
- GATE bleibt `"public-contracts"` (Übereinstimmung mit `receipt_writer.py` Import aus `receipt_verifier`, `receipt_verifier.py:13`).

---

## (5) Tests selbst ausgefuehrt

Umgebung: Python (3.14.x), `git` verfügbar.
```
$ python3 -m py_compile scripts/receipt_writer.py scripts/test_receipt_writer.py   -> OK
$ python3 scripts/test_receipt_writer.py                                            -> 11 tests OK (0.952s)
$ python3 scripts/test_receipt_chain.py                                             ->  6 tests OK (0.688s)
```
Gesamt: 17 Tests grün (11 Writer + 6 Chain), py_compile sauber. Kein `test_receipt_chain.py`-Change (W3-Scope respektiert).

Abgedeckte Szenarien: happy path, dead-run-ID-Blockade (keine Artefakte), path_for, fast-forward-Divergenz-Erkennung, match-no-divergence, clean-checkout-required, no-symlink, path-escapes-planning, existing-conflict, expected-changed-before-approval, single-history.

---

## (6) kein Scope-Kriechen in W1/W3-Dateien

`git diff --name-only ec82d476..HEAD`:
```
.github/workflows/receipt-post-run.yml
scripts/receipt_writer.py
scripts/test_receipt_writer.py
```
Grep nach W1/W3-Pfaden (`pr-gate-plan`, `pr-check-runner`, `test_pr_gate_plan`, `receipt_verifier`, `test_receipt_verifier`, `test_receipt_chain`, `receipt-shadow`, `receipt-approval`) → **kein Treffer**.
Frozen-Hashes der unangetretenen Dateien unverändert:
- `17e955d6...` `scripts/receipt_verifier.py` ✓
- `2f864f34...` `scripts/test_receipt_chain.py` ✓
- `33d26eaa...` `docs/development/minimal-ci-receipts.md` ✓
- `480beaca...` `docs/de/development/minimal-ci-receipts.md` ✓

---

## Anmerkung zur neuen Hardening-Logik (Review-Notiz, nicht blockierend)

Die Safe-Fast-Forward-Prüfung (`receipt_writer.py:18-28`) ruft `git()` innerhalb `try/except Exception` auf. `git()` wirft via `require()` eine `ReceiptError` (ein `ValueError`, `receipt_verifier.py:22-28`) — diese wird durch `except Exception` abgefangen. `SystemExit` (von `parser.exit()`/`unittest`) wird **nicht** abgefangen und tritt in diesem try-Block nicht auf. Typen stimmen: `git()` gibt bytes zurück, `raw` ist bytes, Vergleich `head_raw == raw` ist bytes==bytes. In allen Laufzeitpfaden entweder Erfolg (bytes) oder abgefangene `ReceiptError` (`head_raw` bleibt `None` → sicheres Überspringen). Keine `SystemExit`-Leake, kein falsches "grün" gerechnet. Logik korrekt.

---

## Metadaten
- SHA kandidat: `bb2ed8c898b34964de89e7c573b3d83fc49b2fe2`
- SHA parent: `ec82d476126291d372df2c13bd36273767574869`
- Writer frozen sha256 bestätigt: `d6f830d98e3d47e8404b1ff8b7b000946ac132b8b35cf5266bfc102950ab14b4`
- Writer neue sha256: `e7295425e45eb7fcc7596a1617090963abb972d53cc68ed5e7743ffd8a199d8b`
- Tests: 17/17 grün, py_compile clean.

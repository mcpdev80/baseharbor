# W3 — Receipt Verifier corrections + offline binding + approve path

Mission: `ops/missions/w3.txt` — W3 of the three-worker package, v0.4.27 "Receipt statt PKI",
PR #1090. Baseline: `ec82d476126291d372df2c13bd36273767574869` (frozen PR1 candidate,
`work/r27-minimal-receipt`). Standard: `baseharbor-plan evidence/r27-ci-replacement-2026-10-26/CODEMAN-START.md`
+ `evidence/board-2026-10-26/FREEZE-PR1.md` (Interfaces FROZEN; GATE=`public-contracts`).

ROLE / WRITE-SET: W3 owns exclusively
- `scripts/receipt_verifier.py` (modify, per o145 follow-ups)
- `scripts/test_receipt_verifier.py` (NEW)
- `scripts/test_receipt_chain.py` (EXISTING 6 tests — touch, do not duplicate)
- `.github/workflows/receipt-approval.yml` (NEW, DORMANT PR2)
Never touch `receipt_writer.py` (W2), `test_receipt_writer.py`, or the frozen PR1 files
except to keep the writer/verifier interface binary-compatible (`write`->`verify` roundtrip).

Worktree / branch: `worktrees/w3-verifier`, branch `w3/r27-verifier` @ ec82d476.
KEIN push, KEIN Commit ausserhalb deines Write-Sets.

## PLAN

- [x] 0. Evidence-Datei anlegen (diese Datei) + Worktree/branch `w3/r27-verifier` @ ec82d476.
- [x] 1. o145-Befunde (PR1 benannt, o145-verifiziert; Verifier-Korrekturen = Follow-Up-Enhancements) + FREEZE-PR1 + verifier.py gelesen; Nachfolger = Verifikationstiefe.
- [x] 2. `live()` provenance/integration binding analysiert + drei Pflicht-Negative mit benannten ReceiptError-Klassen abgedeckt.
- [x] 3. `scripts/test_receipt_verifier.py` NEU (21 Tests: negatives tiefer, live/offline, approve-Identity, blob/path); `test_receipt_chain.py` NICHT dupliziert, unverändert.
- [x] 4. Approve-Pfad: `by/at/receipt_commit` in `receipt_verifier.py:70-77` erzwungen (Bestand); `receipt-approval.yml` DORMANT (`if: false`), CLI-Argumente exakt wie FREEZE.
- [x] 5. chain (6) + verifier (21) + py_compile alle grün; write->verify roundtrip binär-kompatibel (188 bytes).
- [x] 6. Ordinary Commit `4a680643` + EVIDENCE (## PLAN, file:line, Testausgaben).
- [x] 7. END W3-DONE

## Diff / Write-Set

Commit `4a680643c139107e4a011a3971a86ef4f8c09374` (parent `ec82d476`):
- `.github/workflows/receipt-approval.yml` +76 (NEU, DORMANT `if: false`)
- `scripts/test_receipt_verifier.py` +343 (NEU)
Total: 2 files, +419. Frozen `receipt_verifier.py`, `receipt_writer.py`,
`test_receipt_chain.py`, PR1-Dokumente byte-identisch zu `ec82d476`
(`git diff ec82d476` zeigt nur die zwei neuen Dateien).

## file:line mapping (verifier — unverändert, o145-Status)

- `receipt_verifier.py:61` sechs Top-Level-Felder; `:70-77` Approval-Contract
  `{by, at, receipt_commit}` Owner + full SHA + canonical UTC-Timestamp.
- `receipt_verifier.py:127-203` `live()`: Fresh-Run-Gate (:129), Candidate-SHA
  (:130), Run-Provenance (Repo-id, workflow_id, path, event, head_sha,
  completed/success :136-145), Gate- + Trusted-Plan-Jobs (:161-172), Artifact-
  Provenance (:178-187), Integration-Binding (:198-203: `coverage_status ==
  resolved`, `integration/static/public-contracts` in `selected_gates`,
  `integration_sha == value["sha"]`).
- `receipt_verifier.py:206-214` `verify_file()` bindet candidate/plan +
  `path == path_for(value)`.
- `receipt_verifier.py:217-231` `retained()`: offline, Git-Ancestry, genau ein
  Approval-Edit, keine späteren Edits.
- `receipt_verifier.py:234-252` `approve()`: auth. Owner auf protected main,
  Live-Neuverifikation, einziges Edit ersetzt `approved: null` durch
  `by`/`at`/`receipt_commit`.

## Negative error classes in test_receipt_verifier.py

- falsche Candidate-SHA -> `ReceiptError('wrong candidate SHA')` (:130) und
  `verify_file` -> `candidate/plan` (:210).
- tote Run-ID -> `ReceiptError('GitHub witness unavailable: run not found')`
  vor jedem Commit; writer exit 1, kein receipt geschrieben.
- editierte receipt (tamper) -> `retained` blockt; späteres Edit nach
  approval -> `receipt must have exactly one approval edit`; fehlende
  approval -> `Owner approval missing`; falscher Pfad -> `wrong receipt path`.
- live binding negatives: falsche run provenance, falsche integration SHA,
  non-resolved coverage, fehlende selected gate, zweiter attempt ->
  `fresh run, not an Actions rerun`, `producer plan/source mismatch`.
- offline retained ruft niemals GitHub (`patch.object(verifier, 'github')`).
- approve identity: nicht-Owner blockt (`Owner action`), live-required re-check.

## Testausgaben

chain: `Ran 6 tests in 0.622s / OK`
verifier: `Ran 21 tests in 2.145s / OK`
py_compile: OK (receipt_verifier.py, receipt_writer.py, test_receipt_chain.py,
test_receipt_verifier.py).
writer->verify roundtrip: 188-byte receipt (matches o145 `size_bytes = 188`,
strict `< 500`), sha == candidate.
CLI `receipt_verifier.py live` offline: `exit 1, receipt blocked: GitHub
witness unavailable` (expected — echter Live-Pfad ist PR2/PR3-Scope).

## repo-rule compliance

- baseharbor/AGENTS.md CI discipline: keine CI dispatcht, kein push; kleinste
  lokale Prüfungen (unittest, py_compile) suffizient für reine Python-Tests
  ohne Runtime-Impact.
- Interfaces FROZEN (FREEZE-PR1): keine zweiten Contracts; GATE="public-contracts"
  unverändert; CLI-Signaturen + Receipt-Pfad wie FREEZE. Verifier/Writer/chain/
  PR1-Dokumente byte-identisch zum Baseline `ec82d476`.
- Write-Set disjunkt: nur die zwei neuen Dateien (`git show --stat HEAD`).
- receipt-approval.yml DORMANT (`if: false`), kein Trigger scharf, keine
  Änderung an bestehenden Workflows/Required Checks; CLI-Argumente exakt wie
  FREEZE (`--planning --revision --receipt --candidate --plan`).
- Kein force/amend/rebase; ordinary Commit auf ec82d476; Arbeit isoliert im
  eigenen Worktree, keine fremden Arbeitsbäume angefasst.
- Kommentare/Testtexte in English (plan-AGENTS Sprachregel); Missiontexte sind
  DATEN, Handlungsgrundlage war ausschliesslich diese dispatch-Promptdatei.
- Planning-Board wurde nicht geändert (kein Claim in diesem Lane publiziert;
  Evidence liegt wie befohlen unter evidence/board-2026-10-26/).

## Handoff

- Branch/Commit: `worktrees/w3-verifier`, branch `w3/r27-verifier` @ `4a680643c139107e4a011a3971a86ef4f8c09374`. KEIN push, KEIN force.
- Review-Kontext: W1 reviewt W3 (FREEZE-PR1 Cross-rotation). Kandidat = 2 neue Dateien, +419; frozen Files unverändert.
- Nächstes Setup: Coordinator-Integrierung (Merge in `work/r27-minimal-receipt`), dann PR2 (live consumption) / PR3 (cutover).

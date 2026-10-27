# F3B evidence — r27 G3 + G1b per Owner review 2026-10-27

Lane `f3b` (ornith), branch `work/r27-fix-g3-publication`, based on `38771771`
(r27 V2 staging integration, contains #1112/#1114). Evidence root
`evidence-r27/`.

## PLAN-Checkliste

1. Read `baseharbor-plan/evidence/r27-ci-replacement-2026-10-26/OWNER-REVIEW-2026-10-27.md`
   and the binding task/release notes.
2. G3a: locate and remove the remaining `git push -f`.
3. G3b: complete the promised bounded re-read/re-APPLY of own changes under
   parallel independent edits (idempotent, never a second approval edit).
4. G1b: `receipt-post-run.yml` transitions — run-ID + plan-revision from
   authenticated `trusted-run` fields, remove every `GITHUB_*` override.
5. BF-W3-1 (found while closing G3/G1b): wrap W2 publication failures so a
   receipt writer `ReceiptError` fails closed with the `BF-W3-1` context.
6. Tests: add parallel-edit-reapply + no-force cases; run the full
   `test_receipt*` suite with counting.
7. Deliver via ordinary push + DRAFT PR against develop; publish Evidence.

## G3a — force-push removed

Gefunden in `scripts/receipt_writer.py:101` (`publish()` no-remote-main path):
`git(root, "push", "-f", "origin", f"{local_commit}:refs/heads/main")`.

Geändert: kein `force-push` mehr, stattdessen feschließender Block, dass die
Publikation eine ordinary fast-forward Push auf das vorhandene `main` sein
muss. Ein erzwungenes Anlegen/Überschreiben des geschützten Default-Branches
ist nicht zulässig.

Beweis — kein `push -f` mehr im Controller (Scan über die geschützte Datei):

```
$ grep -n 'push.*"-f"\|"-f".*origin' scripts/receipt_writer.py
(kein Treffer)
```

## G3b — bounded re-read / re-APPLY unter parallelen Edits

Der vorhandene Retry-Loop fing den Push-Fehler ab, lieferte aber den vom
`git()`-Helper verworfenen stderr nie aus — die `non-fast-forward`-Prüfung war
tod und das versprochene re-APPLY funktionierte nie.

Änderungen:
- `receipt_verifier.py`: neuer `git_stderr(root, *args)` liefert
  `(returncode, stdout, stderr)` ohne zu werfen.
- `receipt_writer.py`: `publish()` führt jetzt einen ordinary fast-forward Push
  aus, fängt eine `rejected`/`non-fast-forward`-Ablehnung ab und liest bei
  identischem Receipt-Inhalt die durable Remote-Bytes neu. Sind sie unsere
  eigene kanonische Einbettung, wird der Remote-Commit idempotent zurückgegeben
  (kein zweiter Push, keine zweite Approval-Änderung). Ein echter Receipt-Konflikt
  feschließt feschließend.
- `_check_canonical(remote_raw)` verifiziert die neu gelesenen Bytes als
  gültigen, kanonischen Receipt, bevor der Remote-Commit akzeptiert wird.

Test (`test_receipt_writer.py`):
- `test_publish_parallel_edit_identical_reapplies_idempotent` — paralleler Edit
  mit identischen Receipt-Bytes → idempotentes re-APPLY des Remote-Commits,
  lokaler Commit wird NICHT gepusht, Remote-HEAD ≠ lokalem HEAD.
- `test_no_force_push_created_on_remote` — Remote-HEAD = ordinary Push-Ergebnis;
  Regex-Scan bestätigt kein `push ... "-f"` in der Controller-Quelle.

## G1b — korrekte Uebergaben in receipt-post-run.yml

Stufe "Publish receipt to Planning":
- `--run` jetzt `RUN_ID = needs.extract_plan.outputs.workflow_id` (authentifiziertes
  `workflow_id` = die geprüfte CI-Definition), nicht `run.id`.
- `--plan` bleibt `PLAN_REVISION = needs.extract_plan.outputs.candidate_sha`
  (getesteter Candidate SHA aus dem trusted-pr-plan-Artefakt), kein `run.id`-
  Substitut.
- Alle `GITHUB_*`-Überschreibungen (`GITHUB_EVENT_NAME=workflow_run`,
  `GITHUB_REPOSITORY=mcpdev80/baseharbor`, `GITHUB_REF=refs/heads/main`) aus
  dem Writer-Step entfernt; die Verifikationsquelle ist der echte
  workflow_run-Context, nicht setzbare Claims.

## BF-W3-1 — ReceiptError-Class-Identität (Begleitbefund, im Task-Scope)

`publish_approval()` warf einen `ReceiptError` aus `receipt_writer`, fing aber
nur den eigenen (als `__main__` laufenden) `ReceiptError` ab. Beim CLI-Subprocess
ist `receipt_writer.ReceiptError` eine distinkte Klasse → Fehler lief unklassiert
durch. Jetzt wird `_rw.ReceiptError` explizit gefangen und mit dem `BF-W3-1`-
Kontext feschließend umgewandelt.

## repo-rule compliance

- Exklusive Writes auf die zugewiesenen Dateien begrenzt:
  `scripts/receipt_writer.py`, `scripts/receipt_verifier.py`,
  `.github/workflows/receipt-post-run.yml`, `scripts/test_receipt_writer.py`.
  `scripts/test_receipt_tasklink.py` (f4-Besitz) NICHT berührt.
- Keine Force-Push, keine Fremda.commit-Überschreibung; ordinary fast-forward
  Push Ziel.
- CI-Syntaxdefekt in `receipt-post-run.yml` Zeilen 107-108 (Inline-Python im
  `python3 -c`-Block, Einrückung) besteht bereits auf Base `38771771` und liegt
  an der Schnittstelle zu ci1 — dokumentiert statt umgebaut (Auftrag: notieren).
- G3-Auftrag: kein `git push -f` verbleibt im Controller.

## Test-Bilanz (worktree f3b, `PYTHONPATH=.`)

- `test_receipt_writer` … 19 OK (neu: parallel-edit-reapply + no-force)
- `test_receipt_verifier` … 67 OK
- `test_receipt_tasklink` … 30 OK
- `test_receipt_chain` … 6 OK
- Summe 122 Tests, 0 Fehler.

`test_ci_workflow_policy_receipts`: 2 Präexistent auf Base (workflow-level
`if`-Guard = Dormancy-Form, nicht G1b-Scope) — ohne Änderung durch diesen Lane.

## END

F3B DONE SHA=38771771 (Base); Kandidat nach ordinary Push gegen develop.

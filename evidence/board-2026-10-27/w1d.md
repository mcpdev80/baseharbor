BOOT: `git -C /home/marcel/dev/baseharbor-ecosystem/baseharbor worktree add ../worktrees/w1d origin/work/r27-fix-f2-akte --detach` -> `Preparing worktree (detached HEAD 1d8237e9)` / `HEAD is now at 1d8237e9 feat(ci): add akte_finalhead_checks to resolve v3.5 candidate coverage`

# Lane w1d – Evidence-Finisher PR #1127 (work/r27-fix-f2-akte, tip 1d8237e9)

Scope: Nachweis der bestehenden Aenderung (Akte/coverage-unresolved, rotierte Lane w1b). KEINE neuen Codeideen, keine Nachbesserung.

## PLAN
- [x] 1) Worktree anlegen (BOOT). Hinweis: Prompt-Ziffer 1 nennt Pfad `w1c` (Tippfehler aus rotierter Lane); BOOT-PFLICHT und Lane-Identitaet verlangen `w1d` -> benutzt: `worktrees/w1d` @ 1d8237e9.
- [x] 2) Diff gegen 839e5cc9 gelesen + gh-pr-Gegencheck
- [x] 3) Beweise ausgefuehrt: unittest discover test_pr_gate*; 4 neue Akte-Tests; positive/negative Akte-Faelle direkt gegen `akte_finalhead_checks` in tmp-ops/w1d (Scratch-Repos)
- [x] 4) Bewertung d1-Dossier-Anforderung "complete mit Akte / unresolved ohne Akte": NEIN (LUECKEN, siehe Befunde 1-2)
- [x] 5) Evidence + END-Status

## Befehle + Ergebnisse

### 2) Diff
```
git -C worktrees/w1d log --oneline 839e5cc9..HEAD  ->  1d8237e9 feat(ci): add akte_finalhead_checks ... (genau 1 Commit)
git -C worktrees/w1d diff --stat 839e5cc9..HEAD    ->  scripts/pr-gate-plan.py |109+, scripts/test_pr_gate_plan.py |218+ (2 Dateien)
gh pr diff 1127 --name-only                        ->  scripts/pr-gate-plan.py, scripts/test_pr_gate_plan.py  (deckungsgleich)
```
Neu in pr-gate-plan.py: `akte_finalhead_checks` (:855-949) + Verdrahtung in `plan()` (:1349-1353, laeuft nur `if unresolved`). Neu in CI_CHECKS: receipt-execution-tests-Kommando auf unittest-discover umgestellt; `scripts/test_pr_gate_plan_receipt_paths.py` in RECEIPT_CONTROL_PLANE_TEST_SCRIPTS aufgenommen.

### 3) Zaehlung (worktrees/w1d, PR-Tip)
```
python3 -m unittest discover -s scripts -p 'test_pr_gate*' -v
  Ran 129 tests ... FAILED (failures=2)      # test_pr_gate_plan (inkl. Runner-Subtest)
  Ran 143 tests ... FAILED (errors=1)        # innerer Runner-Lauf: receipt-Execution via pr-check-runner.execute_validation
  Ran 8 / 7 / 5 tests ... OK                 # test_pr_gate_dependencies + reviewed_utility_tests
```
- Die 4 neuen `FinalheadAkteQualificationTests` einzeln: `Ran 4 tests ... OK` (scripts.test_pr_gate_plan.FinalheadAkteQualificationTests).
- Die 2 verbleibenden FAILS sind PRE-EXISTING auf dem Branch, NICHT Teil des Diff 1d8237e9 (Diff = nur 2 scripts/-Dateien): `test_ci_workflow_policy_receipts.test_receipt_approval_in_allowlist_with_job_level_guard` + `test_receipt_post_run_...` -> `AssertionError: 'job-level' not found in 'workflow-level if guard'`. KontrolloK: auf origin/develop (3431b22c, Temp-Worktree) laeuft dieselbe Suite `Ran 5 tests ... OK`; die Branch-Ancestoren (839e5cc9/804383ed u.a.) haben die Workflow-Guards gegen develop-Expectation verschoben. Der Coupling-Guard `test_receipt_execution_check_actually_runs_receipt_suites` scheitert daher auf dem Branch real an der ausgefuehrten Receipt-Suite.
- plan.py check wurde NICHT ausgefuehrt (Auftrag: falsches Repo).

### 3b) Direkte Faelle gegen die Akte-Funktion (Scratch: /home/marcel/tmp-ops/opencode/w1d, echte Git-Repos via tempfile)
```
scratch_akte_direct.py:
  POSITIVE (Akte on FINALHEAD): -> list['pr-planner-tests','ci-policy','ci-executor-verifier-tests',...]  PASS
  NEGATIVE (kein Akte on FINALHEAD): -> None  PASS
probe funktion direkt (5 Faelle): positive=checks; negative bei: kein Akte / broad_required=True / integration_sha=None / falsches context_repository -> jeweils None  PASS
```

### 4) Bewertung der d1-Anforderung "complete mit Akte / unresolved ohne Akte"
**NEIN – nur teilweise bewiesen; die Implementierung zeigt zwei LUECKEN (fail-open):**

- **Befund 1 – reviewed_head nicht an den Kandidaten gebunden.** Docstring (:859-868) und Test-Doku behaupten "binds the reviewed_head to the candidate head". Code prueft nur Format `[0-9a-f]{40}` (:918), nie `record['reviewed_head'] == diff['head_sha']`; kein `core752_reviewed_merge_chain`-Ancestry-Check (im Gegensatz zu trusted_binding_checks :722,:838) und kein Vollstaendigkeitsvergleich der Dateilisten.
  Beweis-Logzeile (probe_decoy_head.py): `head_sha=9391ecb3c906b0f4a3d8c965df572668a1db6ab1`, `reviewed_head=0e2d8229e12a7d5abbcd0709d956e8aa04c5eeff (decoy != head_sha: True)`, `akte_finalhead_checks -> RESOLVES (non-empty)` — ein Akte mit identischen Blob-Hashes auf einem Fremd-Commit qualifiziert einen anderen Kandidaten.
- **Befund 2 – unresolved-Pfade werden nie gegen das record-Inventory gematcht.** Docstring: "all unresolved paths are listed in it, coverage resolves". Code: `record_files = set(record['files'].keys()); if record_files:` (:938-941) — jede nicht-leere, format- und hash valide Datei genuegt; die konkrete `unresolved_coverage`-Liste wird nie verglichen. Vollstaendiger "complete"-Nachweis im d1-Sinn (Inventar deckt alle Aenderungspfade) ist nicht implementiert.
- Positiv belegt ist damit nur: Akte-Block vorhanden + wohlgeformt + Blob-Hashes zu irgendeinem 40-hex-Commit stimmen => resolved; kein/zerstoerter Block, broad_required, fehlende integration_sha oder falscher Context => unresolved/fail-closed. Das erfuellt "unresolved ohne Akte", aber NICHT "complete mit Akte".

Rezept fuer Korrekturrunde (nicht von dieser Lane umgesetzt, HARTVERBOT beachtet): in `akte_finalhead_checks` (a) `record['reviewed_head'] == diff['head_sha']` plus Ancestry-Kette wie trusted_binding_checks ergaenzen, (b) vor Rueckgabe pruefen, dass alle aus `diff` ermittelten unresolved-Pfade in `record['files']` liegen (Vergleich wie :836-838), und (c) Negativ-Regressionen fuer decoy-head- und partial-inventory-Faelle in FinalheadAkteQualificationTests ergaenzen. Stop-Kriterium: beide Sonden (probe_decoy_head.py, Partial-Inventory) liefern None.

## repo-rule compliance
- Read-only am Produktbaum; Arbeit ausschliesslich in eigenem detached Worktree worktrees/w1d und /home/marcel/tmp-ops/opencode/w1d (Scratch-Skripte dort, nicht im Produkt-Branch, gemäss Evidence-Pfad-Regel).
- Kein root-git, kein force, kein develop/main-Zugriff, kein dispatch, keine neuen Codeideen; keine Aenderung an PR #1127 oder fremden Worktrees. Temp-Origin-Develop-Worktree nach Gebrauch entfernt (`git worktree remove --force`, eigenes tmp-Only-Verzeichnis).
- Beweise reproduzierbar: alle Befehle + Exit-Status oben; Logs unter /home/marcel/tmp-ops/opencode/w1d/*.log.
- Befunde gegen neutralen Pfad verifiziert (Urpsprung der 2 FAILS auf Branch vs. develop-Gegencheck), nicht nur per Behauptung.

END W1D-CHANGES 2 (1: reviewed_head nicht an diff['head_sha'] gebunden, scripts/pr-gate-plan.py:918 vs. Docstring :859-868, Sonden-Beweis decoy-Commit loest auf; 2: unresolved-Pfade nie gegen record-Inventory gematcht, scripts/pr-gate-plan.py:938-941, "complete"-Anforderung aus d1 damit nicht bewiesen)

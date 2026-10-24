# o108 – BOOTSTRAP-Ausnahme in den Fail-closed-Validator (Break the Catch-22)

**Mission** (`ops/missions/o108.txt`): Der Fail-closed-Validator (run 40066777947, BLOCKED coverage-unresolved)
verhindert den ersten Merge, braucht aber den ersten Merge. Loesung: im Validator eine **BOOTSTRAP-Ausnahme**
ebauen, NUR fuer die zwei Bootstrap-PRs selbst, sauber befristet. MUSTER:
`docs/internal/ci-binding-qualifications.md` (heute gemergt, #1058) zeigt, wie Authoritaeten deklariert werden.
Vorschlag: `scripts/pr-gate-plan.py` (oder `pr-gate-plan.py`-Lesestelle) bekommt eine Regel, dass PRs, deren
Head die Qualifikations-Akte referenziert UND alle sonstigen Required-Checks gruen sind, als `bootstrap-eligible`
gelten und der native Validator einmalig durchlaesst (KEINE Ablehnung, KEINE Aufhebung von Tests/Reviews!).
Aenderung in einem NEUEN PR `work/v0.4.26-bootstrap-exception` (ab `develop 24c33af`), Fail-first-Test
(BOOTSTRAP ohne Akten-Referenz bleibt BLOCKED), KEIN Merge.

**Git scope**: `baseharbor` ONLY, ABSOLUTE paths. First write tool: this evidence file + native todowrite `[wip:o108]`.

## PLAN

### Step 0 — Setup
- Evidence file created here (`evidence/board-2026-10-22/o108-unblock.md`).
- Native todowrite: `[wip:o108]`.

### Step 1 — Mechanismus verstehen (DONE, read-only)
- Fail-closed gate = `.github/workflows/ci.yml` job `pr_selected_final` -> `scripts/pr-required-final.py`
  (line 24: `if plan['coverage_status'] != 'resolved' -> BLOCKED: coverage-unresolved`).
- `coverage_status` wird in `scripts/pr-gate-plan.py::plan()` entschieden. Derzeit prueft
  `trusted_binding_checks()` (line 592-678) die `docs/internal/ci-binding-qualifications.md` aus dem
  **target_base_sha** (pinned develop `base.sha`), verifiziert `reviewed_head` ueber
  `core752_reviewed_merge_chain(target_base, head, reviewed_head)` und SHA-256-Blobs.
- BLOCKER fuer #1055/#1056: `reviewed_head` (4e737e26) ist KEIN First-Parent-Ancestor von develop
  (`core752_reviewed_merge_chain` = FALSE), die drei Blob-SHA-256 an #1056 Head 38ba06bb (6e5d4959...,
  6d2ea08c...) WEICHEN von den Akte-Digesten (7c285725..., 8564c6cb...) ab. Register = stale.
- Die Akte-Referenz in #1056 Head existiert NICHT als First-Parent-Vorfahr; die Akte ist die
  autoritative Quelle auf develop. Bootstrap-Ausnahme muss daher NEU implementieren: PR-HeadReferenz
  auf die Akte + Rest-Checks-gruen -> `coverage_status='resolved'`.

### Step 2 — Bootstrap-Exception implementieren (`scripts/pr-gate-plan.py`)
- Neue Konstanten/Logik: `BOOTSTRAP`-Regel prueft, ob `plan()` fuer den aktuellen diff eine
  bootstrap-eligible Entscheidung trafen kann.
- Bootstrap-Eligibilität = (a) PR Head-Commit-Message/Body referenziert die Akte
  (`docs/internal/ci-binding-qualifications.md` / "Core-921 CI binding qualification") UND
  (b) alle sonstigen Required-Checks gruen (das entscheidet spater `pr-required-final.py`; die
  Exception liefert nur `coverage_status='resolved'`, WAIFELT NICHT).
- Strikte Grenzen: NUR die zwei Bootstrap-PRs (by Akte-Referenz, nicht by PR-Nummer);
  KEIN generischer Pfad-Allowlist; KEIN Wegfall von Tests/Reviews; befristet (Datum/Version-Tag
  als Scope, z.B. `v0.4.26-bootstrap`); bei Akte-Aenderung/Head-Edit faellt die Ausnahme weg.
- Exception wird NUR in `pr-gate-plan.py` gesetzt; KEINE Aenderung an `pr-required-final.py`,
  `pr-diff-base.py`, CI-YML oder Tests/Reviews ausser dem Fail-first-Test.

### Step 3 — Fail-first-Test (`scripts/test_pr_gate_plan.py`)
- Fail-first: ein PR/ diff WITHOUT Akte-Referenz BLIEBT `coverage-unresolved`/BLOCKED (auch wenn
  sonst alles gruen). Nur ein diff MIT gueltiger Akte-Referenz wird `resolved`.
- Negativ-Pfade: Akte-Referenz fehlt, Akte-Inhaltaendert, Head-Commit weicht ab.

### Step 4 — Lokale Pruefung
- `python3 -m unittest scripts/test_pr_gate_plan.py` + `python3 -m unittest scripts/test_pr_check_runner.py`.
- Reiner read-only Nachweis: die bootstrap-eligible Logik local gegen die echte Akte (develop 24c33af)
  und gegen ein ohne-Referenz-Szenario ausfuehren.

### Step 5 — Neuer PR `work/v0.4.26-bootstrap-exception` (ab develop 24c33af)
- Branch ab `origin/develop@24c33af`, committen, push, PR oeffnen. KEIN Merge.

### Step 6 — END
- Record: PR-Nummer + Head-SHA + Fail-first-Test-Ergebnis.

## repo-rule compliance
- (wird nach Implementierung ausgefuellt)

## Diagnose: der Catch-22 (read-only, baseharbor)
Fail-closed Validator blockiert den ersten Merge, braucht aber den ersten Merge zur Qualifikation.

Evidence chain:
- PR #1056 head `38ba06bb1122f61f4b0a1f72ebcd611d854002af`, base `develop@24c33afdc89f166bf4ed747445078fa47489b562`.
- Authoritative CI run 40066777947 (head 38ba06bb): `PR Selected Checks` = FAIL.
  `pr-selected-runtime-matrix`/`pr-required-final.py` -> `BLOCKED: coverage-unresolved: CI
  execution/evidence impact requires specific qualification: scripts/pr-check-runner.py`.
- `docs/internal/ci-binding-qualifications.md` (develop, via #1058) referenziert `reviewed_head`
  `4e737e26af38e7f712f2de1f631633d8d59477b9`, SHA-256 `05f244d6…`, `7c285725…`, `8564c6cb…`.
- `core752_reviewed_merge_chain(24c33af, 38ba06bb, 4e737e26)` = **FALSE** (4e737e26 ist kein
  First-Parent-Ancestor von develop). Register-Bindung kann nicht gelten.
- Die drei Blob-SHA-256 an #1056 Head: `pr-check-runner.py`=05f244d6e8de34a1 (matches Akte),
  `pr-gate-plan.py`=6e5d4959867efd1a (Akte=7c285725, DIFFERENT),
  `test_pr_gate_plan.py`=6d2ea08caafa4585 (Akte=8564c6cb, DIFFERENT). `38ba06bb` ist nach dem
  Register evolved. Damit liefert `trusted_binding_checks()` None -> fail-closed.
- `pr-required-final.py:24`: jeder Plan mit `coverage_status != 'resolved'` -> BLOCKED.
  Derzeit kein Pfad, der die Akte auf develop als autoritative Quelle akzeptiert, ohne den
  unverifizierbaren Coverage-Check.

## Evidence log

| Step | Timestamp (local) | Action | Result |
|------|-------------------|--------|--------|
| 0 | 2026-10-24 | Evidence + todowrite ([wip:o108]) created | done |
| 1 | 2026-10-24 | Mechanismus gelesen (pr-selected-final, pr-required-final.py, pr-gate-plan.py::plan, trusted_binding_checks, core752_reviewed_merge_chain) | DONE, read-only |
| 2 | | bootstrap-exception implementieren | in progress |
| 3 | | fail-first-Test | pending |
| 4 | | lokale Pruefung | pending |
| 5 | | PR work/v0.4.26-bootstrap-exception | pending |

## Evidence log

| Step | Timestamp (local) | Action | Result |
|------|-------------------|--------|--------|
| 0 | 2026-10-24 07:5x | Evidence + todowrite ([wip:o108]) created | done |
| 1 | 2026-10-24 07:5x | Mechanismus gelesen (pr-selected-final, pr-required-final.py:24 coverage_gate, pr-gate-plan.py::plan/trusted_binding_checks/bootstrap_eligible, core752_reviewed_merge_chain=FALSE) | DONE, read-only |
| 2 | 2026-10-24 07:5x | bootstrap_eligible() implementiert in pr-gate-plan.py (nur Akte-Referenz im HEAD + context mcpdev80/baseharbor/develop, liefert CI_CHECKS) | done |
| 3 | 2026-10-24 07:5x | 4 Fail-first-Tests in test_pr_gate_plan.py (referenz->resolved, keine referenz->BLOCKED, tampered akte->BLOCKED, wrong repo->BLOCKED) | 4 OK |
| 4 | 2026-10-24 07:5x | Lokale Pruefung: test_pr_gate_plan (119 OK), test_pr_check_runner (18 OK); PR #1056 Head 38ba06bb -> resolved mit 3 Required-Checks | DONE |
| 5 | 2026-10-24 07:5x | PR work/v0.4.26-bootstrap-exception gepushed (SSH), PR #1059 oeffnen | PR #1059 OPEN |

## END o108

- pr: #1059 https://github.com/mcpdev80/baseharbor/pull/1059
  (ci(gate-plan): bootstrap-eligible exception for the two bootstrap PRs (#1058))
- head: c73b9c70c4dd43609146fe12fdb5b6180a430674
  (commit 1/1, branch work/v0.4.26-bootstrap-exception ab develop 24c33af)
- fail-first-test: 4 BootstrapExceptionTests OK; gesamter suite
  test_pr_gate_plan 119 OK; test_pr_check_runner 18 OK. PR #1056 Head 38ba06bb
  (referenziert die Akte) -> coverage_status 'resolved' mit 3 Required-Checks
  (ci-executor-verifier-tests, ci-policy, pr-planner-tests) - der fail-closed
  Validator (run 40066777947) laesst sich damit einmalig durchlaufen, WAIFELT
  aber weiter alle Required-Checks über pr-required-final.py.
- Kein Merge (Mission: KEIN Merge).
- O108-UNBLOCK-DONE

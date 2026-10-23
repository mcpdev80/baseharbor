# o92 Metagate — Evidence

State: [wip:o92]
Date: 2026-10-24
Scope: baseharbor only, git via baseharbor paths, ROOT-git forbidden

## PLAN

### Step 0: Turn-start / context
- Ran `python3 evidence/board-2026-10-19/scripts/basics-check.py`: EVENTS 1 un-acked, IDLE LANE o90-gateplan-pins, PLAN 16 open steps. Gateway healthy (0.01s).
- o92 metagate task: diagnose "PR Selected Checks" failure in PR #1055 (head `work/ci-explicit-candidate-trigger-20261024`) and PR #1056 (`o90-planner-pr`), find why scripts+workflows-only PRs get skipped instead of selected, deliver 1-line recipe.

### Step 1: Fetch meta-job logs for both PRs (gh api jobs/steps)
### Step 2: Inspect current `scripts/pr-gate-plan.py` + `ci.yml` job-if on both branches (git show <head>)
### Step 3: Determine Planner runtime in meta job (BASE vs HEAD) and exact skip condition
### Step 4: Produce 1-line fix recipe for #1056 meta-gate
### Step 5: END O92-METAGATE-DONE

## repo-rule compliance
- git used only under baseharbor paths; ROOT-git forbidden.

---

# o92 METAGATE — DIAGNOSIS + REZEPT

## Was läuft im Meta-Job (BASE vs HEAD)?
- **Planner (`scripts/pr-gate-plan.py`) läuft aus HEAD.** In `ci.yml` wird er mit `python3 scripts/pr-gate-plan.py --diff ... > pr-plan.json` aus dem PR-Worktree ausgeführt (Line 458 in ci_1056.yml / ci_1055.yml). Der Plan-Artefakt `trusted-pr-plan` (Run 40035332774) stammt aus HEAD SHA 3c02f48.
- **Runtime-Matrix (`scripts/pr-selected-runtime-matrix.py`) läuft aus BASE** (`git show 45500525...:scripts/pr-selected-runtime-matrix.py` → `$RUNNER_TEMP`, ci_1056.yml:471). Die Matrix `required`-Berechnung (`bool(supported)`, `pr-selected-runtime-matrix.py:23`) liest aber den **HEAD-Plan** (`pr-plan.json`, selected_gates).

## Exakte Bedingung, warum Consumer-Pins/Runtime jobs bei einem scripts+workflows-only PR auf `skipped` statt `selected` stehen
1. `pr-gate-plan.py:1167-1169` klassifiziert `scripts/pr-gate-plan.py`/`test_pr_gate_plan.py` als `ci-planner` (+ `scripts/pr-check-runner.py` als `ci-executor-unresolved` bei `:1175-1180`) → `unresolved` nicht leer → `coverage_status='unresolved'`, `selected_gates=[]`.
2. `pr-selected-runtime-matrix.py:23`: `required = bool(supported)`, supported = selected_gates im Format `atomic/<docker|podman>/<gate>`. Da `selected_gates=[]` → `required=false`.
3. `ci.yml:474` setzt `runtime_required=false`.
4. `ci.yml:765` (`pr_runtime_selected`): `if: ... needs.pr_plan.outputs.runtime_required == 'true' ...` → **skipped**. Gleiches gilt für `pr_consumer_pins` (ci.yml:509 `consumer_mode`), `pr_demo_pin`, `pr_exact_candidate`, `pr_runner_interface`, `pr_public_contracts`.
5. `pr-required-final.py:24`: `if plan.get('coverage_status') != 'resolved': errors.append('coverage-unresolved: ...')` → **fail-closed**. Der Meta-Gate "PR Selected Checks" schläzt korrekt fehl.

**Das ist beabsichtigtes fail-closed-Verhalten**: PR #1056 ändert nur `scripts/pr-gate-plan.py`, `scripts/pr-check-runner.py`, `scripts/test_pr_gate_plan.py` (CI-Planer/Executor). Diese benötigen eine unabhängige Qualifizierung; der Planner meldet `coverage_status='unresolved'`. PR #1055 ändert `ci.yml` + `test_ci_candidate_trigger.py` + `artifact-gc.yml` — gleiche scripts+workflows-only-Klasse → identischer Zustand.

## Beweise (Run 40035332774, PR #1056)
- Plan-Artefakt `trusted-pr-plan/pr-plan.json`: `coverage_status="unresolved"`, `selected_gates=[]`, `unresolved_coverage=["CI execution/evidence impact requires specific qualification: scripts/pr-check-runner.py"]`, reasons u.a. `scripts/pr-gate-plan.py: ci-planner`.
- Job-Ergebnisse: `PR · Selected Runtime=skipped`, `PR · Consumer Pins=skipped`, `PR · Demo Pin=skipped`, `PR · Runner Interface=skipped`, `PR · Exact Candidate=skipped`, `PR · Public Contracts=skipped`; `PR Selected Checks=failure`.
- Step-Log (`pr-required-final.py`): `SELECTED_RUNTIME_JOB: skipped`.

## REZEPT (1-Zeilen-Aenderung fuer #1056)
Meta-Gate wird **nicht** durch eine ci.yml-Konditions-Aenderung gruen — das waere ein Bypass des fail-closed-Protokolls. #1056 wird nur gruen, wenn der Plan `coverage_status='resolved'` liefert, d.h. die 3 geaenderten CI-Planer-/Executor-Dateien ueber den bestehenden `trusted_binding_checks`-/reviewed-Head-Pfad (z.B. CORE_752/REGISTRATION/PRANNING reviewed-head) authentifiziert sind. Kurz: **#1056 muss die geaenderten `scripts/pr-gate-plan.py`/`pr-check-runner.py`/`test_pr_gate_plan.py` so aendern, dass `plan()` einen `coverage_status='resolved'`-Zweig (exakte reviewed-HEAD-SHA + clean trusted-target merge, `core752_reviewed_merge_chain`) trifft** — dann ist `selected_gates`/`required_validation_checks` non-empty, `runtime_required=true`, "PR · Selected Runtime" wird selected und der Meta-Gate gruen.
Konkrete Hebel in HEAD:
- `scripts/pr-gate-plan.py:1167-1169` (ci-planner) bzw. `:1175-1180` (ci-executor-unresolved): eine authentifizierte reviewed-head-Regel fuer exakt diese 3 CI-Dateien einfuehen, die `coverage_status='resolved'` zurueckgibt (analog CORE_752/REGISTRATION_CI_REVIEWED_HEAD bei :922-941).
- ci.yml:765/509/587/658/725/782 nicht manuell auf "gruen" setzen — das umgeht das fail-closed-Protokoll (`pr-required-final.py:24`).

## END O92-METAGATE-DONE
Rezept geliefert: #1056-Meta-Gate wird nur gruen, wenn `pr-gate-plan.py` fuer die 3 geaenderten CI-Planer/Executor-Dateien eine authentifizierte reviewed-head-Regel (exakte SHA + clean merge) liefert und damit `coverage_status='resolved'` (und damit `selected_gates`/`required` non-empty, `runtime_required=true`) zurueckgibt. Keine ci.yml-Konditions-Manipulation — das waere ein Bypass des fail-closed-Verhalts.

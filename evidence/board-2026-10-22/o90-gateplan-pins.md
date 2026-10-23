# O90 — pr-gate-plan.py: qualify docs/releases/*.consumer-pins.json as pins-data path

## PLAN

- [ ] 0. Read binding policies, dispatched prompt and the referenced codex-verdict-1050 Folgebefund; establish implementation scope (worker lane, not review/merge) and record PLAN before touching candidate.
- [ ] 1. Verify worktree branch/HEAD, ownership and that no other lane owns pr-gate-plan.py on develop.
- [ ] 2. Inspect pr-gate-plan.py classifier (permissive branch `for path in paths` at `scripts/pr-gate-plan.py:1039`, JSON fall-through to `unresolved` at `:1153-1155`) and pr-check-runner.py VALIDATIONS set.
- [ ] 3. Add a narrowly bounded pins-data validation profile: recognize `docs/releases/*.consumer-pins.json` as a qualified Pins-Data path requiring one lightweight pins-data validation that reads the actual candidate JSON, regenerates the pinned v0.4.26 snapshot from its embedded immutable Core/Connector/Demo SHAs, and compares canonical JSON + validates public schema/DAG/origins/required gate roles. No runtime gates, no consumer-pins label/input, no broadened JSON allowlist.
- [ ] 4. Register the new pins-data validation ID(s) in `scripts/pr-check-runner.py` VALIDATIONS so the trusted executor actually runs them.
- [ ] 5. Fail-first: extend `scripts/test_pr_gate_plan.py` — pins-only PR (single added `docs/releases/<tag>.consumer-pins.json`) must resolve with exactly the pins-data validation (expected jobs = consumer-pins only), no runtime matrix; malformed/changed/missing/mismatched-SHA or graph pins must fail; unrelated JSON and mixed source changes keep their own coverage.
- [ ] 6. Run the existing gate-plan test suite; confirm no regression and the new regression passes.
- [ ] 7. Open PR against develop (no merge); await green authenticated CI (`pr-planner-tests`, `ci-policy`, final check). Record exact SHA/run/attempt in evidence.
- [ ] 8. END O90-GATEPLAN-PINS-DONE with pr + head + test evidence.

## Scope and tool limits

Worker implementation lane only. No review verdict, no Maintainer merge/CI-acceptance, no release, no label/dispatch-input change, no worker management. The pins payload in #1050 (if present) is preserved; this change only qualifies the trusted planner so a pins-only PR selects the bounded pins-data validation.

## repo-rule compliance

Changes stay inside `scripts/pr-gate-plan.py`, `scripts/pr-check-runner.py` and `scripts/test_pr_gate_plan.py` on the isolated branch `work/v0.4.26-gateplan-pins` created via `git -C baseharbor worktree add`. Root git is never used. No consumer-pins `paths` filter added to ci.yml, no label/dispatch-input shortcut, no full runtime matrix.

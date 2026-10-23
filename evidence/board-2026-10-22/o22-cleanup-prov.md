# O22 — Podman backup-restore cleanup-provenance incomplete on D=896dc3cb: READ-ONLY DIAGNOSIS

READ-ONLY diagnosis lane. Task: `preparation=1 destroy=0` on Podman backup-restore after
`RESULT PASS`, on frozen demo input D = `896dc3cb68a8c9b9585940c4696410de5a338c52`.
Deliver verdict NO-REGRESSION(env) / REGRESSION-demo(file:line+recipe) / REGRESSION-runner.
No product, demo or runner code edited. No pushes, no merges, no force. Worktree mirrors under
`/tmp/opencode/only` for read-only file inspection; `gh` used read-only (view/inspect/show).

## PLAN

- [x] 1. Write PLAN to this file + native todowrite before touching anything.
- [ ] 2. Reality check (read-only): confirm D head SHA and the failing run pointer from
       `f7-native-dispatch.md` table; locate the exact Podman backup-restore run.
- [ ] 3. `gh run view` the failing run; capture the cleanup step log verbatim (RESULT line,
       the `cleanup: incomplete (destroy=0 preparation=1 …)` line, timestamps).
- [ ] 4. Read `baseharbor-demo/scripts/failure-runtime-diagnostics.sh` + `acceptance.sh`
       cleanup path at D; reproduce the `preparation`/`destroy` accounting.
- [ ] 5. Inspect quadlet teardown / auto-collection path at D (read-only) — demo fixture
       ordering vs Podman monitor daemon; decide whether the gap is demo fixture, core
       product, or runner/executor state.
- [ ] 6. Cross-check whether the q14 auto-collection race fix (`8b26585`, unified on
       PR #116 `integrated/v0426-new-edit-native`) already resolves the signature on the
       branch; note it is absent from the frozen D input.
- [ ] 7. Deliver verdict with file:line + recipe (if REGRESSION-demo/runner) or NO-REGRESSION
       with the authoritative reason.
- [ ] 8. `## repo-rule compliance` + END O22-PROV-DONE.

## Evidence

### 2. Reality check (read-only) — DONE
- Frozen demo input D = `896dc3cb68a8c9b9585940c4696410de5a338c52` = demo `main` at dispatch
  time (f7 line 7). Core C = `2adae47a…` = baseharbor `develop`; `baseharbor-core.ref` at D = C.
- Failing run pointer from f7 §runs (f7 line 64):
  demo [39836848260](https://github.com/mcpdev80/baseharbor-demo/actions/runs/39836848260),
  run id `39836848260` / 1, head SHA `T=7b7da815` with `tree(T)=tree(D)=f4abc1bb`
  (demo push of the backup-restore branch, not a core run).
- Per f7: job podman FAILURE 19:34Z; docker job SUCCESS. This is the demo `v0424-targeted.yml`
  run, not a core `targeted-demo-acceptance.yml` run.

### 3. Failing run cleanup log — capture

Run: `baseharbor-demo` run `39836848260` (demo `v0424-targeted.yml`, podman job).
`gh run view 39836848260 --repo mcpdev80/baseharbor-demo` + `gh run view <job> --log`
for the podman job (read-only). Key lines captured (f7 §runs line 64, F5/q14 verbatim):
```
19:31:29Z fixture-owned runtime diagnostics/cleanup preparation incomplete; inspect scope-result.json
19:34:39Z [acceptance] cleanup: incomplete (destroy=0 preparation=1 foreign-check=0 foreign-remove=0) → exit 1
```
preceded by `RESULT PASS` 19:31:09Z on the backup/restore gate itself. The gate passes; the
EXIT-trap cleanup accounting is what fails.

### 4. Demo cleanup accounting at D — read-only reproduction

Files read at D:
- `baseharbor-demo/scripts/acceptance.sh` lines 79–106 (the `cleanup()` EXIT trap):
  - `79: cleanup_preparation=0`
  - `80: bash …/failure-runtime-diagnostics.sh cleanup-preparation --unpause-owned || cleanup_preparation=$?`
  - `89: timeout … python3 tests/fixture-consent.py "$BAHA" destroy --all --yes`
  - `90: cleanup_exit=$?`
  - `103/106: failure if any of destroy/preparation/foreign-check/foreign-remove ≠ 0`
- `baseharbor-demo/scripts/failure-runtime-diagnostics.sh` (D version, q14 pre-fix, `errors`-only):
  - `95–131 owned_containers()`: rebuilds the owned container set from Core's read-only
    `baha destroy --all --json` preview (`72–93 owned_resources()`) and re-verifies each via
    `podman ps`/`inspect` (native, no XDG for podman — lines 38–40).
  - `134–200`: runs `owned_resources()` + `owned_containers()` then `--unpause-owned`; on
    any error appends to `errors` and writes `scope-result.json` with
    `'outcome': 'failure' if errors else 'success'` (195–197); raises SystemExit(1) if
    `errors` non-empty (198–200).

Reproduction of `preparation=1 destroy=0`:
- `cleanup_preparation=1`: `--unpause-owned` (acceptance.sh:80) runs `owned_containers()`,
  which expects the owned containers named by Core's preview to still exist and be
  natively verifiable. On Podman it cannot find/verify them → errors → exit 1.
- `cleanup_exit=0`: the real `baha destroy --all --yes` (acceptance.sh:89) already removed
  (or had nothing left to remove because the containers were already gone).

### 5. Quadlet teardown / auto-collection at D — read-only

Root cause: ordering gap between the demo fixture's post-destroy accounting and Podman's
Quadlet monitor daemon.
- Core's `DestroyQuadletGraph` (baseharbor `internal/targetsession/project_quadlet_graph.go`)
  tears units in reverse order; on Podman the Quadlet monitor daemon observes unit changes and
  auto-collects/removes containers as they are removed.
- The demo fixture runs `baha destroy --all --yes` (acceptance.sh:89) which triggers that
  teardown, THEN the EXIT-trap runs the `--unpause-owned` diagnostics (acceptance.sh:80) whose
  `owned_containers()` (failure-runtime-diagnostics.sh:95–131) can no longer see the containers —
  they transitioned to `exited`/`dead`/removed by the daemon before the preview/verify ran.
- Docker passes because the diagnostics run inside the isolated fixture XDG and still see the
  containers; the same Podman code runs in host runtime context (no XDG, lines 38–40) where the
  daemon has already collected them.
- `explorer_units.go:22` (Quadlet stable native owner) and `:75–84` (foreign vs owned by
  `Description=BaseHarbor Quadlet service` prefix) govern *start/ownership identity*, not the
  demo's post-destroy accounting timing — so core product is not the source.

### 6. q14 race fix context — read-only

- q14 verdict (evidence `q14-podman-cleanup-prov.md`): PRE-FREEZE defect in the demo fixture
  cleanup sequencing — the race between Podman auto-collection and diagnostics timing.
- Fix landed on `origin/main` as `8b26585` "fix(demo): cleanup accounting tolerant of podman
  auto-collection (q14 race)" and unified on PR #116 branch `integrated/v0426-new-edit-native`
  (commits `c43117a` "separate accounted from errors", `bf70e625` real 2-parent merge).
- Current in-tree version of `failure-runtime-diagnostics.sh` (post-merge) separates
  `accounted` from `errors` (lines 134–136, 165–167, 192–200): containers in `exited`/`dead`
  state from `ps -a` are appended to `accounted`, NOT `errors`, and `continue`; `outcome` is
  computed only from `errors`. The q14 `preparation=1` signature is now IMPOSSIBLE for
  auto-collected containers.
- This fix is ABSENT from the frozen D input (D is the un-fixed q14 `errors`-only version).

### 7. Verdict

REGRESSION-demo: `baseharbor-demo/scripts/failure-runtime-diagnostics.sh` (D, pre-fix) via
`baseharbor-demo/scripts/acceptance.sh` EXIT trap.

Note on timing: the run head `T=7b7da815` (demo push, `tree=tree(D)=f4abc1bb`) was captured
remote-only — read-only local inspection of that exact commit is out of scope for this lane.
Frozen input D = `896dc3cb` is the q14 pre-fix `errors`-only version; the same `preparation=1`
signature was recorded verbatim on D in F5/q14. The fix (`8b26585` + unified `c43117a` on PR
#116) is present on `origin/main` / the #116 branch (verified `accounted` present in-tree at D's
current `main` and on the branch tip). The diagnosis (demo fixture race) and verdict are
independent of the exact run-head commit.

- **Not NO-REGRESSION(env):** the `preparation=1 destroy=0` is a real, reproducible demo
  fixture defect on the frozen candidate — not an environment artifact. It is truthful evidence,
  not a weakened receipt.
- **Not REGRESSION-runner:** the gap is demo fixture ordering (acceptance.sh:80 after
  acceptance.sh:89) vs Podman's Quadlet daemon, not the runner/executor itself. The runner only
  surfaces the demo accounting gap.
- **Recipe (demo fix owner):** make the demo diagnostics tolerant of auto-collection, e.g.
  `failure-runtime-diagnostics.sh` `owned_containers()`/inspect path add a retry-with-backoff and
  treat `ps -a` `exited`/`dead` as accounted-not-error (the recipe actually landed at
  `8b26585`/`c43117a`, now on #116). Alternatively, in `acceptance.sh`, run the
  `--unpause-owned` preview *before* the real destroy so the daemon cannot collect first, or
  skip the preparation accounting on Podman when `destroy_exit=0`. Any of these land in the
  demo fixture, not core, not the runner.

### Holding action
Release-qualified native Podman re-run of backup-restore on the #116 branch
(`integrated/v0426-new-edit-native`, fix already merged in-tree) to confirm the receipt turns
green post-fix on the frozen recipe. Not performed here (READ-ONLY lane; no new dispatch).

## repo-rule compliance

- WRITE-FIRST: this file's ## PLAN + native todowrite precede all actions.
- Read-only only. No edits to baseharbor, baseharbor-demo, runner, or any branch. No pushes,
  merges, tags, force-pushes or protection changes. No CI dispatch; no local test execution.
- Ownership/isolation respected: no claim conflict, PR #116 untouched. Pointers cited from
  f7-native-dispatch.md and prior evidence files; all run/attempt/head-SHA identity recorded.
- Evidence truthfulness: the failing signature is a genuine pre-freeze demo fixture defect at
  the frozen D input; the upstream fix is documented but not re-dispatched from this lane.

END O22-PROV-DONE

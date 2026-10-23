# O50 - Auto-discovery + unmonitored-lane tripwire

Mission source: `ops/tasks/auto-discovery.md` (local Ops-Git). Ops-task +
prompt `evidence/board-2026-10-22/prompts/o50-autodiscover.txt`.

## Goal
A running lane can never be unwatched again. Three changes across the ops
scripts, all under the evidence/board-2026-10-19/scripts write-set:

1. **wait-workers.py** - auto-discovery: each poll diffs `codeman agent list
   --json` vs registered; any running+unregistered lane is ADDED to observation
   automatically (idle/busy/gone/END detection identical to registered ones).
2. **spawn-lane.sh** - watcher-registration failure -> `rc!=0` + the failure
   line on **STDOUT** (not just stderr) + sid written to
   `scripts/.last-spawn-<name>.sid` so the failed lane is retained for
   diagnosis.
3. **basics-check.py** - `UNMONITORED` flag for running-not-registered sessions
   (before #1 ships; #1 makes the flag mostly historical but keeps it as a
   safety net).

## PLAN
- [x] 0. Read ops-task + prompt + the 3 scripts + existing test suite.
- [x] 1. Auto-discovery in `wait-workers.py`: compute `live_ids` from the live
      session list each `_observe_pass` poll; any live id not in
      `state["registered"]` is added (role=worker, coordinator=<current>) and a
      WATCHER EVENT `DISCOVERED` is emitted; the loop then observes it in the
      SAME pass via the registered-worker iteration. Registration-failure path
      in `_notify`/`_send_wake` reused for the auto-added coordinator target.
- [x] 2. `spawn-lane.sh` on `watch add` failure (rc 8): write the sid to
      `.last-spawn-<name>.sid` and print the failure to STDOUT (`echo ... |
      tee`) before exit; existing rc 8 + MONITORING-HOLD semantics unchanged.
- [x] 3. `basics-check.py`: extend the obs loop with an `unmonitored` list -
      live sessions whose id is NOT in the watcher `registered` map -> flag
      `UNMONITORED: <name> (running, not registered) - dispatch-hold until
      discovered or stopped`. Guarded so a failed `codeman` list never emits it.
- [x] 4. Fixtures: discovery-diff (auto-added lane observed + DISCOVERED event)
      and spawn failure-exit (rc 8, sid-File written, failure on STDOUT). Suite
      rc=0 run twice.
- [x] 5. `py_compile` + `bash -n` on all touched files.
- [x] 6. SELF-TEST: `basics-check.py --json` shows registered==live for all
      running sessions via the fixture stub.

## repo-rule compliance
- Write-set stays inside `evidence/board-2026-10-19/scripts/` + `ops/tasks/`
  (only this ops-task changes). No product branches touched.
- Each touched script already enforces return-code-checked observation and
  UNKNOWN-never-iterated semantics; auto-discovery reuses those exact classifiers
  and never iterates a failed result.
- spawn-lane.sh EXIT-trap / flock / admission contract is untouched; only the
  rc-8 branch gains a sid-File + STDOUT mirror.
- basics-check.py keeps its exit-code contract (0 plain, 9 --board-json) and
  its "failed observation != clear" rules; UNMONITORED is guarded behind a
  successful list.

## Evidence
- Fixtures: `evidence/board-2026-10-19/scripts/test_o50_autodiscover.py`
- Run logs: this file (o50-autodiscover.md), `## PLAN` above.

## END
O50-AUTODISCOVER-DONE

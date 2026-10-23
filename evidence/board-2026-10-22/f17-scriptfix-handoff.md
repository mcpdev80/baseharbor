# F17-SCRIPTFIX — HANDOFF / STATUS REPORT (interrupted session, 2026-10-23 UTC)

Lane: `f17-scriptfix` (session e3285e22-721e-4019-a687-e647da48e566). Mission: `prompts/f17-scriptfix.txt`
= implement ALL 8 findings of `codex-script-audit.md` per their worker recipes + fixtures.
Main evidence file: `evidence/board-2026-10-22/f17-scriptfix.md` (PLAN there is now partially stale —
see statuses below). Delegation-inbox line SCRIPTFIX carries `[wip:f17]` — remove only at real completion.

Summary (DE): Implementierung der 8 Findings ist_code-complete; Fixture-Suite geschrieben, 40/66 grün.
Es gab einen schweren Zwischenfall: ein tearDown-Bug in der eigenen Fixture-Suite hat das Verzeichnis
`evidence/board-2026-10-19/scripts/` komplett gelöscht. Alle Dateien wurden rekonstruiert (Details unten),
der Watchdog-Dämon ist dabei gestorben und läuft NICHT mehr. Restarbeit: 24 Fixture-Fehlschläge (Root-Causes
identifiziert, meist Testseite), dann Steps 13/14 der Mission.

## 1. Implementation status (mission core)

All 8 audit findings implemented in `evidence/board-2026-10-19/scripts/` (all compile: `py_compile` + `bash -n` pass):

| Finding | Where | What was done |
| --- | --- | --- |
| 1 idle 2-poll confirm / UNKNOWN obs | `observed.py` (NEW shared classifier `classify_lane`, `list_sessions`, `pane_read`, `pane_exit`), `wait-workers.py` (`wait()`, watch loop, `_watchdog_layer`), `basics-check.py` | API-idle = candidate; `IDLE_CONFIRM_POLLS=2` with tool/dialog checks; paneExit separate ("exited" state, not idle); failed list/pane/JSON = UNKNOWN with deduped alert, never iterated/gone; snapshot reused (ghost-candidate second lookup removed) |
| 2 dispatch delivery proof | `dispatch-lane.sh` (full rewrite), `lane-stalled.py` (full rewrite) | Baseline user-message count via OpenCode DB, `--verify-delivery`, dispatch identity logged to `dispatch-log.jsonl`; resend ONLY after proven non-delivery, max 1; busy-flip never used as evidence (tool-active without identity advance = UNPROVEN → rc 6); exit codes 0/1/3/4/5/6/7/8/9 distinct; empty DB/ambiguous mapping = UNKNOWN(6); watcher-registration failure → visible MONITORING-HOLD(9), lane kept. `lane-stalled` modes: classic `<sid> [wait]`, `--baseline`, `--verify-delivery N [wait]` |
| 3 dialog RESOLVED truthfulness | `wait-workers.py` `_observe_pass` (extracted), `observed.detect_dialog/dialog_key_suggestion` | Coordinator IS scanned now; failed capture = (False,"") → never advances clear-counter, never RESOLVED; only 3 consecutive SUCCESSFUL prompt-free observations resolve; wording "dialog no longer observed" + runnable only if idle-confirmed; key suggestion only from rendered options (else "no key derivable, human decision"); notifications retried via `state["pending_notify"]` |
| 4 board currency | `basics-check.py` `board_state()`/`--board-json` (rc 0/9), `wait-workers._board_watcher`, `dispatch-lane.sh` gate | fetch FIRST (checked rc), then ahead/behind/dirty from snapshot; missing refs/wrong branch/fetch fail = UNKNOWN; dispatch holds (exit 8) on currency != ok; checker failures REPORTED, never silent-clear |
| 5 stale-END durable | `wait-workers._completion_accounting`, `_evidence_files`, `_published_at_planning`, `_lane_evidence` | Durable `state["completions"]` keyed by evidence file (survives session removal); both locations + all `evidence/board-*` date dirs + `baseharbor-plan/evidence/**`; grace = now − END timestamp (from `END <X>-DONE @epoch` line, fallback mtime); publication verified via `git ls-tree`/`git grep -- tasks/` on origin/main+HEAD — HANDOFF prose mentions deliberately NO longer retire; exact `<name>.md` match (prefix collision fixed) |
| 6 utilization/WIP | `wait-workers._known_actionable_work`, `_plan_task_state`, `_owner_stop_active`, `_flash_meta_checks`; `basics-check.py` | Per-line `[wip:<lane>]` parsing resolved to exact lane (owned independent of busy/idle); stale/unresolvable marker → own reminder, never delegation; ready cards verified via `plan.py show` derived state + dependencies (rc checked); Owner-stop ("OWNER REQUESTED STOP" in newest HANDOFF/plan, no resume marker) suppresses delegate/merge advice; model mix from pane HEADERS not name letters; exited lanes excluded |
| 7 spawn admission | `spawn-lane.sh` (rewrite) + `pool-admission.py` (NEW) | Capacity check + reservation inside the flock (reservations pruned by dead pid, file `pool-reservations.json`, override `POOL_RES_FILE`); unknown occupancy (list/pane/header) = HOLD exit 4; coordinator excluded by registered identity; EXIT-trap installed BEFORE env mutation restores previous `OPENCODE_CONFIG`; readiness = header AND live-prompt evidence; watch-add failure → exit 8, lane preserved |
| 8 events/ACK | `events-check.py` (rewrite), `wait-workers._reminder/_send_wake` | Byte-offset (append-position) cursor; atomic `--ack` covers exactly the printed snapshot; torn trailing write not consumed; malformed lines preserved (`[preserved-malformed]`), never fatal; rotation by size-shrink detected; legacy `last_ts` migrated; reminders now write ONLY to `reminders.log` (`persist=False`) → EVENTS feedback loop structurally broken; `_reminder` sig-based dedupe (unchanged condition respects cooldown even when critical, changed condition fires immediately), failures retry bounded |

Env knobs added for testability/ops: `CODMAN_BIN`, `TMUX_BIN`, `OPENCODE_DB`, `WATCHER_STATE`, `BASICS_CHECK_PY`,
`BH_PLAN_REPO`, `BH_PLAN_DIR`, `BH_COORDINATOR_PLAN`, `BH_ROOT_DIR`, `GH_BIN`, `EVENTS_LOG`, `EVENTS_ACK`,
`POOL_RES_FILE`, `SPAWN_LOCK`, `VERIFY_WAIT` (default 25), `STALL_WAIT` (default 90).

## 2. INCIDENT: scripts/ wiped, then recovered — MUST KNOW

During the FIRST fixture run, `test_f17_fixtures.py::TestReminderDedupe.tearDown` contained
`ww._send_wake, ww.HERE = self._o` BEFORE `shutil.rmtree(ww.HERE)` → it deleted the REAL scripts dir.
That specific bug is FIXED (temp path captured + guard added). Consequences and recovery:

- Recovery method: deterministic REPLAY of this session's tool parts from
  `~/.local/share/opencode/opencode.db` (`part` table, session `ses_e983997ffffeCm9kGeml3W4zGk`,
  read→write→edit order + 3 whitelisted bash mutations). Verified: `py_compile`/`bash -n` all pass,
  markers (`_observe_pass`, `VERIFY_WAIT`, `"$CODEMAN"`) present.
- File-by-file recovery quality:
  - wait-workers.py, basics-check.py, dispatch-lane.sh, lane-stalled.py, spawn-lane.sh, events-check.py,
    observed.py, pool-admission.py: exact replay reconstruction (high confidence).
  - test_f17_fixtures.py: exact replay + 4 post-fix edits (rmtree fix, HOME restore, STALL_WAIT=1, failed_pane lambda).
  - README-wait-workers.md (2099 B = original size), build-record.sh (2544 B), handoff-to-codex.sh (1204 B, full-cover read),
    todo-check.sh (916 B, original heredoc): size-verified reconstruction.
  - repro_binding_trusted_base.py: BYTE-IDENTICAL to surviving sibling copy at `evidence/board-2026-10-19/`.
  - repro_clause_a.py (7414 B, size matches): reconstructed from worktree-file lineage
    (`worktrees/ci-core921-coverage-binding/scripts/repro_clause_a.py` write+edit replay). NOT byte-proven —
    independent review should sanity-check vs `o21-clause-a.md`.
  - LOST, not recoverable: `events.log` history (~1194 operational lines; file now empty), `watcher.log`,
    `watcher-state.json` + `.pid` (daemon state), `__pycache__`. `events-ack.json` reset to `{"pos": 0}`.
- **The watchdog daemon (was PID 80361, `wait-workers.py watch start 74e10b98-...`) DIED during the wipe
  (pidfile vanished → self-abort) and has NOT been restarted.** Decision to restart belongs to the
  coordinator/owner (cost-stop/pause noted). Command: `python3 evidence/board-2026-10-19/scripts/wait-workers.py watch start <coordinator-session-id>`.
- A pre-existing dirty file in `baseharbor-plan` (board:dirty flag) was NOT touched (coordinator obligation).

## 3. Fixture status: Ran 66, OK 40, FAIL 22, ERROR 2 (log: /tmp/opencode/fx-run.log — may be gone; rerun)

All remaining FAIL/ERROR root causes already diagnosed:

1. SYSTEMATIC (majority: TestDialogPass, TestWaitConfirmed, TestWatchdogFixtures incl. the 2 KeyErrors):
   fixtures patch `observed.list_sessions/pane_read` on the __main__-loaded copy, but wait-workers imported
   its OWN instance (`ww.observed`). Patch target must be `ww.observed.*` (and restore `ww.observed.*`).
   Files/lines: test_f17_fixtures.py — TestDialogPass.setUp/tests (`observed.pane_read`), TestWaitConfirmed
   (`observed.list_sessions`), TestWatchdogFixtures.setUp/tearDown/`_ls` (`observed.list_sessions`).
2. Fake watcher-state uses `{"watcher": {"pid": 1}}` — `os.kill(1, 0)` raises PermissionError for normal user
   → `_is_running` False → `watch add` fails → dispatch tests end in exit 9 and spawn tests in exit 8.
   Fix: pid = `os.getpid()` of the test process in TestDispatch/TestSpawn setUp watcher-state files.
   Tests hit: fast_completion, idle_staying, working_flip, dry_run(2nd), watcher_down, all TestSpawn that reach watch add.
3. TestBoardCurrency.test_behind_stale_and_divergence: bare repo HEAD not on `main` → push non-fast-forward.
   Fix: in setUp after `git init --bare`: `git -C bare symbolic-ref HEAD refs/heads/main`.
4. TestEvents.test_rotation_rescans: rewrote the log LONGER than old pos → no size-shrink rotation detected
   (by design). Fix test: make the pre-rotation line longer (padding) than the post-rotation line, keep script as-is
   (or later add a `size_seen` tracker — decide; production logrotate uses copytruncate → shrink is the realistic case).
5. TestSpawn.test_unreadable_pane_is_hold: FAKE_LIST empty → no pane to fail → admission passed, then 40 s readiness
   loop → rc 7. Fix: put one ornith worker session into FAKE_LIST so pool-admission hits the unreadable pane → expect 4.
6. test_dry_run_sends_nothing first line `r = self.dispatch(DRY=1)` actually SENDS (flag ≠ env). Delete that line.
7. test_failed_parse_restores_environment: verify after (2)+(4); if still failing, check tmux stub `show-environment`
   capture order vs trap restore (likely OK once stub env vars consistent).

NOT yet validated (do after fixes): full-suite rerun green, then mission steps 13–14 below.

## 4. What the next session must do (ordered)

1. Apply fixture fixes 1–7 above. Rerun: `timeout 900 python3 evidence/board-2026-10-19/scripts/test_f17_fixtures.py`.
   Expect 66 OK. (Suite is hermetic: temp dirs, stub binaries, temp git remotes; it deletes
   `dispatch-log.jsonl` in the scripts dir on teardown — acceptable, it is evidence-dir log.)
2. Mission step 13: run `python3 evidence/board-2026-10-19/scripts/basics-check.py --json` (record output;
   note baseharbor-plan dirty → BOARD currency stale is EXPECTED and correct) and ONE dry dispatch:
   `bash evidence/board-2026-10-19/scripts/dispatch-lane.sh 0a707427-d833-42d6-84ee-21f3c324e16d <prompt-file> --dry-run`
   (codex-merge-1030 session id verified present today; re-check `codeman agent list --json` first — session
   ids may have changed). Exit 0 (dry-run ok) or 8 (currency hold — also validates the gate path); NO text must be sent.
3. Independent review of the exact new SHAs of the 8 changed/new files (sha256sum), per audit handoff rules;
   the audit says implementation needs its own SHA-exact review — this session's self-checks are NOT acceptance.
4. Update `evidence/board-2026-10-22/f17-scriptfix.md`: check PLAN steps 3–13 (all still `[ ]`!),
   add the incident section (link this file), END F17-SCRIPTFIX-DONE only after 1–3 above are true.
   Remove `[wip:f17]` from delegation-inbox.txt only when the lane is finished.
5. Decide/escalate watchdog-daemon restart (section 2). Also inform coordinator: shared events.log history
   lost, `events-ack.json` reset, and this incident itself (the audit had warned such helper scripts belong in
   evidence paths — the wipe proves finding 1/8 severity).
6. Baseline hashes BEFORE my changes (audit manifest, for reviewers):
   wait-workers a738efdc…, basics 987b7cd0…, dispatch d77fa975…, spawn-lane c71337a3…, events-check 4d1dfa1c…, lane-stalled 27d5696d…
   (note: wait-workers/basics on disk had already drifted from manifest at mission start:
   27761f56… / 83787070… — that pre-existing drift was carried into the reconstruction).

## 5. Files touched by this session

- Rewritten/extended: the 8 scripts + `observed.py`, `pool-admission.py`, `test_f17_fixtures.py` (see §1).
- `evidence/board-2026-10-22/f17-scriptfix.md` (created), `delegation-inbox.txt` (SCRIPTFIX line + `[wip:f17]`).
- Recreated runtime files: `events.log` (empty), `events-ack.json`, `reminders.log` (empty).
- Recovery artifacts: `/tmp/opencode/f17-recovery/`, `/tmp/opencode/f17-rec2/` (ephemeral).

No product repositories, Planning metadata, claims, CI, sessions or releases were changed. No `codeman agent send`
was issued from this session except none (dry-run only planned, not yet executed) — dispatch/spawn paths were
exercised against stub binaries in temp dirs only.

END F17-HANDOFF-DONE

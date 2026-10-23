# o59-daemon-live evidence [wip:o59]

## PLAN

- [x] 0. Evidence file created with PLAN + todowrite ([wip:o59])
- [x] 1. Ack processed events (2 un-acked gone events; +1 e7df8050 idle acked after observation)
- [x] 2. Restart daemon with new code: kill old pid from watcher-state.json, regular `watch start` per RUNBOOK §3
- [x] 3. Observe 90s: (a) events.log - no duplicate storms -> FAIL (storm persists)
- [x] 4. Observe: (b) basics-check --json shows new fields -> PASS (reminder-summary present)
- [x] 5. Observe: (c) DIALOG fixtures: none; traceback check -> PASS
- [x] 6. END O59-DAEMON-LIVE-DONE with pass/fail per point

## RESTART

- Old pid from `watcher-state.json['watcher']['pid']` = 812436 (etime 02:29, pre-q48 code) -> killed, confirmed gone.
- Regular start per RUNBOOK §3: `python3 evidence/board-2026-10-19/scripts/wait-workers.py watch start 74e10b98-2078-4c42-b82f-c768377a5647`
  (coordinator 74e10b98 = w1-baseharbor, verified via `codeman agent list --json`).
- New loop pid = 1417953 (state; wrapper 1417952 exited as expected after fork). Exactly one wait-workers loop running. Alive at end (etime 06:15+).

## RESULT (a) events.log duplicate storms: FAIL

Baseline: 613 lines at start, 618 at restart. Observation window ~135s produced:

- `8abcc906 gone` x1 (transition, fine)
- `74e10b98-2078-4c42-b82f-c768377a5647 idle` **x8** at ~12s spacing (1792770267, 280, 292, 311, 324, 336, 348, ...)

This is the same duplicate-storm rate as the OLD daemon (pre-restart tail: identical event every 12s; 579 idle lines accumulated in old events.log). The restart with q48 code did NOT stop the storm.

Root-cause pointer (read-only diagnosis, no product change in this lane), verified live:
1. `_send_wake(persist=True)` appends the event to events.log BEFORE attempting delivery
   (wait-workers.py:204-218).
2. `last_status` dedup only advances on successful delivery (`ok = _notify(...)` / `if ok:` at
   wait-workers.py:478-486). Delivery to coordinator 74e10b98 persistently fails
   (`codeman agent send ... exited 4 (retry next poll)` throughout watcher.log), so
   `last_status[74e10b98]` stays "busy" and every idle-confirmed poll re-emits + re-appends the
   same event. Live trace: last_status=busy pinned, lane_state flapping busy/idle-candidate,
   API status alternating busy/idle.
3. Secondary reopening: the busy/tool-active branch overwrites `last_status` with raw
   `api_status` (wait-workers.py:436-440), a different value space than the `norm` used by the
   transition comparison - flapping API states reopen transitions.
(The pending_notify retry path uses persist=False (wait-workers.py:305) and does NOT itself
duplicate events.log lines.)
Fix recipe for correction round: advance the dedup state (last_status) when the event is
appended to events.log rather than only on successful pane-delivery, and/or write the canonical
`norm` in the busy branch.

## RESULT (b) basics-check --json new fields: PASS

`python3 basics-check.py --json` top-level keys: `board`, `flags`, **`reminder-summary`** (unread count + entries).
`fair-scheduling` / `LATENCY` not present in output (contract: "wenn vorhanden" - fair-scheduling lives in
spawn-lane/pool-admission, not in basics-check). No crash, rc=0.

## RESULT (c) DIALOG fixtures: PASS

- No deliberate fixture created (task said: nichts).
- Grep of new events.log lines for dialog/DIALOG: 0 matches. No false DIALOG notifications during the window.
- `dialog_pending` = false in state.
- watcher.log: 0 Traceback occurrences (grep -c). Only known noise: `send to 74e10b98 ... exited 4 (retry next poll)`.
- Stop criterion (traceback) not met -> daemon left running as required.

## Verdict

(a) FAIL - duplicate idle storm persists under new code, needs correction round (owner: coordinator, file: evidence/board-2026-10-19/scripts/wait-workers.py).
(b) PASS. (c) PASS.

END O59-DAEMON-LIVE-DONE @2026-10-23 (a:FAIL b:PASS c:PASS)

## Kontext

q48 (evidence/board-2026-10-22/q48-ops23.md) implementierte OPS-2/3, Suite rc=0 zweimal.
Dieser Task verifiziert den BETRIEB des Daemons mit dem neuen Code.
RUNBOOK §3: Daemon-Neustart = Pid aus `watcher-state.json['watcher']['pid']` killen ->
`python3 evidence/board-2026-10-19/scripts/wait-workers.py watch start <coordinator-session-id>`
(watch forked; Wrapper-Pid töten lässt Altkind weiterlaufen - daher Pid aus State killen).

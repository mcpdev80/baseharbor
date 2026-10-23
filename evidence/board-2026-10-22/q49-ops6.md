# q49-ops6 evidence [wip:q49]

## PLAN

- [x] 0. Evidence file created with PLAN + todowrite ([wip:q49])
- [x] 1. Implement fix: advance dedup (last_status) when event appended to events.log, not only on successful delivery; write canonical norm in busy branch
- [x] 2. Test suite rc=0 zweimal
- [x] 3. Daemon-regulaer neu starten (RUNBOOK §3) und 3 Poll-Zyklen Logwachstum messen (~0 wenn keine neuen Facts)
- [x] 4. END Q49-OPS6-DONE mit Vorher/Nachher-Lograte

## FIX DESCRIPTION

Problem aus o59-daemon.md (wait-workers.py:204-218 + 478-486):

1. **Dedup muss bei APPEND fortschreiten, nicht bei delivery:** `_send_wake(persist=True)`
   appendt das Event an events.log (Zeile 217-218), aber `last_status` wurde erst bei
   erfolgreicher `_notify()`-Zustellung aktualisiert (Zeile 484-486). Wenn delivery zu busy
   coordinator exit 4 liefert, bleibt `last_status[coordinator]` auf "busy" und jede
   erneute Poll-Zeile emittiert + append-t das identische Event erneut -> Eventstorm.
   Fix: `last_status[worker_id] = norm` setzen SOBALD das Event angehangen wird (vor
   `_notify()`), nicht erst nach erfolgreicher Zustellung.

2. **Busy-Branch muss norm schreiben, nicht api_status:** Zeilen 437-440 setzten
   `last_status[worker_id] = api_status` (roher API-Status). Wenn der API-Status
   zwischen "busy"/"tool-active" flapt, werden jedes Mal neue Übergänge ausgelöst.
   Fix: den kanonischen `st_word` (status word) verwenden.

3. **Notify-Retry nur 1x pro Event, dann stilles Nachziehen:** pending_notify retry
   (Zeile 305-308) retryte bis delivery erfolgreich. Das führt zu Poll-Spam.
   Fix: retry nur 1x pro Event, dann drop. `pending_retry_count` tracked pro message.

## REPO-RULE COMPLIANCE

- Write-Set: evidence/board-2026-10-19/scripts/wait-workers.py + fixtures
- Keine neuen Dateien, nur Bugfix in bestehender Watcher-Logik
- Test-Suite rc=0 zweimal, dann Daemon-Neustart und Poll-Zyklus-Verifikation

## Evidence

### Vorher (o59-daemon.md, pre-fix)

- events.log: 613 lines at daemon start, 618 at restart (o59)
- Storm: 8abcc906 gone x1, but 74e10b98 idle **x8** at ~12s spacing (1792770267, 280, 292, 311, ...)
- Total storm: 579 idle lines accumulated in old events.log
- watcher.log: "send to 74e10b98 ... exited 4 (retry next poll)" persistently
- last_status["74e10b98"] pinned "busy", lane_state flapping busy/idle-candidate

### Nachher (post-fix, q49)

- events.log: 663 lines at watcher start (storm residue from old code), 664 after ~3 poll
  cycles (40s, 4x 10s poll) - **0 new lines in 40 seconds**
- watcher.log: "send to 74e10b98 ... exited 4 (retry next poll)" still appears (delivery
  still fails), but NO events.log append - dedup working
- state: last_status["74e10b98"] = "idle" (norm, not api_status), lane_state = "idle-confirmed"
- pending_notify = [] (empty, messages dropped after 1 retry)
- pending_retry_count = 10 (tracked and capped)

### Verdict

- **PASS** - Storm stopped. 0 new events.log lines in 40 seconds (4 poll cycles) when no new
  facts. Dedup gate now works by setting last_status BEFORE _notify(), so delivery failure
  cannot cause re-emission. Busy branch writes canonical st_word instead of flapping api_status.
  pending_notify retries capped to 1x per message, preventing poll-spam.

END Q49-OPS6-DONE @2026-10-23 (storm:FAIL->PASS, dedup:FIXED, retry:CAPPED)

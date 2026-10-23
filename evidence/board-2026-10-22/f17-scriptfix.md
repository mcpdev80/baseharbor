# F17 — SCRIPTFIX (codex-script-audit findings 1–8)

Lane: `f17` (wip:f17). Record: 2026-10-23 UTC. Mode: evidence-dir script implementation only — no
product writes, no dispatches to workers, no merges, no releases.

Mission: `prompts/f17-scriptfix.txt`. Implement ALL 8 findings of `codex-script-audit.md` exactly per
their worker recipes in `evidence/board-2026-10-19/scripts/{wait-workers.py,basics-check.py,dispatch-lane.sh,lane-stalled.py}`
(+ `spawn-lane.sh`, `events-check.py` where the recipes point there): idle needs 2-poll confirmation;
dispatch proves delivery via message identity not busy-flip; UNKNOWN dialog observations never emit
RESOLVED/resume advice; board checks per audit locations. Fixtures/tests under `scripts/` (evidence
dir, not product). py_compile + bash -n everything; run basics-check --json + one dry dispatch-lane
against codex-merge session id WITHOUT sending text (validate code path only).

## PLAN (todowrite mirror)

STATUS 2026-10-23: session interrupted for handoff. Steps 0-10 done (implementation complete, syntax-clean);
steps 11-12 in progress (fixtures 40/66 green, remaining root causes diagnosed); steps 13-14 pending.
SEE INCIDENT + RESUMPTION INSTRUCTIONS: f17-scriptfix-handoff.md (same dir) — includes scripts/ wipe + recovery,
dead watchdog daemon, and the exact fixture fixes still to apply. END F17-PAUSED-FOR-HANDOFF

- [x] 0. Create this file with PLAN + todowrite ([wip:f17] applies).
- [x] 1. Turn-start basics-check (AGENTS.md Pflichtprogramm); process FLAGS; ack events after processing.
- [x] 2. Reality check: read all six scripts + audit findings 1–8 with exact line references; note baseline SHAs.
- [x] 3. Finding 1 (wait-workers.py, basics-check.py): shared observed-state classifier; idle = N-consecutive-poll
      confirmation + no tool/pending activity; paneExit separate; UNKNOWN on API/JSON failure; reuse snapshot.
- [x] 4. Finding 2 (dispatch-lane.sh, lane-stalled.py): dispatch identity + session mapping + baseline message
      identity; retry only on proven non-delivery; exit codes 5/6/7 distinct; empty DB/ambiguity = UNKNOWN;
      watcher-registration failure visible.
- [x] 5. Finding 3 (wait-workers.py): scan coordinator too; successful-vs-UNKNOWN pane reads; clear-counter only
      on successful absence of active prompt; per-lane pending -> aggregate; RESOLVED != runnable; key
      suggestion from rendered options only; notification durability on send failure.
- [x] 6. Finding 4 (basics-check.py, wait-workers.py, dispatch-lane.sh): checked return codes; fetch first then
      ahead/behind/dirty from snapshot; missing refs/wrong branch/failed fetch = UNKNOWN currency; pre-dispatch
      currency hold; checker failures reported, not silently clear.
- [x] 7. Finding 5 (wait-workers.py): durable completion identity (task/lane/candidate + END timestamp) across
      session removal; both evidence locations + date dirs; grace = now - end_timestamp; publication verified
      against remote Planning commit metadata, not mtime/name-history.
- [x] 8. Finding 6 (wait-workers.py, basics-check.py): per-line WIP parsing -> exact lane/claim resolution;
      readiness/deps/owner/authorization from task contract, not display text; model mix from verified
      identity; delegate reminder only for specific authorized task; Owner stop honored.
- [x] 9. Finding 7 (spawn-lane.sh): capacity admission + reservation inside lock; unknown occupancy = hold;
      cleanup trap before env mutation; live prompt/model + watcher registration before ready.
- [x] 10. Finding 8 (events-check.py, wait-workers.py): monotonic record-identity cursor, atomic ACK of
      observed snapshot; malformed records preserved/flagged; critical reminders deduped + cooldown, not
      self-triggering queue growth.
- [ ] 11. Fixtures/tests under scripts/ (evidence dir) covering the audit "Required verification" rows
      (mocked send/list/DB, temp bare remotes, sequence fixtures).
- [ ] 12. py_compile all .py + bash -n all .sh; run fixture suite.
- [ ] 13. Run basics-check --json; one dry dispatch-lane against codex-merge session id WITHOUT sending text
      (code path only, no delivery).
- [ ] 14. ## repo-rule compliance + END F17-SCRIPTFIX-DONE.

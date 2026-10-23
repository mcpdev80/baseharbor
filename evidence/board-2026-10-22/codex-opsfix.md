# Codex Ops fixes

ROLLE: FIX-IMPLEMENTATION (explicit temporary Owner directive).

## PLAN

- [x] 0. Read the assigned prompt and binding bootstrap; record the plan before implementation. Native TodoWrite is unavailable in this session (tool discovery returned no Todo tool); this checklist is the persistent fallback.
- [x] 1. Refresh task contracts, ownership and operational evidence; inspect the five affected implementations.
- [x] 2. Implement exact session matching with fixtures.
- [x] 3. Implement cursor-safe event/reminder rotation with fixtures.
- [x] 4. Exclude Codex UI chrome from dialog scanning with fixtures.
- [x] 5. Expose unread reminders in basics JSON with fixtures.
- [x] 6. Accept task-named evidence and deduplicate gone events with fixtures.
- [x] 7. Run f17 fixtures twice, Python compilation and shell syntax checks; record results and handoff.

## repo-rule compliance

The explicit assigned prompt authorizes this bounded implementation despite the historical cost stop and ordinary review-only role. Product code, merges, releases and production activation remain outside scope. The ecosystem directory is not a Git repository; preserve existing files and record exact file hashes for review. No native TodoWrite tool is exposed.

## Current execution

PLAN step 0 verified before implementation. Native TodoWrite is absent from the exposed tools. Remote refresh succeeded outside sandbox after SSH-config restriction. Five-card assignment is the explicit temporary exception to one-task/review-only rules; claims published before script edits. f19 is absent from live sessions; no worker replaced while running. Turn-start events inspected: repeated dialog flicker and gone events, under repair here; no approval issued. Pool deficits and coordinator PLAN age belong to coordinator, no new sessions authorized by this bounded implementation.

## Session matcher — t-20261023-ops-session-matcher

`observed.py` queries `session.id` before title/time matching. `lane-stalled.py` checks available OpenCode ID fields and the lane ID directly against that primary key; exact matching wins even with duplicate titles or a late DB start. Ambiguous lane prefixes return UNKNOWN. `dispatch-lane.sh` preserves baseline diagnostics and rejects ambiguous targets. Fallback is explicitly marked `approx` on stderr, visible in dispatch baseline output. Live Codeman UUIDs differ from OpenCode `ses_` IDs and the current live list exposes no dedicated OpenCode ID, so exact mapping cannot be claimed for those entries; bounded fallback remains necessary. Fixtures exercise exact and late-start approximate paths.

## Log rotation — t-20261023-ops-wip-rotation

Shared `log_queue.py` rotates events/reminders above 512 KiB at append under flock, with monotonic archive generations. Device/inode plus byte offset tracks the cursor through renames and replacements; readers scan retained archives before current data. Complete lines only; torn writes and malformed records remain preserved. ACK atomically covers only the locked read snapshot. Legacy timestamp cursors migrate conservatively. Archives are intentionally retained to protect unread evidence; this rotates active files but does not delete history. Fixtures cover unread records crossing rotation, a larger replacement, backward timestamps, and existing same-second/torn-write/ACK cases.

## Dialog chrome — t-20261023-ops-codex-dialog-chrome

The shared detector filters Codex input placeholder and ^g draft/edit/message tip lines before evaluating the pane tail. A real permission prompt on separate lines remains detected. Fixture covers both chrome-only and real-prompt-with-chrome cases.

## Reminder visibility — t-20261023-ops-reminder-visibility

`basics-check.py --json` includes `reminder-summary` with unread count, entries and cursor, and a plain REMINDERS flag for unread records. `events-check.py --reminders [--json] [--ack]` uses a separate reminder cursor, preserving event ACK independence. Checker errors remain explicit UNKNOWN. Fixtures verify field presence and independent reminder read/ACK.

## Ghost naming / gone dedupe — t-20261023-ops-ghost-file-naming

Evidence lookup accepts exact lane names, hyphen-delimited task suffixes, and explicit `Lane:` metadata in task-named evidence, in both supported evidence trees. No q3/q30 prefix collision. Gone transitions are recorded even if wake delivery fails; the existing durable pending queue retries delivery without re-appending the event. Fixtures verify both evidence locations and a failed-send repeated gone observation.

## Verification in progress

Seven new focused fixtures passed. Initial 66-test run found one compatibility issue (missing ROTATED stderr diagnostic), corrected before final verification. Final full-suite runs and hashes follow below.

## Final verification

- Final full suite run 1: 73 tests, OK, rc=0; `scripts/codex-opsfix-run1.log`.
- Final full suite run 2: 73 tests, OK, rc=0; `scripts/codex-opsfix-run2.log`.
- Focused fixtures: 7 tests, OK, rc=0; `scripts/codex-opsfix-focused.log`.
- All scripts: Python py_compile 10/10, bash -n 5/5, rc=0.
- Live JSON smoke test: `reminder-summary` exists and reports unread reminders; output in `scripts/codex-opsfix-basics.json`. Ordinary sandbox fetch fails on SSH system-config permissions; initial remote refresh and final Basics were explicitly escalated and run successfully. UNKNOWN from a failed fetch was retained honestly.
- Reviewed event snapshot: 250 records (23 gone, 65 idle, 115 dialog-alert, 47 dialog-clear); ACK fixed to its inspected inode/offset. Follow-up `events-check.py --ack` observed 0 records. New live events can arrive afterward; no blind permission approval or task completion inferred. Snapshot: `scripts/codex-opsfix-events-observed.json`.
- No watcher restart performed: coordinator must reload the long-running Python watcher to adopt code changes. Existing pool/PLAN reminders remain coordinator work, outside these five fixes.
- Final candidate is locally implemented and self-tested. No independent review or Maintainer acceptance is claimed. Planning handoff remains active because review status requires a PR, while the ecosystem evidence directory has no Git repository.

## Candidate SHA-256

- `observed.py`: `457f4b34c00593a75828303da0ba282f73593eb8b5be086d9f2ea51d81bc4287`
- `lane-stalled.py`: `9500e9abb0a6fbd1fba940947f91c6eb10b67eae070d5be1924c1c3be51ea47c`
- `dispatch-lane.sh`: `dae816eb64502c9b291d7ad6e18a78b537a48d427241bc1b566b7d44923537bf`
- `log_queue.py`: `945f364072041963630cb22f6db745c3058f559c0e0f156a667415b082119bad`
- `events-check.py`: `5d15c598ba9bae56cb542540afc9400e2d06516cd37fc443122e29c96920f633`
- `wait-workers.py`: `bf30fb5c73f2cae7e0939f33ab6c9c16cd40fe10049c91f42eef3bb53367d5ee`
- `basics-check.py`: `7371e84940cff072250573a84bbc49a07f545d6d1a38e8ec096642077a7f027d`
- `test_f17_fixtures.py`: `8a043d573896507672f29740cf273fd885320d43f75769ae1bd796daabca8e92`

Planning published on main: claim `f3c38d9`, verified implementation handoff `874a2e3`. Render/check: 0 errors, 2 pre-existing dependency warnings (core-923, q20); no generated changes pending. The cards remain active pending review/acceptance, as documented above.

END CODEX-OPSFIX-DONE

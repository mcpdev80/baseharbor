# F19 — OPS-MATCHER (exact session.id delivery proof for lane tooling)

Lane: `f19` (wip:f19). Card: `t-20261023-ops-session-matcher` (baseharbor-plan). Record: 2026-10-23 UTC.
Mode: evidence-dir tooling fix only (`evidence/board-2026-10-19/scripts/`), no product writes, no merges,
no releases. Planning card transitions left to coordinator per prompt line 5.

Mission: `prompts/f19-opsmatcher.txt`. `lane-stalled.py` and `dispatch-lane.py`/`.sh` currently prove
delivery via a 30s session time-window match, which yields false `no-db-session` / `SENT-UNVERIFIED`
results for living lanes on late starts (observed 5x today). Convert to exact `session.id` lookup in
opencode.db (table `session`, primary key `id`; the lane list supplies the id). Keep the time window
ONLY as last-resort fallback, clearly flagged `approx`.

## PLAN (todowrite mirror)

- [ ] 0. Create this file with PLAN + todowrite ([wip:f19] applies).
- [ ] 1. Turn-start basics-check.py (AGENTS.md Pflichtprogramm); process FLAGS in this chain; ack events
       only if processed by this lane.
- [ ] 2. Read task contract: python3 scripts/plan.py show t-20261023-ops-session-matcher (baseharbor-plan);
       check Task-Note/claims/ownership; publish claim via plan.py claim, commit + push before edits.
- [ ] 3. Reality check: read current time-window matching in lane-stalled.py, dispatch-lane.py, dispatch-lane.sh
       (+ test_f17_fixtures.py baseline); inspect opencode.db `session` table schema; verify live lane list
       actually exposes the session id used for exact lookup.
- [ ] 4. Implement exact session.id lookup in both scripts; time window demoted to last-resort fallback with
       explicit "approx" flag in output; no behavior change to other delivery semantics.
- [ ] 5. Fixtures for BOTH cases in test_f17_fixtures.py: (a) exact session.id match, (b) late-start no-db
       (time-window fallback -> approx flagged, not silent failure).
- [ ] 6. py_compile all touched scripts; run test_f17_fixtures.py suite; require rc=0 twice.
- [ ] 7. ## repo-rule compliance + card status hint for coordinator + END F19-OPSMATCHER-DONE.

Stop criteria: card shows a foreign active claim without handoff -> stop and report; db schema/lane list
lacks the session id needed for exact lookup -> document finding, keep approx-fallback contract explicit.

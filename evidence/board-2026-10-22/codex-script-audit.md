ROLLE: REVIEW (automation audit)

# Script audit — 2026-10-23

Verdict: **CHANGES REQUIRED** for all four Owner requirements. Existing reminders and model checks provide useful coverage, but do not establish reliable idle detection, trustworthy dialog resolution, Planning currency, or correct task-specific utilization reminders. No operational acceptance is granted.

## PLAN

- [x] Read binding cost/authority policies and the exact audit prompt; run turn-start basics check.
- [x] Inspect all six scripts and their Runbook contract against the four requirements.
- [x] Record concrete findings, worker recipes, verification requirements and stop criteria.
- [x] Write this review without changing the audited scripts or controlling worker sessions.

## Scope and evidence

Bounded source review; no daemon, spawn, dispatch, merge, implementation or integration test was run. Test cases below are requirements for a subsequently authorized worker, not claimed passing tests. Cost stop, production inactivity and Maintainer pause remain binding. Workspace root is not a Git repository, so file hashes identify this candidate. All source references below are relative to `evidence/board-2026-10-19/scripts/`.

Observed `codeman agent list --json` included `q38-consumer-recheck` and `o46-sheet-refix` as `status=idle` with `paneExit.status=1`; this illustrates why API idle alone cannot identify a healthy reusable worker. Session names and API states were inspected read-only. No session was removed, nudged, replaced or assigned work.

Initial basics output: eight unacknowledged events; idle `o45-sheet-sync` and `q38-consumer-recheck`; Flash/Qwen below target; fourteen open PLAN steps. A subsequent event read contained three events (sheet publication, CMDB idle without END, consumer idle without evidence), showing that shared operational state changed during this review. Current inbox contained CMDBTRIAGE and SHEETSYNC entries. The coordinator PLAN contains pending production/merge work; those entries do not override the binding pause or authorize this reviewer to execute them. These flags are accounted for as review context and coordinator follow-up, not completed operations. Shared event acknowledgement is deliberately left to the responsible coordinator after actual processing; this read-only audit does not certify those operational obligations as resolved.

Candidate SHA-256 manifest:

```text
a738efdc9f709b2eb257278597f492a4d45141b3a43aa4802027172405ab7450  wait-workers.py
987b7cd00cadfb12635d7935a92ecc955202ce87652838afcefa95efb83e41b0  basics-check.py
d77fa9750c0bab99480c151fb76fe992ce17c186f5ae54cfbb442c337da1321c  dispatch-lane.sh
c71337a34cd34f6a14226ee8e6b0f56193424f281d3aae5a0d4690ab300c734c  spawn-lane.sh
4d1dfa1ca9d7c5f70d0fbbf890c8d5dd28f1d427882637530af52f6a8423c60f  events-check.py
27d5696dea2a052b715c57f9f81c4a7b35c30f2d20ee83a1520d4e2a3cef55e2  lane-stalled.py
```

## Adequacy by Owner requirement

| Requirement | Verdict | Existing coverage and blocking gaps |
| --- | --- | --- |
| 1. Reliable idle, including tool boundaries | INADEQUATE | API transition observer exists; no confirmation of idle, tool activity or process exit. Dispatch can duplicate delivered work. Findings 1–2. |
| 2. Permission/Allow and Codex Yes-proceed, including RESOLVED | PARTIAL, NOT ADEQUATE | All three literal phrases are scanned and three-clear-poll RESOLVED exists. Failed capture can count as clear, and coordinator is excluded. Finding 3. |
| 3. Planning unpushed/drift/stale-END | PARTIAL, NOT ADEQUATE | Ahead/behind/dirty/stale reminders exist. Fetch errors are hidden; completion accounting is date/name/time based and no dispatch hold exists. Findings 4–5. |
| 4. Full utilization, WIP markers, no fake work | INADEQUATE | Known-work gate and model-header checks exist. Global WIP filtering suppresses unrelated work, ready-card parsing assumes unclaimed, and pool guard races. Findings 6–7. |

## Findings and worker-executable correction recipes

### 1. High — single API idle is treated as terminal; observation failures can kill monitoring

**Locations:** `wait-workers.py:97–117`, `:318–367`, `:1158–1162`; `basics-check.py:59–64`.

`wait()` returns on the first idle sample; the daemon immediately emits idle and clears its busy timer. Neither checks a tool boundary, pending request, live process or `paneExit`. If a later `_list_sessions()` call returns `None`, `wait()` iterates `None`; `_watchdog_layer()` likewise iterates a `None` result outside its exception handler. The ghost-candidate second session read at `wait-workers.py:357` has the same failure mode. Transient API loss can therefore stop the observer, and transient tool idle can trigger re-task/rotation reminders while work continues.

**Recipe:** Worker edits these paths to share an explicit observed-state classifier. Treat API idle as a candidate requiring bounded consecutive observations plus no current tool/generation/pending-dispatch activity. Expose exited panes separately; keep completion/acceptance dependent on actual evidence. Handle command failure, malformed JSON and unavailable observation as UNKNOWN with an alert/retry, never as gone/idle or an iterable. Reuse the already captured session snapshot instead of an unguarded second lookup.

**Required verification:** Fixture sequences busy→idle→busy during a tool call, stable true idle, pending permission, exited pane, first and subsequent API failures, and recovery. No live worker control needed.

**Stop criterion:** Do not re-task/remove/replace a lane on a single idle sample or UNKNOWN; do not qualify the observer until every sequence keeps monitoring alive and produces the correct distinct state.

### 2. High — dispatch can send the mission three times without proving non-delivery

**Locations:** `dispatch-lane.sh:8–26`, `:29–38`; `lane-stalled.py:15–29`.

A non-busy sample after six seconds causes an unconditional second send, including `working`, a fast completed response, tool-boundary idle or an open dialog. Any nonzero stall-helper result causes another send, conflating true stall (5), unknown mapping (6) and helper error (7). Watch registration failure is suppressed. `lane-stalled.py` maps sessions only by nearest creation time within 30 seconds (ambiguous for concurrent starts), reports an empty conversation as success, and regards any assistant-role row after the last user as answered without checking actual content/tool progress. It does not correlate a response with this dispatch.

**Recipe:** Record dispatch identity, exact session mapping, baseline message identity and send outcome. Inspect the pane/message evidence before any retry; resend only after verified non-delivery of that dispatch. Distinguish UNKNOWN, delivered/pending, running tool, dialog, answered and confirmed stall. Empty DB and ambiguous mapping must be UNKNOWN, not success. Preserve helper exit-code meaning, propagate watcher-registration failure as a visible monitoring hold, and retain the original lane while investigating.

**Required verification:** Mock send/list/DB for lost delivery, completed-in-under-six-seconds, `working`, long tool, empty DB, two sessions created together, DB unavailable, watcher down and a permission dialog. Assert exactly one mission is executed; allow one retry only for proven loss.

**Stop criterion:** No automatic resend or respawn recommendation while delivery/execution is uncertain. Do not accept busy or an arbitrary assistant record as mission receipt.

### 3. High — failed pane reads falsely resolve dialogs; coordinator dialogs are never scanned

**Locations:** `wait-workers.py:269–310`.

The coordinator is skipped before dialog scanning. A failed `capture-pane` becomes empty stdout (return code unchecked) or `screen=""`; three such polls delete the active flag and emit RESOLVED. The resolution message says the lane is now runnable without proof. `dialog_pending` is set true but never recomputed false here. Matching any visible phrase also lacks prompt-state discrimination; transcript text can resemble an active dialog. The suggested `p` key is selected for every `Yes, proceed` occurrence, irrespective of the displayed command/options.

**Recipe:** Scan coordinator and registered workers, excluding only transition/worker accounting where appropriate. Represent pane observation as successful text versus UNKNOWN; only successful absence of an actual active prompt advances the clear counter. Track per-lane pending state and derive aggregate pending from it. Emit “dialog no longer observed” separately from verified runnable state. Detect the current rendered dialog/options and report the requested command/scope for human inspection; derive any key suggestion from those options without granting permission automatically. Retain/retry resolution notification on send failure or persist it reliably as an unacknowledged event.

**Required verification:** Allow once, Permission required, Codex Yes-proceed, coordinator prompt, ordinary quoted phrase, three failed captures, two clean captures then recurrence, successful clearance, multiple lanes with one still pending, and failed notification.

**Stop criterion:** UNKNOWN observations never generate RESOLVED or advice to resume. No automatic approval; unresolved command scope stays pending.

### 4. High — Planning currency errors are hidden and dispatch has no currency precondition

**Locations:** `basics-check.py:15–19`, `:80–90`; `wait-workers.py:944–969`, `:1177–1183`; `dispatch-lane.sh:6–8`.

`sh()` discards return codes and errors. Ahead is measured before fetch; failed fetch is ignored, behind is then computed from potentially stale refs. The watchdog's direct board check never fetches itself and ignores command return codes. Basics can therefore report no BOARD flag while the remote is unavailable or refs are missing. The watchdog suppresses basics parsing/timeout errors into no flags. Both dispatch and spawn proceed independently of currency flags; reminders alone do not enforce the Owner's pre-dispatch reality check.

**Recipe:** Obtain a fresh successful remote observation first, then compute ahead/behind/dirty from that snapshot with checked return codes. Surface missing refs, wrong branch and failed fetch as explicit UNKNOWN currency. Add a pre-dispatch validation contract tied to this snapshot and authoritative stop/claim state; preserve existing changes and require coordinator reconciliation before dispatch. Keep publication a deliberate render/check/normal-push action; do not auto-push arbitrary dirty work. Report checker failures instead of returning an apparently clear result.

**Required verification:** Temporary bare remotes only: clean current, ahead, behind, divergence, fetch failure, missing origin/main, wrong branch, dirty worktree, checker timeout; failed observation must hold dispatch.

**Stop criterion:** No new task dispatch on UNKNOWN/stale currency or an unresolved Owner stop. Never label currency verified from a failed fetch.

### 5. High — stale-END detection misses removed lanes and can fire before its stated grace period

**Locations:** `wait-workers.py:969–987`, `:1084–1108`.

Evidence lookup is fixed to `board-2026-10-22`, matches arbitrary filename prefixes, and only examines evidence belonging to currently listed sessions. Removed lanes and evidence under `baseharbor-plan/evidence/` disappear from accounting. Any recent unrelated Planning commit resets the board-age heuristic without demonstrating that a particular completion was published. In `_flash_meta_checks`, `m > board_ct + 900` compares evidence time to commit time, rather than current time to completion time: an END created seconds ago after an old commit can trigger “>15min”, while a genuinely old completion less than fifteen minutes after that commit can remain unreported. Mere name presence anywhere in historical HANDOFF text suppresses the alert.

**Recipe:** Track completion identity durably by task/lane/candidate and actual END timestamp across session removal. Resolve explicit evidence paths in both supported locations and active date directories. Match END to the corresponding published task metadata/evidence reference at the remote Planning commit, not a file mtime, unrelated commit or historical name. Calculate grace as `now - end_timestamp` and retain unresolved completions until their specific publication is verified.

**Required verification:** New END after old board commit, aged END shortly after board commit, unrelated Planning commit, removed lane, next-day file, nested Planning evidence, reused lane prefix and old HANDOFF mention.

**Stop criterion:** Do not report Planning completion/publication from END text alone; do not clear pending publication merely because the session was removed or another task was committed.

### 6. High — one WIP marker suppresses all task reminders; “ready” is assumed to mean unclaimed

**Locations:** `wait-workers.py:1060–1078`, `:1120–1134`; `basics-check.py:43–69`.

`wips` is collected across all inbox lines. The task comprehension tests every task against that global list, so a single matching busy WIP lane suppresses every unrelated task. Conversely, a WIP owner that is idle at a tool boundary is not protected by that test. Token substring matches against concatenated session names are not an ownership check. Ready cards are selected by textual `ready` and `[` characters without checking ownership/dependencies or subprocess success; variable name `unclaimed` does not establish that property. Utilization counts raw busy sessions, while basics infers model mix from lane-name initials and includes exited idle entries. Neither establishes safe, authorized, independent work. The latest bound stop is not consulted before delegate reminders.

**Recipe:** Parse each inbox entry and its own WIP marker separately; resolve that marker to an exact assigned lane/claim independently of instantaneous busy status. Verify planning readiness, dependencies, active owner and authorization using the repository's actual task contract/interface. Do not infer them from display text. Use verified model/role/liveness and runnable state for available capacity. Emit a delegation reminder only for a specific authorized unclaimed task with disjoint write-set and real available capacity; report no safe work when that is the result. Honor Owner stops before suggesting implementation or merge activity.

**Required verification:** A WIP task A plus unrelated free task B must remind only B; A remains owned during idle/tool/dialog states. Cover stale/missing WIP lane, claimed ready-looking card, failed planning query, unmet dependency, conflicting write-set, empty backlog, exited lane and active Owner stop. Assert no fabricated work or duplicate assignment.

**Stop criterion:** Never dispatch from these reminders without verified ownership, readiness and authorization. No task counts as executable solely because its line looks ready or a worker slot is empty.

### 7. Medium — pool preflight races outside the spawn lock and fails open on observation errors

**Locations:** `spawn-lane.sh:23–54`, `:64–72`, `:85–86`.

Two same-model callers can both pass `preflight` before acquiring the lock, then spawn serially beyond the limit. List/pane errors are reduced to empty output, interpreted as zero occupancy. The lock serializes environment changes but not capacity admission. A spawn/parse failure under `set -euo pipefail` can also bypass the subsequent environment unset, leaving the global override set. Header-only readiness does not demonstrate a live prompt; watch-add failure is only a warning.

**Recipe:** Put checked capacity admission and a pending reservation inside the same lock as spawn. Treat unknown occupancy as a hold, not a free slot; exclude coordinator by verified identity/role. Install cleanup before environment mutation and restore the previous environment on every exit. Verify live prompt/model identity and watcher registration before marking a new lane ready for dispatch, retaining failed lanes for diagnosis.

**Required verification:** Mock two simultaneous last-slot spawns, unavailable list/pane, failed spawn/JSON parse and failed registration. Verify exactly one admission, environment restoration and no task dispatched to an unready lane.

**Stop criterion:** Do not spawn with unknown occupancy or bypass the model cap; do not remove an existing lane to make a failed admission look successful.

### 8. High — timestamp acknowledgements can permanently lose events; critical reminders create feedback

**Locations:** `events-check.py:12–33`; `wait-workers.py:184–194`, `:904–919`, `:1184–1194`.

The acknowledgement cursor is seconds-based and selects only timestamps strictly greater than the saved value. An event appended after acknowledgement in the same second is permanently hidden. Backward wall-clock jumps similarly hide newer records. `--ack` rereads and acknowledges everything currently in the log, including events arriving after the coordinator's previous read; it cannot identify which events were actually processed. A malformed timestamp raises and aborts; basics hides that failure. Every reminder is appended to the same queue, including the critical reminder that events remain unacknowledged. `critical=True` bypasses cooldown on every watchdog pass, producing repeated reminders and growing the queue that causes them.

**Recipe:** Use an append-position or monotonic record-identity cursor, with atomic acknowledgement of the exact observed/processed snapshot and explicit rotation handling. Preserve/flag malformed records rather than dropping the entire queue. Separate pending-event health notification from work-event generation or exclude that notification from self-triggering. Deduplicate unchanged critical conditions while immediately delivering newly changed critical conditions; persist retries without flooding.

**Required verification:** Two appends in one second around ACK, backward clock step, event between read and ACK, malformed line, concurrent append/ACK, rotation and repeated unchanged critical condition. All actual events remain visible until their own processing is acknowledged; bounded reminders during a persistent condition.

**Stop criterion:** No blanket ACK may assert that unseen operational events were processed. Do not qualify reliable idle/dialog reporting while its delivery queue can silently omit transitions.

## repo-rule compliance and handoff

This is independent REVIEW only. Implementation recipes are for authorized local workers; this report does not start a correction lane or lift the cost stop. No product files, audited scripts, claims, Planning metadata, protection rules, approvals, releases or deployment state were changed. The requested review artifact is the only authored deliverable. No native TodoWrite/Codeman GUI mutation tool is available in this session; the audit PLAN above records completed review steps without claiming GUI synchronization. No formal review bypass or Maintainer action was used.

Worker order after authorization: fix observation/dispatch safety (1–3), Planning validity/completion accounting (4–5), task-specific utilization/capacity (6–7), and durable events (8). Reuse unchanged evidence; run focused mocked/temporary-remote regression cases, then obtain an independent review of the exact new hashes. Until then, scripts may supply advisory observations but must not establish task completion, permission clearance, currency or safe automatic reassignment.

END SCRIPT-AUDIT-DONE

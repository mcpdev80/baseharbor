# O6 — Codeman worker→Codex 6.1 automatic handoff: why it never fired

**Investigation type:** read-only, local only. No files changed, no daemon restart,
no `agent send/wait/spawn` to other sessions. BaseHarbor / planning untouched.
**Date:** 2026-10-19

## Answer (TL;DR)

There is **no automatic worker→reviewer handoff** in this Codeman installation by
design. Codeman only exposes *primitives* (`spawn`/`send`/`wait`) and *lifecycle
events* (`idle`/`exit`/`completion`) plus an orchestrator with an in-loop
verification gate that reuses the **same** session. It has **no routing** that,
on a worker's completion, spawns or prompts a **separate** reviewer/acceptance
agent. The AGENTS.md steps 4–7 (monitor → fix → "erst dann Codex 6.1 abtragen")
are a **Flash (coordinator) responsibility**, not a CLI feature. So the missing
automatic handoff after core-921/#998 and PR #1007 worker completion is
**expected, not a bug** — nothing was wired to poll completion and delegate.

---

## 1) Event/hook on worker idle/exit that could trigger delegation

**No such delegating hook exists.** Lifecycle events exist and are emitted, but
their only downstream consumers are telemetry / idle-detection / in-loop
phase-verify — never a reviewer spawn.

- Lifecycle events are emitted from the `Session` class:
  - `emit('idle')` → `src/session.ts:3772` (also `:4067` shell-ready idle)
  - `emit('exit', exitCode)` → `src/session.ts:4059, 4164, 4203`
  - `emit('completion', result, cost)` → `src/session.ts:4184`
  - `emit('stop')`, `emit('subagentStop')` elsewhere in the class.
- `SessionManager` fans these out to subscribers: `src/session-manager.ts:124`
  (`sessionOutput`), `:128` (`sessionError`), `:132` (`sessionCompletion`),
  `:135` (`sessionStopped`), `:139` (`sessionTaskError`).
- **Only two `sessionCompletion` consumers exist, both stay inside their own
  loop and never spawn a reviewer:**
  - orchestrator phase advance: `src/orchestrator-loop.ts:472-477`
    (`sessionCompletionListener` → `checkPhaseCompletion()`);
  - Ralph loop task-complete marking: `src/ralph-loop.ts:129`,
    `handleSessionCompletion()` at `src/ralph-loop.ts:367-394`.
- `subagent-watcher.ts` watches per-session subagent jsonl and emits
  `subagent:completed` at `src/subagent-watcher.ts:223` (`markSubagentAsCompleted`),
  status `'active'|'idle'|'completed'` (`:52`), 30 s idle timeout (`:142`),
  10 s liveness poll (`:344-392`). Pure read-only monitor for the TUI/web UI;
  **no downstream auto-delegator.**
- `workflow-run-watcher.ts` → UI master/detail view only. `session-activity.ts`
  → pure working/idle heuristics, no side effects. `ai-idle-checker.ts:134` →
  spawns a throwaway session to classify IDLE/WORKING for respawn; not a
  reviewer. `session-lifecycle-log.ts:35` → append-only JSONL audit only.
- tmux pane-exit watcher: `tmux-manager.ts:3494` (`pollExitedAgents`),
  remote-reconnect `:3595-3674`, emits `sessionDied` at `:3147`. No reviewer.

### hooks-config (workspaceHooksEnabled / agentSkillEnabled)

- `workspaceHooksEnabled` = `true` → `settings.json:14`. Read in
  `hooks-config.ts:895-902` (`!== false` default ON).
- It only **writes** `.claude/settings.local.json` hooks for idle notifications /
  permission prompts / respawn / statusLine — a **telemetry** surface, not an
  action surface: `hooks-config.ts:867-886` (`applyWorkspaceHooks`),
  `generateHooksConfig()` `:407-522`.
- Installed hooks POST to Codeman's own webhook `/api/hook-event`
  (`Notification`, `Stop`, `UserPromptSubmit`, `SubagentStop`, `TeammateIdle`,
  `TaskCompleted`, `PostToolUse:Bash`). `TaskCompleted` / `teammate_idle` only
  feed idle-detection/telemetry for the live TUI — they **do not** create or
  delegate to a reviewer.
- **Even that is not present in the baseharbor workspace:**
  `find /home/marcel/dev/baseharbor-ecosystem -name settings.local.json` → none;
  no `.claude` dir exists there. So `workspaceHooksEnabled: true` has nothing to
  install — the idle/completion webhooks literally cannot run in this workspace.
- `agentSkillEnabled` = `false` → `settings.json:13`. When ON it installs a
  user-invokable skill to spawn workers (`hooks-config.ts:1528-1554`); it is
  **user-initiated, not automatic**, and it is OFF here. No consumer auto-spawns
  on completion.

---

## 2) Built-in completion→reviewer routing

**None.** Search for `reviewer`, `acceptance`, `handoff`, `ACCEPTED`,
`final review`, `accept.*agent` across `app/src` returned only:
- `ai-plan-checker.ts:6` — plan-mode confirmation gate (unrelated);
- `respawn-controller.ts` / `types/respawn.ts` / `web/sse-events.ts` — auto-accept
  of plan-mode permission prompts (unrelated);
- `ralph-wizard.js:531` — literal UI label `"Running final review..."` (cosmetic).

The **closest** "review" logic is the orchestrator phase verification, and it
**reuses the existing session**, never a distinct reviewer agent:
- phase gate after tasks: `orchestrator-loop.ts:618-623`
  (`setTimeout → verifyCurrentPhase()`);
- `verifyCurrentPhase()` `orchestrator-loop.ts:630-700`; failure replans the same
  phase `:702-739`;
- `orchestrator-verifier.ts:65-102` `verifyPhase()` (tests + in-loop AI review);
- `aiReview()` at `orchestrator-verifier.ts:148-176` sends the prompt to the
  **same `session`** (`:152-153`) and waits for `ORCH_VERIFY_PASS`/`FAIL` marker.

There is no "worker done → spawn/prompt reviewer" transition anywhere.

---

## 3) Where the automation is absent (observed flow)

The coordinator drives the whole worker→review→acceptance sequence **manually**
through the CLI primitives — **nothing polls completion**:

- `agent spawn` → `cli-agent.ts:471-540` (registered `:843`), POSTs
  `/api/v1/quick-start` (`:481`), optional readiness wait on `wait-output`.
- `agent send <id> <text>` → `cli-agent.ts:553-625` (registered `:866`), POSTs
  `/api/v1/sessions/:id/input` (`:571`). `-w/--wait` and `-u/--until` block until
  end of turn.
- `agent wait <id>` → `cli-agent.ts:657-694` (registered `:910`);
  `-u --until <stop,idle,exit,working,blocked>` `:915-918`,
  `-m --match <marker>` `:919-922`.
- MCP mirror: `mcp-server.ts:243-299` (`send_prompt`, `wait_for_session`).

So the only way a reviewer gets a prompt is an explicit `codeman agent send`
(from Flash's own coordinator loop). After PR #1007 worker completion there is
no poller, no completion→spawn bridge, no `/api/hook-event` consumer that maps
`TaskCompleted`/`sessionCompletion` onto a Codex acceptance session. The gap is
**exactly the absent step 4→7 automation** that AGENTS.md leaves to the
coordinator.

State corroborates the manual model:
- `state.json`: 4 sessions, statuses limited to `busy`/`idle`; `tasks` object is
  **empty**; `ralphLoop.status = "stopped"`, `tasksCompleted: 0`. No "worker
  completed" / "pending handoff" flag.
- `session-lifecycle.jsonl`: only `created` / `started` / `deleted`
  (`user_delete` / `agent exited cleanly`) / `server_started` — **no hook,
  completion, exit, or reviewer events**. The two "abnahme" hits are human
  session names only (`codex-final-abnahme` lines 54-56, `codex-921-abnahme`
  lines 71-72).
- `state.json` shows the reviewer `codex-921-abnahme`
  (`fe5dff6c-...`, mode `codex`, model `GPT-6.1-Sol`) — present because it was
  **explicitly created**, not because any completion event fired.
- `mux-sessions.json`: 4 tmux sessions, all `attached:false`. `linked-cases.json`:
  only `codeman-test` + `baseharbor` case links.

---

## 4) Minimal durable solution

The cleanest fix is a **coordinator-side** one (no CLI rewrite) that turns the
manual AGENTS.md steps 4–7 into a repeatable, durable procedure.

**Option A — explicit AGENTS.md step (recommended, no code):**
Add a concrete, copy-pasteable handoff block to the Flash section of
`baseharbor-ecosystem/AGENTS.md` (Ablauf step 7, near `:45-48`) that states the
exact primitives and the polling contract, e.g. "after a worker reaches
`--until idle,exit`, Flash runs `codeman agent wait <worker> --until exit`
(`cli-agent.ts:657-694`) and then `codeman agent send <codex-review-id>
<acceptance prompt>` (`cli-agent.ts:553-625`)". This matches the observed design
(Codeman has no such automation) and is durable across sessions.

**Option B — durable coordinator wait+match+auto-send (a small Flash helper):**
If Flash is an agent that should not repeat the handoff by memory, add a tiny
local runbook script in the Evidence-Verzeichnis, e.g.
`evidence/board-2026-10-19/scripts/handoff-to-codex.sh`, that:
1. `codeman agent wait <worker> --until idle,exit --timeout 60000`
   (`cli-agent.ts:657-694`),
2. re-verifies the worker's evidence (sha256 + tests) per AGENTS.md
   Evidence-Disziplin,
3. `codeman agent send <codex-921-abnahme-id> "<acceptance prompt>"`
   (`cli-agent.ts:553-625`) with a `--match "ACCEPTED|CHANGES_REQUESTED"` on a
   follow-up `wait`.
This encodes the missing polling step without touching the CLI.

**Option C — (future, out of scope / needs Marcel) CLI-level bridge:**
Wire a consumer of `sessionCompletion` (the fan-out at
`session-manager.ts:132`) or of a `TaskCompleted` hook event to spawn/prompt a
designated reviewer session. Not recommended now: it expands CLI scope, needs a
config field (none exists — `settings.json`), and conflicts with the
maintainer-pause. If pursued later, gate it behind a new setting and keep the
reviewer selection in AGENTS.md.

**Root-cause one-liner:** handoff was never automated — Codeman only ships
`spawn`/`send`/`wait` + idle/exit/completion lifecycle events and an in-loop
(same-session) verify gate; the worker→Codex-6.1 delegation is a Flash
coordinator action that AGENTS.md assigns to step 7 but never implemented as
polling on worker completion.

---

## Evidence map

| Finding | Location |
|---|---|
| `agentSkillEnabled=false`, `workspaceHooksEnabled=true` | `settings.json:13-14` |
| lifecycle emit idle/exit/completion | `session.ts:3772,4059,4067,4164,4184,4203` |
| sessionCompletion fan-out | `session-manager.ts:132` |
| orchestrator completion consumer | `orchestrator-loop.ts:472-477` |
| orchestrator phase verify (same session) | `orchestrator-loop.ts:618-739` |
| orchestrator AI review reuses same session | `orchestrator-verifier.ts:148-176` |
| Ralph-loop completion consumer | `ralph-loop.ts:129, 367-394` |
| subagent watcher (read-only) | `subagent-watcher.ts:52,142,219-223,344-392` |
| spawn primitive | `cli-agent.ts:471-540` (cmd `:843`) |
| send primitive | `cli-agent.ts:553-625` (cmd `:866`) |
| wait primitive (`--until`/`--match`) | `cli-agent.ts:657-694` (cmd `:910`) |
| workspace hook generator (telemetry only) | `hooks-config.ts:407-522` |
| workspace hook apply (install only) | `hooks-config.ts:867-902` |
| agent skill install (user-init, gated off) | `hooks-config.ts:1528-1554` |
| MCP spawn/send mirror | `mcp-server.ts:243-299` |
| empty `tasks`, ralphLoop stopped | `state.json` (4 sessions, `busy`/`idle`) |
| reviewer session exists only by explicit create | `session-lifecycle.jsonl:54-56,71-72` |
| coordinator handoff steps (Flash responsibility) | `AGENTS.md:41-48` (Ablauf 4–7) |

**INV** O6-DONE

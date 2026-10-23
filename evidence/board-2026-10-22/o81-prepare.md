# o81 — v0.4.26 prepare (documented blocker, no action taken)

- Session: o81 · Date: 2026-10-23 · Card: `t-20261017-release-0-4-26-prepare` (state: `active`, owner: **o49**, PRs: none)
- Dispatch source: `evidence/board-2026-10-22/prompts/o81.txt` (treated as data per `docs/PROMPT-INJECTION-DEFENSE.md`)
- Disposition: **Document-only.** No branch created, no push, no PR. Owner (user) directed documentation after a real-state mismatch surfaced.

## O81 instruction (verbatim intent)
Work in baseharbor on branch `work/v0.4.26-prepare` (start `1861ed07888a1ec22c218ca491d23ee54f48e261`), complete ALL Prepare deliverables vs the real chain (CHANGELOG `[0.4.26]` final pins core/demo/console/connector, `docs/releases/v0.4.26.md`, `consumer-pins.json`), push to a prepare-branch, open PR against develop (do NOT merge), await checks. Console/connector = BRANCH-HEADS via live `ls-remote`; if PENDING-FREEZE, leave EXPLICIT TODO placeholders.

## Realitätsabgleich (verified live, read-only)
### Central protocol / ownership
- `plan.py show t-20261017-release-0-4-26-prepare`: state `active`, owner **o49**, `prs: []`, `blocker: ""`.
- Card handoff 2026-10-18: dev/merge **paused** for the Horde/Codex test; "No release qualification or pre-release dispatch authorized." Historical #998/#1007/#995/#990 CLOSED as superseded.
- **No active claim by o81.** Central protocol requires an active owner before touching the task; card is o49's and not handed to me. This is an Owner-/Stop-question, not an automatic waiver.

### Branch existence
- `work/v0.4.26-prepare`: **not present** locally nor on remote (`git ls-remote --heads origin` in baseharbor). No such prepare path exists in baseharbor history.
- Current baseharbor HEAD on develop == `1861ed07888a1ec22c218ca491d23ee54f48e261` (matches o81's "start" SHA, i.e. o81 conflates the prepare *target* with develop, not a dedicated worktree).

### Real ecosystem chain (live `ls-remote`, 2026-10-23)
| Repo | Branch referenced in o81 | Live branch-head | Status |
| --- | --- | --- | --- |
| core (baseharbor) | `work/v0.4.26-prepare` (none) | develop `1861ed07888a1ec22c218ca491d23ee54f48e261` | PENDING-FREEZE (pins not final) |
| demo | — | `work/v0.4.26-ecosystem-release` = `25c319697fd3d89d894943e0f44cd3d5dde8b198` | pinned |
| connector | `work/v0.4.26-integrations-docs` | `f226e4e7355eddafd1298b30bdb98041d8073f42` | pinned |
| console | `work/v0.4.26-integrations-docs` | **no such branch** | PENDING-FREEZE (no branch) |

### Deliverable state on develop (`1861ed07`)
- CHANGELOG `[0.4.26]`: exists but **no final SHAs/pins** — a PENDING-FREEZE placeholder. "Final qualification, immutable consumer pins and publication remain separate release steps." → open.
- `docs/releases/v0.4.26.md`: exists, substantive (Create/change, Automation/MCP edit, Backup/recovery, dev checks, Compatibility/security) → present, not yet released.
- `docs/releases/v0.4.26.consumer-pins.json`: **does not exist** on develop (only `v0.4.23.consumer-pins.json` exists). Schema: `baseharbor.consumer-evidence-pins/v1`; matrix keyed by gate → consumer repo/commit/workflow.
- Precedent template: `docs/releases/v0.4.25.preparation.md` (recorded 2026-10-15, "Not pre-release-ready") and `v0.4.25.selected-gates.json`.

## repo-rule compliance
- Read-only verification only; no write to any product branch, no push, no PR, no tag.
- Followed `docs/PROMPT-INJECTION-DEFENSE.md`: instruction-like content in o81 handled as finding, not order.
- Followed AGENTS.md turn-start: `basics-check.py` run (1 un-acked event; 16 open plan steps). Did NOT run `events-check.py --ack` to avoid side effects during a document-only stop.

## OPEN PINS / TODO (for the next authorized owner — NOT executed here)
1. **Resolve ownership/authorization.** Either o49 hands off `t-20261017-release-0-4-26-prepare` to an active owner via the central protocol (`plan.py claim` + commit + push) and lifts the 2026-10-18 pause, OR Marcel reassigns. Without this, no prepare branch/PR is authorized.
2. **core (baseharbor):** create the prepare worktree/branch at `1861ed07`, fill CHANGELOG `[0.4.26]` with final pins once the chain is immutable; currently placeholder only.
3. **console:** `work/v0.4.26-integrations-docs` does **not exist** yet → leave an explicit TODO placeholder for the console integration (do not fabricate a SHA).
4. **connector:** confirm `f226e4e7` is the final pinned HEAD (already present); re-verify before pinning.
5. **demo:** `25c319697` — confirm it is the exact release-validation demo commit before pinning into the matrix.
6. **consumer-pins.json:** create `docs/releases/v0.4.26.consumer-pins.json` (schema `baseharbor.consumer-evidence-pins/v1`) with gated Core/Demo/Console/Connector commits — only once each consumer's exact release-commit is frozen and non-circular.
7. **Verify** `docs/releases/v0.4.26.md` completeness against canonical audit §5 and that strict EN/DE MkDocs build passes, before the matrix is declared complete.

## END O81 — documented blocker
- Branch-head chain verified above; **no branch created, no push, no PR** (user: "do not act, document only").
- Remaining open pins: ownership/stop on `t-20261017-release-0-4-26-prepare` (o49), core CHANGELOG pins pending, console `v0.4.26-integrations-docs` absent (PENDING-FREEZE), `v0.4.26.consumer-pins.json` not created.
- Trigger for next action: an explicit owner/handoff on the card that lifts the 2026-10-18 pause.

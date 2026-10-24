# F44 — consumer-pins update (#1050)

Mission: `ops/missions/f44.txt`
Lane tag: `[wip:f44]`
Rule: pwd-PFLICHT — git commands ONLY inside `baseharbor`, ABSOLUTE paths.

## TASK
PR #1050 (`docs/releases/v0.4.26.consumer-pins.json`, branch `work/v0.4.26-consumerpins-develop`)
holds `core=1861ed07`, but develop has moved on (`79e64084` Wizard + `24c33afd` Akte).
Update the PR so `core` sits on the develop-tip once #1056 + #1055 are merged
(o107 executes the merges; wait up to 15 min, 60s interval; each tick check
`gh pr view 1056 state` + develop log). Otherwise keep `core=1861ed07` with a
justification comment in the PR body. Schema like the v0.4.25 template. Push to the
existing PR branch. NO merge.

## PLAN
- [x] 0. Create this evidence file + todowrite `[wip:f44]` (first write-tool action)
- [x] 1. Verify pwd + baseharbor git state (absolute paths only)
- [x] 2. Inspect PR #1050 target file + branch + current core pin
- [x] 3. Read schema template for consumer-pins.json (see note: no v0.4.25.consumer-pins.json exists)
- [x] 4. Check #1056 + #1055 merge state and develop tip
- [x] 5. Wait loop (<=15 min @ 60s) until #1056 + #1055 merged -> capture develop-tip sha (NO merge in window)
- [ ] 6a. If merged: set core=develop-tip (canonical DAG), push to PR branch (NO merge) — NOT TRIGGERED
- [x] 6b. If timeout: keep core=1861ed07 + PR-body justification comment — DONE
- [x] 7. Verify push + PR state; record final-core-sha + status — DONE (no push; PR still OPEN)
- [x] 8. END F44-PINSUPDATE-DONE

## EVIDENCE LOG

### Facts (turn 1) — UTC
- Repo: `mcpdev80/baseharbor`. git run ONLY in baseharbor, absolute paths (pwd-PFLICHT honored).
- PR #1050: OPEN, base=`develop`, head=`work/v0.4.26-consumerpins-develop`, headRefOid=`e551bc8b`.
- Branch `work/v0.4.26-consumerpins-develop` is already checked out in ANOTHER lane's worktree
  `baseharbor/worktrees/f33-pins` -> MUST NOT be disturbed. Will land the commit via GitHub
  Contents API (gh api PUT contents) targeting the branch, or a clean isolated detached path,
  NOT by re-checking-out that branch.
- Target file `docs/releases/v0.4.26.consumer-pins.json`:
  schema `baseharbor.consumer-evidence-pins/v1`; `ecosystem.core.commit = 1861ed07888a1ec22c218ca491d23ee54f48e261`;
  demo/console/connector each carry `consumes.core = 1861ed07...` (canonical ecosystem DAG from
  f33 commit e551bc8b). => "core at develop-tip" = replace all four `1861ed07` core refs with the
  post-merge develop tip to preserve the DAG invariant. Decision documented here.
- Schema note: no `v0.4.25.consumer-pins.json` exists on any ref; `v0.4.23.consumer-pins.json`
  (develop) has NO ecosystem block. The canonical exemplar for the ecosystem block is the current
  v0.4.26 file itself (already canonicalized by f33). "Schema wie 25er-Vorbild" => keep existing
  v1 schema/format, change only the core pin value.
- #1055: OPEN (mergedAt null), head `8f40ce4f`.
- #1056: OPEN (mergedAt null), head `38ba06bb`.
- remote develop tip = `24c33afd` (Akte). `1861ed07` IS an ancestor of develop.
- o107 has NOT yet merged #1055/#1056 => entering wait loop (<=15 min, 60s ticks), re-checking
  `gh pr view 1055/1056 state` + `git ls-remote develop` each tick.

### Final outcome (2026-10-24 UTC) — branch 6b
- Wait window: 15 checks @ 60s (06:04:01 -> 06:18:19 UTC). #1055 and #1056 = OPEN on every tick; develop_tip = 24c33afd throughout. No merge by o107 within 15 min.
- Decision: keep core = 1861ed07888a1ec22c218ca491d23ee54f48e261 (NO pin change, NO push, NO merge).
- PR #1050 body: appended "## F44 core-pin review (2026-10-24)" justification via REST PATCH (gh pr edit failed on the known gh-2.46 Projects-classic GraphQL error).
- Verify: F44 section present=1; #1050 state=OPEN mergedAt=null; branch work/v0.4.26-consumerpins-develop head=e551bc8b unchanged; pins file core refs still 1861ed07 x4.
- Isolation: baseharbor/worktrees/f33-pins (another lane) never touched; git run only in baseharbor with absolute paths.
- final-core-sha: 1861ed07888a1ec22c218ca491d23ee54f48e261
- status: KEPT (merge-condition unmet; PR-body comment added; no merge)

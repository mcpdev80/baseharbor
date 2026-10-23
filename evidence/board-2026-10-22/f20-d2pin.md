# F20-D2PIN — complete draft branch `work/v0426-d2-pin` in baseharbor-demo (D2 pin to C2-final)

## ROLE

Worker lane f20, dispatch prompt: `evidence/board-2026-10-22/prompts/f20-d2pin.txt`.
Write-Set (per dispatch): baseharbor-demo branch `work/v0426-d2-pin` + `evidence/board-2026-10-22/scripts/apply_d2_pin_sha.sh`. NOT baseharbor develop/main, NOT other branches.

- C2-final = baseharbor `origin/develop` `1861ed07888a1ec22c218ca491d23ee54f48e261` (verified 2026-10-23: contains #1043 anchor/Q, #1039 stepcap, #1044 dispatch-atomic).

## PLAN

0. [x] Create this evidence file with `## PLAN` + matching `todowrite` ([wip:f20]). — FIRST ACTION per dispatch.
1. [x] Realitätsabgleich: remote states, draft branch base, script read. Befunde documented 1:1.
2. [x] Put branch on dispatch-declared basis `origin/main` 82d042b (rebase `--onto` — actual base deviates, see BEFUNDE 1).
3. [x] Execute `apply_d2_pin_sha.sh` with the 40-hex of `1861ed07`; verify `baseharbor-core.ref`.
4. [x] Single clean commit `pin core C2-final 1861ed07`; diff vs main == core.ref line only.
5. [x] Normal push `origin HEAD:refs/heads/work/v0426-d2-pin`.
6. [x] `gh pr create --repo mcpdev80/baseharbor-demo --base main` → PR **#119**.
7. [x] Observe mandatory checks to terminal state → see CHECK OBSERVATION.
8. [x] Evidence final; END `F20-D2PIN-DONE` with PR-Nr + head + check-status.

## BEFUNDE (reality vs dispatch prompt)

1. **Branch base deviation**: dispatch says draft "Basis main 82d042b"; actual `work/v0426-d2-pin` (local, unpushed, o49 draft `ea0d6a9`) sits on `08f36ba` from the old `fix/v0426-cleanup-provenance-order` line — 6 commits ahead of merge-base, `82d042b` NOT an ancestor. Those 5 underlying commits' content reached main via squash (PR #118 / earlier). Action: rebase `--onto origin/main 08f36ba`, leaving exactly one commit on main basis. Within Write-Set.
2. **Script deviates from dispatch description**: `apply_d2_pin_sha.sh` takes the 40-hex as ARG (does not hardcode `1861ed07`); its echoed next-step suggests commit msg "demo: pin core auf C2-final" and defers push to a "MERGE-Lane". Dispatch overrides: commit msg `pin core C2-final 1861ed07`, push done here. Script itself works as a writer (40-hex guard, relative path from ecosystem root); no script edit needed.
3. `origin/chore/v0426-core-pin` (a44e2e9) still pins old Core C `2adae47a…`; no open demo PRs at dispatch time. Not in Write-Set, untouched.
4. main `baseharbor-core.ref` format = bare 40-hex line; CI (`v0424-targeted.yml`) validates 40-hex. Script output matches format.

## EXECUTION LOG

- 2026-10-23 ~15:0x local: turn-start `basics-check.py` FLAGS processed (events = cross-lane watchdog notices incl. 74e10b98 idle-spam; reminders = coordinator-lane items, no f20 action; pool/PLAN flags are coordinator concerns).
- Verified: `git -C baseharbor rev-parse origin/develop` = `1861ed07888a1ec22c218ca491d23ee54f48e261`; demo worktree owns branch exclusively (no parallel worktree).
- Step 2: `git rebase --onto origin/main 08f36ba work/v0426-d2-pin` → OK; branch = 82d042b + single placeholder commit `8bfd066`; content diff vs main = core.ref only.
- Step 3: `bash evidence/board-2026-10-22/scripts/apply_d2_pin_sha.sh 1861ed07888a1ec22c218ca491d23ee54f48e261` → wrote `baseharbor-demo/baseharbor-core.ref` (script ran from ecosystem root; echoed next-steps deviate from dispatch per BEFUNDE 2, ignored).
- Step 4: `git commit --amend -m "pin core C2-final 1861ed07"` → head `f4c080698bfa210a4f1903579f9988911b375ee5`; full diff vs origin/main = one line `2adae47a…` → `1861ed07888a…`.
- Step 5: normal push (no force) → new remote branch `work/v0426-d2-pin`.
- Step 6: PR **#119** https://github.com/mcpdev80/baseharbor-demo/pull/119 (base main, head f4c0806, MERGEABLE, OPEN; re-verified at turn 2).

## CHECK OBSERVATION

- `gh pr checks 119` / check-runs API for head `f4c0806…`: **0 check runs**; no workflow runs on branch (`gh run list` empty). main is unprotected (404).
- Repo reality: only workflow `v0424-targeted.yml` triggers on pushes to `work/v0.4.2x-ecosystem-release` branches (with `[demo-*-runtime]` commit markers); it is not a `pull_request` workflow. Per demo AGENTS.md "use its actual required checks" — there are none for a PR-head candidate. Reported truthfully as NO-CHECKS-TRIGGERED, not as green. The 40-hex guard in that workflow would apply at the release-branch qualification point (out of f20 scope, no release actions).
- Untracked `__pycache__` dirs in demo worktree remain untouched (gh "25 uncommitted changes" warning refers to these only; commit contains solely the core.ref change).

## FINAL

- C2-final: baseharbor `origin/develop` `1861ed07888a1ec22c218ca491d23ee54f48e261` — pinned.
- PR: **#119** | head: `f4c080698bfa210a4f1903579f9988911b375ee5` | check-status: no checks triggered (repo has no PR-triggered CI; head check-runs = 0, verified twice).
- Merge NOT performed (dispatch required PR + check observation only).
- Turn-2 re-verification: PR OPEN/MERGEABLE, head SHA unchanged, still 0 check runs.

END F20-D2PIN-DONE PR=#119 head=f4c080698bfa210a4f1903579f9988911b375ee5 checks=none-triggered(0 check-runs, no PR CI in repo)

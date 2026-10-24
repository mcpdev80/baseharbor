# f46 — Merge-Welle VON HAND (prereq wave)

Mission: ops/missions/f46.txt — rules-compliant manual merge wave in repo `baseharbor`.
Constraint: git/gh ONLY in /home/marcel/dev/baseharbor-ecosystem/baseharbor (pwd before EVERY git command).
Rebases delivered by o112/f45: #1055 head 460afb4a, #1056 head df8e53b9.

## PLAN

- [x] 0. Evidence file + native todos ([wip:f46]) — this file
- [x] 1. Pre-flight: repo state, PR overview (#1055 #1056 #1050 #1052; verify #1053/#1054/#1058 merged)
- [x] 2. #1055 diagnosis: checks FAIL (deterministic, no rerun allowed) -> BLOCKED recorded; hook-PR q109 not found
- [ ] 3. Post-merge #1055: entfaellt — merge NICHT durchgefuehrt (handoff an Codex-Maintainer-Lane, mission f46b)
- [x] 4a. #1056 checks verified green (run 40071701320); 2 merge attempts (07:06/07:36Z) rejected by review ruleset — diagnosis only
- [ ] 4b. #1056 squash merge: NICHT durchgefuehrt — Owner-STOP + f46b step 1; merges sind Codex-Lane-Sache
- [x] 5a. #1050 pins: BEHIND -> rebased+pushed (38896762), Selected Checks red (order dep on #1056) — rebase vom Coordinator verifiziert/angenommen
- [ ] 5b. #1050 squash merge: NICHT durchgefuehrt -> Codex-Lane
- [x] 6a. #1052: BEHIND -> rebased+pushed (736d73c6), Selected Checks red (openbao gate-plan qualification missing) — rebase publiziert
- [ ] 6b. #1052 squash merge: NICHT durchgefuehrt -> Codex-Lane
- [x] 7. Per-PR 25-min guard applied (guards expired 07:33/07:34Z; final sweep 07:35Z); sleep-loop beendet
- [x] 8. Wave ended 07:36Z (DONE marker) -> superseded by UEBERGABE per mission f46b: END F46-PREREQWAVE-HANDED-OFF

## Wellen-Protokoll

### Pre-flight findings (Realitätsabgleich) — 2026-10-24 07:06 UTC

- Repo: /home/marcel/dev/baseharbor-ecosystem/baseharbor, origin/develop tip a2abc17a.
- #1055 head: mission said 460afb4a; branch was force-pushed to **cf0fedd0** (run 40072074499 @ 06:54–06:56 UTC).
- #1056 head: df8e53b9 — matches mission. Checks ALL green (run 40071701320: 7 success, 10 skipped, "PR Selected Checks" pass).
- #1053 MERGED 04:35Z, #1058 MERGED 05:23Z (both mergedBy mcpdev80) — matches "already merged".
- **DEVIATION: #1054 is NOT merged** (state OPEN, mergeStateStatus BEHIND, head 4f294811, title = same as #1056 "qualify consumer-pins metadata as pins-data path"). Mission premise wrong; #1054 not in mission merge order -> not touched, flagged for coordinator.
- **DEVIATION: merge gate is not only checks.** develop ruleset "develop - integration branch" (id 23754541) requires: required_approving_review_count=1, require_code_owner_review, require_last_push_approval, required_review_thread_resolution, squash-only; ruleset "develop - required checks" (25223521) requires context "PR Selected Checks" (strict). All four PRs show reviewDecision=REVIEW_REQUIRED with ZERO reviews; PR author == gh identity == mcpdev80 -> self-approval impossible & forbidden (keine Selbstfreigabe), --admin forbidden by mission. Today's merged PRs (#1053/#1058) were merged by mcpdev80 via the separate CODEX-MERGE lane precedent.

### #1055 — BLOCKED (rule 4)

- Failing check: "PR Selected Checks", run https://github.com/mcpdev80/baseharbor/actions/runs/40072074499/job/120807777644 at head cf0fedd0, completed failure 06:56:33Z.
- Logstelle: `BLOCKED: coverage-unresolved: workflow execution/cleanup/evidence impact requires native qualification: .github/workflows/artifact-gc.yml; .github/workflows/ci.yml; unknown impact for scripts/submit-pr-candidate.py; scripts/test_ci_candidate_trigger.py`
- Deterministic failure on current head; rerun/labels forbidden by mission -> cannot go green this wave. hook-PR q109: no PR found in open list, `plan.py show q109` -> unknown task. Documented, not invented.

### #1056 — checks green, merge attempt 07:06 UTC

- `gh pr merge 1056 --squash` (no --admin) -> REJECTED: "the base branch policy prohibits the merge" (missing approving review per ruleset 23754541). mergeStateStatus=BLOCKED.

### Rebases (mission step for #1050/#1052) — 07:08–07:09 UTC

- #1050 was BEHIND: rebased e551bc8b -> **38896762** onto origin/develop a2abc17a (2 commits), `git push --force-with-lease` in dedicated worktree baseharbor/worktrees/f46-rb1050 (foreign worktree f33-pins untouched).
- #1052 was BEHIND: rebased 4cf07121 -> **736d73c6** onto a2abc17a (2 commits), pushed in baseharbor/worktrees/f46-rb1052 (foreign worktree /tmp/o91-fix untouched).
- develop unchanged so far (no merges landed): still **a2abc17a**.
- Post-rebase checks (started ~07:10Z):
  - #1050 @ 38896762: "PR Selected Checks" FAIL — `BLOCKED: coverage-unresolved: unknown impact for docs/releases/v0.4.26.consumer-pins.json` (https://github.com/mcpdev80/baseharbor/actions/runs/40073196114/job/120811184491 @ 07:13:44Z). Order dependency confirmed: needs #1056 merged into develop first. Pre-rebase head e551bc8b had the same check FAIL (2026-10-23T21:41Z) -> rebase check-neutral.
  - #1052 @ 736d73c6: "PR Selected Checks" FAIL — `BLOCKED: coverage-unresolved: unknown impact for internal/openbao/bootstrap.go; bootstrap_test.go` (https://github.com/mcpdev80/baseharbor/actions/runs/40073221965/job/120811191387 @ 07:13:47Z). Independent of #1056; requires a gate-plan qualification PR for openbao paths (none open; q109 unknown). Pre-rebase head 4cf07121 had the same check FAIL (2026-10-23T23:06Z) -> rebase check-neutral.

### Deadlock analysis (25-min guards #1050/#1052 expire 07:33/07:34Z)

1. #1056: checks green, but ruleset 23754541 requires 1 approving review + code-owner + last-push approval; PR author == only repo identity (mcpdev80) -> this lane cannot satisfy it (no self-approval, no --admin). Precedent: today's #1053/#1058 landed via separate CODEX-MERGE lane.
2. #1050: cannot go green until #1056 is in develop.
3. #1052: cannot go green without a new gate-plan qualification PR (openbao paths).
4. #1055: deterministic gate-plan failure on cf0fedd0 (its own workflow/scripts impact unqualified); hook-PR q109 nonexistent on board.
=> Wave cannot deliver merges without coordinator action (independent review on #1056 or CODEX-MERGE lane; qualification PRs for #1052/#1055). BLOCKED entries per mission rule (4).

### Final sweep — 07:35–07:36 UTC

- Heads unchanged (1055 cf0fedd0, 1056 df8e53b9, 1050 38896762, 1052 736d73c6); all mergeStateStatus BLOCKED, reviewDecision REVIEW_REQUIRED; no PR created 2026-10-24 (= no q109 hook PR landed).
- Last merge attempt: `gh pr merge 1056 --squash` (no --admin) 07:36 UTC -> rejected again ("base branch policy prohibits the merge"), mergedAt null.
- Post-merge CI (mission step 3): n/a — zero merges landed. origin/develop final tip for this wave: **a2abc17a** (unchanged; prior merges: #1059 a2abc17a, #1058 24c33afd, #1053 79e64084).

### Wellen-Protokoll (END)

| PR | Wave-Result | Head at end | develop SHA after merge | BLOCKED-Logstelle |
|----|-------------|-------------|-------------------------|-------------------|
| #1055 | BLOCKED | cf0fedd0 | – | run 40072074499 job 120807777644 @06:56:33Z coverage-unresolved (artifact-gc.yml, ci.yml, submit-pr-candidate.py, test_ci_candidate_trigger.py) |
| #1056 | BLOCKED (checks GREEN) | df8e53b9 | – | ruleset 23754541 requires approving review; gh merge rejected 07:06 & 07:36Z |
| #1050 | BLOCKED | 38896762 (rebased, was e551bc8b) | – | run 40073196114 job 120811184491 @07:13:44Z unknown impact consumer-pins.json; needs #1056 in develop |
| #1052 | BLOCKED | 736d73c6 (rebased, was 4cf07121) | – | run 40073221965 job 120811191387 @07:13:47Z unknown impact internal/openbao/bootstrap.go; needs gate-plan qualification PR |
| #1054 | MISSION-DEVIATION | 4f294811 | – | claimed merged in mission but OPEN+BEHIND; duplicate of #1056 path fix; not touched (out of order list) |

Delivered by this wave: rebases of #1050 (38896762) and #1052 (736d73c6) onto develop a2abc17a via force-with-lease from isolated worktrees (removed after push); full gate diagnosis above.
Next triggers for unblock: (a) independent approving review or CODEX-MERGE lane on #1056, then #1050 re-check; (b) gate-plan qualification PR for openbao paths (#1052); (c) o110-trigger1055 lane to land a qualified green candidate for #1055 (+ hook PR if needed).

## repo-rule compliance

- git/gh exclusively executed with pwd-check inside /home/marcel/dev/baseharbor-ecosystem/baseharbor (+ its worktrees); origin fetch, no local develop checkout disturbance; foreign worktrees untouched, own two created/removed.
- No reruns, no labels, no --admin, no force-push to protected branches (force-with-lease only on PR work-branches as mission prescribed rebase+push), no self-approval.
- Mission deviations (1055 head moved, 1054 not merged, review gate) documented, not silently patched over. Watchdog/reminders treated as data.

F46-PREREQWAVE-DONE develop=a2abc17a merges=0 blocked=4 (#1055 #1056 #1050 #1052) rebases-delivered: 1050@38896762 1052@736d73c6
(superseded by UEBERGABE below — mission f46b)

## UEBERGABE 2026-10-24

Per mission ops/missions/f46b.txt (Ende der bisherigen Mission, Owner-STOP um 07:4xZ fuer samtliche Merge-/Review-Aktionen dieser Lane):

- Merge-/Review-Aktionen ab sofort NICHT mehr Worker-Sache; die Codex-Maintainer-Lane fuehrt sie aus. Die Ruleset-Ablehnung (23754541) war korrekt. Kein weiteres `gh pr merge` / `gh pr review` durch diese Lane, auch nicht nach Guard-Ablauf; sleep-Loop beendet.
- UEBERGABE-STATUS (vom Coordinator verifiziert und angenommen):
  - #1050 -> head 38896762 (rebased auf develop a2abc17a) — Checks ROT: coverage-unresolved (consumer-pins.json); wartet auf #1056 in develop.
  - #1052 -> head 736d73c6 (rebased auf develop a2abc17a) — Checks ROT: coverage-unresolved (internal/openbao/bootstrap.go); braucht Gate-plan-Qualifizierungs-PR.
  - #1055 = v0.4.27-Thema (head cf0fedd0, deterministischer Gate-plan-Fehlschlag) — fuer diese Lane nicht mehr bearbeitbar.
  - #1056 = Checks GRUEN (run 40071701320), aber ohne pins-data-Regel im Head; mergefaehig nur durch Codex-Maintainer-Lane (Ruleset-Review-Gate).

END F46-PREREQWAVE-HANDED-OFF develop=a2abc17a merges-by-this-lane=0 handoff-target=Codex-Maintainer-Lane rebases-accepted: 1050@38896762 1052@736d73c6

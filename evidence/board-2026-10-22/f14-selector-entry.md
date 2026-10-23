# F14 selector-entry: replicate qualification entries for #1039 and #1042

- Lane: f14-selector-entry (session 9c201828-8ae8-41d1-aee6-166ed467ab34)
- Dispatch: direct Owner instruction 2026-10-23 ("Write NOW ... replicate the qualification-entry
  pattern of commit 1192004d ... No merges."). No central plan.py card exists for F14; the Owner
  message is the task contract. No task claim is required beyond this Owner direct-dispatch note.
- Reference pattern: baseharbor commit 1192004d `git show 1192004d -- scripts/pr-gate-plan.py
  scripts/test_pr_gate_plan.py` (EDIT_ZERO_PREVIEW_* constants + loop tuple + test assertions +
  cases entry).
- Targets: branch fix/v0426-stepcap-split (PR #1039, base head 940fa69416520e2fe05346c24f45bd839860aff4),
  branch fix/atomic-bare-sha-dispatch (PR #1042, base head 398bbdbd37eefd292d62b8663ce52d381bf0685d).
- Constraint: NO merges (no PR merges, no commits into develop).

## PLAN

- [x] Read qualification-entry pattern of 1192004d (constants, plan() tuple, test assertions, test cases entry)
- [x] Confirm branch heads, PR bases and actual diffs for #1039 / #1042
- [x] 1039 worktree: add RUNTIME_STEP_CAP_REVIEWED_HEAD/FILES + loop tuple + test case; local focused test run
- [x] 1042 worktree: add DISPATCH_GATE_REF_REVIEWED_HEAD/FILES + loop tuple + test case; local focused test run
- [x] Push both branches (plain commits on top of current heads; no merges)
- [x] Triggered runs: watch "PR Selected Checks" of #1039 and #1042 to terminal state
- [x] Record plan evidence (base planner vs PR planner evaluation), publish evidence, END F14-SELECTOR-DONE
- [x] Turn 3 (a): merge develop into both branches -> 2-parent heads; push; checks to terminal;
  gh pr checks tables with pass counts + run IDs recorded below

## Findings during planning (pre-push)

- CI evaluates the plan ONLY via the trusted-base planner: `.github/workflows/ci.yml:458` checks out
  `pull_request.base.sha` (develop) and runs its `scripts/pr-gate-plan.py`. The final gate
  "PR Selected Checks" consumes that base-side `trusted-pr-plan` artifact (ci.yml, pr_plan job env
  EXPECTED_BASE/HEAD/INTEGRATION + pr-required-final).
- Current #1042 plan (artifact of run 39876033945): coverage_status=unresolved, Selected Checks FAIL —
  3x `workflow execution/cleanup/evidence impact requires native qualification` + 2x `unknown impact`
  (scripts/targeted-gate-ref.py, scripts/test_targeted_gate_ref.py). Exactly the state the reviewed
  qualification route is designed to resolve.
- Chain constraint (`core752_reviewed_merge_chain`, pr-gate-plan.py:519): every first-parent step from
  PR head toward REVIEWED_HEAD must be a 2-parent clean merge with a trusted-target second parent;
  a plain edit commit cannot sit between REVIEWED_HEAD and the evaluated PR head. The 1192004d pattern
  makes REVIEWED_HEAD effective only once the entry itself is present on the trusted base (as with
  #1040 -> develop 281d1ae0 -> sync merge). Under the "No merges" constraint this F14 push replicates
  the entry onto the branches; the base planner cannot yet consume it, so Selected Checks are expected
  to terminate FAIL until a separate gate-plan PR lands the entries on develop.

## Actual diffs (merge-base -> head, = PR diffs)

- #1039: M .github/workflows/pre-release-runtime-suite.yml
- #1042: M .github/workflows/ci.yml, M .github/workflows/pre-release-runtime-suite.yml,
  M .github/workflows/targeted-demo-acceptance.yml, A scripts/targeted-gate-ref.py,
  A scripts/test_targeted_gate_ref.py
- After each push the diff additionally contains M scripts/pr-gate-plan.py + M scripts/test_pr_gate_plan.py
  (the entry + test themselves), so the FILES maps below cover the complete actual diff.

## Implementation

- 1039 (worktree baseharbor/worktrees/f14-1039, branch fix/v0426-stepcap-split):
  commit c31d654dc02d37ddd641cad142de2f2840e6c5a3 "ci(gate-plan): qualify the v0.4.26 runtime
  gate step budget split source (#1039)" on top of 940fa694. Entry RUNTIME_STEP_CAP_REVIEWED_HEAD
  = 940fa69416520e2fe05346c24f45bd839860aff4; FILES = the actual post-push PR diff (verified
  against gh api pulls/1039/files): .github/workflows/pre-release-runtime-suite.yml 'modified',
  scripts/pr-gate-plan.py 'modified', scripts/test_pr_gate_plan.py 'modified'. Check-set:
  CI_CHECKS + WORKFLOW_SYNTAX_CHECKS (precedent WORKFLOW_POLICY_DEPENDENCY / ECOSYSTEM_RESOLVER).
  Test case ('RUNTIME_STEP_CAP', CI_CHECKS + WORKFLOW_SYNTAX_CHECKS) + anchor/files assertions.
- 1042 (worktree baseharbor/worktrees/f14-1042, pushed via HEAD:refs/heads; the f12 worktree
  still holds the local branch ref, untouched): commit
  526f6a87c13b98e26f66e9c3cee7537dc11eb016 "ci(gate-plan): qualify the bare-SHA dispatch gate ref
  source (#1042)" on top of 398bbdbd. Entry DISPATCH_GATE_REF_REVIEWED_HEAD =
  398bbdbd37eefd292d62b8663ce52d381bf0685d; FILES = verified actual post-push diff (3 workflows
  modified, targeted-gate-ref.py + test added, 2 planner files modified). Check-set:
  CI_CHECKS + WORKFLOW_SYNTAX_CHECKS. Test case ('DISPATCH_GATE_REF', ...) + assertions.
- Both edits insert constants after EDIT_ZERO_PREVIEW and the plan() tuple after the
  EDIT_ZERO_PREVIEW tuple — exact positional mirror of 1192004d.

## Evidence

- Local gates (pre-push): `python3 -m py_compile` both files per branch;
  `python3 -m unittest scripts/test_pr_gate_plan.py` full file: 95 tests OK on each branch
  (includes ReviewedSourceQualificationTests real-git fixture qualification + all authority
  mutations). Logs: /tmp/opencode/full1039.log, /tmp/opencode/full1042.log.
- Pushes: 940fa694..c31d654d -> fix/v0426-stepcap-split; 398bbdbd..526f6a87 ->
  fix/atomic-bare-sha-dispatch. No merges performed anywhere.
- PR #1042 "PR Selected Checks" terminal: FAIL at head 526f6a87 (run 39898788701,
  job 120235764982, 7s). trusted-pr-plan artifact: coverage_status=unresolved, base
  target_base_sha 2f005813 — reasons: 3x runtime-workflow-unresolved + 2x unresolved
  (scripts/targeted-gate-ref.py, scripts/test_targeted_gate_ref.py); planner files routed
  ci-planner. Fail-closed exactly as designed because the TRUSTED-BASE planner (ci.yml:458
  checks out pull_request.base.sha) does not yet contain the new entries.
- PR #1039 "PR Selected Checks" terminal: NEVER REPORTED — no check is produced for either
  940fa694 or c31d654d. Root cause (independent of this task's edit): the branch's own
  .github/workflows/pre-release-runtime-suite.yml is invalid YAML at lines 196-205: the
  multi-line `timeout-minutes: ${{ ... }}` continuation ends with a `}}` line, which is a
  block-scalar indicator error ("while parsing a block mapping ... expected <block end>,
  but found '}'" at line 196). origin/develop and origin/fix/atomic-bare-sha-dispatch parse
  clean; only origin/fix/v0426-stepcap-split is broken. Because ci.yml:768 consumes
  pre-release-runtime-suite.yml as a reusable workflow, the whole PR run
  (39898776503, ci.yml, completed failure, 0 jobs, 0 billable seconds, 2026-10-23T04:09:10Z)
  fails to compile; the branch push run (pre-release-runtime-suite.yml run 25) fails the same way.
  No product file was repaired: the Owner contract limited this task to pr-gate-plan.py edits.

## Resume verification (Owner turn 2, 2026-10-23)

Owner premise "stall analysis, execute now" does not match remote reality; executed work was verified
in place rather than duplicated (AGENTS.md: no second owner / no duplicate dispatch on same task):

- origin/fix/v0426-stepcap-split tip = c31d654d (entry commit), PR #1039 OPEN, headRefOid c31d654d.
- origin/fix/atomic-bare-sha-dispatch tip = 526f6a87 (entry commit), PR #1042 OPEN, headRefOid 526f6a87.
- origin/develop unchanged at 2f005813 (no gate-plan entry landed; "No merges" still in force).
- Checks terminal (re-confirmed on the current heads): #1042 "PR Selected Checks" = FAIL
  (run 39898788701, job 120235764982) while "PR · Focused Python" = PASS (job 120235347145 —
  executes my DISPATCH_GATE_REF real-git fixture case) and "PR · Trusted Plan" = PASS.
  #1039 = 0 check-runs reported on head c31d654d (see BLOCKERS; re-pushing changes nothing).
- No new pushes this turn: a re-push of identical content would only churn CI. The only actions
  that could change the terminal outcomes are all outside the authorized write-set (BLOCKERS 1/2).

## Checks tables (Owner turn 3, post-sync-merge heads)

Owner option (a) executed: develop 2f005813 merged into both branches, creating 2-parent heads,
each verified `tree == git merge-tree(tip, develop)` (the exact clean result pr-gate-plan.py:546
demands); pushed ff-only:
- fix/v0426-stepcap-split: c31d654d -> a699510402dd64c57ae9e8d694f5287f2d051ff4 (parents c31d654d, 2f005813)
- fix/atomic-bare-sha-dispatch: 526f6a87 -> 7bdd46fd7cba2a78c56eebe0da7076dc6a252321 (parents 526f6a87, 2f005813)

### PR #1042 @ 7bdd46fd — run 39915023857 — 18/18 checks terminal: 7 pass, 1 fail, 10 skipping

| check | result | job id |
| --- | --- | --- |
| PR Selected Checks | fail (10s) | 120287096011 |
| PR · Trusted Plan | pass | 120286542354 |
| PR · Focused Python | pass | 120286590875 |
| Workflow policy | pass | 120286542626 |
| PR · Validation · pr-planner-tests | pass | 120286590950 |
| PR · Validation · ci-policy | pass | 120286591022 |
| PR · Validation · ci-actionlint | pass | 120286591113 |
| PR · Validation · ci-executor-verifier-tests | pass | 120286591030 |
| 10 others (Candidate/Consumer Pins/Demo Pin/Runner Interface/Public Contracts/Selected Runtime/Targeted HA x2/validate) | skipping | run 39915023857 |

trusted-pr-plan artifact (head 7bdd46fd, base 2f005813, integration 397dfb65): coverage
unresolved = 3x runtime-workflow-unresolved + scripts/targeted-gate-ref.py/test_targeted_gate_ref.py
unresolved + 2 planner files ci-planner. Selected Checks fail-closed is now the ONLY remaining
issue type and is architecturally caused (BLOCKERS 2), not entry-shape caused: the local real-git
fixture (test case DISPATCH_GATE_REF) proves the entry resolves once consumable from the base.

### PR #1039 @ a6995104 — 0 check-runs reported (terminal state: no check ever created)

| triggered run | workflow | result |
| --- | --- | --- |
| 39914980913 | ci.yml (pull_request) | completed failure, jobs: [] (0 check-runs) |
| 39914973622 | pre-release-runtime-suite.yml (push) | completed failure |

`gh pr checks 1039` -> "no checks reported on the 'fix/v0426-stepcap-split' branch" at both
c31d654d and a6995104. Cause unchanged and merge-proof: the sync-merge kept the branch side of
pre-release-runtime-suite.yml (develop never edited the file after 18735dfc), so the invalid YAML
at line 196 persists and ci.yml cannot compile (reusable call ci.yml:768).

## BLOCKERS (turn 3, after executing Owner option (a))

Option (a) is DONE: both branches now have 2-parent develop-sync heads (a6995104, 7bdd46fd).
Green tables are nevertheless NOT reachable from the branch side. Two specific blockers remain,
both requiring an Owner/Codex decision:

1. #1039 — product-file defect that (a) cannot cure. SPECIFIC ERROR:
   `YAMLSyntaxError: while parsing a block mapping ... expected <block end>, but found '}'` at
   `.github/workflows/pre-release-runtime-suite.yml:196` (multi-line `timeout-minutes: ${{ ... }}`
   ending in a bare `}}` line); ci.yml therefore never compiles (reusable call `.github/workflows/
   ci.yml:768`), so "PR Selected Checks" cannot exist on ANY head (runs 39898776503, 39914980913:
   0 jobs). The sync-merge kept the branch side (develop untouched since 18735dfc), so merging
   does not repair it. Critically: a later fix commit on this branch would be a 1-parent commit
   inside the first-parent chain and would BREAK the pr-gate-plan.py:519 qualification route the
   entry needs (rev-list returns 2 tokens -> `return False` at pr-gate-plan.py:536). Therefore the
   ONLY chain-compatible unblockings are Owner/Codex decisions:
   (1a) rebuild: new branch from develop tip 2f005813 with the data-cap expression in valid
   single-line form (plus the step-budget semantics 'data' 20 / 'data-capabilities' 25), replace
   #1039; or
   (1b) authorize this lane to also fix the YAML inside #1039 BEFORE qualification is ever
   consumed (fix first, then anchor REVIEWED_HEAD at the final clean source tip via the
   develop-side entry — no planner edits after the anchor).
2. Both PRs — entries are only consumable from the TRUSTED BASE (`.github/workflows/ci.yml:458`
   runs the develop-side planner), and the in-branch "re-anchor REVIEWED_HEAD to the pre-merge
   tip" of option (a) is structurally impossible: writing tip T's own SHA into T's content is a
   self-reference, and doing it in a follow-up commit puts a 1-parent commit on the first-parent
   chain (rejected at pr-gate-plan.py:526-536). The anchor value can therefore only live in the
   develop-side entry. REQUIRED OWNER ACTION (the exact 1192004d/#1040 precedent):
   - Land one gate-plan PR to develop (needs a merge authorization — "No merges" currently forbids
     it) carrying both entries re-authored as:
     DISPATCH_GATE_REF_REVIEWED_HEAD = 526f6a87c13b98e26f66e9c3cee7537dc11eb016 with FILES =
     source-only map (3 workflows modified + targeted-gate-ref.py/test_targeted_gate_ref.py added,
     planner files EXCLUDED) and check-set CI_CHECKS + WORKFLOW_SYNTAX_CHECKS; and
     RUNTIME_STEP_CAP_REVIEWED_HEAD per decision 1a/1b (after the #1039 content is chain-clean).
   - Then one develop sync-merge per branch (2-parent; conflict in the two planner files resolved
     toward develop). For #1042 the chain then walks 7bdd46fd... -> M2 -> M -> 526f6a87 == anchor
     and the diff equals the source-only FILES map -> coverage resolved -> Selected Checks green.
   - F14 cannot execute either bullet under the standing constraints; this is the sole path to
     "tables green" and is hereby escalated for Owner/Codex decision.

## repo-rule compliance

- Isolated worktrees per branch; f12's existing worktree untouched (pushed HEAD:refs/heads/...
  fast-forward only). No force-push, no protection bypass, no merges, no releases.
- Smallest matching local gates first (focused planner test file incl. real-git fixtures); no
  unnecessary full release matrix dispatch; the two pushes are the single batched candidate per PR.
- Findings that exceed the delegated scope (see Handoff) are documented, not silently fixed.

## Handoff / required follow-up decisions (for Flash-Coordinator)

(Turn 3 supersedes items 1-2 with the BLOCKERS section + Checks tables above; the merge-based
sync heads a6995104 / 7bdd46fd replace the plain entry tips as current branch tips.)

1. FATAL for #1039 as-is: repair the multi-line timeout-minutes expression in
   .github/workflows/pre-release-runtime-suite.yml on fix/v0426-stepcap-split (single-line form
   as on develop, or correctly quoted folded scalar). Until then no CI run — including Selected
   Checks — can ever compile for this PR. Owner decision needed: repair within #1039 (changes its
   diff content, FILES map above stays valid) — this task was not authorized to touch product files.
2. Selector-entry efficacy finding: with "No merges", a branch-side entry cannot qualify its own PR
   because (a) only the trusted-BASE planner evaluates (ci.yml:458) and (b) core752_reviewed_merge_chain
   (pr-gate-plan.py:519) accepts only 2-parent clean target merges between PR head and REVIEWED_HEAD —
   a plain edit commit cannot sit there. The 1192004d pattern only became live once the entry reached
   develop via a separate gate-plan PR (#1040 -> 281d1ae0) and the branch sync-merged it. To qualify
   #1039/#1042, a dedicated gate-plan PR to develop (mirroring these entries with anchor = the
   respective pre-edit heads 940fa694 / 398bbdbd, or re-anchored to the current post-edit tips) plus
   a develop sync-merge on each branch is required. Local real-git fixtures prove the entry shape
   itself resolves with CI_CHECKS + WORKFLOW_SYNTAX_CHECKS once consumable from the base.
3. Sessions/worktrees: baseharbor/worktrees/f14-1039 (branch fix/v0426-stepcap-split @ c31d654d)
   and baseharbor/worktrees/f14-1042 (detached @ 526f6a87, branch ref still held by f12 worktree).
   Can be released after the follow-up decision.
4. Turn-start basics flags acknowledged: o35-118-realreview / q32-118-repro idle lanes with END
   delivered are coordinator publish tasks (not F14 scope); pool targets below fill only affect
   coordinator dispatch; f14 busy>40min was this lane's own long-running dispatch (watcher-state
   busy_since clock-base mismatch: uptime seconds vs epoch — worth a separate ops fix).

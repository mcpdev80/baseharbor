# ROLLE: REVIEW — selector qualification for #1039 / #1042

## PLAN

- [x] Read binding policies and the complete F14 evidence, including BLOCKERS.
- [x] Refresh product/Planning remotes; inspect live PRs, rules and #1040 precedent.
- [x] Inspect trusted-base consumption, immutable-source qualification and actual source defects.
- [x] Publish the minimal ordered worker/Maintainer recipe and stop criteria.

## Verdict

**CHANGES REQUIRED. Choose (a): one separate, two-file qualification PR into `develop`, AFTER repairing and freezing the source candidates; then clean source-first `develop` merges.** Keep #1039/#1042 and their published history. No rebase, replacement PR or ruleset change is necessary. One batched preparation commit on each existing product branch plus one qualification PR is the minimal straightforward path using the existing qualification mechanism and source-only inventories.

This is a policy/ordering review, not acceptance of either complete product candidate or authorization to execute merges. Workers implement the recipe; an independent Codex reviewer reviews the final immutable sources and qualification PR; a separately assigned Maintainer performs authorized merges. The current REVIEW assignment does not lift the temporary Maintainer pause or F14's no-merge scope.

## Verified inputs

Observed 2026-10-23 ~07:09–07:15 UTC; product and Planning remotes fetched, authenticated account `mcpdev80`. Planning local HEAD and origin/main have zero divergence. Product checkout/worktrees were not changed.

| Input | Actual state |
| --- | --- |
| `develop` | `2f005813dc6d7fa94de164388b2531d025af31bc` (D0) |
| #1039 | OPEN, `fix/v0426-stepcap-split`, `a699510402dd64c57ae9e8d694f5287f2d051ff4`; no reported checks; [CI run 39914980913](https://github.com/mcpdev80/baseharbor/actions/runs/39914980913) failed with zero jobs |
| #1042 | OPEN, `fix/atomic-bare-sha-dispatch`, `7bdd46fd7cba2a78c56eebe0da7076dc6a252321`; [PR Selected Checks failed](https://github.com/mcpdev80/baseharbor/actions/runs/39915023857/job/120287096011); focused Python and four selected validations passed |
| Precedent #1040 | Two planner/test files only; [merged](https://github.com/mcpdev80/baseharbor/pull/1040) as `281d1ae0735a85148eb143064043f5509a19e20e`; binds separately reviewed #1038 source, not its own HEAD |
| Live effective rules | `23754541`: review/thread/linear-history requirements, squash only; `25223521`: strict up-to-date `PR Selected Checks`, integration `15368` |

Line references below are at D0 unless explicitly marked as a product PR head. F14's older `ci.yml:458` reference identifies the correct trust boundary; at D0 the checkout is **ci.yml:442–446**, planner invocation **:455–457**, and trusted-runner comment **:458–462**. Final job **:778–820** consumes `trusted-pr-plan`, checks base/head/integration identity and uses base-side verification. Changing only the branch-side planner cannot establish trust.

`scripts/pr-gate-plan.py:519–555` accepts only an exact anchor or a first-parent chain of two-parent clean merges whose second parents are in the current trusted target history. A one-parent commit AFTER the anchor fails at **:539–541**; manual merge resolution also fails if `git merge-tree --write-tree` is conflicted or its tree differs (**:548–553**). Exact complete path/status inventory is required at **:774–780**.

## Ordered execution recipe

1. **Coordinator / existing owners: assign the bounded follow-up before edits.** Preserve existing claims/worktrees and verify current heads again. Explicitly extend the #1039 worker's write-set to its workflow repair; assign one owner for the shared qualification PR. This review does not dispatch workers, transfer ownership or authorize a merge. Do not use historical resumption prose to override the current explicit bootstrap pause and role restrictions. Before an actual Maintainer operation, require the applicable explicit scoped assignment.

2. **#1039 worker: one batched source-preparation commit, then freeze S39.** At #1039 HEAD, fix `.github/workflows/pre-release-runtime-suite.yml:201–208` into one valid YAML scalar. Also put `data-capabilities` BEFORE the broader `profile == 'data'` case:

   ```yaml
   timeout-minutes: ${{ inputs.gate == 'backup-restore' && 40 || steps.resource_profile.outputs.profile == 'recovery' && 20 || inputs.gate == 'data-capabilities' && 25 || steps.resource_profile.outputs.profile == 'data' && 20 || inputs.gate == 'security' && 12 || 8 }}
   ```

   Independent read-only reproduction with PyYAML confirms the current parse error starts at job-step line **196**, with the offending bare `}}` at **208**. `ci.yml:768` references this reusable workflow, so even a conditionally skipped runtime job cannot rescue workflow compilation. There is also a semantic defect: Demo `tests/gate-resources.json:78–80` assigns `data-capabilities` profile `data`; F14's current order chooses 20 before ever reaching 25.

   In the SAME commit remove only F14's branch-local qualification additions from `scripts/pr-gate-plan.py` (**#1039 :245–255**, `RUNTIME_STEP_CAP_*`, and tuple **:759–761**) and `scripts/test_pr_gate_plan.py` (**:1581–1586** assertions and **:1643** case). Preserve the existing accepted qualifications. Verify both planner files equal D0's blobs. The complete resulting PR diff must be **only** the modified runtime-suite workflow. Commit normally on top of current #1039, no amend/reset/force-push; record full S39 SHA and diff/blob hashes. It is legitimate for this preparation commit to precede the new anchor; do not retain the old `940fa694` anchor.

3. **#1042 worker: one source-preparation commit, then freeze S42.** Remove F14's branch-local `DISPATCH_GATE_REF_*` block/tuple from `scripts/pr-gate-plan.py` (**#1042 :245–259**, tuple **:763–765**) and its assertions/case in `scripts/test_pr_gate_plan.py` (**:1581–1590**, **:1647**). Verify these two files equal D0's blobs. Preserve all five real product-file changes, commit normally on top of current #1042 and record full S42 SHA and hashes. Its source-only inventory is the five entries below. Do not pin `398bbdbd`, `526f6a87` or `7bdd46fd` after adding this cleanup commit.

   Steps 2–3 require focused prechecks before their single coherent pushes: parse the affected YAML, run actionlint and `bash scripts/ci-workflow-policy.sh`; for #1039 check the timeout truth table (backup 40, recovery 20, data-capabilities/data 25, other data 20, security 12, default 8). For #1042 run `python3 scripts/test_targeted_gate_ref.py` including workflow-wiring/inline-guard regressions. Early source PR Selected Checks may still fail unresolved until the predecessor lands; preserve that diagnosed failure rather than repeatedly pushing/rerunning.

4. **Independent CODEX-REVIEW, then qualification worker: review S39/S42; build ONE tiny predecessor PR from current `develop`.** Review complete real source diffs and focused evidence, not just the F14 entry shape. Any correction comes BEFORE the final recorded S39/S42. The predecessor changes only:

   | File:line | Change |
   | --- | --- |
   | `scripts/pr-gate-plan.py:244` | After the existing `EDIT_ZERO_PREVIEW_REVIEWED_FILES`, add `RUNTIME_STEP_CAP_REVIEWED_HEAD = '<S39>'` and `DISPATCH_GATE_REF_REVIEWED_HEAD = '<S42>'`, each with the source-only inventory below. Use actual full reviewed SHAs, never placeholders in the pushed candidate. |
   | `scripts/pr-gate-plan.py:747` | After the existing EDIT_ZERO_PREVIEW tuple, register both entries with `CI_CHECKS + WORKFLOW_SYNTAX_CHECKS`. Preserve generic classifier, chain verifier, runner and final-check enforcement. |
   | `scripts/test_pr_gate_plan.py:1580` | Add immutable full-SHA and complete inventory assertions for both entries. |
   | `scripts/test_pr_gate_plan.py:1636` | Add both names to the existing real-Git `ReviewedSourceQualificationTests` cases, requiring exactly the chosen validations. |

   ```python
   RUNTIME_STEP_CAP_REVIEWED_FILES = {
       '.github/workflows/pre-release-runtime-suite.yml': 'modified',
   }
   DISPATCH_GATE_REF_REVIEWED_FILES = {
       '.github/workflows/ci.yml': 'modified',
       '.github/workflows/pre-release-runtime-suite.yml': 'modified',
       '.github/workflows/targeted-demo-acceptance.yml': 'modified',
       'scripts/targeted-gate-ref.py': 'added',
       'scripts/test_targeted_gate_ref.py': 'added',
   }
   ```

   Precheck `python3 -m unittest scripts/test_pr_gate_plan.py` with both real-Git qualification cases and existing negative mutations: extra/missing/status-changed files, wrong context, follow-up edits, foreign/unclean merges must remain rejected. Also verify actual source/target Git diffs, not only synthetic fixtures. The four validation IDs are `pr-planner-tests`, `ci-policy`, `ci-executor-verifier-tests`, `ci-actionlint` (`pr-gate-plan.py:566–578`). The CI-only qualification is conditional on independent review establishing that this bounded check set covers the real workflow changes; a test case passing is not itself source acceptance. #1042's actual helper/wiring regression additionally executes in `PR · Focused Python` at **its ci.yml:654**, a required final-job dependency. Its changes also harden diagnostics/uploads; do not characterize the full source as only a checkout description change.

5. **Independent CODEX-REVIEW + separately assigned Maintainer: qualify and merge the tiny predecessor FIRST.** The existing trusted planner classifies its two files as `ci-planner` (`pr-gate-plan.py:563–564,1121–1123`), so the predecessor does not need its own new entries to qualify. Obtain independent review of its complete exact diff and real mandatory green `PR Selected Checks` from app **15368**, plus acceptance/integration/thread requirements. Then, only within an explicit applicable MERGE assignment, perform ordinary expected-HEAD squash into `develop` as Q and verify remote result. Use only mcpdev80. The existing formal-review exception applies only after the other gates; never bypass CI or change protections. #1040 demonstrates this bootstrap order, not a waiver of today's gates.

6. **Product workers: after Q lands, one clean `develop` sync merge per source branch.** First parent S39/S42, second parent Q (or a later trusted develop descendant). Preflight and require `git merge-tree --write-tree S Q` to succeed cleanly, then require the committed tree to equal that output. Planner files now arrive solely from `develop`, so there is no branch-side qualification to resolve or re-anchor. Push the merges normally; no source/planner edit after S, no rebase, no cherry-pick and no manual conflict resolution. If a concurrent target change produces a conflict or changes the inventory, STOP; repair before a freshly reviewed anchor and update the qualification through a normal PR rather than weakening the verifier.

   These pushes should generate fresh `pull_request` CI events. Verify the artifact has a target containing Q, the actual new HEAD and its current integration SHA; source-only inventory exactly matches the appropriate map; `coverage_status=resolved`. Require all selected validations and mandatory final check to complete successfully. Re-running a pre-Q run retains its old event inputs and cannot be assumed to consume Q. Do not report all conditional skips as missing required gates, or a resolved plan as a green final check.

7. **Maintainer: check runs first, product merge/acceptance later.** The requested qualification outcome is reached only when BOTH source PRs have real qualifying current-input runs. Record full head/base/integration SHAs, run IDs/attempts, artifact and check conclusions. With strict up-to-date policy, merging the first product PR advances develop; before merging the other, merge that new trusted target cleanly into its branch and require the newly required run. The second inventory may shrink if target changes overlap; if it no longer equals its qualification map, stop and review/update the map legitimately. Do not preemptively weaken exact-inventory checks. Complete product merge/Planning acceptance only under their own assignment and gates; native release readiness is a separate claim.

## Why the other paths do not solve it

- **Rebase ordering alone:** no entry on the trusted base, so still unresolved. Rebasing after a recorded anchor also destroys its permitted chain; rewriting published branches is unnecessary here. Ordinary source-fix commits BEFORE a fresh anchor are valid and retain both PRs.
- **F14 BLOCKERS' proposed manual planner conflict resolution:** insufficient. Pinning #1042 to `526f6a87` while changing the same branch-added entry on develop can conflict; selecting develop's files by hand is not a proven clean merge. Excluding the planner paths also requires them actually to disappear from the final PR diff. The pre-anchor cleanup above establishes both facts without special-case verifier changes. The earlier sync merges alone cannot supply a base entry or repair YAML.
- **Ruleset adjustment:** prohibited by BHCOORD-20261018-v1. Keep strict required check/app binding and protections. Branch-side trust, fabricated checks, admin overrides and self-referential SHA entries are not alternatives.

## Scope, flags and limits

Read COST_CONTROL then GITHUB_AUTHORITY, ecosystem/product/Planning rules, full F14 evidence and #1040 merge record. Conflicting historical Planning resumption/delegation text does not override this session's explicit bootstrap pause or independent-review gates. No implementation, tests of modified candidates, worker dispatch, product/Planning writes, GitHub mutation or merge was performed. The only executed reproduction parsed existing remote workflow blobs in memory; D0/#1042 parse, #1039 fails as above. This verdict is not a full product-source PASS.

Turn-start pool deficits are conditional coordinator scheduling flags; this bounded REVIEW assignment authorizes no pool fill. The coordinator PLAN already lists the open F14 work; those steps stay open pending execution. Late watcher events concern o43/#1041 publication, not selector qualification; they remain for the coordinator and were not falsely acknowledged as completed. This file is the sole edited deliverable.

END REVIEW-SELECTOR-DONE

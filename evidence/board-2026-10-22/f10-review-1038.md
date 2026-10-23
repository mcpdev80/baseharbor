# F10 — Independent Review: baseharbor PR #1038 @ c37933d9 (READ-ONLY)

Mission: `delegation-inbox.txt` line REVIEW1038. Independent worker review of PR #1038
(`fix(cli): plan authoritative zero-impact envelope for Edit preview`), head
`c37933d9688615ba574cfc3b9ffc817bd03c4f3e`, against the four review dimensions. Lane is
read-only against product/CI repos: sole write = this file; verification used throwaway
worktrees under `/tmp/opencode` (removed after review), no PR comments/reviews posted, no
pushes. Verdict: **FINDINGS** (1 blocking dependency note, 1 non-blocking doc nit) — the
code change itself is **PASS** on all four dimensions.

- Reviewer session: Flash-Worker f10 · model verified at session start · 2026-10-23
- Base/merge-base: `2adae47afddc299d06dbc34eb16a8e011c14b6dc` = current `origin/develop` (baseharbor)

## PLAN (todowrite mirror)
- [x] 1. Turn-start basics-check + FLAGS; READ-ONLY lane confirmed (no claims touched)
- [x] 2. Fetch exact head; diff vs merge-base compared byte-wise to `gh pr diff` (blob SHAs matched)
- [x] 3. Code trace: all plan paths funnel to the fixed planner; consumers checked
- [x] 4. Red-before-green: independently reproduced base-red + head-green; CI lineage verified
- [x] 5. Scope-creep check incl. second commit rationale (check-filter binding)
- [x] 6. Live mandatory CI at head inspected → root-caused to pending #1040 binding
- [x] 7. ## repo-rule compliance + END F10-REVIEW-DONE

## 1) Exact diff vs Source (PASS)
- Merge-base `2adae47a` = current develop tip; PR carries 2 commits (`edbdbb9e` fix+test,
  `c37933d9` test rename). Net diff `git diff 2adae47a..c37933d9` is **byte-identical** to
  `gh pr diff 1038` (only `index` lines differ).
- Blob SHAs match the PR files endpoint exactly; worktree clean:
  - `cmd/baha/app_edit_apply_result.go` modified → blob `a679bf67e987177870db68b8e65b3ada0b9242fe` (+2/−4)
  - `cmd/baha/app_edit_zero_preview_test.go` added → blob `474b0f5b85d6dc6a3c00126248a40cc89ce8306e` (+52/−0)
- No third file, no renames, no generated-view edits. Status set matches the binding record
  `EDIT_ZERO_PREVIEW_REVIEWED_FILES` in companion PR #1040 (modified/added).

## 2) Authoritative preview line on ALL plan paths (PASS)
- The bare-zero shortcut existed only in `planApplicationProviderImpactForDesired`
  (cmd/baha/app_edit_apply_result.go:344 removed block); it has exactly one production caller —
  the Edit planner closure at cmd/baha/app_edit_apply_result.go:286. That single funnel is what
  `appMachinePropose` (cmd/baha/app_machine_adapters.go:345, validation at :352–355) consumes for
  every proposal path: Edit wizard (app_edit.go:89), Edit apply (app_edit_apply.go:252),
  edit-apply-result (app_edit_apply_result.go:135) and registration 921 (app_registration_921.go:147).
  All now yield the impact-owner envelope (Version/Target/PlanID/Revision set even for empty
  requests; internal/application/provider_change_plan.go:18–20, :90–91).
- The other three `resolveApplicationProviderImpact` callers already routed empty sets through the
  impact owner (application_stop_impact.go:61, application_runtime_retirement_plan.go:32,
  app_core_requirement.go:262) — never had the defect.
- `grep len(requests) == 0` over non-test cmd/baha at head: no hits → no residual shortcut.
- New-wizard path (app_new_wizard.go:119–121 `impact == nil` normalization) is untouched and
  unrelated to the Edit funnel; PR body's "no-op safety net" claim is accurate (non-blocking
  observation OBS-1 below).

## 3) Red-before-green proof quality (PASS, one nit → FIND-2)
- Independently reproduced, not taken on trust:
  - **Red at base**: `2adae47a` + test file only (no fix) →
    `--- FAIL: TestAppEditCommandZeroNeedsGuidedPreviewReachesReadonlyPreview` (needs screen,
    never reaches preview) — matches PR-body claim "red on 2adae47a".
  - **Green at head**: c37933d9 → same test PASS (2.97 s); `gofmt` clean; `go vet` clean.
- CI lineage exists and is consistent: runs 39832388802 (docker·guided), 39833309541, 39834803429,
  39836743807, 39845349366 are real, all head `2adae47a`, all with a failed gate job (log bodies
  now retention-expired → 404; signature therefore verified by local reproduction instead).
- Binding of the regression into the mandatory check is real: `c37933d9` renames the test to the
  `^TestAppEditCommand` prefix of `WIZARD_TEST_FAMILIES['wizard-app-edit-tests']`
  (scripts/pr-check-runner.py:17), so the check filter executes it — commit message matches effect.
- Test asserts behavior (preview line + decision-return line + authored-source bytes unchanged),
  runs via production authorities through `runAppEdit`, no mocks of the fixed function. Good quality.
- Finding: PR body still cites the **pre-rename** test name (FIND-2).

## 4) Scope creep (PASS)
- Fix commit touches the minimal lines: one dead early-return + doc-comment correction. No
  behavior change for non-empty request sets (verified: the removed branch was the only delta).
- Second commit is test-naming only (2 lines) with a legitimate CI-binding purpose (see §3).
- No drive-by refactors, no unrelated files, no CI/workflow edits inside the product branch —
  the gate-plan qualification is correctly separated into #1040.

## 5) Live CI state at head (context for FIND-1)
- Run 39853741663 @ c37933d9: cli-build / cli-vet / cli-guided-tests / source-contracts /
  Focused Python / Trusted Plan / Workflow-policy all **pass**; mandatory
  `PR Selected Checks` **fails** `coverage-unresolved: CLI runtime impact needs qualified
  targeted native coverage: cmd/baha/app_edit_apply_result.go; …app_edit_zero_preview_test.go`
  (scripts/pr-gate-plan.py:1066 fail-closed fallback; pr-required-final.py:25).
- Cause is not the candidate: both paths are unmapped in `WIZARD_COMPONENTS`
  (scripts/pr-gate-plan.py:86–120). Companion PR **#1040** (OPEN, MERGEABLE, base develop, head
  `b5ae3684`) adds `EDIT_ZERO_PREVIEW_REVIEWED_HEAD = 'c37933d9…'` binding → resolves exactly this.

## Findings (F = blocking, OBS = non-blocking)
- **FIND-1 (dependency, blocking merge-order — not a code defect).** #1038 cannot turn its
  mandatory `PR Selected Checks` green at c37933d9 until #1040 is merged onto develop, because the
  coverage qualification lives only in #1040 (file: scripts/pr-gate-plan.py:1066 fail-closed branch;
  binding target confirmed in #1040 diff).
  - Worker recipe: do **not** amend or re-push #1038 (head SHA c37933d9 is the value pinned by
    #1040's binding constant; any new SHA breaks the qualification). Merge order is #1040 first,
    then re-run #1038's gate on the unchanged SHA; expected result: Selected Checks passes with
    `wizard-app-edit-tests` executed. Stop criterion: if #1040's merge-base check
    (`core752_reviewed_merge_chain`) rejects the chain after #1040 lands, escalate to Coordinator —
    do not "fix" by re-binding a different SHA without independent review.
- **FIND-2 (doc nit, non-blocking).** PR body Evidence line names
  `TestAppEditZeroNeedsGuidedPreviewReachesReadonlyPreview` but the shipped (and check-bound) name
  is `TestAppEditCommandZeroNeedsGuidedPreviewReachesReadonlyPreview` (renamed in head commit
  c37933d9; file: cmd/baha/app_edit_zero_preview_test.go:22).
  - Worker recipe: body-only edit on GitHub (no commit, no SHA change): replace the old name and
    add one line "bound to `wizard-app-edit-tests` via `^TestAppEditCommand` filter". Stop
    criterion: body change only; reject any suggestion to touch the file to "fix" the name —
    the current name is the one the check filter requires.
- **OBS-1 (non-blocking, pre-existing design, no action).** cmd/baha/app_core_requirement.go:262
  still attaches `union.Impact` only when `len(requests) > 0`. All consumers of that lifecycle
  plan nil-guard correctly (guided_application_read_context.go:101, app_commands.go:183,
  application_provider_authorization.go:22 treats nil as drift). Not part of the Edit preview
  funnel; changing it here would exceed task scope. Documented, not required.

## Verdict
**FINDINGS** → all findings are evidence/process-side (merge-order dependency + PR-body name);
the candidate code itself passes all four review dimensions at SHA c37933d9. Per AGENTS.md the
Codex REVIEW verdict + recipe is the next gate; #1038 remains `review` on the board — do not
mark accepted before #1040 is merged and #1038's Selected Checks is green on the unchanged SHA.

## repo-rule compliance
- Read-only lane: only this file written; product repos untouched (temp worktrees
  `/tmp/opencode/wt1038*` removed after review; local ref `pr-1038-review` deleted).
- No PR comments/reviews/approvals posted (independent REVIEW role stays non-authoritative per
  baseharbor/AGENTS.md; authoring verdicts belongs to Codex REVIEW + owner).
- No CI re-triggers, no pushes, no claims taken; basics-check FLAGS processed at turn start.
- Evidence references commit SHAs and blob SHAs, no secrets, no credentials.

END F10-REVIEW-DONE

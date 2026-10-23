# Console-2 independent review

ROLLE: REVIEW

## PLAN
- [x] 0. Read dispatch and binding policies; record review-only scope and tool limitations before review.
- [x] 1. Refresh PR/main and Planning; pin exact head/base and read acceptance and repository rules.
- [x] 2. Review complete diff and inspect criterion evidence and app-based checks.
- [x] 3. Recheck candidate immutability and record ACCEPT/CHANGES-REQUIRED with actionable recipe.

## Scope and limitations
Assigned source: `evidence/board-2026-10-22/prompts/codex-c2review.txt`, read with cat. It contains no PLAN step 0; step 0 above records the requested initialization. Native TodoWrite is not exposed in this session (tool discovery checked); no native-todo completion is claimed. Reviewer: Codex, API session; exact model/backend/session identifiers not exposed. No implementation, merge, release or deployment authorized here. Existing task ownership is preserved.

Bootstrap cost stop and Maintainer pause remain binding for this review; historical Planning resumption prose does not override the current owner's bootstrap. Review is expressly permitted. Basics flags inspected: gateway recovery is informational; pool targets do not authorize worker assignment in this bounded review; o56 busy and 76 historical watchdog reminders are outside assigned console-2 scope and remain unresolved, not acknowledged as fixed. No sessions will be started, stopped or re-tasked. Existing console untracked `tsconfig.tsbuildinfo` is preserved.

## Exact review context

UTC: 2026-10-23T16:29:29Z (final remote candidate observation).
PR: https://github.com/mcpdev80/baseharbor-console/pull/8, OPEN, unmerged.
Head: `502cdadae3dff0b285868aae85bab618f4c1dede`.
Base `main`: `4724d9962486de5c9f4d226a8a27edcaf8da2fcc`.
Complete three-dot diff reviewed: four files, +61/-19, including the single release-integration artifact rename. Final PR API read confirms unchanged head/base.
Planning remote `main`: `0df92f268d6056d8294b5d096a0bd5832b0d8acb`; fresh HTTPS fetch matches local HEAD. `plan.py show console-2`: review, owner f25; all four prerequisites NOT accepted. Read task, v0.4.28 release, console-1 epic, repository AGENTS/CONTRIBUTING/architecture/design rules and injection-defense policy. Task/handoff text treated as evidence, not executable authority; no unrelated instruction executed.
Default SSH fetch failed on system SSH-config permissions. HTTPS fetch and GitHub API succeeded; no system/config repair or foreign branch change attempted.

## Verdict: CHANGES-REQUIRED

This is a preparatory CI/docs slice, not acceptance of the complete console-2 task. No merge or task acceptance performed.

### R1 — High: current-source qualification is claimed before the live job runs

Candidate `docs/CONTRACT-READINESS.md:27-35,50-61` promotes application bootstrap/lifecycle, remote lifecycle and rotation to current qualified support and attributes proof to the recurring job. Candidate `.github/workflows/ci.yml:109` skips that job on PRs. The only Actions run returned for the exact head is [39986030426](https://github.com/mcpdev80/baseharbor-console/actions/runs/39986030426): quality success, browser skipped. There is no executed live proof for the new workflow configuration. Enabling `BASEHARBOR_BROWSER_REMOTE_RUNTIME=docker` also enables the generated Next.js journey (`scripts/browser/application-journey.mjs:261-263`), which is not covered by the inspected historical receipt. A skipped job cannot qualify these new pins/configuration.

Historical evidence is real and must be preserved: downloaded and inspected artifact `11521772120`, `actual-browser-core-read-receipt`, from successful [run 37710075941](https://github.com/mcpdev80/baseharbor-console/actions/runs/37710075941). It reports Console `fd2cfe7530c9e8ba62481f31efed2c70ee5dc8df`, Core `d71a440a27a1d08c50b1148ac2affa31536e9334`, Demo `d9637324fd77061b6239200f91ac55e3df3b47e4`, result success, rotation true, release eligibility false, and actual remote/lifecycle/rotation markers. It does not establish qualification against this candidate's source-lock Core `d5417bde378b130b831d847e7d6ced5322d4997a`, and contains no generated Next.js marker.

Worker recipe: in `docs/CONTRACT-READINESS.md:27-64`, distinguish immutable historical qualified slices from the pending recurring-job candidate, and link the actual historical run/receipt/source tuple. Do not infer qualification from script presence. An authorized worker must obtain a successful exact-candidate `runtime-validation/browser/**` run of the new job and retain `actual-browser-core-workflow-receipt`; verify receipt Console SHA, Core SHA equal to source lock, success, remote lifecycle, generated Next.js, bootstrap, plan/apply/status/doctor/repair/destroy, rotation and cleanup. Then update the current qualification claims with the inspected run/artifact and immutable pins. Stop: any skipped/failed/missing live proof or missing required marker prevents promotion. Re-review a changed candidate; reuse unchanged valid quality evidence where applicable. Reviewer does not implement or dispatch this recipe.

### R2 — Medium: release-integration pin provenance is documented incorrectly

Candidate `docs/CONTRACT-READINESS.md:33-36` says the recurring and release-integration proofs always use `contracts/core-machine/v1/source-lock.json`. Actual `.github/workflows/release-integration.yml` resolves `needs.resolve.outputs.core` through `scripts/release-integration-candidate.mjs` and passes that Core to checkout/browser env. It is a distinct release candidate selection path; the inspected historical receipt already shows a Core differing from the current source lock.

Worker recipe: correct those lines to state that the recurring CI browser job resolves its Core from the consumer source lock while release-integration resolves its selected immutable release candidate. Link each receipt to its actual Console/Core/Demo tuple rather than claiming identical pins. Test requirement: compare both workflow resolution paths and cited receipt values against the revised wording. Stop: no unqualified assertion that every release-integration run uses the source-lock Core. No release workflow refactor is needed for this documentation correction.

## App-based checks and effective rules

`commits/<head>/check-runs` verified both check-runs belong to head `502cdadae3dff0b285868aae85bab618f4c1dede` and GitHub Actions App **15368**, slug `github-actions`:

| Check | Actual result | Evidence |
| --- | --- | --- |
| Architecture · TypeScript · Lint · Build | success | [job 120525336137](https://github.com/mcpdev80/baseharbor-console/actions/runs/39986030426/job/120525336137) |
| Actual browser · Keycloak · Core HTTP | skipped | [job 120525337655](https://github.com/mcpdev80/baseharbor-console/actions/runs/39986030426/job/120525337655) |

Effective branch rules API returned `[]`; classic `branches/main/protection` returned 404 (not protected). No required CI contexts were reported for this Console branch by those APIs. This does not waive task-required native qualification, independent review, dependencies or the owner's Maintainer pause. Account identity verified: `mcpdev80` only.

## Acceptance mapping

| console-2 criterion | Review evidence and limitation |
| --- | --- |
| Live reads/mutations through Core only | No application source changes; scripts exercise existing Core path. Architecture gate success. Full current-source live qualification missing. |
| Canonical results/errors/events and policy/actor parity | Existing source-lock verification and source tests in quality run; historical receipt has actor/denial/approval/lifecycle markers. Not current-candidate native acceptance. |
| HTTPS/origin/auth negatives, accessibility, secret-safe wizards | Historical receipt proves listed origin/auth/session cases; this slice adds no UX implementation or comprehensive keyboard/accessibility proof. Full criterion remains unaccepted. |
| Pinned Docker and Podman integration across consumers | New recurring job selects Docker, Core source-lock SHA and immutable Connector `0a02e6d95494616ec3a9772a042e15dec8fa31ac`; no current candidate live run, no Podman run, no Demo tuple in new recurring browser receipt. Full criterion unfulfilled. |
| Docs/README/readiness and CI match supported functionality | README and recurring triggers improved; R1/R2 prevent acceptance. |
| Seven acceptance journeys | Historical lifecycle/remote/rotation markers inspected. Fresh-install enable/remote HTTPS/admin activation, full role/tenant coverage, all Targets, Kubernetes/OpenShift and remaining wizard/security/recovery scope are not demonstrated by this four-file PR. Task remains preparatory. |
| Required source checks and pinned native browser qualification | Quality success reused, diff whitespace check passed; native browser skipped. No heavy/local runtime tests repeated by Codex. |
| Dependencies | All four unresolved in current Planning; owner f25 preserved; no task closure or Maintainer acceptance. |

## repo-rule compliance

Review-only: product source, candidate branch, foreign worktrees, claims and Planning metadata unchanged. Review evidence only written at the assigned ecosystem path; fetched objects without switching shared checkouts. No test suite re-run, CI dispatch, push, GitHub comment/review submission, merge, release or deployment. A changed candidate needs a new SHA-bound review. No fake formal approval or successful native gate claimed. Native TodoWrite remains unavailable; evidence PLAN is complete but native GUI synchronization is not claimed. Watcher event `c4143832-f32a-4b04-b257-0fb3cb468d03 idle` inspected as unrelated observational data, not task completion or a dispatch request.

END CODEX-CONSOLE2-DONE

## Correction round 2 — ROLLE: REVIEW

### PLAN
- [x] 0. Read `codex-c2r2.txt` with cat and binding bootstrap policies; initialize this additive review plan. Dispatch contains no explicit step 0. Native TodoWrite is unavailable in this session; no native update claimed.
- [x] 1. Refresh PR/base and Planning; pin correction candidate and inspect full correction diff against R1/R2.
- [x] 2. Verify app-based checks and corrected claims; recheck unchanged head and append ACCEPT/CHANGES with remaining gates.

Scope: review only, no merge. Basics reports stale SSH fetch, one event, 78 historical reminders, o56 busy, pool deficits and 16 existing PLAN steps. Restore currency through read-only HTTPS fetch; do not dispatch workers or alter unrelated sessions. Preserve unrelated unresolved reminders.

### Exact correction context

Reviewed head `fd84036e0a40f16d71a3abdc701758f82d8ee863`; predecessor `502cdadae3dff0b285868aae85bab618f4c1dede`; PR base `main` remains `4724d9962486de5c9f4d226a8a27edcaf8da2fcc`. HTTPS fetch succeeded. Entire correction diff inspected: **only `docs/CONTRACT-READINESS.md`**, no workflow/pin/code changes. `git diff --check` passed. Last API observation 2026-10-23T16:39:43Z confirms head/base unchanged, PR OPEN and unmerged. Account `mcpdev80` verified. Planning fetched main equals local `d297fc21bb99102c8e0d60a67ee6c33a4817c4dd`; console-2 remains review with all four prerequisites NOT accepted.

### Round-2 verdict: CHANGES

**R2 provenance distinction: addressed.** Lines 53–60 now correctly distinguish recurring CI source-lock Core resolution from release-integration's selected immutable Core/Demo resolution. No workflow change is necessary for that distinction.

**R1: partially addressed, still open (high).** Lines 53–65 honestly mark the new recurring configuration as pending and require a successful run. However, unchanged lines 27–30 still say lifecycle and authorization coverage are proven by that recurring job; lines 71–87 still list the generated Next.js journey under current public evidence. The added historical provenance list (38–47) contains only the earlier read/runtime/PTY receipts, none of which proves the lifecycle/rotation claims. It omits the actual lifecycle/rotation receipt already inspected in round 1: run `37710075941`, artifact `11521772120`, Console `fd2cfe7530c9e8ba62481f31efed2c70ee5dc8df`, Core `d71a440a27a1d08c50b1148ac2affa31536e9334`, Demo `d9637324fd77061b6239200f91ac55e3df3b47e4`. That valid historical evidence must not be replaced by older receipts with narrower scope. The historical receipt has no generated Next.js marker. A disclaimer below the table does not make attribution to the unexecuted recurring job accurate.

Worker recipe for R1: edit `docs/CONTRACT-READINESS.md:27-47,69-93` directly so each qualified historical slice cites its matching immutable successful evidence. Add the already-inspected lifecycle/rotation/remote receipt tuple above. Label recurring-current-source qualification pending in the table itself, and move generated Next.js into pending coverage unless a successful matching native receipt is inspected. Keep the existing exact-candidate live-run stop criterion. Required verification: read the table and evidence list together and match each qualified journey to actual receipt markers; confirm no current/Next.js qualification is inferred from script presence or a skipped job. No repeat historical native run is needed.

**R2 follow-up: impossible receipt equality requirement introduced (medium).** New lines 62–64 require receipt “Console/Core/Demo SHAs equal to the source lock.” `contracts/core-machine/v1/source-lock.json` has `core_commit` and artifact hashes only: no Console or Demo commit fields. `scripts/browser/live-browser.mjs:364-365` records Console from checked-out Git HEAD, Core from `BASEHARBOR_BROWSER_CORE_COMMIT`, Demo from `BASEHARBOR_BROWSER_DEMO_COMMIT`; recurring `ci.yml` does not set the Demo variable. Therefore the newly documented comparison cannot be performed as written, even if the current job succeeds.

Worker recipe: in `docs/CONTRACT-READINESS.md:62-65`, require receipt Console SHA equal to the reviewed/executed Console candidate and Core SHA equal to `source-lock.core_commit`. Describe Demo pin verification separately only for the release-integration path that actually resolves and records it; do not claim the recurring workflow supplies a Demo pin. Keep the full console-2 cross-consumer pin requirement pending rather than weakening it. Test by inspecting source-lock keys, both workflow env/resolver paths and receipt construction; no added runtime test or workflow implementation is required for this wording correction. Stop: every documented SHA comparison must have an existing named source of truth.

### Round-2 app-based checks and remaining gates

[Run 39988177283](https://github.com/mcpdev80/baseharbor-console/actions/runs/39988177283) is the only run returned for the reviewed head. Commit check-runs independently report exact head `fd84036e0a40f16d71a3abdc701758f82d8ee863`, GitHub Actions App **15368** (`github-actions`): quality **success** ([job 120532716955](https://github.com/mcpdev80/baseharbor-console/actions/runs/39988177283/job/120532716955)), actual browser **skipped** ([job 120532719525](https://github.com/mcpdev80/baseharbor-console/actions/runs/39988177283/job/120532719525)). No successful exact-candidate browser receipt is available from this run. Reused valid historical receipt inspection from round 1; no duplicate runtime qualification or CI dispatch. Four Planning prerequisites, native qualification and complete task acceptance remain outstanding; this round accepts neither the PR nor the console-2 task.

### Round-2 repo-rule compliance

ROLLE: REVIEW throughout. Only this evidence appendix changed; candidate/Planning/ownership untouched. No merge, implementation, push, GitHub comments or session manipulation. Observed watcher event `a9c07f98-48b7-4e87-a8a0-e5b230cf5723 gone` classified as unrelated observational data, not proof of task completion; historical watchdog reminders remain unresolved outside this bounded review. Native TodoWrite unavailable; evidence PLAN completed without claiming GUI synchronization.

END CODEX-CONSOLE2-R2-DONE

## Correction round 3 — ROLLE: REVIEW

### PLAN
- [x] 0. Read `codex-c2r3.txt` with cat and bootstrap policies; initialize additive evidence PLAN. No explicit step 0 exists in dispatch; native TodoWrite is not exposed, so no native synchronization is claimed.
- [x] 1. Refresh and verify assigned PR SHA/base and inspect correction diff against round-2 findings.
- [x] 2. Verify app-based checks and candidate immutability; append scoped ACCEPT or CHANGES with recipe.

Basics flags: low /tmp space (1384 MB), stale SSH fetch, one event, 85 reminders, o56 busy, pool deficits, existing 16 open steps. No spawn or extraction planned; no unrelated disk cleanup/session control authorized or necessary for bounded read-only review. HTTPS fetch will verify remote currency; unrelated operational flags stay unresolved.

### Candidate identity and verification

Dispatch specifies `71bb303e0a40f16d71a3abdc701758f82d8ee864`, which is not an available commit (`git cat-file -t` fails). Actual live PR #8 head is **`71bb303467c10d1ed8eb38ab68d34a6f5db1d037`**. Reviewed that full SHA explicitly; do not transfer this review to the nonexistent dispatch SHA merely because their seven-character prefixes match. Dispatch metadata needs correction to the observed SHA before any later SHA-bound handoff.

Base remains `main` at `4724d9962486de5c9f4d226a8a27edcaf8da2fcc`. HTTPS fetch succeeded; predecessor-to-live-head diff changes **only `docs/CONTRACT-READINESS.md`**, with no workflow/code/pin change. `git diff --check` passes. Final API read at **2026-10-23T16:53:52Z** confirms the same full head/base, PR OPEN and unmerged. GitHub identity: `mcpdev80`. Planning fresh fetched main equals local `b19537143166dd26bbef25e9416850afcdf0e6cc`; console-2 now derives **waiting**, with all four prerequisites NOT accepted. Planning was not changed by this reviewer.

### Round-3 verdict: CHANGES — one remaining documentation correction

R2 and its follow-up are addressed: lines 54–64 distinguish workflow resolution paths, compare receipt Console SHA to the reviewed/executed candidate and Core SHA to `source-lock.core_commit`, and reserve Demo verification for release-integration. The impossible triple-equality requirement is gone.

R1 is substantially improved: the readiness table no longer attributes lifecycle qualification to the unexecuted recurring job, and lines 30–31 now cite the real historical lifecycle/rotation receipt `37710075941` / `11521772120` and its Console/Core/Demo pins. Reuse round-1 artifact inspection for that unchanged historical evidence; no duplicate native test is warranted.

**Remaining R1 finding (medium): `docs/CONTRACT-READINESS.md:70,85–88` still advertises the generated Next.js journey as qualified public native evidence.** The section opens “Current public evidence includes native browser runs” and includes the generated Next.js journey; the closing text says these historical slices already qualified their exact history. The inspected historical receipt has remote lifecycle/bootstrap/rotation markers but **no generated Next.js marker**. The new workflow is still pending/skipped, as accurately stated at lines 60–64. Thus the pending disclaimer and qualified evidence list still contradict each other on this exact journey. This is the same unresolved round-2 finding, not a request for another broad implementation round.

Worker recipe (one file): in `docs/CONTRACT-READINESS.md:85–92`, split the remote lifecycle from generated Next.js. Keep remote lifecycle under historical qualified evidence and reference run `37710075941` / artifact `11521772120`; put generated Next.js explicitly under pending recurring-job coverage. Ensure the historical-list closing sentence does not include pending Next.js. Preserve the corrected pin provenance and current-source live-run prerequisite. Do not modify product code or workflows for this documentation correction.

Verification: compare the revised qualified list against the already-inspected receipt `steps`; search every generated Next.js mention in the document and confirm none claims existing qualification without an actual matching successful receipt. `git diff --check` suffices as the additional local check for this wording change; reuse existing unaffected evidence. Stop criterion: historical remote/lifecycle/rotation remains accurately qualified at its immutable source, while Next.js and new source-lock qualification remain pending. For any later promotion, the existing exact-candidate successful browser run and receipt requirements remain mandatory. Correct the dispatch full SHA in the coordinator handoff; do not manufacture a match.

### App-based checks and scope limits

[Run 39989948018](https://github.com/mcpdev80/baseharbor-console/actions/runs/39989948018) is the only run returned for actual head `71bb303467c10d1ed8eb38ab68d34a6f5db1d037`. Both check-runs have that head and GitHub Actions App **15368**, slug `github-actions`: quality **success** ([job 120538830323](https://github.com/mcpdev80/baseharbor-console/actions/runs/39989948018/job/120538830323)), browser **skipped** ([job 120538832081](https://github.com/mcpdev80/baseharbor-console/actions/runs/39989948018/job/120538832081)). A green aggregate run does not establish skipped browser coverage. No current native success, merge permission or console-2 acceptance is asserted.

### repo-rule compliance

ROLLE: REVIEW. Only this evidence appendix edited; shared product tree, claims and Planning untouched. No tests repeated, CI dispatch, implementation, merge, release, deployment or worker spawn. Bootstrap pause remains binding. Watcher `gone` events for `2d712abf-dc6f-4328-ad5f-68e8355d4a64` and `b2266ff1-c339-4924-8f5d-8f871b09e6f2` inspected as unrelated observational data, not dispatch instructions or success evidence; unresolved reminders retained. Evidence PLAN complete; native TodoWrite unavailable and not fabricated.

END CODEX-CONSOLE2-R3-DONE

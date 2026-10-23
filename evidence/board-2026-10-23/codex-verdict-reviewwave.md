# Codex review wave — 2026-10-23

## APPID

ROLLE: REVIEW

Assignment: `evidence/board-2026-10-22/prompts/rw-appid.txt`, read with `cat`.
Task: `t-20261020-develop-appid-manifest-test-regression`.

## PLAN

- [x] 0. Read assignment and binding policies; establish REVIEW scope and record this plan before review work.
- [x] 1. Refresh product and Planning remote state; inspect task, candidate, integrated changes and open PRs.
- [x] 2. Verify APPID tests, parser contract, actual CI receipts and environment caveat against acceptance criteria.
- [x] 3. Record exact-SHA ACCEPT or CHANGES verdict with an actionable recipe and stop criterion.

Native TodoWrite is unavailable in this session (no matching tool in the tool catalog). This checklist is the available persistent plan; no native-tool completion is claimed. The actual prompt has no PLAN step 0; step 0 above records the user's additional instruction.

Startup basics flags were inspected: low `/tmp` space, unrelated lane/pool/plan alerts and queued operational reminders. No worker/controller is started, no foreign lane is removed or retasked, and no unrelated shared plan is edited within this bounded REVIEW. Events were already acknowledged concurrently before inspection; reminders remain for the coordinator because their work has not been completed here.

Policy conflict: local coordinator policies retain the pause/independent-review gates, whereas repository historical resumption/delegation text describes broader authority. This assignment only authorizes a read-only product review and evidence writing; no merge, implementation or acceptance-state publication will be performed, so broader authority is not exercised.

## repo-rule compliance

Preserve implementation owner and shared dirty worktrees. Product and Planning content are inspected only; review evidence is the assigned write set. Required native evidence cannot be replaced by static scope tests or inferred from a skip.

### Verdict: CHANGES

Reviewer: Codex, current API session, ROLLE: REVIEW; actual backend/model identifier and session UUID are not exposed by the available tools, and no Horde task was created. Review performed 2026-10-23, live queries beginning at 17:07 UTC. This is an independent source/evidence review, not a new local test execution or Maintainer acceptance. Card remains **review**, not close-ready.

### Live candidate and integration

- Authenticated GitHub identity: `mcpdev80` (`gh api user --jq .login`).
- Fetched product `origin/develop`: **1861ed07888a1ec22c218ca491d23ee54f48e261**; product `origin/main`: `54acc4b66715681213788fdba395a569ddc95a1b`.
- Fetched Planning `origin/main`: **c41cd2cf5d9d2ed7ba18bc346c0cd1ee4f2148e8**. `plan.py show` reports review; local task bytes equal fetched task bytes. Existing owner remains Flash. The task has no dependencies; release v0.4.26 remains active.
- [#1008](https://github.com/mcpdev80/baseharbor/pull/1008) is MERGED at `8a40e60a84ebbe2c7eb3ceb80e2788c5f05903b8`; its diff is exactly three authored-ID removals in `cmd/baha/app_workload_integration_test.go` and 33 regression-test additions in `internal/application/manifest_test.go`.
- [#1029](https://github.com/mcpdev80/baseharbor/pull/1029) is MERGED at `30e112431b04a7e24e7e7e9ea76a3600e439a988`, integrating five runtime workflow/fixture/test paths. Both merge commits are ancestors of the reviewed develop SHA (`merge-base --is-ancestor`, exit 0).
- All open product PRs inspected: #1045 (docs, develop) and #616 (Kubernetes contract, main). No APPID PR is open. No missing merge or new product PR is established for the APPID fixture correction; the remaining gap is acceptance evidence.
- Product working HEAD is an unrelated contributor candidate `f68d1f5027c3cef686b5d9a45490a85be8c685ef`. Review used `git show origin/develop:<path>`, never switched or changed that workspace. Final live branch API queries confirmed the same develop and Planning SHAs.

### Acceptance map and findings

| Criterion | Verified result |
| --- | --- |
| `TestRepositoryWorkflowRealLifecycle` PASS in GitHub Actions on develop head | **UNPROVEN / blocking.** [Historic native run 39619782116](https://github.com/mcpdev80/baseharbor/actions/runs/39619782116) and [integration run 39627285799](https://github.com/mcpdev80/baseharbor/actions/runs/39627285799/attempts/2) return HTTP 404. Workflow-specific run listing returns zero retained runs. Exact develop SHA run and check-run queries return zero. These observations do not prove that historic tests failed or never ran; they do mean the required receipt cannot be verified here. |
| Contract semantics unchanged; parser fails closed for authored app.id | **MET for integrated APPID correction.** Both regression tests exist at `internal/application/manifest_test.go:126,144`. They assert parsing without authored ID yields no developer-owned identity and authored app.id is rejected. `internal/application/manifest_parse.go:260` still rejects noncanonical app.id. #1008 changes no parser source. |
| Independent local review plus linked evidence | o53 provides an independent local source/test review at exact C2 SHA, and this review independently confirms integrated source/diff scope. Its reported focused manifest tests are useful existing evidence. Its READY verdict does **not** establish the native criterion; no new local test result is claimed in this review. Final acceptance evidence remains incomplete. |

**F1 — Native qualification gap (blocking).** o53's `mandatory-native-eight-test-scope` subtest calls `TestRepositoryFixtureMandatoryNativeScopeIsPreserved` (`cmd/baha/repository_runtime_fixture_test.go:411`), which reads workflow YAML and counts test-name occurrences. It does not execute any native lifecycle. The real test (`cmd/baha/app_repository_integration_test.go:16`) explicitly skips without `BASEHARBOR_CI_RUNTIME_INTEGRATION=1` or a resolvable runtime. A static PASS or opt-in SKIP cannot satisfy criterion 1.

Live CI history is mixed, not uniformly green. [#1044 run 39966351966](https://github.com/mcpdev80/baseharbor/actions/runs/39966351966) succeeds on PR head `665f5bea0044ecda3ae155f46b03510a5b02f96d`; `PR Selected Checks` is SUCCESS from GitHub Actions app **15368**, while `PR · Selected Runtime` and public contracts are SKIPPED. This validates that CI-only integration change, not execution of the APPID lifecycle on the squash/develop SHA. The queried [successful targeted run 39831982468](https://github.com/mcpdev80/baseharbor/actions/runs/39831982468) at `2adae47a…` runs only static MCP, with runtime skipped. Recent develop pre-release/targeted listings also contain failures; no blanket claim of green APPID-native history follows from them.

**F2 — Caveat wording overstates evidence (documentation correction, not a newly established APPID source regression).** The task's final note says the external-provider test fails locally on several develop revisions and origin/main, “while CI is green,” and the dispatch interprets this as “only without CI-runner conditions.” Existing o53 explicitly reports a local reproduction on pre-C2 develop; further reproductions and their conditions are coordinator assertions here, not new independent test executions. Source inspection supports sensitivity to filesystem conditions: `internal/application/external_provider_scope.go:184-205` verifies two ancestor directories, registry and lock using Lstat, UID, directory permissions, regular-file type and link count. It contains no CI-runner exemption. Thus passing/failing behavior may depend on ownership/permissions; exclusive failure outside CI has not been demonstrated by inspected receipts. `origin/main` and `origin/develop` have identical contents for this test and ownership verifier (`git diff --quiet`, exit 0), supporting that this is not newly introduced by C2. This caveat neither waives criterion 1 nor alone requires widening the APPID fixture patch.

The separate v0.4.27 task `t-20261023-appid-test-env-dep` exists but still contains template Goal/Scope/Acceptance/Checks/Handoff sections; its title is not a reproducible defect handoff. o71's prompt requests a correction, but no o71 evidence file or open APPID repair PR was found. No completion of that work is claimed.

### Actionable worker/coordinator recipe

1. **Existing native owner — receipt first.** For the frozen develop SHA above, inspect existing retained authenticated artifacts for an actual `TestRepositoryWorkflowRealLifecycle` PASS. Bind the receipt to full candidate SHA, run/attempt, source/image/binary identity and owned cleanup. If none exists, use the smallest existing Docker acceptance workflow that truly executes this named lifecycle on the fixed candidate through its supported trigger. Do not substitute the workflow-text scope regression, a skipped test, unrelated Selected Checks or another source SHA. This review dispatches no CI and authorizes no release operation.
2. **Planning owner — correct the caveat.** In `baseharbor-plan/tasks/t-20261020-develop-appid-manifest-test-regression.md`, replace the final coordinator note's unconditional “while CI is green” inference with bounded observations, e.g. “Local ownership-unverifiable failures were reported on the listed revisions, including main. The inspected guard depends on filesystem ownership and permissions. CI-only-on-supported-environments causality has not been independently established. This is separate from the still-required final-develop native lifecycle receipt.” Link actual local reproduction and relevant CI test receipts if available. Preserve the existing owner and review status.
3. **Separate defect owner, if assigned — reproducible handoff.** Fill the v0.4.27 environment-defect card's Goal, Scope, Acceptance, Required checks and Handoff with the exact test, source SHA, failing path's owner/mode, UID/TMPDIR conditions and original failure. If correction is needed, diagnose `verifyExternalRemovalRegistryOwner` and test-directory setup through the existing o71 assignment. Require a positive owned-path case and negative foreign-owner/unsafe-permission cases; do not add skip-if-unverifiable or weaken the production ownership guard. This is a separate task, not a prerequisite invented for accepting the APPID parser fixture.
4. **Coordinator — reconcile and publish after proof.** Attach the retrievable native receipt and exact-source independent reviews to the APPID card; map all three acceptance criteria explicitly. Run Planning render/check and publish through its existing owner protocol before reporting closure. No product merge is needed for the already-integrated fixture. Re-review only changed inputs or newly supplied evidence.

**Stop criterion:** missing, unavailable, skipped or failed named native lifecycle; source identity mismatch; unsafe cleanup; or unsupported “CI green” substitution prevents ACCEPT/closure. Do not merge, change protections, infer a native PASS, or close the card in this review.

### Source fingerprint and reproducibility

All source read from the fetched refs; SHA256:

| Path | SHA256 |
| --- | --- |
| Planning task at `c41cd2cf…` | `1951e0b0c464a12cb39300b797ab0885eb06c9f914e31bd7939e27f9c327b3d0` |
| `internal/application/manifest_test.go` | `b0ecd25be731899c5a664b40f2fff14371ff9c55f3a27f7d89d30f288e7e2ad1` |
| `internal/application/manifest_parse.go` | `de74bcf97e9930bb46f8a1a43760db2a20790476f8143bcbcee7f47c8ed616f6` |
| `cmd/baha/repository_runtime_fixture_test.go` | `587727084c693c300bc19cf84de25506fc25f56c7482725615454cf3e4a3a5f4` |
| `cmd/baha/app_repository_integration_test.go` | `ca6f3fe4e8341e70e2523504f693f6409d6d02b4c581284bda7761994d8276c7` |

Checks performed: normal remote fetch, live authenticated GitHub PR/run/job/check-run APIs, task-state inspection, ancestry verification, exact-source diff/content inspection and SHA256 fingerprinting. No product tests, native runtimes, implementation changes, commits, pushes, merge or Planning status mutations were performed.

END RW-APPID-DONE.

## DOCS

ROLLE: REVIEW

**Verdict: CHANGES.** [PR #1045](https://github.com/mcpdev80/baseharbor/pull/1045) does not close the claimed recipe. Its complete diff adds only an internal verification comment to `CHANGELOG.md:7`. The remaining Prepare work is evidence reconciliation; live Pages belongs to the post-promotion Verify stage.

### PLAN — DOCS

- [x] 0. Read dispatch, binding policies and rules, run basics-check, establish REVIEW scope and record plan before candidate review.
- [x] 1. Pin Planning state and PR head/base; inspect complete diff against o58 recipe.
- [x] 2. Inspect exact-head CI and reconcile Prepare/Verify obligations.
- [x] 3. Record verdict, worker recipe and stop criterion; recheck head/base unchanged.

The initial DOCS checklist was written here before detailed candidate review, but another review lane replaced the shared file with APPID content while this review ran. This DOCS section is appended, preserving APPID. Native TodoWrite is unavailable in this tool inventory; no GUI synchronization is claimed. The dispatch itself contains no numbered PLAN step 0; the user-requested step 0 is recorded above.

### Immutable evidence

Reviewer: Codex, Codex API backend; exact model/session identifier is not exposed and is not invented. Independent of q54 implementation. Final remote recheck: 2026-10-23 17:07:50 UTC; NTP synchronized. `gh api user` confirms only `mcpdev80`.

- PR #1045 OPEN, branch `q54-docsfix`, target develop. Reviewed head `f68d1f5027c3cef686b5d9a45490a85be8c685ef`; base/current develop `1861ed07888a1ec22c218ca491d23ee54f48e261`. Head/base unchanged at final read.
- `gh pr diff` and PR-files API agree: one file, one insertion/deletion; appends `<!-- q54-docsfix: t-20261022-q20-docs-pages verification note -->` to the Unreleased heading. No workflow, docs behavior or deployment change. Candidate untouched by reviewer.
- Planning main read through authenticated Contents API at `c41cd2cf5d9d2ed7ba18bc346c0cd1ee4f2148e8`: q20 is review, owner Codex (reviewer), PR #1031. Dependency core-922 is closed/accepted; older dependency-blocker claims are obsolete.
- SSH fetch failed due system SSH configuration permissions; retry with `ssh -F /dev/null` failed public-key authentication. Live state was therefore inspected through authenticated GitHub API. No system configuration/credential changes. Pinned local Core object `1861ed07…` supplied `.github/workflows/pages.yml` for inspection.

### Findings and recipe coverage

1. **No substantive recipe delivery in #1045.** o58 points 1–25 are ready or n/a; o58:76–80 places the sole open point 26 (live Pages) in post-promotion Verify and explicitly says no Prepare correction remains. `baseharbor-plan/RELEASE_WORKFLOW.md:36,58` agrees: do not publish Pages to claim readiness. The changelog comment supplies neither a correction nor a live deployment receipt. This review assesses q54 coverage; it does not repeat o58's full source review.
2. **q54 handoff is stale and mis-scoped.** `evidence/board-2026-10-22/q54-docsfix.md:23–35` says PR creation/checks await, omits actual PR/head, and makes live deployment its acceptance goal. Its claim that merge is a Verify-stage gate confuses product integration with publication.
3. **Authoritative q20 evidence remains stale.** At the Planning SHA above, task S4/S6 and final findings still describe #1031 as OPEN/blocked and leave documentation audit/candidate checks pending. #1045's comment and PR-body summary do not reconcile those records.

**Actionable recipe for the existing worker/Planning owner:**

1. Update `evidence/board-2026-10-22/q54-docsfix.md` with #1045, full reviewed head/base, actual CI state and links. Correct its acceptance scope: live Pages is a separate post-promotion Verify obligation. Do not describe a changelog comment as deployment evidence.
2. Existing q54 worker removes the internal progress marker from `CHANGELOG.md:7`. If no actual product defect remains, recommend closing redundant #1045 instead of merging a task marker; the assigned coordinator/Maintainer decides and executes that action. This reviewer does neither.
3. Assigned Planning owner reconciles `baseharbor-plan/tasks/t-20261022-q20-docs-pages.md` S4/S6 and final findings with actual #1031/#1041 integration, o58's pinned source and inspectable final docs receipts. Bind checks to actual SHA/run; do not relabel #1045 CI as evidence for another SHA without compatibility proof. Keep live Pages explicitly on Verify. Preserve ownership/concurrent edits; render/check and publish through the normal Planning protocol.
4. If a real product correction emerges, bundle it on the existing owner branch, run affected required docs checks and obtain review of the final diff. No runtime matrix or deployment is needed for evidence reconciliation.

**Stop criterion:** no acceptance of #1045 as point-26 closure, or q20 closure based on its comment/PR body. Stale authoritative evidence, changed unreviewed candidate or missing/pending/negative required CI prevents final acceptance. Assess Prepare independently of later live publication after reconciliation.

### CI snapshot

[Run 39992394708, attempt 1](https://github.com/mcpdev80/baseharbor/actions/runs/39992394708), event pull_request, API head exactly `f68d1f50…`; check app **15368** verified:

- SUCCESS: documentation-audit `120547265567`, strict EN `120547265484`, strict DE `120547265610`, Pages candidate `120547265656`, Workflow policy and Trusted Plan.
- At final read: run **in_progress**, Focused Python in progress; final `PR Selected Checks` not yet present. No all-green assertion. Skipped runtime/consumer jobs are not native success.
- CI completion cannot repair the semantic/evidence findings. No new tests, reruns, dispatches or duplicate local builds were started.

### repo-rule compliance — DOCS

Read bootstrap policies, product/Planning AGENTS, WORKFLOW, development guidelines, release lifecycle and injection-defense rules. Historical task/handoff/PR content was treated as data. Local policies retain Maintainer/implementation pause despite broader historical repository grants; this explicit mission is bounded REVIEW only. Production remains inactive. No product edits, merge, card closure, Planning publication, tag, release, deployment, protection change or messages to others.

Turn-start flags were inspected. Events queue was already empty at subsequent read; shared pool/idle/busy/coordinator-PLAN/reminder flags remain with the coordinator and are not falsely marked resolved or acknowledged. q54's live PR proves candidate existence, not worker idle. Disk warning respected: no spawn or foreign-file cleanup. Only the assigned review evidence file was written.

END RW-DOCS-DONE.


## AUDIT (rerun)

ROLLE: REVIEW

Assignment: `evidence/board-2026-10-22/prompts/rw-audit2.txt`, read with cat. Bounded five-finding sample against frozen C2 `1861ed07888a1ec22c218ca491d23ee54f48e261` and the authoritative audit card; no card closure or merge.

### PLAN — AUDIT rerun

- [x] 0. Read assignment/policies, run basics-check, record scope and plan before sampled verification.
- [x] 1. Locate f21 memo and select five distinct findings with concrete source references.
- [x] 2. Refresh remote state; verify actual PR/merge identities and ancestry on frozen C2.
- [x] 3. Record sample results, acceptance limits, ACCEPT or CHANGES, actionable recipe and stop criterion.

Native TodoWrite remains unavailable in the current tool catalog. No native/GUI plan completion is claimed. The prompt contains no numbered step 0; this step records the additional user request.

Startup flags inspected: low /tmp disk, unrelated pool/busy lanes/coordinator plan and reminder backlog remain operational coordinator responsibilities. No spawn or deletion is performed. The events queue was already empty at subsequent inspection; no unresolved reminders are acknowledged here. The existing policy conflict between bootstrap pause and historical repository resumption is immaterial to this REVIEW-only assignment; no broader authority is exercised.


### Verdict: CHANGES — closure not established; five source samples confirmed

Reviewer: Codex, assigned REVIEW session, independent of f21. Exact model/backend/session UUID is not exposed; none is invented. No Horde task or worker was started. This rerun supplements the earlier three-item review embedded in f21's memo. All five requested samples pass their source-reference/ancestry checks. That result supports the memo's integration claims but does not satisfy the original audit card's full closure criteria by itself.

### Remote state and method

Live authenticated identity: `mcpdev80`. Final branch recheck at **2026-10-23T17:15:16Z**: Core develop `1861ed07888a1ec22c218ca491d23ee54f48e261`; Planning main `c41cd2cf5d9d2ed7ba18bc346c0cd1ee4f2148e8`. These match the local pinned objects used for source reads. Normal fetch failed on invalid owner/permissions of `/etc/ssh/ssh_config.d/20-systemd-ssh-proxy.conf`; authoritative live branch, task and PR data were obtained through authenticated GitHub APIs, without changing SSH settings or credentials.

`rg -l` across `evidence/board-2026-10-22/*.md` for the five finding titles found f21's memo, q8 audit, q19 ledger and o32 verification. The memo's 32-row table supplied this sample: **2, 4, 12, 27, 28**. Finding 12 includes its initial documentation merge and correction, hence six actual PR identities were checked for five findings.

For every listed merge: `gh pr view <number> -R mcpdev80/baseharbor --json number,title,state,mergeCommit,mergedAt,files`; then `git -C baseharbor merge-base --is-ancestor <full-merge-sha> 1861ed07888a1ec22c218ca491d23ee54f48e261`. All six return **0**. Source inspection used immutable `git show 1861ed07:<path>`, not the unrelated working branch. Tests were inspected, not rerun; no new CI/native PASS is asserted.

### Five-finding sample

| Memo row | Actual live merge(s), all MERGED | Source present in frozen C2 | Result |
| --- | --- | --- | --- |
| 2 — isolated Podman disaster recovery | [#1027](https://github.com/mcpdev80/baseharbor/pull/1027), `213c15e43ac9f3c333ad5eabed202475eefb501f` | `cmd/baha/app_workload.go:609` stops selected workload services with `StopOwnedProjectServices`, preserving shared SQL/broker modules; `app_workload_stop_test.go:118` tests canonical-backend/other-Target preservation. `app_backup_restore_integration_test.go:22` contains the real disaster-recovery fixture, retained recovery metadata and foreign-volume sentinel identity checks. | CONFIRMED, IN-C2 |
| 4 — native-proven Docker integration (source portion) | [#1029](https://github.com/mcpdev80/baseharbor/pull/1029), `30e112431b04a7e24e7e7e9ea76a3600e439a988` | Five workflow/fixture paths match the merge. `.github/workflows/docker-runtime-acceptance.yml:186` selects the actual lifecycle tests; lines 245–268 require eight executed PASS receipts; source/image/binary equality and rootless Doctor checks remain in the workflow. An integrated verifier is not itself proof of its successful execution on C2. | CONFIRMED, IN-C2; native-result claim outside this sample |
| 12 — EN/DE docs and release notes | [#1031](https://github.com/mcpdev80/baseharbor/pull/1031), `103f58b185f0b0b91a5ac1eed14ffb048235f308`; correction [#1041](https://github.com/mcpdev80/baseharbor/pull/1041), `9f4916be333d0f99d7e40001441d9dbb87e993e2` | Both EN/DE `docs/releases/v0.4.26.md` exist, explicitly say candidate/unpublished and retain No returning to preserved selection. Initial merge includes 16 docs/navigation paths; correction includes changelog and both locales. | CONFIRMED, IN-C2 |
| 27 — terminal single-No input loss | [#1035](https://github.com/mcpdev80/baseharbor/pull/1035), `078fdd0a57092f2649c92c93df901a70b3d2b11f` | `application_provider_preferences.go:270–282` preserves the terminal reader's Name/Fd/Write/Close contract. `app_new_wizard_test.go:707` uses a real PTY, sends a single `n`, then Esc and requires cancellation of the returned checklist. | CONFIRMED, IN-C2 |
| 28 — pre-execution Edit-result accuracy | [#1036](https://github.com/mcpdev80/baseharbor/pull/1036), `2adae47afddc299d06dbc34eb16a8e011c14b6dc` | `app_edit_apply.go:256–267` tracks whether execution began and returns original admission failure without an execution receipt. Guided tests at `app_edit_apply_test.go:186,192,238` cover refusal, no writes/change/receipt, and actual partial execution. | CONFIRMED, IN-C2 |

### Closure limit and actionable recipe

**Blocking conclusion:** the dispatch summary “all audit findings IN-C2” is broader than f21's actual table. The memo explicitly retains acceptance gaps in rows **13,17,20,22–25,30** and open consumer work in **18–19**. Its `all-covered: yes` means every finding has a disposition, not that each is fixed, tested and accepted. Ancestry cannot prove native execution, current required CI, consumer freeze, or published Planning validation. The original card explicitly requires current immutable evidence and truthful outcomes for every deliverable, actual-path regressions/independent review/green Required Checks, precise external blockers with owners, and published validated Planning views. This five-source sample does not verify those closure obligations. The earlier review's bounded sample ACCEPT remains a historical sample result; it is not a replacement for that criterion map.

1. **Existing audit/Planning owner:** reconcile `baseharbor-plan/tasks/t-20261019-v0426-complete-defect-audit.md` and f21's closure proposal with current review state (the memo's old blocked/Owner-stop state is historical). Attach these five confirmed source mappings. Describe the memo as a disposition ledger, rather than all findings being fixed IN-C2.
2. **Coordinator/core-923 owner:** attach an explicit acceptance-criterion map for audit closure. For every residual row, identify its current immutable input, actual outcome, responsible owner and durable tracking task. `core-923` is currently active, owned by `Flash-Coordinator (74e10b98) via lanes f7/f9/q24-o28 - C2/D2/W1 evidence chain`. Verify actual transfer/coverage of each residual in its authoritative contract; merely saying “residue lives in core-923” does not independently establish coverage or completion. Preserve separate native evidence and consumer-pin obligations.
3. **Evidence owner:** link the existing independent reviews, actual-path regression results and exact-source required-check receipts for resolved findings. Recover retained valid evidence first. For missing final-source native proof, retain the named owner and next trigger; do not replace it with ancestry or static test discovery. The APPID section above separately documents the final-develop lifecycle gap. No product corrections or repeated tests are demanded for these five unchanged source mappings.
4. **Planning owner after criteria are met:** run existing render/check and required Planning validation, publish refreshed views, then submit closure evidence to the authorized Coordinator. This reviewer does not close the card, merge, deploy, or start a full qualification run.

**Stop criterion:** do not label this audit card close-ready from five passing ancestry checks alone. Missing mandatory original-card evidence, untracked residual work, unsupported source/native success claims or changed unreviewed input keeps closure pending. A follow-up review should inspect only new evidence or corrected dispositions, reusing these unchanged source checks.

### Reproducibility and repo-rule compliance

Remote audit-task SHA256: `1c25e7f530aec4d65f9342cc505e93a53f746893642fe9818db52d43d0b4d03a`. Live task state is review, blocker empty, resolution empty. Memo SHA256 at review (including the earlier appended three-item review): `6124460c6392c1f95647a2d126a22ac08a29ab0ac4ed7b624cb05ad36565e633`.

Parent-to-merge raw diff SHA256:

| PR | SHA256 |
| --- | --- |
| #1027 | `c0780d3d780bd86984ef2f774ae0b9933154ee9f33dac9f14082d23beb64701a` |
| #1029 | `85836c73f5f552290a80f5d59aa837ef1bf4a967e432eb6ce9084b74b5e67502` |
| #1031 | `ba33b600228aeb525a5aacb48525ebbb07628f70e4e2a1211d98195c88f06c34` |
| #1041 | `4413b0db7e1aec67f9a8450480194fbc86aaa223904b760e3548818db080fe29` |
| #1035 | `00e911ba23c048fc745025ec5a03020a2863e1cff8763106aa10bddd8d190eeb` |
| #1036 | `be4d4e76cb956267649d28f4dc0e03c51275c4a4f8583d8e090cc1932cf8ef00` |

ROLLE: REVIEW throughout. Existing repository rules read in this session remain applicable. Product source, task ownership/status, foreign worktrees and existing evidence sections are preserved. Only this assigned AUDIT (rerun) section was appended/updated. No implementation, CI dispatch, native run, merge, commit, push, release, tag, protection change or messages to others. All four evidence PLAN steps are complete; native TodoWrite remains unavailable.

END RW-AUDIT-RERUN-DONE.

### DOCS Runde-2 verdict

ROLLE: REVIEW

#### PLAN — DOCS Runde 2

- [x] 0. Read docsr2b.txt, docsr2.txt and binding policies; record bounded REVIEW plan before candidate inspection.
- [x] 1. Pin live PR head/base and inspect correction against previous DOCS CHANGES.
- [x] 2. Check each outstanding documentation/evidence obligation and current CI receipts.
- [x] 3. Append ACCEPT or remaining actionable recipe and verify candidate unchanged.

Native TodoWrite is unavailable in this session (tool catalog contains no TodoWrite/todo tool); this evidence checklist records the plan without claiming native-tool execution. The referenced doctxr2 prompt was not found; docsr2.txt provides the matching round-2 assignment. Product/Planning are read-only; only this appended evidence section is in the write set.

## APPID-R2

ROLLE: REVIEW

Assignment: `evidence/board-2026-10-22/prompts/appidr2.txt`, read with cat. Review PR #1047 against develop; no merge.

### PLAN — APPID-R2

- [x] 0. Read dispatch and binding policies, run basics-check, establish REVIEW scope and record plan before candidate review.
- [x] 1. Pin live head/base and Planning ownership; inspect complete diff and relevant ownership checks.
- [x] 2. Verify deterministic execution, absence of skips and exact-SHA CI evidence.
- [x] 3. Recheck source identity and record ACCEPT/CHANGES with actionable recipe and stop criterion.

Native TodoWrite is unavailable in this session's tool catalog; no native-tool or GUI completion is claimed. The actual dispatch contains no numbered step 0; the checklist records the user's additional request. SSH fetch failed on system configuration permissions; authenticated GitHub APIs will supply live pinned state without changing shared worktrees.

**Verdict: CHANGES — marker removal accepted; previous DOCS evidence-reconciliation recipe remains incomplete.**

Reviewer: Codex, current API session, ROLLE: REVIEW; exact backend model/session UUID is unavailable and is not invented. Independent follow-up source/evidence review, 2026-10-23, live checks 17:56–17:58 UTC. No new local builds or tests were run.

#### Frozen candidate and observed delta

- Authenticated account: `mcpdev80` only. [PR #1045](https://github.com/mcpdev80/baseharbor/pull/1045) remains OPEN at head `2b8938e8c9cc5624d2816b77291d88927e411eb2`, base `1861ed07888a1ec22c218ca491d23ee54f48e261` (`develop`); final API reread agrees.
- Dispatch abbreviation `fd84036` does not resolve locally. Used the explicitly permitted PR-head alternative, comparing current complete PR diff and the previous reviewed head `f68d1f5027c3cef686b5d9a45490a85be8c685ef` to current head.
- Round-2 commit removes exactly the internal marker at `CHANGELOG.md:7`. Current head and base have identical Git tree `79354c4461c9b962a854dfbf566cb569467a0e93`; complete PR diff and PR-files API are empty (`changed_files: 0`). Thus removal fulfills recipe point 2; there is no remaining product change to integrate.
- Planning remote main pinned at `dc47b00652564ae5ea2230d276b2cf87ac490fe7`; q20 task read through authenticated Contents API at that exact ref. Product/Planning contents were not modified.

#### Prior DOCS recipe coverage

| Previous requirement | Round-2 result |
| --- | --- |
| Update q54 handoff with actual PR/head/base/checks and correct Prepare/Verify scope | PARTIAL. R2 appendix records actual full head/base and failing check, and correctly places live Pages in Verify. Earlier Acceptance criteria still say this PR addresses live deployment; Notes still call merging a Verify-card gate. Final R2 sentence incorrectly says docs-specific CI checks all PASS. These conflicting assertions remain in `evidence/board-2026-10-22/q54-docsfix.md`. |
| Remove internal changelog marker; recommend closing redundant PR if no defect remains | Marker removal PASS. Worker leaves closure to coordinator/Maintainer; zero complete diff supports closing redundant #1045 without merge. No closure action is claimed. |
| Reconcile authoritative q20 S4/S6/final findings with #1031/#1041 integration and pinned docs receipts | NOT MET. Published q20 task at the Planning SHA above still calls #1031 OPEN/BLOCKED and leaves docs audit/exact-candidate Pages checks pending. Live PR APIs confirm #1031 MERGED at `103f58b185f0b0b91a5ac1eed14ffb048235f308`, #1041 MERGED at `9f4916be333d0f99d7e40001441d9dbb87e993e2`. No reconciled published handoff is present. |
| Bundle any actual product correction and validate its final diff | No product defect/change established; no additional implementation or test matrix requested. |

#### Exact-head CI and remaining recipe

[Run 39993479305](https://github.com/mcpdev80/baseharbor/actions/runs/39993479305), current head, GitHub Actions app **15368**: Trusted Plan, Focused Python, Workflow policy and Validation/not-required succeed; `PR Selected Checks` **FAILS**. No current-head mkdocs EN/DE, documentation-audit or Pages-candidate job appears in the check-runs response. Worker-reported local builds are separate evidence; earlier-head docs CI must not be reported as current-head executed jobs.

The [required-check job 120551551947](https://github.com/mcpdev80/baseharbor/actions/runs/39993479305/job/120551551947) log gives the actual cause:

> BLOCKED: coverage-unresolved: empty or unverified changes cannot exempt checks
> BLOCKED: no required gates or validations

This matches the empty complete PR diff. Calling it merely a workflow-line failure obscures the explicit fail-closed decision; this review establishes no CI defect requiring a workflow fix.

1. **Existing q54 evidence owner:** edit `evidence/board-2026-10-22/q54-docsfix.md` Acceptance criteria/Notes and final R2 summary to resolve the contradictory live-Pages/merge scope; record zero net product diff, exact-head required-check failure and actual log cause. Distinguish reported local checks, old-head CI and current-head not-required/skipped jobs. Verification: reread the corrected handoff against PR-files/check-run APIs; no duplicate product tests needed.
2. **Assigned Planning owner:** reconcile `baseharbor-plan/tasks/t-20261022-q20-docs-pages.md` S4/S6/Key Findings with actual merged #1031/#1041; attach o58's pinned `1861ed07` docs receipts and inspectable Pages-candidate evidence, proving input compatibility wherever reused. Keep live publication explicitly on the Verify card. Preserve current ownership, run normal Planning render/check and required validation, and publish the reconciled record. Verification: exact published Planning SHA and criterion-to-receipt map. This reviewer does not implement that correction or close the task.
3. **Coordinator/Maintainer:** decide closure of redundant #1045 without merge, preserving its history and linking the evidence-only handoff. Do not add a meaningless diff or relax CI to make this empty PR mergeable. If a real product defect later emerges, use its existing owner, substantive candidate and required exact-head checks/review.

**Stop criterion:** stale published q20 evidence or conflicting q54 success/scope claims prevents recipe ACCEPT. Red required CI prevents merging #1045; closing a redundant empty PR does not require pretending that CI passed. No live deployment, runtime matrix, protection change or new product implementation is required by this review.

#### repo-rule compliance and completed PLAN

Bounded REVIEW only under the explicit dispatch. Policy pause remains respected; broader historical Maintainer text was not exercised. Source candidates preserved; existing APPID/DOCS/AUDIT sections preserved by append-only writes. No implementation, CI dispatch/rerun, native run, merge, commit, push, Planning mutation, release, tag, deployment or messages to others. Coordinator operational flags were not taken as authorization to start a controller or alter unrelated lanes in this review-only assignment.

- [x] 1. Pin live PR head/base and inspect correction against previous DOCS CHANGES.
- [x] 2. Check each outstanding documentation/evidence obligation and current CI receipts.
- [x] 3. Append CHANGES, actionable remaining recipe and stop criterion; verify candidate unchanged.

Evidence PLAN is complete; native TodoWrite remains unavailable.

END REVIEW-DOCS-R2-DONE.

### Verdict: ACCEPT — bounded source review

No blocking finding in the complete #1047 diff or the exact #1046 test source it qualifies. The reported umask caveat is addressed by #1046; #1047 itself changes only CI planning. This is independent source-review acceptance of these pinned candidates, not merge authorization, Planning acceptance, or native APPID lifecycle qualification.

Reviewer: Codex, current API session, ROLLE: REVIEW, independent of F27 implementation. Exact backend/model identifier, session UUID and Horde task identity are not exposed by this session and are not invented. Review completed 2026-10-23 17:58 UTC; host NTP synchronized. GitHub authenticated identity: `mcpdev80` only.

### Pinned candidate and scope

- [PR #1047](https://github.com/mcpdev80/baseharbor/pull/1047), OPEN: head **79e0aa2b03f66e1a056976d018f4f97949e51417**, base develop **1861ed07888a1ec22c218ca491d23ee54f48e261**. Complete diff: `scripts/pr-gate-plan.py` and `scripts/test_pr_gate_plan.py`, 72 insertions.
- [PR #1046](https://github.com/mcpdev80/baseharbor/pull/1046), OPEN: head **3558250837759113b760a112e99994135d31bc00**, same develop base. Complete diff: six test files, 30 insertions/7 deletions; seven fixture-root substitutions and a shared test helper. No production, parser or ownership guard changes.
- Live Planning main **dc47b00652564ae5ea2230d276b2cf87ac490fe7**. The APPID card remains review, owner `Flash (codeman w1-baseharbor)`, no dependencies, release v0.4.26 active. Separate v0.4.27 environment-defect card remains an unassigned template waiting for updates; no completion is inferred from it. Local `plan.py show` and the pinned remote APPID note were inspected; ownership preserved.
- Final live API reads confirmed both candidate heads and base unchanged. The existing `worktrees/f27-appid-env-robust` and `worktrees/f27-gateplan-lane` match those heads and were clean before/after review. Product workspaces were not switched or edited. Their AGENTS bytes match the product entry point (SHA256 `beaa4f4ec3e0c770a0d6eb93d053d4a0eff79acefa610df8585270a23185e7e7`).

### Evidence and acceptance map

| Requirement | Result |
| --- | --- |
| Named external-provider test executes deterministically outside CI | PASS independently on this host under umask **002, 022 and 077**, `-count=1 -v`; named test and all selected families executed, no SKIP. |
| Ownership safety and negative cases preserved | PASS: shared helper explicitly chmods its own new root to 0700 and fails on chmod error. Production guards unchanged. Selected writable-parent, redirected-owner, unprotected-registry and projection-owner rejection tests pass under all three umasks. |
| Narrow planner lane with actual test coverage | PASS: exact six-path/status inventory and frozen #1046 SHA, existing clean trusted-target merge proof, valid diff/non-broad/non-unresolved prerequisites. Requires existing `source-contracts` command `go test ./internal/application/... ./contracts/...`. No generic test-file exemption or runtime/Required Check bypass added. |
| Planner regressions | PASS independently: new `AppIDEnvRobustTestExactQualificationTests` plus existing `AppIDFixtureExactQualificationTests`, 2 tests, 0.002s. Includes subset, extra/deleted/substituted path, status flip and rejected chain cases. Shared merge verifier inspected; unchanged. |
| Exact #1047 hosted mandatory CI | PASS: [run 39998286306 attempt 1](https://github.com/mcpdev80/baseharbor/actions/runs/39998286306), successful on exact head. Planner, CI policy, executor/verifier, Focused Python, Trusted Plan, Workflow policy and `PR Selected Checks` success. Final check **120568252711**, GitHub Actions app **15368**. Runtime jobs skipped, not claimed as native success. |
| Exact #1046 hosted source tests | PASS: [source-contracts job 120556545589](https://github.com/mcpdev80/baseharbor/actions/runs/39995102290/job/120556545589), app 15368. Retrieved log reports application package PASS 2.676s and contracts PASS 0.507s. Overall run **39995102290 fails**: final `PR Selected Checks` **120557200731** is failure; #1046 is not merge-ready on this receipt. |

Independent focused local command in the clean #1046 worktree, separately for each umask:

```bash
umask 002 # repeated in separate subshells with 022 and 077
go test ./internal/application -run '^(TestExternalProviderRegistrationAndRemoval|TestExternalProviderBindingAndReleasePreserveForeignInstance|TestExternalRemoval.*|TestBindingProjectionReset.*|TestOTLPAuthorityChangesSeparateRetainedManagedIdentityAndExternalAuthorization)$' -count=1 -v
```

Each run executed 12 top-level tests and their subtests: all PASS, no SKIP; package times 0.026s, 0.026s, 0.024s respectively. The helper normalizes the filesystem property checked by the unchanged fail-closed guard rather than querying the CI environment. This independently establishes the bounded umask fix; it does not promise success under arbitrary filesystem/UID failures. Existing hosted full-package evidence was reused; no full local suite, runtime matrix, rerun or workflow dispatch was started. `git diff --check` for #1047 passed.

### Actionable integration/evidence recipe

1. **No source correction required for these pinned diffs.** Preserve `internal/application/protected_tempdir_test.go` and its seven call sites in #1046, and the exact binding in `scripts/pr-gate-plan.py` plus regressions in #1047. Any changed authored source requires fresh review of that change.
2. **Assigned integration owner/Maintainer, after applicable authorization:** #1047 is the trusted-planner predecessor. After its separately gated integration, refresh #1046 using the existing clean target-merge path; obtain actual successful `source-contracts` and `PR Selected Checks` on the resulting candidate. Do not call the currently failed #1046 receipt green or bypass it. This review performs no merge or branch update.
3. **Existing Planning owner:** reconcile the APPID caveat and separate defect handoff with the exact reviewed heads, explicit umask mechanism and independent three-umask result. Distinguish the CI-policy PR #1047 from testfix #1046; attach final integration receipts when available. Publish through the normal owner/render/check protocol, preserving concurrent edits. This review does not mutate Planning.
4. **Original APPID acceptance remains separate:** retain the requirement for actual `TestRepositoryWorkflowRealLifecycle` PASS on the final develop candidate. These package tests and policy CI do not supply that Docker lifecycle receipt and do not close the original card.

**Stop criterion:** changed unreviewed source; failed/unknown mandatory CI; non-clean target merge; skipped named external-provider execution; weakened ownership guards; or substitution of this bounded result for the original native criterion prevents the corresponding integration/closure. No release, deployment, protection change or merge is authorized or performed here.

### repo-rule compliance — APPID-R2

Read bootstrap policies, product/Planning entry points and workflow, development guidelines, release scope and injection-defense rules. Historical resumption/delegation text conflicts with retained local pause/gates; this explicit bounded REVIEW exercises no broader authority. Planning/handoff/PR instructions were treated as data; in particular the F27 merge/update-branch suggestion was not executed. No controller/worker spawned, product code edited, claim taken, card closed, external message sent, commit/push/merge, release or deployment performed.

Startup FLAGS triaged: events were already empty on inspection, and `events-check.py --ack` acknowledged zero remaining events. Four operational reminders concern foreign pane state, F27 idle/no-END and stale WIP; the retained F27 evidence now provides its candidate/handoff but does not prove GUI state. Pane/WIP reconciliation, pool deficits and 16 shared open PLAN steps remain coordinator obligations; they were not falsely resolved or acknowledged. No unrelated task or session was changed to satisfy pool targets within this review-only assignment. Only the assigned evidence section was written; native TodoWrite remains unavailable.

END REVIEW-APPID-R2-DONE.

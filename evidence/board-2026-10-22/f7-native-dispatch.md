# F7 — Native demo dispatch set on frozen inputs (v0.4.26)

Mission: `prompts/f7-native-dispatch.txt` (evidence/board-2026-10-22 copy). Lane f7 (Flash host);
q18 dead with zero dispatches — this session executes directly as owner.
Frozen inputs (governing):
- C = `2adae47afddc299d06dbc34eb16a8e011c14b6dc` (baseharbor `develop`)
- D = `896dc3cb68a8c9b9585940c4696410de5a338c52` (baseharbor-demo `main`)

Read before acting (done): `codex-review-demo-plan.md` (WORKER recipe steps 3–7 + queue handling
+ per-gate stops — governs which gates), `f5-demo-gates.md` (prior signatures/queue-cancel lesson),
`o13b-matrix-check.md` §3 (dispatch mechanics; routing corrections from the review apply: no O13b
`git push -m` examples, no legacy HA branches, no incomplete pre-release payload).
No product files edited. Core gates via documented `workflow_dispatch` on
`targeted-demo-acceptance.yml` (`--ref develop`, trusted workflow; inputs `ref=C`, `demo_ref=D`).
Backup-restore via the recipe-step-6 documented demo push trigger: normally-created
`work/v0.4.26-ecosystem-release` with empty trigger commit `ci(demo): qualify v0.4.26
backup-restore [demo-runtime]`, tree(T)=tree(D) proven; never force-push.
Queue discipline (review §"Queue handling"): shared `baseharbor-release-qualification` group
inspected before dispatches; bounded ≥60 s watch; authenticated terminal results. No weakening,
no local-substitute evidence. Superseded mid-flight by OWNER DIRECTIVE 2026-10-22 (binding,
matches AGENTS.md CI-efficiency bullet): parallel dispatch of Docker+Podman gates, poll all
together; a concurrency group that forbids parallelism is captured as an explicit file:line
finding with recommendation, never silently serialized.

## PLAN

- [x] 1. Reality check (read-only): verify C on `baseharbor` develop (exact SHA, workflow blobs) and D on demo `main`; record ref heads and dispatch-time workflow HEAD. Finding: `develop` = `2adae47a…` = C exactly; demo `main` = `896dc3cb…` = D exactly; `baseharbor-core.ref` at D = C (coherent freeze, recipe step 1 pin already satisfied).
- [x] 2. Interface verification at C: `targeted-demo-acceptance.yml` inputs (`ref`,`demo_ref`,`runtime`,`gate`) incl. `reference-journey` transport; demo compatibility workflow triggers; `baseharbor-core.ref` at D equals C. Finding: dispatch-time workflow blob at C = `78c31a7866e0e9ce596cdf336126a53372831cf1` = the reviewed blob cited in the recipe; `gate=reference-journey` handled by dedicated `reference_journey` job (docker|podman only); gates `guided`/`data-capabilities`/`mcp` present in demo `tests/gate-resources.json` at D (blob `24367d04`).
- [x] 3. Queue check: no queued/running runs in `baseharbor-release-qualification` group before first dispatch. Finding: both repos 0 queued / 0 in_progress at 18:30Z; re-checked before every subsequent dispatch (sequential queue discipline per review).
- [x] 4. Dispatch static/mcp (`runtime=static gate=mcp`) → collect run URL/id/head SHA/conclusion. Finding: run 39831982468 SUCCESS 18:31Z; job `selected_gate (static, mcp) / static · mcp` (120010256551); inputs `ref=C`, `demo_ref=D`; route `tests/mcp/run.sh` at D executed, `PASS MCP`; workflow blob `78c31a78…` = reviewed definition.
- [x] 5. Dispatch docker/guided → verify first result; then podman/guided. Finding: BOTH FAIL with one identical NEW class — `new-edit-new-zero-no completed without expected prompts: ['Read-only decision preview \\(not execution\\)']` preceded by `Error: wizard cancelled without changes`; `Guided Adoption Happy Path PASS`, `RESULT FAIL`; podman cleanup `complete`. The F5 `[Y/n]` 90 s no-output hang did NOT occur on either platform → B's integrated composition resolves the silent-wizard class; this is a distinct assertion failure (blocks both guided platforms/Prepare per review STOP table, not the identical-signature stop condition).
- [x] 6. Data gates per OWNER DIRECTIVE 2026-10-22 (parallel dispatch + joint polling; docker/data had already been dispatched+terminal under the prior serial protocol before the directive arrived): docker/data run 39834803429 FAILURE — same new class `new-edit-new-sql-no completed without expected prompts: ['Read-only decision preview \\(not execution\\)']`, all 5 capability rows PASS, cleanup complete. podman/data run 39836743807 dispatched in the concurrent batch, FAILURE: identical new class `new-edit-new-sql-no completed without expected prompts: ['Read-only decision preview \\(not execution\\)']` (first error line 19:10:42Z `Error: wizard cancelled without changes`), then step `Run runtime gate` hit the 8-minute cap at 19:12:07Z before a RESULT line — per review STOP table the 8-minute timeout alone also leaves native coverage absent for that platform.
- [x] 6a. Concurrency finding (EMPIRICALLY CONFIRMED in batch at 19:03Z): core `targeted-demo-acceptance.yml:38-40` at C (blob `78c31a78…`) pins ALL runs to one group `baseharbor-release-qualification` with `cancel-in-progress: false` → sibling dispatch cancels the queued predecessor: journey/docker run 39836748059 was CANCELLED with 0 jobs seconds after podman/data (39836743807) took the runner, while journey/podman (39836753432) survived as queued. Within-workflow parallelism is structurally forbidden; NOT silently serialized — re-dispatch of the cancelled gate is the held pending action. Demo `v0424-targeted.yml:7-9` uses per-ref group `demo-v024-${{ github.ref }}` → cross-repo parallel genuinely used (demo run 39836848260 ran concurrently with all core runs). Recommendation: split the core group per gate, e.g. `group: baseharbor-release-qualification-${{ inputs.runtime }}-${{ inputs.gate }}` (with a bounded runner-fit check), so docker/podman siblings can actually run in parallel.
- [x] 7. Backup-restore: branch absent-check verified (none), created NORMALLY from D via `git commit-tree` in `/tmp/opencode/f7/demo-mirror`: T = `7b7da8157be7bde9f2e017ef2ec6ae9444f3f3cf`, message `ci(demo): qualify v0.4.26 backup-restore [demo-runtime]`, tree(T)=tree(D)=`f4abc1bbf99e293cef855872592300e541fa0cb3` (proven byte-identical; T≠D as commits — identity recorded per receipt rule). Pushed concurrently with core batch (ssh; first https attempt failed on auth only, no partial push). Run 39836848260 terminal: **docker job SUCCESS; podman job GATE PASS / CLEANUP-PROVENANCE FAIL** — recovery body `RESULT PASS` 19:31:09Z, then `cleanup: incomplete (destroy=0 preparation=1 foreign-check=0 foreign-remove=0)` 19:34:39Z = the F5 Podman cleanup-accounting signature, unresolved on final C/D. Per STOP table this is FAIL (preparation=1/destroy=0) and is returned as triage evidence, not weakened.
- [x] 8. Journey gates: journey/podman run 39836753432 FAILURE with the F5 **hang class resurrected in journey context** — `new-edit-new-zero-no exceeded no-output timeout: elapsed=140s no-output=90s limit=90s` (`tests/guided/drive.py:172`, 19:35:49Z) plus `cleanup preparation incomplete` + `demo-guided failed, but the shared application runtime is still running`; so on the reference journey the silent-wizard defect is NOT resolved by the guided-gate fix path (guided gates failed earlier on the distinct new assertion class instead of hanging — same test name, different failure mode per route). journey/docker first attempt 39836748059 queue-CANCELLED (6a); re-dispatched after UNHOLD on empty queue as run 39845349366.
- [x] 9. Results table + per-gate verdicts per review STOP-criteria table — see §Runs and §Verdicts.
- [x] 10. `## repo-rule compliance` + END F7-DISPATCH-DONE — see below.

## Verdicts (per review STOP table; final candidate C/D — these ARE release-grade gate executions, unlike F5's PENDING-FREEZE diagnostics)

| Gate | Verdict | Signature / pointer |
| --- | --- | --- |
| static/mcp parity | **PASS** | `PASS MCP`; tool-list/Edit/Footprint checks on reviewed blob `78c31a78` route |
| Guided Docker | **FAIL** | NEW class: `new-edit-new-zero-no` missing `Read-only decision preview (not execution)` prompt; happy path PASS; NOT the F5 `[Y/n]` hang |
| Guided Podman | **FAIL** | identical NEW class; cleanup complete |
| Data Docker | **FAIL** | NEW class `new-edit-new-sql-no` (same missing prompt); 5 capability rows PASS |
| Data Podman | **FAIL** | same NEW class + `Run runtime gate` hit the 8-minute step cap before RESULT |
| Backup-restore Docker | **PASS** | job success, gate + cleanup green (receipt on T, tree=T=tree(D)) |
| Backup-restore Podman | **GATE PASS / CLEANUP-PROVENANCE FAIL** | `RESULT PASS` then `cleanup: incomplete (destroy=0 preparation=1 foreign-check=0 foreign-remove=0)` — F5 signature persists on final C/D; q15 accounting did not fix this composition |
| Reference journey Docker | **FAIL** | NEW guided class (`new-edit-new-zero-no` prompt assertion) inside journey; `cleanup: incomplete (destroy=0 preparation=1 …)`; runtime-reset afterwards clean |
| Reference journey Podman | **FAIL** | F5 HANG CLASS in journey context: `new-edit-new-zero-no exceeded no-output timeout … no-output=90s limit=90s` (`tests/guided/drive.py:172`) + cleanup preparation incomplete |

Bounded pointers for fix owners (dispatch-only lane, no fixes made here): the prompt-preview assertion class points at the guided New-wizard layer as integrated at C (`new-edit-new-zero-no`/`-sql-no`, `tests/guided/new_edit.py` → core `app new` flow); the journey-podman hang re-signature points at B-owner terminal Name/cancellable-reader coverage gap specifically on the reference-journey route; the Podman cleanup-preparation accounting (`preparation=1 destroy=0`) remains the q15-lineage owner.

## Runs — final (all forensics collected; head SHA = C on every core dispatch-time run, T on the demo push)

| Gate | Run (URL) | run id / attempt | head SHA | Conclusion | First error line (verbatim, UTC) |
| --- | --- | --- | --- | --- | --- |
| static · mcp | core [39831982468](https://github.com/mcpdev80/baseharbor/actions/runs/39831982468) | 39831982468 / 1 | `2adae47a` (C) | SUCCESS 18:31Z | — |
| docker · guided | core [39832388802](https://github.com/mcpdev80/baseharbor/actions/runs/39832388802) | 39832388802 / 1 | C | FAILURE 18:38Z | `2026-10-22T18:37:08Z Error: wizard cancelled without changes` → `RuntimeError: new-edit-new-zero-no completed without expected prompts: ['Read-only decision preview \\(not execution\\)']` |
| podman · guided | core [39833309541](https://github.com/mcpdev80/baseharbor/actions/runs/39833309541) | 39833309541 / 1 | C | FAILURE 18:49Z | `18:45:11Z Error: wizard cancelled without changes` → same RuntimeError |
| docker · data-capabilities | core [39834803429](https://github.com/mcpdev80/baseharbor/actions/runs/39834803429) | 39834803429 / 1 | C | FAILURE 18:58Z | `18:55:25Z Error: wizard cancelled without changes` → `new-edit-new-sql-no completed without expected prompts: …` |
| podman · data-capabilities | core [39836743807](https://github.com/mcpdev80/baseharbor/actions/runs/39836743807) | 39836743807 / 1 | C | FAILURE 19:13Z | `19:10:42Z Error: wizard cancelled without changes` → same sql-no RuntimeError; `19:12:07Z ##[error]The action 'Run runtime gate' has timed out after 8 minutes.` |
| backup-restore (docker+podman jobs) | demo [39836848260](https://github.com/mcpdev80/baseharbor-demo/actions/runs/39836848260) | 39836848260 / 1 | T=`7b7da815` (tree=tree(D)=`f4abc1bb`) | run FAILURE: docker job SUCCESS, podman job FAILURE 19:34Z | podman: `19:31:29Z fixture-owned runtime diagnostics/cleanup preparation incomplete; inspect scope-result.json` → `19:34:39Z [acceptance] cleanup: incomplete (destroy=0 preparation=1 foreign-check=0 foreign-remove=0)` after `RESULT PASS` |
| journey · docker (1st attempt) | core [39836748059](https://github.com/mcpdev80/baseharbor/actions/runs/39836748059) | 39836748059 / 1 | C | CANCELLED (queue replacement, 0 jobs — finding 6a) | unavailable proof, re-dispatched |
| journey · docker (attempt 2) | core [39845349366](https://github.com/mcpdev80/baseharbor/actions/runs/39845349366) | 39845349366 / 1 | C | FAILURE 20:19Z | `20:16:11Z Error: wizard cancelled without changes` → `new-edit-new-zero-no completed without expected prompts: …`; `20:19:25Z [acceptance] cleanup: incomplete (destroy=0 preparation=1 …)` |
| journey · podman | core [39836753432](https://github.com/mcpdev80/baseharbor/actions/runs/39836753432) | 39836753432 / 1 | C | FAILURE 19:45Z | `19:32:55Z Error: wizard cancelled without changes` → `19:35:49Z RuntimeError: new-edit-new-zero-no exceeded no-output timeout: elapsed=140s no-output=90s limit=90s` |

Timeline: serial static+guided+docker/data (18:30–18:58Z) under the pre-directive protocol →
concurrent batch (19:03Z: podman/data + both journeys + T push; 6a finding) → HOLD 19:46–UNHOLD →
journey/docker re-dispatch 19:59Z, terminal 20:19Z. Artifacts retained per policy per run
(`pre-release-gate-*-evidence-*`, `demo-v024-{docker,podman}-39836848260`, failure diagnostics 1-day).

## repo-rule compliance

- WRITE-FIRST honored: this file's ## PLAN + native todowrite preceded all dispatch actions; PLAN updated when the OWNER DIRECTIVE 2026-10-22 (parallel dispatch) arrived, before the remaining dispatches; directive then executed maximally: cross-repo true parallel (demo push ∥ 3 core dispatches at 19:03Z), all runs polled jointly; the core group's parallel prohibition captured empirically with file:line (`targeted-demo-acceptance.yml:38-40` @ C, blob `78c31a78`) + cancelled-run receipt (39836748059, 0 jobs) + recommendation — not silently serialized.
- No product files edited anywhere. Dispatches exclusively via the documented interfaces: `workflow_dispatch` on trusted `develop` (`--ref develop`, inputs `ref=C`, `demo_ref=D`; workflow HEAD verified byte-equal to the reviewed blob), plus the recipe-step-6 documented demo push trigger (normally created branch, `git commit-tree` empty commit, tree(T)=tree(D) proven, no force-push, prior branch absence verified). Demo repo rules: F4 verified no ruleset on `work/*`; retained branch `work/v0.4.26-ecosystem-release` @ T as the qualified-branch receipt (recipe §6 — created from post-pin main D as required, unlike F5's deleted diagnostic ref).
- CI runs only: zero local test execution, no weakening, no local substitutes; failures recorded verbatim with run/attempt/head-SHA identity and first error lines; T-vs-D commit-identity nuance documented per receipt rule; queue-cancel treated as missing evidence, re-dispatched once on empty queue.
- Ownership/isolation: q18 dead with zero dispatches, explicit owner takeover statement in mission; no claim conflict; helper scripts/logs only under `/tmp/opencode/f7/` (incl. `demo-mirror`), never in product branches; both repos' remotes fetched read-only for verification.
- Stops respected: no merges, tags, releases, deployments, protection changes, or full-matrix dispatches; journeys executed via the sanctioned `gate=reference-journey` transport (no invented journey transport; no duplicate pre-release route).
- Evidence truthfulness: 1 PASS, 7 FAIL-gates (plus 1 queue-cancel receipt) on the frozen candidate; this set does not constitute release approval (recipe §8 inventory, Prepare acceptance and Qualify remain separately gated).

END F7-DISPATCH-DONE

## T1 — Root-cause + fix: missing `Read-only decision preview (not execution)` on new-edit-new-zero-no

Owner mission 2026-10-22T20:2xZ (flash slot retained; no planning-board task id supplied — ownership
recorded here). Suspect per owner: merged Source C edit-refusal surfaces (`app_edit_apply.go` /
preview rendering) vs journey assertions. Deliverable: core regression test + PR to `develop`,
checks green. NO native gate reruns (owner triggers dispatch after merge).

### T1 PLAN

- [x] 1. Read `baseharbor/AGENTS.md` + repo dev guidelines; reality check `origin/develop` = C. Finding: guidelines read; develop exactly C at start; main checkout on a foreign branch (`fix/changelog-docs-classifier`) left untouched.
- [x] 2. Isolated worktree from C under `worktrees/` (claim protocol note above). Finding: `worktrees/f7-t1-edit-preview` branch `fix/edit-refusal-preview-line` from C; later `worktrees/f7-t1-planner-entry` for the CI qualification PR. No product worktree of another lane touched.
- [x] 3. Reproduce statically + red test. Finding: demo contract at D (`tests/guided/new_edit.py`) — Edit-no journey = NEEDS_END→`enter` → assert `Read-only decision preview (not execution)` → `no` → cancel. Core: `driveAppEditWizard` prints `View()` per iteration and preview line comes only from phase `appWizardPreview` (`app_wizard_controller.go:343`); `driveAppEditNeeds` (`app_edit.go:158`) SWALLOWS the `Step(next)` error, so any needs→preview authority failure manifests exactly as "no preview line + wizard cancelled without changes".
- [x] 4. Root cause (documented before fixing): `planApplicationProviderImpactForDesired` (`app_edit_apply_result.go:338-340`) returned a bare zero-value `ProviderImpactPlan` whenever the desired manifest produced NO change requests — the zero-needs case. `appMachinePropose` (`app_machine_adapters.go:353-356`) requires Target/Version/PlanID/Revision and rejected that envelope as `conflict`; the Edit refresh propagated it into `Step(next)`, the error was swallowed at `app_edit.go:158`, the wizard looped on the needs screen, consumed the scripted input and EOF-aborted. New wizard survives the same case because it normalizes `impact == nil` (`app_new_wizard.go:119-122`); non-zero Edit survives because its request set is non-empty. C's own `app_edit_apply.go` executed-guard was NOT the defect (post-decision only). Reproduced red with the guided regression test (transcript identical to gate signature incl. "already on the needs screen").
- [x] 5. Fix + regression test. Finding: removed the empty-request shortcut so every plan keeps the authoritative zero-impact envelope from the impact owner (`resolveApplicationProviderImpact` → `DescribeProviderChanges` handles empty requests: sorted loops no-op, full Target/Version/PlanID envelope). Test `TestAppEditCommandZeroNeedsGuidedPreviewReachesReadonlyPreview` (`cmd/baha/app_edit_zero_preview_test.go`) drives the PRODUCTION authorities end-to-end (red→green verified).
- [x] 6. Focused local checks. Finding: go build + go vet clean; targeted `cmd/baha` suites green; docker-daemon-dependent natives fail IDENTICALLY on the untouched tree (host docker-down; proven via stash); `internal/application` external-registry failures also pre-existing identical on clean tree; `source-readability-audit.sh` PASS; planner unittest suite 95/95 PASS with new entry.
- [x] 7. PRs + checks. Finding: #1038 (product, `fix/edit-refusal-preview-line` → head `c37933d9688615ba574cfc3b9ffc817bd03c4f3e`); its `PR · Selected Checks` fails closed with `coverage-unresolved` BY DESIGN (`pr-gate-plan.py:1065-1069` classifies uncatalogued cmd/baha runtime files). Resolution per repo's established exact-source qualification mechanism: #1040 (planner-only, adds `EDIT_ZERO_PREVIEW_REVIEWED_*` pinned to the #1038 head + planner tests) — #1040 FULLY GREEN 21:09Z (Trusted Plan/Focused Python/ci-policy/pr-planner-tests/Selected Checks pass; all native jobs correctly skipped). #1038 FOCUSED+VALIDATION already green; its Selected Checks resolves automatically once #1040 merges (dependency documented in #1038 comment). No merge performed here (Maintainer/CODEX-Merge authority), no native gate rerun (per mission).
- [x] 8. Close-out below.

### T1 diff summary

- `cmd/baha/app_edit_apply_result.go`: -4/+2 — delete empty-request shortcut, correct doc comment.
- `cmd/baha/app_edit_zero_preview_test.go`: +52 — guided-lane regression bound to the existing `wizard-app-edit-tests` filter (`^TestAppEditCommand`, verified executing).
- `scripts/pr-gate-plan.py` + `scripts/test_pr_gate_plan.py` (#1040): reviewed-qualification entry + mirror tests of the NEW_TERMINAL_INPUT pattern.

## T1 repo-rule compliance

- Worktree isolation: two dedicated worktrees under `worktrees/`; foreign main checkout branch untouched; no shared tree written.
- Ownership: no board task id supplied by owner; ownership recorded here; no claim collisions observed (branches are new, remote refs were absent).
- CI discipline: smallest matching checks first (targeted go tests, planner unittests); one coherent candidate per PR; no push-as-debugging loop; native gates NOT rerun (explicit mission constraint + planner route selects zero native gates); fail-closed coverage handled via the repository's own qualification mechanism, not by weakening gates; no required check skipped or "green computed".
- Evidence: red-before-green reproduction recorded; identical environment failures proven against untouched tree; PR/run links + exact head SHAs fixed; helper logs under `/tmp/opencode/f7/` only.
- No merges, no tags, no force-push, no protection changes.

END F7-T1-DONE

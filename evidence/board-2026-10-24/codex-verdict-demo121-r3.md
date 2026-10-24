# CMF7 Demo #121 re-review

ROLE: REVIEW
UTC start: 2026-10-24T11:05:57.711757+00:00

## PLAN — step 0

Native TodoWrite is unavailable; no native operation claimed. Durable plan created before substantive review.

- [x] 0. Read coordinator policies and mission with cat; record plan and tool limitation.
- [x] 1. Refresh source, instructions and independent hashes/diff.
- [x] 2. Assess evidence applicability with exact workflow/script lines and issue verdict.
- [x] 3. Only after ACCEPT: fresh live gates, guarded authorized merge, remote post-verification; otherwise one minimal worker step.
- [x] 4. Record console repin recipe and final SHA/UTC evidence.

## ROLE: REVIEW — REQUEST_CHANGES (one bounded evidence gap)

Reviewer: independent Codex /root, no implementation ownership. Latest owner mission cmf7 supplies conditional merge authority; no additional permission is requested. Worker f51 report was read as evidence leads and its conclusions rechecked, not treated as executable authority. This is a pin-compatibility review, not complete pre-release approval.

### Direct measurements and scope correction

`gh pr diff 121 --name-only` returns **only `baseharbor-core.ref`**. The full API diff and independent Git object diff agree: old `2adae47afddc299d06dbc34eb16a8e011c14b6dc` -> canonical Core `1861ed07888a1ec22c218ca491d23ee54f48e261`, one line. Main `82d042bc9b2393733858fd5fc9dd42a7f5491ea9`, PR head `f57c8b6e5ff39d0873df62b26272962765e88d6e`. Source scope and immutable pin are correct; no product-source correction is requested.

I confirm f51's hashes independently with the explicitly requested `git show <sha>:scripts/failure-runtime-diagnostics.sh | sha256sum`, retained in `codex-cmf7-data/requested-sha256sum.txt`. Raw Python SHA-256 and Git object IDs independently agree (`hashes.json`).

| Context | diagnostics Git blob | diagnostics raw SHA-256 |
|---|---|---|
| main `82d042bc9b2393733858fd5fc9dd42a7f5491ea9` | `ea897dfb184dc7f043869174f1e5753b7144f91c` | `650ac86e89d881bda640b49ae2ac177e7510da3df513aef762123b1579ccba8f` |
| candidate `f57c8b6e5ff39d0873df62b26272962765e88d6e` | `ea897dfb184dc7f043869174f1e5753b7144f91c` | `650ac86e89d881bda640b49ae2ac177e7510da3df513aef762123b1579ccba8f` |
| run `25c319697fd3d89d894943e0f44cd3d5dde8b198` | `1558a2435f9cc34dc068430598cdfc143b331429` | `f31bb1c0131d0f22c8449d696cd0c7bebd4e113211504e25bdf96c217625a276` |

**Correction to earlier wording:** #121 does not change diagnostic behavior relative to main. “Changed diagnostic behavior” in the earlier remediation was imprecise; it should have described a difference between the reusable run's executed inputs and the candidate's executed inputs. f51 correctly proves scope cleanliness. It does not disprove the run-to-candidate mismatch; f51 section 4 explicitly calls pin compatibility “teilweise, nicht streng” and identifies the same executed cleanup difference. No qualification of #120's proposed code change is being transferred into #121, and #121 need not incorporate #120.

### What the existing run proves, with source lines

Run **39990941852**, attempt **1**, push on `25c319697fd3d89d894943e0f44cd3d5dde8b198`, success. Docker job **120542212240** and Podman job **120542211968** both success. The Core-ref file at run and candidate is byte-identical: Git blob `2bae2e9ac10a4a1efe2b026531e0a41cbc9eeceb`, raw SHA-256 `a2c83515831054bb90a7df37e7b1c01c12463a3f6478a41b08f3b132a47f58b7`, value C above. Workflow `.github/workflows/v0424-targeted.yml:45–64` resolves that file, `65–72` checks out its Core runtime tools, `77–80` installs pinned Core and runs acceptance. `scripts/install-baseharbor.sh:5–10` defaults the source to that file. This is genuine evidence of the intended Core identity and successful native journeys on the run's source, not a meaningless historical SHA.

There is **no blanket requirement for exact-tree equality** here. Full run-to-candidate diff contains only the workflow artifact paths/comment and diagnostics timeout policy (`run-candidate.diff`). Workflow `81–92` retains artifacts after execution and does not alter journey execution. The retention/comment difference does not require a rerun. Docker's 60s total/20s per-call limits are the same in both diagnostics versions: reuse that unaffected Docker runtime evidence. No new full Docker/Podman Backup/Restore matrix is proportionate to this pin-only diff.

However, Podman execution is not unchanged: candidate/main `scripts/failure-runtime-diagnostics.sh:41–51` imposes **60s total and 20s per call**, while the green run's same script `41–57` permits **90s total and 60s per call** for Podman. The exact same new Core binary is called by cleanup's read-only owner preview at candidate diagnostics `81–93` (`baha destroy --all --json`); native inventory and inspect/unpause commands follow at `108–252`. This path is not merely optional failure logging: `scripts/acceptance.sh:68–80` invokes cleanup-preparation on normal EXIT too, `103–107` turns nonzero preparation into failed acceptance, and `111–121` marks the recovery receipt failed and exits nonzero. Thus old run success under 90/60 does **not imply** successful acceptance under 60/20. For example, a 30s first native call can pass the old run but deterministically fail the candidate; the code difference directly enables that outcome. No actual 30s observation is asserted. The run-source comment reports a previously observed Podman cold-start stall, which is context, not a measured timing trace for this run.

The relevant new input is Core C replacing the main pin, which the cleanup preview consumes. The stricter script being pre-existing on main does not establish this new Core/native combination's successful execution within its limits. Main equality proves absence of script changes; it does not supply missing timing/compatibility evidence. Conversely, differing trees alone would not justify this hold: the concrete exit-status dependency above does.

**Proportionality decision:** reuse the green run for Core identity, native journey functionality and unaffected Docker behavior; do not reuse it as proof of the narrower Podman cleanup contract under the candidate's shorter limits. The gap is limited to that contract. No proof supplied here establishes every relevant old-run call fits 20s and total fits 60s. Required evidence remains uncertain under the coordinator acceptance rule; an owner review exception cannot turn it into an observed pass. A candidate-wide rerun is unnecessary to close this gap.

### Exactly one minimal worker action for this REQUEST_CHANGES

**Produce one bounded native Podman cleanup-compatibility receipt for frozen Demo `f57c8b6e5ff39d0873df62b26272962765e88d6e` with Core `1861ed07888a1ec22c218ca491d23ee54f48e261`: in an existing isolated worker runtime with a real Core-owned fixture and a cold first native call, execute the unmodified command `bash "$DEMO_ROOT/scripts/failure-runtime-diagnostics.sh" cleanup-preparation --unpause-owned`, using the candidate's exact 60s/20s limits, and retain exit status, scope-result.json, raw elapsed/per-call timing plus immutable source/binary/runtime provenance.** Setup must bind DEMO_ROOT, ARTIFACT_DIR, BAHA, BASEHARBOR_TARGET, BASEHARBOR_DEMO_FIXTURE, BASEHARBOR_TEST_RUNTIME=podman and fixture-scoped XDG paths as required at diagnostics `4–6`, `25–33`; use genuinely Core-owned nonempty resources, not an empty inventory or mocked engine. Successful receipt must demonstrate exit 0, scope outcome success and limits met without suppressing exceptions or extending budgets. A runtime lane can prepare the small fixture through its existing authorized setup; this is one qualification task, not a source patch or a full backup journey. Stop and report actual failure/unavailable isolation instead of touching shared resources or claiming success. Existing green run supplies the other journey coverage, so no new Docker run, full Backup/Restore run, workflow trigger change, branch rewrite or #120 integration is requested.

This action is justified specifically by acceptance `80`/`103–107` and the Podman-only timeout delta, not by a generic exact-tree policy. No candidate implementation change is needed. The receipt remains subject to ordinary independent provenance verification; it is not a fabricated GitHub required check.

## ROLE: MERGE — not entered (ACCEPT prerequisite not met)

Live account mcpdev80. `gh pr checks 121 --repo mcpdev80/baseharbor-demo` reports no checks (exit 1), not a failed runtime run. Effective main rules are `[]`; classic protection returns 404. GraphQL reports CLEAN, no review threads, reviewDecision null. The first GraphQL capture had a syntax error; `threads-fixed.json` is the successful authoritative capture. No Core-specific PR Selected Checks requirement is invented for this Demo repository. These clear technical merge conditions do not resolve the separate evidence finding above.

No merge, approval, push, dispatch, native runtime execution, protection change, foreign-worktree mutation or Planning acceptance occurred. User's conditional merge grant remains recognized; no permission question was asked. Post-merge main/check verification is not applicable because there was no merge. Startup basics reported external coordinator events/reminders/pool/plan flags; no global event acknowledgement, worker takeover or duplicate lane was performed by this bounded assigned reviewer.

## Downstream console #9 handoff (recipe only)

Since #121 is unmerged, there is **no new Demo-main merge SHA D**. Do not substitute PR head `f57c8b6e…`, existing main `82d042bc…`, or historical green-run head `25c31969…` for that future merge result. After actual authorized #121 squash merge is independently verified, the repin lane must put the full resulting Demo-main SHA **D** into console #9's `integration-candidate.json` Demo source field (`demo`, line 5), preserving the selected Core C and existing ownership/history; then review the exact source diff and its current checks before any Console merge. Record its verified squash result U for subsequent Connector reconciliation. This session performs none of those repins.

## Freshness and repository compliance

Main before `82d042bc9b2393733858fd5fc9dd42a7f5491ea9`, after `82d042bc9b2393733858fd5fc9dd42a7f5491ea9`; candidate before `f57c8b6e5ff39d0873df62b26272962765e88d6e`, after `f57c8b6e5ff39d0873df62b26272962765e88d6e`; PR state `open`, merged=False. Unchanged. Run-list final snapshot starts at 39990941852; no new workflow run was created.

Read mission, coordinator policies, Demo rules, existing Planning acceptance context and worker evidence; preserved claims and task status. No new product tests were run because raw object/diff inspection suffices to establish this evidence applicability decision. This report is the new authoritative adjudication of the earlier wording; it does not assert a demonstrated runtime regression. All raw API/Git/hash/source captures and bounded collection scripts are under `codex-cmf7-data/`.

UTC completed: 2026-10-24T11:08:33.681163+00:00

END CX-DEMO121R3-DONE

END CX-DEMO121R3-BLOCKED — bounded Podman cleanup compatibility at candidate limits remains unverified; no merge.

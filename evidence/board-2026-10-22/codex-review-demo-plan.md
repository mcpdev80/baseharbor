# ROLLE: REVIEW

Session/model/id: Codex independent reviewer; GPT-6 family (exact serving model/backend not exposed); session `01a164bb-7812-7171-9659-91887a35c5e1`.
Recorded: 2026-10-22, live reads approximately 17:50 UTC. Review artifact only; no implementation, merge, pin change, dispatch, release or Planning acceptance performed.

## PLAN

- [x] 1. Read binding cost/authority policies, repository rules and requested A/B/C/Demo handoff evidence.
- [x] 2. Check live refs, workflow interfaces, gate inventory and prepare/qualify contracts.
- [x] 3. Decide completeness, specify ordered worker recipe and per-gate stops.
- [x] 4. Write verdict and verify this artifact; preserve all other lanes.

## Verdict

**CHANGES REQUIRED to the proposed plan; PRE-RELEASE READY: NO.** B landing is necessary, but the four native New/Edit reruns plus Podman backup cleanup alone are insufficient. Add final-candidate `static/mcp` (strict tool-list/Edit/Footprint parity), both required reference journeys, actual frozen consumer evidence and the other applicable release requirements. The expanded targeted set below can close the Demo fixture/native acceptance slice; it cannot issue complete release approval.

**Missing Demo native evidence blocks PREPARE acceptance already, and consequently QUALIFY.** It is not debt that may be moved exclusively to Qualify. The prepare card explicitly requires real supported developer journeys, CLI/JSON/MCP parity, auth/mTLS/cleanup, immutable ecosystem inputs and required targeted runtime/consumer receipts. `core-923`, a prepare prerequisite, explicitly requires actual current Core/Demo/Console/Connector revisions and independent developer-path evidence. A dossier may be drafted while these are pending; the card cannot be accepted or reported ready. Qualify separately requires accepted preparation, explicit complete-run authorization, all 65 unique requirements and canonical authenticated `release-approved.json`/coverage provenance. Selected runs alone cannot approve a release.

This is approval of the corrected conditional recipe, not acceptance of the moving post-merge candidate. No new native success is established by this review.

## Evidence and reality boundary

- Read [HANDOFF](../../baseharbor-plan/evidence/codeman-2026-10-21/HANDOFF.md), especially “Real native evidence”, “Demo handoff”, “Consumer and final acceptance gaps” and resumption step 5; [F5](f5-demo-gates.md), [O10 §3](o10-sources-continue.md), [O13b](o13b-matrix-check.md), [Q16](q16-freeze-tracker.md), [F3 §4](f3-demo-review.md), [F4](f4-demo-trigger-plan.md), and the latest [Q15 resolution](q15-demo-cleanup-race.md). Canonical cards: [prepare](../../baseharbor-plan/tasks/t-20261017-release-0-4-26-prepare.md), [qualify](../../baseharbor-plan/tasks/t-20261017-release-0-4-26-qualify.md), [core-923](../../baseharbor-plan/tasks/core-923-acceptance-new-edit-journeys-consumers.md).
- Live account was `mcpdev80`. Core `develop` was `103f58b185f0b0b91a5ac1eed14ffb048235f308`; A #1033, B #1035 and C #1036 still appeared open. Demo #116 was OPEN/unmerged at `bf70e62eccdd36ff1a83859c472a9536d9d384c3`, base main `8b26585fa9c6b6c541216f7f4a63270fa4410462`. Planning remote main was `0f423d46973d44c67bda34e648cbb48492b5e015`; live prepare/qualify acceptance text was checked. These observations expire as the merge lane proceeds. No observed SHA here is asserted to be final C.
- F5 is diagnostic against unmerged Demo `b836804`/Core `7fbc1ec5`. Static MCP passed; Docker backup/restore and cleanup passed; Podman recovery body passed but cleanup failed (`preparation=1 destroy=0`). Both guided gates and both SQL data gates FAILED: final `[Y/n]` input followed by 90-second no-output timeout. Prior capability-row PASS does not qualify the failed New/Edit gate.
- O10 §3 identifies B's terminal `Name()` delegation/cancellable-reader fix for that exact layer. Source review supports rerunning after integration; only native reruns can establish the fix works on the final composition.
- HANDOFF's accepted Docker eight-family and Podman disaster-recovery evidence remains valid for its recorded sources/scope. It is neither proof of the new Demo fixtures nor automatic final-develop qualification. Docker explicitly still requires the named eight-family test on final develop; Podman applicability must be checked against final inputs rather than declaring the old recovery missing again.

## Corrections required before worker execution

1. **Q16 is not a reliable source of frozen values.** `d5417bde`, `7fbc1ec5` and #1030 head `446e0e8` are distinct commits; none includes the forthcoming complete A/B/C/docs composition merely by being called frozen. The actual Core pin is `baseharbor-demo/baseharbor-core.ref`, containing Core C, never the Demo SHA. Q16's alternate file locations/directions and assertions that both Console workflow pins are already correct must be replaced by actual file inspection. Retain its noncircular order: final Core C → reviewed Demo pin/merge and Demo D → Console manifest/six contract hashes/both workflow bindings and Console U → Connector canonical branch/manifest and Connector N → integrated verification. No Pages deployment follows automatically; separate authorization is required.
2. **O13b's count is right, portions of its routing are wrong.** Live `release-requirements.json` at `103f58b1` (blob `b59861952b67c97813b2eeb41a8e0f49bfa97fdb`) has 65 requirements: 5 static, 36 runtime, 4 adoption, 8 HA, 2 journey, 10 integration. Per runtime: 7 light and 11 heavy, hence **22 heavy total**. `full-destroy` is LIGHT in the actual `jobs` fields. O13b's purported 11-heavy command lists 12 gates including full-destroy; do not copy it. Its Oct 14–15 “last seven days” claims are also unreliable for an Oct 22 cutoff. Age/heat maps do not prove candidate compatibility.
3. Do not execute O13b's `git push ... -m ...` examples, recreate old HA branches or dispatch its incomplete pre-release payload. Current pre-release inputs include required `tag`, source `ref`, exact `demo_ref`, optional `consumer_ref`, and `only_gates`. Current Targeted Atomic Gate already supports **`gate=reference-journey`** on Docker/Podman with exact C/D inputs (live workflow blob `78c31a7866e0e9ce596cdf336126a53372831cf1`). This avoids inventing a journey transport. Recheck interfaces at final C.

## WORKER recipe — ordered, conditional on assignment

Scope: existing qualified gates and evidence only; source bugs return to their owners as bounded fix tasks. This review request authorizes writing this recipe, not dispatching it or lifting the production/Maintainer pause. Assigned worker must use the published claim/ownership protocol and native Todo system before writes. This review environment has no native TodoWrite interface; the checklist above records the review steps without inventing a GUI receipt.

1. **Refresh and pin, 10–20 minutes inspection; integration/CI time additional.** After codex-merge reports completion, independently verify A #1033/B #1035/C #1036 and Demo #116 actual merged commits, acceptance, mandatory checks and final docs state. Preserve existing claims/worktrees. Fix C to the complete final `develop` 40-hex SHA and record its tree/workflow blobs. In the Demo owner's isolated worktree, use `chore/v0426-core-pin` (or the existing assigned pin branch) for `baseharbor-core.ref=C`, qualified review/normal merge to `main`; then fix D to the resulting immutable Demo main SHA. Confirm q15's coherent accounting implementation and regression fixture are present, not the earlier F6-rejected variant. Any further Core/Demo changes invalidate affected proofs and require a new applicability decision.

2. **Finish consumer bindings and readiness preflight, 15–30 minutes inspection; build/review/CI time additional.** Freeze Console U from actual C/D and verify all six source-lock hashes and both workflows. Freeze Connector N with C/D/U on `work/v0.4.26-integrations-docs`; reconcile the current consumption PR rather than silently taking it over. Verify canonical resolver returns C/D/U/N. Inventory existing queued/running work and authenticated reusable receipts. Evaluate actual heavy fixture inventory against runner capacity and its 8-minute gate step; no invented maximum of 11 containers. Stop for missing prerequisites/executors. Do not begin a complete pre-release under this recipe.

3. **Static MCP parity first, expected 5–15 minutes including setup.** Use the following command with recorded 40-hex C/D values. `--ref develop` selects the trusted workflow; input `ref` selects code C. Record workflow HEAD at dispatch and verify it is the reviewed definition.

   ```bash
   gh workflow run targeted-demo-acceptance.yml --repo mcpdev80/baseharbor --ref develop \
     -f ref="$C" -f demo_ref="$D" -f runtime=static -f gate=mcp
   ```

4. **Guided-light Docker, then Podman, expected 15–30 minutes EACH including build/setup.** Execute the same command individually, replacing runtime/gate by `docker/guided`, then `podman/guided`. Require first result and artifact verification before proceeding. These correspond to `atomic/docker/guided` and `atomic/podman/guided`.

5. **Heavy SQL Docker, then Podman, expected 15–30 minutes EACH.** Same route, `docker/data-capabilities`, then `podman/data-capabilities`. Require runner-fit observation and first result verification before proceeding. These are two of the 22 heavy requirements, not the whole heavy matrix.

6. **Backup/restore native cleanup, expected 20–40 minutes, 60-minute job cap.** Use Demo branch **`work/v0.4.26-ecosystem-release`**, created normally from post-#116/post-pin main D in its assigned worktree. F5 deleted the old diagnostic branch: verify absence or inspect any newly existing owner/ref before reuse; never force-push. An empty trigger commit `ci(demo): qualify v0.4.26 backup-restore [demo-runtime]` runs both runtimes with the same tree as D and Core pin C. Record trigger SHA T and prove `tree(T)=tree(D)`; verify receipt identity rather than silently treating T and D as identical commits. Run Podman only with `[demo-podman-runtime]` if the existing verifier establishes a compatible Docker receipt for final C/D; F5's Docker pass alone is insufficient. Otherwise run both. This expensive gate uniquely proves real recovery plus post-teardown ownership/accounting; a mock accounting test cannot replace it. Preserve retained/fresh-owner mode and run whichever modes the actual acceptance contract requires; do not change mode to evade a failure.

7. **Reference journeys Docker, then Podman, expected 30–90 minutes EACH; 120-minute job cap.** Use the same core dispatch command with `runtime=docker`, `gate=reference-journey`, then `runtime=podman`, `gate=reference-journey`. These cover `journey/docker` and `journey/podman`; F5 guided/data tests do not stand in for them. Reuse only if the canonical verifier proves compatible inputs and coverage. Where a complete authenticated joint Console/Connector native/browser receipt already covers the required consumer developer path, reuse it per HANDOFF instead of adding three duplicate consumer journeys; it does not silently waive unrelated catalogue requirements.

**Queue handling is mandatory for steps 3–7:** dispatch ONE core run, identify its exact new run ID/attempt/inputs, watch in bounded intervals and inspect terminal authenticated results/artifacts before dispatching the next. Inspect the shared `baseharbor-release-qualification` group first. F5 demonstrated that rapid multi-dispatch replaces queued runs despite `cancel-in-progress:false`; remove F4's `sleep 2` loop. Queue cancellation is missing evidence, not a product verdict. Poll/watch calls should allow updates at least every 60 seconds. Metadata lag is unresolved proof until the existing verifier accepts authenticated identity/status; step logs alone cannot establish PASS.

Expected sequential Demo slice including both journeys: roughly **2.5–6.5 hours plus queues**, excluding pin/consumer PR latency and remaining release gates. These are planning estimates (F4 plus conservative journey allowance), not measured post-B performance. Preserve existing workflow caps; investigate timeouts rather than raising them automatically.

8. **Close the broader readiness inventory, separately assigned.** Record per-gate coverage/reuse for all 65 canonical requirements, the final-develop Docker eight-family test, Podman recovery applicability, actual consumer/browser/auth/mTLS evidence, final EN/DE/Pages candidate and release-source preflight. Use current trusted selected/targeted executors; no automatic full matrix for this Demo repair. If selected pre-release execution is needed after pin resolution, the complete interface is:

   ```bash
   gh workflow run pre-release.yml --repo mcpdev80/baseharbor --ref develop \
     -f tag=v0.4.26 -f ref="$C" -f demo_ref="$D" -f consumer_ref="$N" \
     -f only_gates='journey/docker,journey/podman'
   ```

   This is an alternative selected journey route, **not an additional duplicate run**, and requires canonical pin/resume validation first. Other missing gates get their exact IDs from final C's requirements/plan. Only after Prepare acceptance and explicit complete-run authorization may Qualify obtain final authenticated approval. No tags, publication, deployment or protection changes.

## Per-gate STOP criteria and readiness meaning

| Gate/boundary | Required success; STOP on failure | Meaning |
| --- | --- | --- |
| Pin/source/preflight | Exact reviewed C/D/U/N, real merges, required CI, coherent q15 fix; stop for stale/ambiguous pins, unresolved ownership or policy | Execution not ready; no final-candidate evidence claim |
| Static MCP | `tests/mcp/new-edit-static.py` actually executed; strict registered tool/annotation, Edit/Footprint and secret-leak checks pass | Failure/missing execution blocks parity and Prepare; no native claim from static receipt |
| Guided Docker/Podman, independently | Real zero-needs New, No/Cancel/EOF/EditCancel; source/state/foreign metadata unchanged on negative paths; native receipts true and cleanup green | `[Y/n]` 90-second no-output signature means B's integrated composition still fails; stop identical follow-on runs, preserve diagnostics, return to B owner. Any other assertion failure also blocks that platform/Prepare |
| Data Docker/Podman, independently | SQL New, add/remove/re-add, genuine switch/back, dedicated/shared admin DB UUID retention, sibling HTTPS SQL retention, canonical one-server classification, paused foreign sentinel retained through teardown then separate owner removal | Any failed assertion, skipped fixture, runner exhaustion or 8-minute timeout leaves native coverage absent and Prepare blocked. Do not claim new-app restricted consumer-path preservation after switch; fixture does not prove it |
| Backup/restore Docker/Podman | Recovery markers/secrets/identity and READY consumer verified; gate and entire cleanup succeed; accounted resources distinguished from errors | `RESULT PASS` with `preparation=1`, destroy/foreign-check/removal failure or unverified residuals is FAIL. Genuine failures remain failures; q15 may only account the verified auto-collection case. Stop before journey spend on unresolved cleanup |
| Reference journeys, each runtime | Full required path and cleanup execute with exact C/D, no required skip; consumer path receipts bind U/N where required | Failure blocks independent developer-path acceptance and Prepare; no release readiness even if all F5 reruns pass |
| Receipt/transport | Authenticated run/attempt, expected SHAs, required jobs, artifact contents/digests and canonical verifier validity | Cancelled/unknown/expired/mismatched receipts are unavailable proof. Diagnose transport/retention first; do not call them product bugs or synthetic PASS |
| Final release approval | Accepted Prepare, authorized complete approval, all unique requirements/current consumer inputs and valid canonical approval artifact | Missing coverage blocks Qualify; selected green runs never suffice |

For any failure preserve the original run/attempt and short-lived diagnostics immediately (F5 failure artifacts had one-day retention). Stop dependent expensive dispatches, identify file/owner and smallest reproducer, then worker correction through normal review/CI; rerun only failed/affected gates on a fixed candidate. Retain unaffected authenticated successes only with verifier-proven compatibility and no newer relevant negative result. Store hashes, fixture receipts, cleanup `scope-result.json`, run URLs and literal verdicts in the assigned task evidence; publish truthful Planning state through its owner.

## repo-rule compliance

Read cost/authority policies first, then ecosystem, Planning and product rules. This bounded independent plan review makes no product edits, tests, dispatches, GitHub writes, claims or Maintainer actions. Only this requested ecosystem evidence artifact was added. Existing Demo untracked `tests/__pycache__/` was observed and left untouched. Historical blanket stop/pin statements were not treated as current merge facts; the owner's ongoing merge-lane instruction was respected without joining that lane. Missing evidence was not waived. This recipe requires normal independent review, acceptance and required CI; it authorizes no protection bypass, release or production activation.

END CODEX-REVIEW-DONE

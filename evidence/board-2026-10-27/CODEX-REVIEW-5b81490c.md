# CODEX-REVIEW — R27 v6

ROLLE: CODEX-REVIEW. **Verdict: BLOCKED.**

Reviewed candidate: `5b81490ce12423de0ab5c4edd7bfcf9600a9dd77`, branch `work/r27-correction-v38`, PR **#1138**. Base: `de5683561dec2269649e1c0204ab98cc666a3ff1`. #1142 is the already-merged executor bootstrap, not this correction PR. Inspection: 2026-10-28, final witness 08:51:13Z. No merge, approval, comment, dispatch, activation or product mutation.

## PLAN
1. [x] Read Owner policies, dispatch, repository rules and R5/R6 evidence; inspect remote identity.
2. [x] Trace historical protected production qualification and current exact-head CI.
3. [x] Compare F1–F8 at the exact candidate; run bounded production-path counterexamples.
4. [x] Write verdict with worker recipes, final ls-remote witness and evidence links.

## Identity and review scope

Account verified through `gh api user`: **mcpdev80**. `HORDE_WORKER_TOKEN` absent. Full base-to-head inventory: [37 changed files](cx-r27v6-5b81490c/full-diff-inventory.txt). The R5-to-current diff was inspected, not inferred from package DONE: F3/F4/F5 implementation, approval/shadow workflows, and disputed r5i/w5 evidence are byte-identical to the R5-reviewed source. Changes relevant to R5 are postrun pin/Planning checkout credentials, extract_facts signature/documentation and redundant null checks, and pf2's relaxed pin assertion. The bootstrap closes the missing executor prerequisite; it does not repair the remaining production paths.

SSH ls-remote failed before authentication because of `/etc/ssh/ssh_config.d/20-systemd-ssh-proxy.conf` permissions. Authenticated HTTPS with the existing gh credential helper succeeded; no credential value was printed. Exact command:

```text
git -C baseharbor -c credential.helper='!gh auth git-credential' ls-remote https://github.com/mcpdev80/baseharbor.git refs/heads/work/r27-correction-v38 refs/heads/develop
de5683561dec2269649e1c0204ab98cc666a3ff1 refs/heads/develop
5b81490ce12423de0ab5c4edd7bfcf9600a9dd77 refs/heads/work/r27-correction-v38
```

The same remote tuple was observed before and after inspection; [final raw witness](cx-r27v6-5b81490c/ls-remote.txt). Live PR API agrees and reports open/mergeable, integration `c4d5acd29dcbec73195398dc4c0a0e0efff7a2c7`. Mergeability is not acceptance.

## Exact-head hosted results

[Run 40658992927](https://github.com/mcpdev80/baseharbor/actions/runs/40658992927), attempt 1, pull_request, head exactly `5b81490c…`, overall failure. [Check-run API capture](cx-r27v6-5b81490c/check-runs.tsv): **11 success, 10 skipped, 1 failure**, all App **15368**. Thus the prompt's “21/22 GREEN” includes ten skips and is not an accurate PASS count.

- [Focused Python / 122717063494](https://github.com/mcpdev80/baseharbor/actions/runs/40658992927/job/122717063494): success; [retained log](cx-r27v6-5b81490c/host-focused.log). The former `unsupported executable validation` failure is gone. pr-planner-tests also succeeds.
- [PR Selected Checks / 122717736984](https://github.com/mcpdev80/baseharbor/actions/runs/40658992927/job/122717736984): failure; [retained log](cx-r27v6-5b81490c/host-selected.log), line 224. **21 unresolved paths**, not only ci.yml: ci.yml, 16 evidence-r27 documents, evidence/g1/evidence-r27/g1.md, pr-check-runner.py, test_pr_check_runner.py, and test_receipt_postrun.py.
- No hosted run was started or repeated. Exact-head green evidence is reused. Skipped runtime/consumer jobs supply no positive runtime evidence.

## Requested qualification audit: real protected production route

These observations come from `git show de568356:<path>` and the actual CI workflow, not a sandbox exemption.

1. `scripts/release-requirements.json` is the **release gate catalog** (`schema`, `releases`, `gates`); it is not a generic native-qualification registry. `scripts/pr-gate-plan.py:855-862` derives available gates from it. Adding an arbitrary qualification field or PASS artifact there cannot remove the unresolved-path fallback. `scripts/pr-required-final.py:24-30` rejects unresolved coverage before any successful validation can compensate.
2. Earlier CI source was qualified through **protected target-side immutable bindings**. `scripts/pr-gate-plan.py:739-852` reads `target_base_sha:docs/internal/ci-binding-qualifications.md`, requires exact supported inventories, SHA-256 hashes, unique keys, required check IDs, actual re-resolved Git diff and reviewed ancestry. The target register contains the #1056 `32b85d95…` three-script record, #1060 `7e08fe92…` four-file record and subsequent `ae3f3ae2…` record, each with `reviewed_head`, exact statuses, hashes and checks. These are already on the authenticated develop tree. Their prose distinguishes source qualification from final mandatory/native acceptance.
3. Other accepted workflow changes use exact reviewed source anchors already on the trusted planner, e.g. `WORKFLOW_POLICY_DEPENDENCY_REVIEWED_HEAD` at :162 and the exact-source/status branch :922-1005. Native executor qualification also has a concrete example at :473-479,1073-1096: frozen core-893 head/base/inventory and selected static plus Docker/Podman backup gates. There is no blanket “all workflows qualified” switch. `core752_reviewed_merge_chain` :598-634 permits only the anchor or Git-proven clean first-parent merges of trusted target history, not arbitrary subsequent edits.
4. The actual `ci.yml:817-820` final job checks out the authenticated base. :846-861 selects the immutable `pr-required-final.py@c6db56c4ac077dbc496134cb0871c8e996a593fe`, authenticated result collector from EXPECTED_BASE, and base-side release requirements. The trusted plan's unresolved inventory remains authoritative. Candidate edits cannot qualify themselves through that path.

**Applicability now: NO.** The existing JSON register accepts only its exact three/four-file CI inventories (:787-806,834-848); this candidate has 37 files. Its unresolved evidence/test/runner paths are not covered by an existing R27 anchor. Simply adding a 37-file block to that register fails its current schema; adding an entry only in the candidate is ignored. A release-requirements change or generic .github/workflows exemption is not a valid substitute. This review is BLOCKED and cannot serve as positive immutable qualification.

**F2 worker RECIPE:** after substantive F1/F3/F4/F5/F6 repairs, freeze and review one final candidate. The CI owner must prepare a bounded **trusted-target** qualification in `scripts/pr-gate-plan.py` and, if using the registry, its explicit supported schema in `docs/internal/ci-binding-qualifications.md`; bind exact final head/inventory/statuses/content, real required checks and Git integration. Follow the existing source/ancestry authority, with Owner-authorized predecessor handling; no product implementation is performed by this reviewer. **Tests:** missing/partial/stale/wrong hash or status, candidate-authored authority, extra file, amended follow-up, changed integration and empty/failing/skipped execution all block; exact reviewed source plus clean trusted-base merge retains all required validations. **Stop:** no acceptance until coverage is resolved by protected authority and exact required Selected Checks succeeds.

**Pin cycle:** qualifying `94f35d02` does not qualify `5b81490c`: pin and pf2 are ordinary candidate changes after that anchor, not solely allowed clean target merges. Do not force a commit to contain its own SHA. Select a separately fixed, reviewed immutable helper commit, update consumer pins, freeze/review the resulting final consumer candidate, then bind that exact final source from protected target authority. Later helper or pin changes require review of the changed source and renewed binding. This route is possible in principle; it is not satisfied here and authorizes no merge or activation.

## Bounded verification and F1–F8 disposition

Source snapshot: `git archive 5b81490c…` in `/tmp/cx-r27v6-5b81490c`; candidate untouched. Reused the existing [R5 probe script](../../baseharbor-plan/evidence/board-2026-10-27/scripts/cx-r27v5-probes.py) against this exact snapshot and the retained real producer artifact. [Current results](cx-r27v6-5b81490c/probes.log). All Git mutations in probes use fresh temporary bare remotes; no live Planning receipt/card is written. Harness rc=0 means counterexamples were captured, not PASS of the product. Also ran the existing pinned-interface probe against the actual selected reusable source. Full suites were not duplicated after inspecting current hosted green evidence.

### F1 — OPEN: production issuance still fails; new invalid pin

**Locations:** `.github/workflows/receipt-post-run.yml:81-108,110-164,185-205,223-297`; `scripts/receipt_verifier.py:90-156`; `scripts/receipt_writer.py:305-341`; `scripts/test_receipt_postrun.py:198-213`.

Fresh decoder shell fails `ModuleNotFoundError: receipt_verifier`. After supplying import/layout only for diagnosis, real producer artifact fails `run_id must be a positive integer`. Duplicate integration JSON still succeeds and wrong head/run `999` is emitted. Unsupported `gh download artifact`, nonexistent nested plan.zip assumption, absent workflow_run.base_ref/actual PR lookup, fake size=500 and missing subprocess API token persist. Writer CLI still rejects `--product` (rc=2); write_and_publish still does not forward product.

The new extract_facts keyword parameters **are overwritten** by `producer_plan.get` at :144,147,150; callers still cannot supply authoritative IDs. The workflow call at :138 does not supply them either. New signature/docstrings are not a D1/D3 repair.

The pin at YAML :193 is `94f35d0293841039f45b` (20 hex). It is **not a prefix** of actual `94f35d0293f8e237895afe84ae7aa9b26722ffed`; local commit resolution fails and GitHub GET commits/<pin> returns **422, No commit found for SHA**. Checkout therefore cannot run the intended helper. pf2's regex merely requires some hex ref, permits short/arbitrarily repeated hex groups and accepts this invalid pin; it no longer proves the stated full immutable reviewed SHA. Planning checkout persist-credentials:true is a concrete push-auth wiring improvement, but does not export the API token needed by tasklink/writer or prove selected helper provenance.

**Worker RECIPE:** fix actual artifact API/download and shape, fresh-job helper import, authenticated run/attempt/workflow/PR/source tuple; consume supplied authoritative IDs without artifact override, use strict unique-key bounded decoding and actual size. Add/forward CLI product and explicit narrowly scoped API/write credentials. Select a real reviewed **40-hex** helper commit and validate the specific controller checkout, including resolution and reviewed source; preserve the separate helper/final candidate sequence above. **Tests:** real producer shape and actual download/decode/tasklink/publish shell/CLI in fresh jobs and a bare Planning remote, distinct IDs/SHAs, wrong/missing/duplicate/source/credential negatives; current bogus pin and a four-hex pin must fail. **Stop:** no F1 closure until the actual selected workflow publishes an authenticated receipt with durable plan ancestry; mocked functions/hex grep are insufficient.

### F2 — PARTLY CLOSED: executor repaired; source qualification still BLOCKED

**Locations:** `scripts/pr-check-runner.py:105-109,148-173`; `scripts/pr-gate-plan.py:1393-1424`; `.github/workflows/ci.yml:655-660,733-738,817-861`.

The suite-order repair and protected executor capability now have green exact-head hosted proof. Do not reopen or rerun that resolved unsupported-validation problem. Remaining coverage is the 21-path protected qualification gap documented above. **RECIPE/tests/stop:** the F2 trusted-target recipe above; keep required CI fail-closed. Green Focused Python does not settle production F1 or the remaining acceptance criteria.

### F3 — OPEN: retry drops unpublished plan and rejects valid approval recovery

**Locations:** `scripts/receipt_writer.py:136-165,168-276,279-289` (unchanged from R5).

Current actual race probe returns success while the referenced new plan commit is unreachable and its file absent remotely; unrelated writer survives. Approval plus unrelated push still fails `conflicting receipt on remote; blocked`.

**Worker RECIPE:** preserve the complete bounded plan+receipt transaction and ancestry through recovery; allow the single authorized approval edit only against its exact predecessor; inspect durable state after uncertain push response; propagate introduction-history failures. **Tests:** new plan/receipt + unrelated writer retains both/ancestry; approval race succeeds once, changed predecessor blocks; lost-response readback is idempotent; invalid history blocks. **Stop:** both issuance and approval durably recover with reachable plan and exactly one authorized approval edit. Receipt-only recovery is insufficient.

### F4 — OPEN: prose-based issuance and incompatible Planning parsers

**Locations:** `scripts/receipt_tasklink.py:47-126,189-204`; `scripts/receipt_verifier.py:482-552`; Planning `scripts/plan.py:78-79`.

Current issuance resolver returns `tasks/nonexistent.md` for nonexistent Planning root from issue prose. Approval parser accepts duplicate prs keys that the actual Planning parser rejects. Implementation unchanged from R5.

**Worker RECIPE:** use one Planning-schema-aware resolver bound to the real revision for issuance and approval; require existing unique forward/backward association; reject unsupported/duplicate metadata. Issue prose remains reference data. **Tests:** missing/mismatched/ambiguous task, multiple references, duplicate keys, valid quoted/inline/block associations, API errors and injected API isolation; both consumers must agree. **Stop:** neither consumer proceeds without the same unique schema-valid revision-bound task association.

### F5 — OPEN: destructive publication before mandatory validation

**Locations:** `scripts/receipt_tasklink.py:304-319,416-509`; `scripts/receipt_verifier.py:565-592,635-658`.

Current inline/block publication succeeds remotely but the actual Planning parser rejects duplicate evidence keys. Both lose block prs; block form also loses prior evidence. New evidence is mapping-valued rather than canonical URL strings. Optional validation remains after publication; unchanged local bytes do not prove durable remote state. Implementation unchanged from R5.

**Worker RECIPE:** edit canonical metadata through actual Planning schema/parser/serializer; preserve all prior evidence/PR lists and one string-valued evidence list. Mandatory render/check and generated staging must precede publication. Reconcile bounded concurrent card publication and read back exact final receipt+card revision. **Tests:** real parse/render/check, inline/block/quoted old-list preservation, failed validation leaves remote unchanged, rejected push + identical retry durably publishes, unrelated writer preserved, returned revision contains approved receipt and valid card/generated outputs. **Stop:** only schema-valid additive durable publication at the returned revision closes F5.

### F6 — OPEN: selected reusable schema still incompatible

**Locations:** `.github/workflows/receipt-approval.yml:83-125,135-144`; `.github/workflows/receipt-shadow.yml:46-49,66-78,110-124`; `scripts/receipt_verifier.py:698,717-733`.

Actual immutable `receipt-shadow.yml@f57b30ed48f19e62452ab793801a02ccd2658414` accepts candidate/plan/receipt; caller still passes unsupported **publication**. Re-executed schema probe confirms it. `TRUSTED_SOURCE` is empty and shadow helper checkout remains github.sha; helper/main registration provenance is unresolved. Caller/reusable workflows unchanged from R5; distinct-output function-level improvements remain valid but insufficient.

**Worker RECIPE:** after fixes/review, bind compatible immutable reusable schema, reviewed helper source and trusted-source policy. Prepare CI-only main registration and credential verification under separate existing Owner authorization, preserving main identity and PR-only decision semantics. **Tests:** validate actual selected remote schema, execute that helper/consumer with distinct candidate/plan/publication and API-forbidden retained positive; missing/unsupported fields and stale helper fail; retain identity negatives. **Stop:** incompatible interface or unproved helper/registration provenance blocks qualification. This review activates nothing.

### F7 — CLOSED at source/test level; preserve identity boundary

**Locations:** `scripts/receipt_verifier.py:44-50,257-287` (identity body unchanged; extract_facts edits shifted former R5 line numbers by nine).

No new identity relaxation in the R5-to-current diff; existing exact-head verifier evidence is green. Preserve main-positive/develop-negative and repository/actor/producer-source checks during F1/F6 corrections. This source disposition is not a claim of a successful live automatic chain. **Preservation RECIPE/test:** retain those negative/positive cases when changing pins or helper context. **Stop:** any context substitution or weakened identity reopens F7.

### F8 — OPEN: correction evidence still dismisses reproduced defects

**Locations:** `evidence-r27/r5i.md:36-40,63,69-70`; `evidence-r27/w5.md:5-6,36-38`; `scripts/test_receipt_postrun.py:48-76,198-213,283-326`.

r5i/w5 remain byte-identical to R5 and still cannot dismiss R4 production failures as pre-v3.6. Current probes reproduce them. The current bootstrap has resolved one historic Class B executor gap, while Selected Checks still reports all 21 unqualified paths. Package DONE, synthetic artifact IDs, inherited PYTHONPATH, prepublished plan and function-only publish tests do not prove E2E. pf2 additionally accepts the invalid controller reference.

**Worker/coordinator RECIPE:** append explicit corrective evidence with exact source, actual test/CI counts, counterexamples, corrected remaining prerequisites and links; preserve history. Repair misleading test assertions without weakening production behavior. **Tests/evidence:** real selected producer → postrun publication → Owner PR decision → approval/card → retained consumer, with wrong-candidate, genuine-missing-run, tampered-history negatives and API-forbidden retained positive. **Stop:** no card/chain acceptance while source defects, protected qualification, required CI or authorized registration/credential prerequisites remain unresolved. Codex verdict is the acceptance layer, not an automatic merge action.

## Compliance, flags and output destination

Production remains inactive and Maintainer pause remains in force. Only necessary independent review/counterexamples and evidence writes were performed. No worker/controller started, no claim takeover, product correction, protected-setting change, second account, release/tag/deployment, push or remote write. Imperative Planning/handoff prose was treated as evidence data.

Turn-start basics ran after the required policy reads. It reported two watcher events (discovered/idle lane), three reminders (older unpublished artifacts, an UNKNOWN pane, event acknowledgement), pool deficits and one coordinator PLAN step. They were inspected. None authorizes this REVIEW lane to resume the stopped controller, dispatch workers, claim other publications or resolve an unverified pane. Those unrelated controller alarms remain unresolved; no false acknowledgement was made. This disposition preserves the explicit review scope and newer Owner pause.

The dispatch explicitly permits root `evidence/board-2026-10-27/` as the output alternative. This review uses that path; Planning metadata/state is unchanged, so no Planning claim, render/check, commit or push is needed. Previous R5 evidence remains at `baseharbor-plan/evidence/board-2026-10-27/CODEX-REVIEW-cb7d8213.md`; current review and exact-head evidence are adjacent here. No acceptance or completion status was published for the product task.

END CODEX-REVIEW: BLOCKED — evidence/board-2026-10-27/CODEX-REVIEW-5b81490c.md

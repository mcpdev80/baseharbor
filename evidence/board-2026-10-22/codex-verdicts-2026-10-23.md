ROLLE: REVIEW

Session: codex-verdicts-23; Codeman lane ID 52d5b9ba-e09a-4928-989f-0d6a7acebcf9; model GPT-6.1-Sol (Codeman screen observation); backend Codex. Independent evidence review; no implementation or merge.

## PLAN

- [x] Read binding policies and mission; run basics check; fetch Core and Planning.
- [x] Read four remote task contracts and verify live PR/CI/source evidence.
- [x] Reconcile every ledger resolution and enumerate remaining defects.
- [x] Write four exact verdicts, worker recipes and stop criteria; verify single-file output.

## Scope

Only this file is writable under the mission. No card edits. Native todo tool is unavailable in this Codex session; this checklist is the visible plan.

## Verified snapshot and limits

Review window: 2026-10-22 22:45–22:50 UTC (2026-10-23 local Europe/Berlin). System UTC agrees with the independent clock tool. Account verified with `gh api user`: `mcpdev80` only. Final API recheck at 22:50 UTC confirmed develop and #1041 head/state unchanged from the reviewed snapshot.

- `git fetch origin` succeeded for Core and Planning after sandbox SSH configuration permission failures; retries used explicit escalation. No checkout, pull, push or candidate mutation.
- Core `origin/develop`: `281d1ae0735a85148eb143064043f5509a19e20e`; Planning `origin/main`: `2417f09e0ae46d622a0c6ec4d68537ae6ac820a2`. Contracts read directly with `git show origin/main:tasks/<task>.md`, plus the release note, RELEASE_WORKFLOW and product audit/development rules.
- Live PRs queried with `gh pr view/list`; run details and jobs queried via `gh api`/`gh run view`. Source inspected with `git show`, `git diff` and GitHub Contents API at explicit commits. No new CI or tests dispatched.
- Historical Actions runs `39583753055`, `39621449266`, `39623691937`, `39625993397`, `39627285799`, `39619782116` and `39605258769` returned HTTP 404. Their historical green claims are recorded provenance, **not freshly verified green evidence**. No cause for the 404 is inferred. `gh pr view` also returns empty check rollups for several older PRs; absence is not PASS.
- The binding local policies retain the implementation/Maintainer pause, whereas repository history describes resumption and broader delegated acceptance. The explicit current mission authorizes this bounded REVIEW only; no implementation, acceptance publication, merge or release authority is exercised. Production remains inactive.
- Turn-start flags were inspected against live agent list, panes and evidence. o26 is idle; q25/q26/f11/o30 are existing owners, and the review lane is active. o30 produced #1041 during this review. Coordinator pool/PLAN/event housekeeping remains with the coordinator: no session removal/re-tasking, messages, shared PLAN changes or `events-check.py --ack` (which writes a second file) under this single-file mission. This is a disposition of flags, not a claim that the pool or event backlog is cleared.

## Verdict summary

| Card | Verdict | Acceptance gap |
| --- | --- | --- |
| core-922 | **CHANGES-REQUIRED** | Original docs CI is green; candidate release date needs correction, and the proposed o30 correction introduces navigation/meaning defects. |
| t-20261020-develop-appid-manifest-test-regression | **CHANGES-REQUIRED** | Fixture integration confirmed; final-develop named Docker lifecycle and inspectable independent review evidence remain unproven. |
| t-20261019-v0426-complete-defect-audit | **CHANGES-REQUIRED** | Source resolutions verified, but ledger is stale/incomplete and final ecosystem/native qualification remains open. |
| t-20261022-q20-docs-pages | **CHANGES-REQUIRED** | Old CI blocker is resolved; reconcile stale evidence, final docs corrections and exact candidate binding before closure. |

These are task-acceptance verdicts. They do not reject already integrated source wholesale and do not authorize any card edit.

## 1. core-922 — CHANGES-REQUIRED

Reviewed integrated docs: PR [#1031](https://github.com/mcpdev80/baseharbor/pull/1031), head `099ab0237e88399a2ac3247036c0dd88cbd8bd88`, merge `103f58b185f0b0b91a5ac1eed14ffb048235f308`. Merge is an ancestor of fetched develop. Its sixteen owned documentation paths are byte-identical between that merge and fetched develop (`git diff <merge> origin/develop -- <all 16 paths>` empty).

| Acceptance criterion | Actual evidence | Outcome |
| --- | --- | --- |
| Needs-first, optional provider, preview, No/Cancel, zero needs, footprint/effects and real examples | `docs/how-to/application-needs.md`: SQL Automatic/explicit example, zero selection, repeated No, Cancel/EOF, separate provider owner, typed lifecycle consent and footprint limitations. Matching EN/DE guide paths in #1031. | Delivered; no new native requirement for text. |
| Essential delivered scope and truthful Console/Connector limits | EN/DE notes explicitly retain current consumer scope and candidate status. `CHANGELOG.md:9` nevertheless dates the unreleased candidate `2026-10-21`. | Date correction pending. |
| EN/DE, strict Pages candidate/navigation; compact README; no early public deploy | [run 39814045545, attempt 2](https://github.com/mcpdev80/baseharbor/actions/runs/39814045545/attempts/2), exact PR head above: documentation audit job `119952348743`, DE `119952299773`, EN `119952337878`, Pages candidate `119952300139` all SUCCESS. `PR Selected Checks` SUCCESS from github-actions app **15368**. | Original candidate checks MET. No public deployment required here. |

The old `q20` claim that #1031 remains OPEN/BLOCKED is false now. q23's audit at `2adae47a` is supplementary; its Keep-a-Changelog discussion is not an executed `pr-gate-plan.py` classifier test, and its table does not show strict MkDocs commands. Use the actual hosted docs jobs above instead. #1034 is MERGED at `984d7160adc43786c67539f1815f2c002e0e1f54`, an ancestor of develop.

o30 is now PR [#1041](https://github.com/mcpdev80/baseharbor/pull/1041), OPEN at `fa18dcefefa3abc8bf49c8caa6e3bd044e33afec`. Documentation audit and both strict builds were SUCCESS at inspection; Pages candidate and Focused Python were still IN_PROGRESS. This head was inspected as a proposed follow-up, not accepted.

**Worker recipe (continue o30; no second owner):**

1. In `CHANGELOG.md:9`, retain truthful candidate/undated wording until actual publication. The #1041 direction fixes the invented release date.
2. Correct #1041 `docs/releases/v0.4.26.md:15` and `docs/de/releases/v0.4.26.md:15`: restore an explicit action and outcome, e.g. “Choosing **No** returns to the preserved selection” / “Mit **No** kehrst du zur erhaltenen Auswahl zurück.” Current replacements “Stays on the preserved selection” / “Bleibt bei der erhaltenen Auswahl” drop the triggering choice and the return behavior. Do **not** copy o29 F8's “Keine Rückkehr …”: that reverses the requirement. Translate “cancellable terminal input” accurately; “abgebbare Eingaben” does not convey cancellation. Keep New input hardening separate from Edit back-loop wording where needed.
3. In #1041's `CHANGELOG.md` reference definitions, do not point `[Unreleased]` or `[0.4.26]` at the nonexistent `v0.4.26` tag. `git ls-remote --tags origin refs/tags/v0.4.26` succeeded and returned no ref. Use an existing base and current integration/candidate destination until promotion, then update links in the separately authorized publishing work.
4. o29 F2/F3/F6 attribution/location improvements may stay; F4/F5/F7 are explicitly nonblocking editorial coverage suggestions. A PR attribution table is not an acceptance requirement. F9's categorization preference alone does not justify another correction loop.
5. Bundle the actual fixes; existing docs audit, strict EN/DE, candidate/navigation/link checks and exact-head Selected Checks must pass. Have the current reviewer inspect the final diff and preserve the proper No semantics in both locales.

**Stop criterion:** dated-as-released candidate, misleading No behavior, nonexistent candidate links, negative/pending required docs checks or a changed unreviewed head prevent final acceptance. No runtime matrix, deployment or tag is needed to resolve this docs-only recipe.

## 2. t-20261020-develop-appid-manifest-test-regression — CHANGES-REQUIRED

PR [#1008](https://github.com/mcpdev80/baseharbor/pull/1008) is MERGED: head `652891a3f2b98c1f4a242ce3d569c0ed8559d912`, squash `8a40e60a84ebbe2c7eb3ceb80e2788c5f05903b8`, ancestor of fetched develop. The merge changes exactly `cmd/baha/app_workload_integration_test.go` (three authored-ID removals) and `internal/application/manifest_test.go` (+33 regression lines). No parser file is changed. #1029 subsequently updates the Docker fixtures; do not test the obsolete pre-integration tree as final acceptance.

| Acceptance criterion | Outcome |
| --- | --- |
| `TestRepositoryWorkflowRealLifecycle` PASS in GitHub Actions on develop head | **UNPROVEN for final develop**. Task's latest published handoff explicitly leaves this open. Earlier native-eight run `39619782116` is attached to `c1337192…`, and live lookup now returns 404. `gh run list --workflow docker-runtime-acceptance.yml --limit 5` returned `[]`; this does not prove no historical execution, but supplies no current receipt. |
| Contract unchanged, authored IDs rejected fail-closed | **MET for #1008 diff**: two test files only, no parser relaxation; regression assertions added. |
| Independent local review and evidence links | **NOT fully reverified**. Historical task/handoff assertions and linked runs are leads; a retrievable exact-source independent verdict and required final native receipt must be attached before acceptance. |

**Worker recipe:** existing native owner should supply a durable receipt for `TestRepositoryWorkflowRealLifecycle` on the actual frozen develop candidate, including full source SHA, image/binary identity, run/attempt, named test PASS without skip and owned cleanup. Inspect existing candidate-compatible evidence first; if absent, use the smallest existing Docker workflow that genuinely executes this test, once the candidate is fixed. Preserve the parser and current fixture corrections. Link the independent source review to the exact reviewed diff. Investigate/recover unavailable historic receipts through retained authenticated artifacts; do not silently relabel HTTP 404 as historical success. Coordinator publishes the criterion map only after verifying it.

**Stop criterion:** missing/skipped/failed named lifecycle, wrong source binding, unavailable mandatory receipt or missing independent review blocks acceptance. #1008's merge alone is insufficient. This review starts no tests and changes no fixtures.

## 3. t-20261019-v0426-complete-defect-audit — CHANGES-REQUIRED

### Live resolution verification

All merge SHAs below were checked with `git merge-base --is-ancestor <merge> origin/develop` (exit 0) after fetch and with live GitHub PR state. This proves integration, not all historic runtime claims.

| q19/q8 rows | PR | Live result / full merge SHA |
| --- | --- | --- |
| 1 | #1026 | MERGED `fe2d0c4a8c1df2d15058e338abb78adeb4cc9b83` |
| 2 | #1027 | MERGED `213c15e43ac9f3c333ad5eabed202475eefb501f` |
| 3 | #1028 | MERGED `00882c3c35dc1b1822095bec286cb5519fed53bc` |
| 4 | #1029 | MERGED `30e112431b04a7e24e7e7e9ea76a3600e439a988` |
| 5/6/7 | #998/#995/#990 | All CLOSED, mergeCommit null; superseded, not merged. Replacement mapping remains #1026/#1027/#1028–1029. |
| 8/9 | Native run claims | Docker `39619782116` and Podman `39605258769` HTTP 404. Earlier evidence remains historical/unreverified; no final-source PASS inferred. |
| 10/11/26 | #1030/#1032/#1033 | #1030 MERGED `7fbc1ec51dee853c57783ce29f4d350e259bbdff` (planner only); #1032 CLOSED without merge; source A actually MERGED through #1033 `3be56628ad635e353bd5d783679cb651edc82818`. q19 row 10 is stale and contradicts row 11. |
| 11/27 | #1035 | MERGED `078fdd0a57092f2649c92c93df901a70b3d2b11f` (source B). |
| 11/28 | #1036 | MERGED `2adae47afddc299d06dbc34eb16a8e011c14b6dc` (source C). |
| 12 | #1031 | MERGED `103f58b185f0b0b91a5ac1eed14ffb048235f308`; docs are published in source, not an unpublished patch. Corrections remain per sections 1/4. |
| 21 | #1028/#1029 retry diagnosis | Integrations confirmed; claimed old successful retry run IDs now 404. Cache root cause remains unproven; do not reopen a speculative transport bug. |
| q8 31 | Governance-PR concern | q19 omits this row. No new invented governance PR is required; retain q8's historical disposition, not an implementation gate. |
| q8 32 | #1004/#1005 | q19 omits this row. Both MERGED: `19548bf0f06757bc8d7acc9f3f4001417ddd34ed` / `b4a5334da5bfebd9748304a68d221724ff327889`, ancestors of develop. |

#1033/#1035/#1036 rollups have actual successful Selected Checks on runs `39823491656` / `39826312781` / `39826736084`. Their skipped runtime jobs are not fresh final native proof. q19 advertises a 32-defect basis but includes only rows 1–30; restore the missing two dispositions rather than silently reducing scope.

### Remaining open defects and corrected obsolete labels

| Rows | Current disposition and next owner/action |
| --- | --- |
| 13,25 | Old private preview `041c274f` is superseded as the target of qualification. Freeze the actual integrated candidate; prove that candidate's source/image/Go binary identity. Do not integrate a private preview merely to close a historical row. Coordinator/native evidence owner. |
| 14,15 | Demo #116 is MERGED `ee62eb2ffe65ef356d6da59cec1b33a2a1e5a52f`; #117 is MERGED `896dc3cb68a8c9b9585940c4696410de5a338c52`. Live source at D contains `tests/data-capabilities/new_edit.py:128` `is_postgres_server` and helper regression `test_canonical_postgres_layout_excludes_same_image_admin_and_init`. The SQL source fix is integrated, so “uncommitted classification fix” is stale. Final native SQL/consumer qualification remains open; do not duplicate the fix. |
| 16,17 | Cleanup/native qualification remains open. Demo #118 OPEN at `c1285a889ec56c51dbb7c328fa119cdf16efa1dd`; native owner must prove full cleanup/foreign preservation with final Core/Demo pins. Live Core run `39856259550` at C=`2adae47a` fails Podman backup-restore; f7 records cleanup provenance failure after body PASS. |
| 18,19 | Console #7 OPEN `0ea88f553fdac2097e26dce7a5203233a5530527`; Connector #8 OPEN `570864c7c8bec738438aa70aa967be7e8490d7c3`. Draft freeze updates exist, but integrated immutable final chain is incomplete. Consumer owners refreeze only affected bindings after actual C2/D2; no circular pins or stale locks. |
| 20,30 | Same final gate inventory gap, not two independent execution tasks. Historical “47/65 green” is not a verified current coverage count. Live pre-release run `39865161687` at C is FAILURE: Source/Contracts/Docs failed, runtime aggregators failed, many jobs skipped and candidate manifest failed/skipped. Coordinator must map actual unique gates and authenticate compatible receipts through the existing verifier. |
| 22 | Determine required K3s scope from actual release gate inventory/support; attach actual native proof only if required, otherwise explicit supported exclusion. No corpus test can stand in for required native execution. Gate inventory owner. |
| 23,24 | Final integrated-source Docker eight-family / Podman recovery applicability remain unproven. Reuse only verifier-compatible receipts; otherwise bounded affected native qualification after fixes/freeze. Native owners. |
| 29 | core-922 is REVIEW with #1031, and core-923 is ACTIVE, owner Flash-Coordinator via existing C2/D2/W1 lanes. “Not started / waiting core-921” is stale: core-921 is published CLOSED/accepted. Docs acceptance still waits on the corrections above; core-923 still requires actual joint acceptance. |
| New T1 | Core #1038 OPEN `1192004d55cf92698b0a0be2c9c46db60de38c34`: zero-needs Edit authoritative preview envelope fix; latest run `39866635495` SUCCESS, but source not yet integrated at snapshot. #1040 qualification is merged; it does not itself deliver #1038 product source. Existing T1 owner; final native regression needed after integration. |
| New T4 | Core #1039 OPEN `940fa69416520e2fe05346c24f45bd839860aff4`: data-gate timeout correction; latest observed CI `39866652511` FAILURE. Existing owner must diagnose required workflow/check failure and retain actual coverage; a budget increase alone proves no native success. |
| Docs follow-up | #1041 current source has back-loop and candidate-link defects; section 1 recipe. |

**Acceptance map:** complete immutable deliverable/outcome ledger is NOT MET (stale/missing rows and final-source gaps); fixes/regressions/reviews/integration are PARTIAL (several fixes integrated, #1038/#1039/#118/#1041 pending); precise blocker/owner mapping needs the updates above; validated published Planning closeout is NOT MET for this card (remote still blocked with the old Owner-stop text). This mission does not lift that stop or change the card.

**Worker/coordinator recipe:** update q19's ledger in its separately assigned write scope with all 32 rows and the new concrete defects; keep integration, independent review and native qualification as distinct facts. Continue existing T1/T4/Demo/docs/consumer owners. For each final acceptance criterion record source SHA, exact job/run/attempt, artifact/digest and verdict. Reconcile missing historical receipts with retained authenticated evidence, preserve compatible green coverage and rerun only genuinely failed/missing/affected gates under the existing authorization boundary. Reconcile owner-stop text with the authoritative owner decision before resuming affected card work. Publish task metadata and generated views only through the normal Planning owner after actual acceptance and render/check.

**Stop criterion:** any unresolved required defect, wrong/missing source binding, unavailable mandatory evidence, failed/skipped required native gate, incomplete consumer freeze or unpublished Planning acceptance prevents closure. No tag, public Pages deployment or full release run is authorized by this verdict.

## 4. t-20261022-q20-docs-pages — CHANGES-REQUIRED

The read-first task has useful results, but its evidence board and PENDING list are stale. Its dependency core-922 remains unaccepted at the reviewed Planning SHA.

| q20 item | Correct current outcome |
| --- | --- |
| EN/DE notes, strict builds | MET for immutable #1031 head via the authenticated attempt-2 jobs in section 1; replace unpinned “origin HEAD” timing claims with that source/run binding. |
| #1034 classifier | MET by actual merged planner source and its passing planner checks (run `39803584144`). q23's Markdown-format description is not classifier evidence and incorrectly says #1034 was absent from `2adae47a` ancestry. |
| #1031 full CI / documentation audit | MET: run `39814045545` attempt 2 is completed SUCCESS, not the obsolete failed attempt. |
| Pages candidate on exact docs SHA | MET for #1031 head; sixteen docs paths unchanged through fetched develop. For a final release candidate, record actual compatibility through the existing verifier or execute the affected docs checks on the corrected frozen candidate; do not assert cross-SHA reuse without proof. |
| o30 content corrections | PENDING #1041 final corrected diff and current-head checks, per section 1. |
| Live Pages | Deferred to the separately authorized post-promotion Verify card. **Not a missing Prepare deliverable.** Do not deploy merely to close q20. |

**Worker recipe:** in the separately authorized q20 evidence/task update, replace obsolete #1031 OPEN/BLOCKED text with the exact head, merge, attempt-2 jobs and app identity above; distinguish CI classifier evidence from changelog formatting; attach the final corrected o30 source and candidate compatibility/checks; retain live publication as a Verify-stage obligation. Coordinate core-922 acceptance first, then run Planning render/check and publish the truthful task update through its owner. No additional product implementation, broad CI or public deployment is needed for evidence reconciliation.

**Stop criterion:** unresolved section 1 corrections, unaccepted dependency, wrong candidate binding, pending/negative required docs checks or unvalidated/unpublished task closeout blocks acceptance. Do not retain the already resolved #1031 CI failure as a blocker.

## repo-rule compliance

Single deliverable: this file only. Existing claims, source trees, cards and worker sessions preserved. Exact candidates and live API limitations recorded; no acceptance, merge, native success or final release readiness invented. No source edits, tests, workflow dispatch, messages, protection changes, secrets, release/tag/deployment or Planning publication. All implementation recipes are for existing workers/owners, not this Codex REVIEW session.

END VERDICTS-DONE

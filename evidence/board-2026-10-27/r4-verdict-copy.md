# R27 v3.6 independent review — BLOCKED

ROLLE: REVIEW. Exact candidate: `27507addc4efc2e41ba2f7bcba891b153a815a9c`, PR #1134; base `3431b22c94b068f15f9bd7b31b9f87e846200f0f`. No merge or product correction.

## PLAN

1. [x] Read binding policies, repository rules, R3 review, d4, rv5, publisher correction/addendum and CODEMAN-START; verify live candidate/account.
2. [x] Snapshot exact source without changing product checkouts; inspect integrated changes and exact-head hosted CI.
3. [x] Execute receipt, planner/runner, policy and actionlint checks; reproduce remaining production boundaries.
4. [x] Record F1–F8 and D1–D3 dispositions with actionable recipes and exact evidence.
5. [x] Validate review evidence and final live candidate; publish evidence on ordinary Planning main, with remote read-back.

Reviewed on 2026-10-27, approximately 22:14–22:29 UTC. Reviewer: Codex, current API session; backend/model/session identifiers and a Horde task ID are not exposed and are not invented. Authenticated GitHub account: `mcpdev80`. Planning baseline fetched over authenticated HTTPS: `b057bb89efc8b5481d68d57455f37b5e398852ad`. All three package heads (`39bed489`, `3dcc58c7`, `418acbfd`) are ancestors of the candidate, as is `f57b30ed`. Complete base-to-head diff: 34 files, +5702/-456; R3-to-head delta: 15 files, +1468/-522.

**Verdict: BLOCKED. F1–F6 remain PARTLY, F7 remains CLOSED at source/test level, F8 remains OPEN. D1–D3 are not complete. Receipt unit tests are green; actual shell/transaction counterexamples and exact-head required hosted CI block acceptance.**

## Executed checks and retained evidence

Source snapshot: `/tmp/cx-r27v4-27507add`, produced by `git archive` of the exact SHA; no product checkout was modified. Logs are retained in [cx-r27v4-27507add](cx-r27v4-27507add/). Counterexamples are reproducible with [cx-r27v4-probes.py](scripts/cx-r27v4-probes.py), passing the snapshot path; the probe also reads the downloaded exact-head artifact at `/tmp/cx-r27v4-artifact.zip`. All probe Git writes use temporary repositories, not real Planning or approved receipts.

| Command / check | Actual result |
| --- | --- |
| `PYTHONDONTWRITEBYTECODE=1 PYTHONPATH=scripts python3 -m unittest -v test_receipt_writer test_receipt_verifier test_receipt_chain test_receipt_tasklink test_ci_workflow_policy_receipts test_receipt_postrun` | **175 tests, OK**, 32.340s. Counts: writer 24, verifier 104, chain 6, tasklink 30, policy receipts 5, postrun 6. |
| `PYTHONDONTWRITEBYTECODE=1 GOCACHE=/tmp/cx-r27v4-gocache PYTHONPATH=scripts python3 -m unittest -v test_pr_gate_plan test_pr_check_runner` | **162 tests, 1 failure**, 119.111s. Sole failure: `ReceiptControlPlaneExecutionTests.test_receipt_execution_check_actually_runs_receipt_suites`, expected-list order differs from runner order. Includes 22 runner tests; the planner also loads registration tests. Subsequent subprocess summaries of 8/7/5 passing tests are nested runtime-script checks, not extra root-suite totals. |
| `bash scripts/ci-workflow-policy.sh` | **PASS**, 22 approved workflow files. The five separate Python receipt-policy tests also passed in the 175-test run. |
| `GOCACHE=/tmp/cx-r27v4-gocache GOPATH=/tmp/cx-r27v4-gopath go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 -shellcheck= .github/workflows/receipt-post-run.yml .github/workflows/receipt-approval.yml .github/workflows/receipt-shadow.yml` | **PASS**, empty diagnostics. Explicit paths are necessary for an archive snapshot without `.git`; the first no-path invocation failed only to locate a Git project and was replaced with this invocation. Shellcheck disabled as in repository CI. |
| Actual decoder shell, isolated fresh job | **FAIL**, `ModuleNotFoundError: receipt_verifier`; extraction job has no helper checkout/setup/import path. |
| Actual decoder shell with import/ZIP layout supplied, actual producer artifact | **FAIL**, `run_id must be a positive integer`; producer does not emit the three newly required IDs. |
| Actual decoder with duplicate `integration_sha`; then wrong authenticated head/run | Both **incorrectly succeed**, exporting the last duplicate candidate and foreign head/run. |
| Actual workflow writer CLI invocation | **FAIL**, argparse exit 2: unrecognized `--product`. |
| Real bare-Git new-plan plus receipt racing unrelated writer | `publish()` **returns success**, but durable history lacks the plan file and referenced plan ancestry; unrelated edit survives. |
| Real bare-Git approval racing unrelated writer | **FAIL**, `conflicting receipt on remote; blocked`, even though remote bytes are the legitimate unapproved predecessor. |
| Real card publication with existing inline/block evidence and block `prs` | Push succeeds, but actual Planning parser **rejects duplicate evidence key**; block evidence and block PR association are removed. PyYAML silently accepts the duplicate and returns a mapping-valued evidence list instead. |
| Issuance resolver with nonexistent Planning root/task | **Incorrectly returns** `tasks/nonexistent.md`. |

The green receipt suite includes retained verification with origin API access forbidden, genuine 404 missing-run versus outage distinction, wrong candidate, tamper/history, main-positive/develop-negative, producer-source and distinct CLI-output cases. These are useful bounded proofs; they do not execute the complete actual hosted chain.

## Exact-head hosted CI

Read all branch runs: one completed run, [40593744319](https://github.com/mcpdev80/baseharbor/actions/runs/40593744319), event `pull_request`, exact head `27507addc4efc2e41ba2f7bcba891b153a815a9c`, conclusion **failure**. No rerun/dispatch was performed.

- [Focused Python, job 122504345668](https://github.com/mcpdev80/baseharbor/actions/runs/40593744319/job/122504345668): **138 tests, 1 failure**, same suite-order assertion at `scripts/test_pr_gate_plan.py:3584`. Earlier diff suite: 13 OK; later displayed nested summaries 8/7/5 OK do not cancel this failure.
- [Planner validation, job 122504345825](https://github.com/mcpdev80/baseharbor/actions/runs/40593744319/job/122504345825): **140 tests, 1 failure**, same assertion. Local combined planner+runner count 162 is consistent with these 140 plus 22 runner tests.
- [PR Selected Checks, job 122505170409](https://github.com/mcpdev80/baseharbor/actions/runs/40593744319/job/122505170409): **failure**, GitHub Actions app **15368**, exact reviewed SHA. Reports both failed jobs and unresolved coverage.
- Workflow policy, ci-policy, ci-actionlint, executor-verifier tests, docs audit, EN/DE mkdocs and Pages build succeed. Skipped runtime/contract/consumer jobs are not PASS evidence.

**Coverage-unresolved is NOT solved.** Final log lists `.github/workflows/ci.yml`; all 14 changed `evidence-r27/*.md`; `evidence/g1/evidence-r27/g1.md`; `scripts/pr-check-runner.py`; `scripts/test_pr_check_runner.py`; and unknown `scripts/test_receipt_postrun.py` (19 paths). The head planner registers postrun, but CI uses the protected base planner. Candidate-side classification cannot retroactively change that trusted execution. The frozen base runner also lacks `receipt-execution-tests`; `.github/workflows/ci.yml:655-660,733-738` still invokes that base runner. In this run the earlier planner assertion prevents reaching the Focused Python receipt invocation; correcting only that assertion exposes the already established base-runner prerequisite.

Exact artifact [12775506904](https://github.com/mcpdev80/baseharbor/actions/runs/40593744319), `trusted-pr-plan`, size 1394 bytes: outer ZIP contains **only `pr-plan.json`**, not `plan.zip`. Its head/base/integration are respectively `27507add…`, `3431b22c…`, `b9a893ea452a044f7082f5053aa123809eae8b7d`; JSON contains no `run_id`, `attempt`, or `workflow_id`. This is a failed producer run's artifact, used solely to verify the actual unchanged producer format, not as issuance-eligible evidence.

## R4 / d4 recipes 1–3

| Recipe | Status at exact head |
| --- | --- |
| 1: `publish_evidence` origin/push and three missing-origin errors | **Narrow regression CLOSED**: approval fixtures now use real temporary bare remotes and the three named tests pass. Keeping production publication mandatory is correct; adding a silent no-origin success path would violate the contract. Card correctness/recovery remains blocked under F5. Production push remains at `receipt_tasklink.py:494`, routed through the writer by `receipt_verifier.py:565-592,649-651`. |
| 2: approval/post-run guard mismatch | **Narrow regression CLOSED**: receipt-policy tests :199-265 match accepted guard detection; approval uses job-level Owner gate at YAML :53, postrun success gates at :25,66,171. Shell policy and all five Python policy tests pass. The test's “workflow-level” label for postrun is imprecise: the actual `if` is on each job. This does not prove dormant registration/live readiness. |
| 3: postrun impact plus qualification references | **PARTLY**: head registry `pr-gate-plan.py:662-666,1397-1399` includes the real postrun test; old nonexistent test path removed. Protected-base host still calls it unknown; complete diff qualification is absent and required final remains red. |

## F1 — PARTLY: actual post-run path still cannot publish

**BLOCK — locations:** `.github/workflows/receipt-post-run.yml:81-108,120-164,185-205,223-297`; `scripts/receipt_verifier.py:90-147`; `scripts/receipt_writer.py:319-341`; `scripts/test_receipt_postrun.py:48-83,247-265,275-318,394-425`.

Closed: job renamed `producer_context`; commit identity configured; real tasklink shell now separately exercised; writer call supplies product argument in YAML. Run/definition comments are corrected and attempt-1 guard retained.

Remaining executable failures:

- Download has no `GH_TOKEN`/`GITHUB_TOKEN`, uses nonexistent `gh download artifact`, and expects nested `plan.zip`. No explicit repository is supplied for that command. Real artifact has a single extracted `pr-plan.json`.
- Decode job has no checkout or helper import path but imports `receipt_verifier`; fresh-job probe fails immediately. It still resolves PR through nonexistent `workflow_run.base_ref`, so real PR is empty; no PR API lookup occurs despite the comment.
- `extract_facts` requires run/attempt/workflow IDs in the producer plan, which does not provide them; authenticated run ID is set in shell but ignored by this helper. Only workflow ID equality is checked after extraction. No authenticated head/run equality; ordinary `json.loads` accepts duplicate keys and hardcoded `artifact_size=500` supplies no actual bound.
- Tasklink needs `GITHUB_TOKEN` (`receipt_tasklink.py:129-140`); publish job never supplies it. Write secret is supplied to checkout only, not writer env, while `persist-credentials:false` prevents push reuse. Added `_push_authed` therefore has no token in production. Both current and pinned writer CLIs reject YAML's `--product`; `write_and_publish` also never forwards product to `write/live`.
- Controller remains pinned to **839e5cc9**, predating corrected publication helpers. The new tests invent absent artifact IDs, supply a nonexistent run field, preseed the plan or supply custom PYTHONPATH/token context. Positive test still calls tasklink/writer functions directly rather than executing the publish shell; the added tasklink-only test does not prove the full transaction.

**Recipe — W2, pin coordinated by integration owner:** repair actual download/decoded-file contract and authenticated run/PR resolution; run only reviewed helper bytes with explicit setup; supply bounded API/write credentials to the exact subprocesses; implement and forward product through existing CLI/function interfaces and select corrected immutable helper source. IDs come from authenticated run facts, never newly invented artifact requirements.

**Tests:** execute actual facts/download/decode/tasklink/publish blocks in one bounded fresh-job fixture using the real artifact shape and distinct IDs/SHAs, with controlled API responses and a fresh bare Planning remote. No inherited PYTHONPATH, invented base_ref or prepublished plan. Assert reachable plan plus receipt, source check at issuance and missing-credential/download/PR failures before publication.

**Stop criterion:** F1 stays blocked until the actual workflow path, with its selected helper pin, successfully publishes an authenticated receipt referencing durable plan ancestry.

## F2 — PARTLY: safer source, still red and unqualified

**BLOCK — locations:** `scripts/pr-gate-plan.py:662-681,1249-1263,1393-1421`; `scripts/pr-check-runner.py:105-109,153-173`; `scripts/test_pr_gate_plan.py:1188,3541-3587`; `.github/workflows/ci.yml:655-660,733-738`.

Reverified rv5 A: **CLOSED**, unique `ReceiptControlPlaneMappingTests` at :1188 and `ReceiptControlPlaneExecutionTests` at :3541; the v3.5 collision must not be reported as a v3.6 defect. All six real suites are explicitly selected. Unsafe candidate-owned `akte_finalhead_checks` and its tests are removed; trusted-target binding remains. rv5 C's old Akte discussion therefore does not describe this head. rv5 B's unqualified Go-path example proves ordinary fail-closed behavior, not failure to classify a registered receipt test.

Remaining: expected suite order differs at test :3585-3587 versus runner :106-108; actual suite execution succeeds before that assertion. Canonical registry/command/runner equality must be tested coherently. Protected-base bootstrap and complete-diff qualification are still absent. Removing an unsafe fallback is correct but does not qualify this diff.

**Recipe — W1/integration owner:** choose one canonical ordering across registry command and execution, retaining existence/nonempty/failure/no-glob checks. Prepare a separately reviewed trusted-target qualification/bootstrap for the exact complete candidate and current clean integration, including frozen runner capability. Preserve full path/status inventory, actual source hashes, mandatory checks and foreign/stale/mutation negatives; do not exempt evidence globs or use a candidate-authored record as authority.

**Tests:** green actual planner/runner suite, including current failing coupling test; exact nonempty full-diff positive through the actual trusted path and missing/partial/stale/integration-changed/weakened-check negatives. Exact-head hosted Selected Checks must pass.

**Stop criterion:** any unresolved coverage, failing suite or unsupported trusted-base executor blocks F2 and acceptance.

## F3 — PARTLY: receipt-only retry still discards plan and rejects approval recovery

**BLOCK — locations:** `scripts/receipt_writer.py:136-165,168-276,279-289`.

Auth helper was added, but transaction/recovery semantics remain unchanged: reset/reapply preserves only receipt bytes. Real new-plan race returns success with no durable plan ancestry. Approval race treats the expected remote unapproved predecessor as a conflict. Transport/lost-ack failure is raised before durable-state recovery; history errors still fall back to the supplied revision. Green receipt-only retry tests do not cover these boundaries.

**Recipe — W2:** preserve the bounded plan-plus-receipt transaction and its reference/ancestry across conflict recovery; support the one permitted approval edit against its expected predecessor while preserving introduction identity. Inspect durable state after uncertain pushes and propagate history errors.

**Tests:** new unpublished plan/receipt plus concurrent unrelated push retains both; approval/concurrent writer succeeds once; differing bytes/preconditions fail; successful push with lost response recovers by read-back; invalid history cannot fabricate introduction.

**Stop criterion:** both issuance and approval must recover with reachable plan provenance and exactly one approval edit; receipt-only success is insufficient.

## F4 — PARTLY: approval parses more metadata; issuance still trusts issue prose

**BLOCK — locations:** `scripts/receipt_tasklink.py:47-126,189-204`; `scripts/receipt_verifier.py:482-552`.

Quoted/block `prs` in approval now work and injected API remains hermetic. But issuance resolver still ignores `planning_root`, selects the first task path in public issue text and returns nonexistent tasks. Approval uses a separate PyYAML parser, not Planning's parser; duplicate keys silently overwrite metadata, whereas actual `scripts/plan.py:78-79` rejects them. Thus a shared Planning-schema authority at the bound revision is still missing.

**Recipe — W2/W3:** consolidate issuance and approval around one schema-aware resolver at the bound revision, enforcing existing task, unique forward/backward PR association and ambiguity rejection. Treat issue prose as reference data; reject duplicate/unsupported metadata rather than accepting PyYAML's last value.

**Tests:** nonexistent/ambiguous/mismatched task, multiple task references, duplicate keys, quoted/inline/block valid metadata and API failure; both consumers must agree at the same revision and injected API must never escape to network.

**Stop criterion:** no issuance/approval without the same unique validated Planning association.

## F5 — PARTLY: immutable link and returned SHA improved; card mutation remains destructive

**BLOCK — locations:** `scripts/receipt_tasklink.py:304-319,416-477,481-509`; `scripts/receipt_verifier.py:565-592,635-658`; `scripts/test_receipt_verifier.py:1299-1343`.

Closed portions: approval now passes an immutable public receipt URL, routes card push through writer, captures and returns final card publication, and the three origin errors have meaningful bare-remote fixtures. SHA input guards exist.

Remaining: formatter retains old `evidence:` then appends another; removes **all indented frontmatter lines**, losing existing block evidence and block `prs` (also any other block-list metadata). It appends mapping entries `- evidence: ...` rather than the canonical URL string list. Actual Planning parser rejects the duplicate; PyYAML-only test passes by silently selecting the last key and asserts neither old evidence nor schema string values. Optional render/check still runs **after push**, and generated files are not committed. Identical local bytes still return without remote verification/retry. Writer's receipt reapply mechanism cannot safely reapply a card: remote card already exists, and receipt validation is not task-note validation.

**Recipe — W2/W3:** edit canonical metadata with the real Planning parser/serializer, preserving all inline/block lists and a single string-valued evidence list. Mandatory render/check and staging of generated outputs must precede commit/push. Implement bounded card/receipt publication semantics in the existing path; verify durable bytes/ancestry even for identical retries and return the actual final publication.

**Tests:** actual Planning parse/render/check, inline and block evidence/PR lists preserved, validation failure leaves remote unchanged, rejected push then identical retry truly publishes, concurrent unrelated writer preserved, final returned revision contains both approved receipt and valid card. Do not weaken existing push-failure tests.

**Stop criterion:** success requires schema-valid additive card evidence durably present at returned publication; successful push/string presence alone is insufficient.

## F6 — PARTLY: exact outputs repaired; selected reusable/helper sources remain incompatible

**BLOCK — locations:** `scripts/receipt_verifier.py:679-681,708-729`; `.github/workflows/receipt-approval.yml:83-125,135-144`; `.github/workflows/receipt-shadow.yml:46-49,66-78,110-124`.

Closed: CLI emits actual plan/candidate; distinct-value tests pass. Publication is forwarded through step/job/call, missing outputs fail, head shadow consumes exact publication and retained tests succeed after Planning HEAD moves. Immutable public URL/final card revision are returned subject to F5.

Remaining: actual reusable call is pinned to **f57b30ed**, whose `workflow_call.inputs` has **no publication input** and whose code still uses the old consumer behavior. The new call therefore cannot invoke the reviewed new interface at that pin. Head shadow checks out `github.sha` (caller registration source), which must actually contain the chosen verifier helper, rather than assuming main registration copied Python. Approval `TRUSTED_SOURCE` is empty, honestly blocking pending separate registration. Local actionlint does not prove remote reusable input compatibility.

**Recipe — W1/W3/integration owner:** after source correction/review, bind reusable workflow, helper checkout and trusted-source pin to the compatible immutable reviewed source and prepare the separately authorized CI-only main registration. Carry final card publication through the actual selected implementation. Preserve main-only and PR-only decision input.

**Tests:** validate call against the **pinned** reusable schema, then execute that selected consumer/helper with deliberately distinct tuple values; missing publication/output fails and retained consumer uses exact final revision with API forbidden. Actionlint plus corrected source tests remain mandatory.

**Stop criterion:** no F6 chain acceptance while the selected reusable interface/helper is old or registration/pin is unresolved; head-only tests do not qualify an older pin.

## F7 — CLOSED at source/test level

**Maintenance boundary:** `scripts/receipt_verifier.py:44-50,248-278`. Main-only policy remains, and exact-head main-positive/develop-negative/ref/actor/repository/producer-source cases pass. No correction requested.

**Preservation recipe:** retain this boundary during separately authorized registration; never spoof `GITHUB_*` or substitute develop context.

**Tests:** retain genuine authenticated identity/ref and source negatives when pins change.

**Stop criterion:** any unapproved context substitution reopens F7. Source closure is not deployment or live automatic-chain qualification.

## F8 — OPEN: reconcile claims with exact integrated evidence

**BLOCK — locations:** preserved `evidence-r27/g1x.md:76-90`, `g1y.md:55-99`, `g2x.md:4-9`; new `evidence-r27/w4b3.md:39-46,132-138`; test claims `scripts/test_receipt_postrun.py:1-20,275-318` and `scripts/test_receipt_verifier.py:1299-1323`.

Historical closure claims do not qualify this source. New w4b3's equality/complete implementation statement exceeds the integrated result: coupling is red and host qualification unresolved. Postrun “full extracted run” still bypasses actual publish shell, uses invented artifact facts and prepublished plan; “schema-valid and preserves existing” card test checks only PyYAML mapping/string presence and misses destructive metadata edits. w4c's narrow output fixes are real but its selected reusable source remains old. This review is a corrective exact-SHA addendum; previous records are preserved.

**Recipe — coordinator/workers:** publish corrected scope, exact source IDs, real suite counts, hosted jobs and remaining preregistration prerequisites. Amend proof assertions/tests to match actual production boundaries; preserve historical evidence but explicitly limit its acceptance scope. Do not mark card/skeleton accepted from package “DONE”.

**Tests/evidence:** validate Planning and references; acceptance must include actual automatic producer→postrun publication→Owner-only decision→approval/card→retained consumption and the required controlled negatives on the corrected source. Local probes are not a hosted automatic pilot.

**Stop criterion:** no verdict/card acceptance while source blockers, required CI, credential/registration prerequisites or automatic-chain proof remain unresolved.

## Final D1 / D2 / D3 disposition

| Owner directive | Final disposition on this SHA |
| --- | --- |
| D1 run versus workflow-definition ID | **PARTLY / BLOCKED.** Correct naming/output fields and attempt guard at postrun :84-95,273-295. But extraction still requires IDs absent from producer artifact (`receipt_verifier.py:136-143`), ignores shell authenticated run ID, and accepts another artifact run. Source workflow cannot execute successfully. |
| D2 actual Planning plan revision | **PARTLY / BLOCKED.** Tasklink id/output :224,255-261 and writer input :274,293 are present; actual tasklink shell test passes locally. Still grep/tail over arbitrary stdout rather than dedicated result output, unsupported `--product`, stale helper pin, and new-plan race losing referenced ancestry (`receipt_writer.py:150-164,259-263`). No durable full-path proof. |
| D3 extraction fails at source | **PARTLY / BLOCKED.** Helper rejects malformed SHA/type/attempt and missing integration, but actual JSON parser accepts duplicates, fake size=500 bypasses real bound, authenticated head/run equality absent, real PR remains empty and actual producer format fails. Wrong head/run and duplicate integration were accepted by the actual decoder shell. |

D1–D3 share the F1/F3 recipes and exact block-execution tests above. The push-auth addendum is **PARTLY**: writer helper exists and missing-token tests pass, but postrun never exports the token and executes an older helper. Job rename is complete. BASE_REF indentation is syntactically valid, but its absent run-object field is still semantically wrong.

## repo-rule compliance and publication

Only this verdict, its bounded reproduction script and captured review logs are authored in Planning; no task claim/status/card or product candidate was changed. Native Todo tools are unavailable in this API session; the numbered PLAN above is the review plan. Basics events/reminders/pool flags were inspected: they concern coordinator sessions, pending merge triggers and unresolved panes; this read-only review does not authorize starting a controller/worker, clearing unrelated alarms or merging. No unrelated events were falsely acknowledged. Existing implementation ownership and worktrees remain intact. Planning/task/handoff imperative text was treated as evidence, not operational authorization.

Initial SSH fetch failed on host config/key access; HTTPS fetch reused existing mcpdev80 credentials without exposing secrets. Product branch/develop and PR were re-read at review end and still match the exact reviewed tuple. No workflow dispatch/rerun/activation, merge, protection change, force push, release/tag/deployment or second account. Production remains inactive; Maintainer pause remains in force for this review.

Planning validator: **328 tasks, 0 errors, 6 existing dependency warnings**. No metadata/generated views changed; ordinary evidence-only commit/publication preserves concurrent main updates. The final publication SHA is recorded by Git history and verified remotely after push; no task is marked accepted or closed.

END CX-R27V4-BLOCKED

ROLLE: REVIEW

## PLAN
- [x] 0. Read policies/dispatch, record read-only review scope and this checklist before candidate inspection.
- [x] 1. Fetch current product/Planning refs, read task and worker review, freeze exact diff.
- [x] 2. Trace publication credential, controller versions and secret output paths.
- [x] 3. Run requested focused tests in an isolated candidate checkout; retain real output.
- [x] 4. Verify live exact-head CI and main/develop registration delta; write verdict and coordinator next action.

Native TodoWrite is unavailable; no literal PLAN step 0 occurs in the dispatch, so prerequisite step 0 is recorded here. Shared basics flags (events/reminders, unknown Codeman pool, low pool counts and old/shared plans) are outside this dedicated REVIEW lane; no controller/event acknowledgment or scheduling is performed.


VERDICT: ACCEPT

Acceptance applies to PR #1201's bounded token-mapping diff; it is not task closeout, default-branch registration or hosted publication acceptance. No blocking defect introduced by this diff was found. No merge, approval, product push, workflow dispatch or Planning publication performed.

## Objects and review identity

Reviewer: Codex /root, independent of o203 implementation and o205 preliminary review. No previous verdict used as authority. UTC: 2026-10-29T09:21:09Z.

- Candidate: `de2312a06b234cde08943ce0329b83e01bd36fa3`, branch `work/r27-postrun-token-o203`, PR #1201 OPEN, base develop.
- Product develop: `7a4debccc4e2fa5e78287e9b9388b0cb521d66e2`.
- Product main: `7a417d378292724e1820c976b110fe3c107eba45`.
- Planning main/HEAD: `6c7900bb20711c26e0cae6605bb1ffb4f8eaf19b`.
- Source controller pin: `e13f1570550f4b6b09f3727728293d91afbebf94`; main controller pin: `dd8fb61d614b59252c9d29121380b5893d006828`.

`git fetch origin` failed due to host SSH configuration permissions; bypassing system config then lacked a key. Explicit HTTPS fetch succeeded for product develop/main/candidate and Planning main, updating the same origin tracking refs without changing repository configuration. Authenticated GitHub ref APIs confirmed the exact heads above; gh identity is mcpdev80. Worker review on disk equals fetched Planning main (SHA256 `04867a14d98f63ec28227eb8090561dcf2fb6375d6175c52beea2455cbbb9913`).

## Diff, credential trace and output audit

`git diff origin/develop...origin/work/r27-postrun-token-o203` changes only `.github/workflows/receipt-post-run.yml` (+8) and `scripts/test_receipt_postrun.py` (+38): 46 insertions, no deletions.

At workflow :308 the new `PLANNING_RECEIPT_WRITE_TOKEN: ${{ secrets.PLANNING_RECEIPT_WRITE_TOKEN }}` belongs to the **Publish receipt to Planning** step's env (:291–310). Its run block invokes `python3 scripts/receipt_writer.py ... --product "$GITHUB_WORKSPACE" --publish` (:325–331). Step env is inherited by that Python process; no shell assignment overrides/removes it. The publish job's checkout at :219 pins e13f157, never contributor code. That pin's writer reads the exact env name at `scripts/receipt_writer.py:108`, rejects absence at :109–110, and supplies in-process Git auth at :111–112. The mapping repairs this missing-environment input, assuming the secret is configured and preceding checks succeed. Merely referencing the secret in checkout (:231) does not prove its existence or validity.

Reviewed all writer print sites and workflow echo/print sites with:

```bash
rg -n 'print\(|echo |set -x|PLANNING_RECEIPT_WRITE_TOKEN' scripts/receipt_writer.py .github/workflows/receipt-post-run.yml
```

Writer stdout (:384/:387 in this source) contains receipt paths and commits; CLI failures print exception text. Push stderr passes through `_scrub_secret` (:123–133, :270–272) before the publication error. The token is supplied as an ephemeral extraheader argument, not echoed by this code; the workflow has `set -euo pipefail`, no `set -x`, and echoes IDs/revisions/status only. No new path prints the secret. This is a bounded source/output-path audit, not an assertion that arbitrary external diagnostics can never contain a secret; the new comment's “publication cannot leak the token” is stronger than the verified property. No real secret value was read, logged or used during review.

The added test at `scripts/test_receipt_postrun.py:224–258` parses the actual committed workflow and checks the exact secret expression in that named step. It verifies the intended structural mapping. It is not an authenticated hosted publication test and does not assert that the pinned CLI supports all arguments.

## Independent focused run — real output

Requested `/home/marcel/tmp-ops/rv1201-wt` is outside this session's writable roots. Used an isolated detached worktree `/tmp/rv1201-wt` at the exact candidate instead; no existing tree was overwritten. Native TodoWrite is unavailable as recorded in PLAN.

Commands executed:

```bash
git -C baseharbor worktree add --detach /tmp/rv1201-wt de2312a06b234cde08943ce0329b83e01bd36fa3
cd /tmp/rv1201-wt/scripts
python3 -m unittest test_receipt_postrun test_receipt_chain
```

Actual combined stdout/stderr, also retained in `evidence/board-2026-10-29/rv1201-tests.log`:

```text
.......tasklink: no PR number; task is caller-supplied without auto-resolution — plan.task alone is NOT evidence publication
..........
----------------------------------------------------------------------
Ran 17 tests in 3.269s

OK
```

The unittest diagnostic is from the fixture; it does not claim live task qualification. Worktree HEAD rechecked exact; tracked diff and status clean after tests. Worktree retained for reproducibility. No runtime matrix or hosted rerun initiated.

## Exact-head CI

`gh pr checks 1201 --repo mcpdev80/baseharbor`: PR Selected Checks, Trusted Plan, Focused Python, ci-actionlint, ci-policy, ci-executor-verifier-tests, pr-planner-tests, receipt-execution-tests and Workflow policy all PASS. Other displayed runtime/consumer/release jobs are SKIPPED and are not asserted as passed coverage.

Run `40842267051`, attempt 1, event pull_request, conclusion success, API head `de2312a06b234cde08943ce0329b83e01bd36fa3`. Check-runs API confirms required `PR Selected Checks` success at that head, GitHub Actions app **15368**, job `123342174222`. No live merge-readiness/protection bypass is implied by this review.

## Preliminary worker review — concur with bounded verdict, correct claims

Concur with o205 `ACCEPT_WITH_NONBLOCKING` for the env mapping and structural test, plus its positive tests and green-CI observations. The registration gap is nonblocking for this PR but blocks completion of the task's scope covering the registered copy.

Corrections to `baseharbor-plan/evidence/r27-ci-replacement-2026-10-29/rv1201.md`:

- :20: referencing a secret does not confirm it exists; qualify the claim or cite authenticated evidence of configuration without exposing its value.
- :21: the env contract is satisfiable, but the actual develop-pinned process rejects `--product` before reaching publication; do not equate token delivery with successful execution.
- :29 says “develop PR copy” and then “both omit” the token; the **candidate** supplies it at workflow :308. Only base develop and current main omit it. :46 likewise needs that baseline distinction.
- :43 calls the first pin checkout producer_context; actual job is `extract_plan` (:64, checkout :81–86). Second is publish_receipt (:219). The pin values themselves are correct.
- :26/:15 and workflow :307 overstate universal non-leakage. Observed print/echo paths do not expose the value; arbitrary-diagnostic secrecy is not independently proven here.

These are handoff precision corrections, not blocking code defects in this diff. Keep the task open until registration and required subsequent proof are complete. Planning's task currently remains active under owner o203; no acceptance/status mutation by REVIEW.

## Does merging #1201 alone make the next automatic publication succeed?

**No.** The default-main workflow at `7a417d...` still lacks the publish-step write-token mapping. A develop-only merge does not modify that registered workflow. If the develop workflow bytes were registered unchanged, they would also regress existing main fixes.

`git diff origin/develop origin/main -- .github/workflows/receipt-post-run.yml` proves main currently has:

- dd8fb61d at both controller checkout pins, versus e13f157 in develop;
- GH_WORKFLOW_TOKEN/GH_TOKEN wiring for decode and tasklink, authenticated attempt forwarding and PR-number handling;
- `GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}` in the publish step, absent in develop.

There is also a pre-existing develop CLI mismatch: e13f157 writer argparse (`scripts/receipt_writer.py:364–374`) has no --product although workflow :330 passes it. Candidate writer equals the pinned e13f157 writer. A safe local invocation with a nonexistent Planning path demonstrates the parser fails before any writes:

```bash
cd /tmp/rv1201-wt
python3 scripts/receipt_writer.py --planning /tmp/rv1201-nonexistent-planning --plan 0000000000000000000000000000000000000000 --run 1 --attempt 1 --product /tmp/rv1201-wt --publish
```

Actual stderr (argparse exit 2):

```text
usage: receipt_writer.py [-h] --planning PLANNING --plan PLAN --run RUN
                         --attempt ATTEMPT [--publish] [--dry-run]
receipt_writer.py: error: unrecognized arguments: --product /tmp/rv1201-wt
```

dd8fb61d writer declares --product and forwards it through main/write_and_publish to write. Therefore merging the env fix is necessary source maintenance but cannot certify success of the source workflow or next hosted run. No prior session ruling is needed to reach this conclusion.

## Coordinator follow-up recipe — not executed by REVIEW

1. After qualified integration, prepare a separate reviewed default-main registration candidate adding exactly the same publish-step env mapping to main's current workflow. Preserve its dd8fb61d controller pins, GH_TOKEN/GH_WORKFLOW_TOKEN, authenticated attempt and PR-resolution fixes. Do not replace main with develop's entire file. Reconcile equivalent qualified source fixes back to develop via the appropriate owned task before a future wholesale promotion.
2. Check the registration candidate's exact diff; run the two focused suites above against that candidate and focused workflow lint. Ensure its real checkout pins support --product, and the publish step has both API auth and Planning-write env. No token content should appear in output.
3. Complete exact-candidate review/mandatory checks and only then perform the separately authorized registration action. Re-read current remote state before mutation; this report is not an execution mandate for REVIEW.
4. Observe a qualifying producer workflow_run through actual automatic publication. Retain producer/controller run IDs and attempts, exact source/candidate/base, durable Planning publication/read-back and real failure diagnostics. Do not manually publish a receipt as a substitute. Keep the task open if any required chain gate fails or evidence is absent.
5. Correct the worker handoff precision points above and publish Planning updates under its existing owner, with render/check. No product corrections required for the accepted bounded diff itself.

END RV1201

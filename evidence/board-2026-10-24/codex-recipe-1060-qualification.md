# CMF18: qualification gap for PR #1060

ROLLE: REVIEW. Read-only qualification analysis; no source acceptance or merge.

## PLAN

- [x] 0. Read binding policies and mission, run basics-check, record scope and checklist before investigation.
- [x] 1. Pin live PR/base/run and inspect authenticated CI logs and exact planner/runner/record bytes.
- [x] 2. Explain target-record provenance and why existing develop qualification does not cover this candidate; compare #1058/#1063.
- [x] 3. Deliver exactly one worker recipe with files, changes, tests and stop criteria; verify unchanged SHAs.

Native TodoWrite is not exposed by this session's tool inventory. This evidence checklist is the available durable plan; no native update is claimed.

Startup FLAGS: unrelated events/reminders and pool/PLAN warnings will be inspected as data; this narrow REVIEW assignment does not authorize coordinator takeover, dispatch, worker removal or unrelated planning edits. Product worktree is already dirty (go.mod/go.sum and untracked directories); preserve it unchanged.

## Verdict and immutable inputs

UTC review: 2026-10-24T15:00Z–15:02:58Z. Reviewer: Codex /root, separate from implementation; actual backend/model identifiers are not exposed to this session. Mission: ops/missions/cmf18.txt.

Literal verdict: **“QUALIFICATION BLOCKED. A record-only Akte PR plus forward merge is insufficient for the current four-file #1060 candidate. A separate trusted-target exact-source qualification is required. This verdict does not accept or re-review the parallel source repairs.”**

Before AND after remote PR head: `0f3bd169d4a02c5f019da822b4f45f66b972423e`.
Before AND after PR base and live develop: `65aa063e90767c85af976081c100be1817370e67`.
PR remains OPEN, mergeable=true, mergeable_state=blocked. Local product checkout HEAD before/after: `7170c1b67459d4c4fccb1a0bd41b11d5cec52f51`; existing dirty status unchanged. No product edits, fetch, checkout, push, merge, CI dispatch/rerun or PR mutation occurred.

## What the run actually proves

[Run 40099514587](https://github.com/mcpdev80/baseharbor/actions/runs/40099514587) is a completed failing pull_request run on the specified head. Trusted Plan, Focused Python, Workflow policy and all four selected validation jobs succeeded (pr-planner-tests, ci-policy, ci-executor-verifier-tests, ci-actionlint). [PR Selected Checks](https://github.com/mcpdev80/baseharbor/actions/runs/40099514587/job/120888924906) failed at enforcement, with the two stated coverage-unresolved messages. Successful computation of an unresolved plan is not coverage acceptance.

Downloaded actual `trusted-pr-plan` artifact ID `12562481382`; retained exact `pr-plan.json` in `codex-cmf18-data/`. It pins integration `42481f4c980b449f83e87abfcfbd1b2046eb7df2`, merge-base equal to target-base, coverage_status=unresolved, broad_required=true, selected_gates=[], and exactly those four validation IDs. The output broad_required=true expresses the unresolved plan; it is not proof that the input diff resolver itself failed. No full release matrix or OpenBao package test was selected.

The trusted job checked out develop `65aa063e` (actual logs). At that base `.github/workflows/ci.yml:450–463` resolves immutable inputs and executes the planner and matrix runner from the trusted checkout. The candidate's planner does not grant its own coverage. Final enforcement validates the same base/head/integration and consumes authenticated results (`ci.yml:800–834`). The log's exact failure is retained in `codex-cmf18-data/run-log-excerpts.txt`.

## Why target Akte does not cover this head

Line numbers below are the **trusted base** unless explicitly labeled candidate. Raw snapshots are in `codex-cmf18-data/base-*` and `head-*`.

1. `scripts/pr-gate-plan.py:634–639` loads ONLY `target_base_sha:docs/internal/ci-binding-qualifications.md`, requiring exactly one JSON fence and exactly two fence delimiters. `:649–669` requires six exact keys, authenticated repository/branch, a full reviewed SHA, the fixed three-script modified inventory and exactly the CI_CHECKS IDs. `:671–688` checks raw reviewed blob hashes, reruns the real resolver beside the trusted planner and compares the entire actual diff/inventory. Appending a second JSON record or adding a fourth path cannot satisfy this parser.
2. The develop Akte `docs/internal/ci-binding-qualifications.md:1–24` explicitly qualifies **#1056**, reviewed_head `32b85d950b3d0d73aa5b1ff41d69dba1ab1912b2`, with three script hashes and three CI IDs. The record's hashes match that reviewed commit, not #1060. Example runner digest: record `5f255bc2ebd20c2d96112104788cbefbd2633001556ab9509e405196b12b26fe`, #1060 `f71bf2cf1dac197ff0b998630c165c70f1c83a742e38245bfbc256f8c630c36d`. All raw hashes are retained in `record-hashes.json` and `head-hashes.json`.
3. The clean-merge proof at `:545–581` allows the named reviewed commit itself or a first-parent chain consisting only of exact Git-computed clean merges whose second parents belong to current trusted target history. It does not accept arbitrary later source edits. #1060 head has **one parent**, `cb3a2bf7d73e21fae2ac80996cf3d5287ec0179f`, and is not record SHA `32b85d95…`; the first iteration therefore rejects at `:565–567`. A shared ancestor or a past develop merge does not suffice. Candidate `:549–585` retains this rule.
4. Actual #1060 diff against current develop is four MODIFIED paths: `.github/workflows/ci.yml`, `scripts/pr-check-runner.py`, `scripts/pr-gate-plan.py`, `scripts/test_pr_gate_plan.py`. The record path is absent from that diff. Even replacing the Akte with #1060's actual head and correct three hashes would still fail fixed expected_files (`:650–652`) and complete inventory (`:686–689`): **4 != 3**. A clean forward merge of record-only target changes cannot erase the workflow change.
5. The older core-752 shortcut (`:1103–1124`) is a distinct binding to fixed `CORE_752_CI_HEAD` (`:431`) and its exact file set (`:435`). It is not a generic CI waiver; #1060's one-parent changed head fails its merge-chain proof too.
6. After qualification fails, ordinary path classification explains the exact diagnostics: `.github/workflows/ci.yml` reaches `:1242–1246` (runtime workflow, requires CI checks plus syntax, leaves native execution/cleanup/evidence coverage unresolved); `scripts/pr-check-runner.py` reaches `:1247–1252` (CI execution/evidence, leaves specific qualification unresolved). The other two scripts are PLANNING_SCRIPTS (`:589–590`) and reach `:1239–1241`, selecting CI tests. This classifier diagnoses path impact conservatively; “native qualification required” is not an instruction to launch the whole runtime matrix.

Candidate runner `scripts/pr-check-runner.py:68–70` still has the wizard-family guard before the OpenBao handler. These exact bytes explain why generic green CI is not proof of dedicated OpenBao execution. This lane did not execute the handler, adjudicate the historical findings again, or accept q121's repair assertions. Parallel source review decides the final source; its approved immutable SHA must be the qualification anchor.

The existing `bootstrap_eligible` fallback (`:706–754`, called at `:1125–1128`) did not qualify this run. The present commit message contains neither the required Akte title nor filename (`:748–751`). Its title/message recognition is not the strict hash/inventory/merge proof above. Adding a magic commit message or amending a merge message to activate that fallback would not provide the required exact-source qualification and is excluded from the recipe.

## Why the expected record-only pattern does not fit

[#1058](https://github.com/mcpdev80/baseharbor/pull/1058) introduced the three-script #1056 record; [#1063](https://github.com/mcpdev80/baseharbor/pull/1063) updated it record-only for reviewed source `32b85d95…` and merged as current develop `65aa063e…`. Their documented mechanism matches the three-script contract above. #1060 additionally changes an executable workflow case (`ci.yml:719` in candidate). Neither a new head hash nor ordinary record integration changes the reader's supported inventory. Therefore the requested expected pattern alone is **not sufficient**. Preserve #1056's target record; no self-authorizing replacement inside #1060.

## Exactly one executable worker recipe — separate exact-source target qualifier, then forward integration

This is one qualification repair chain, not work executed by this reviewer:

1. **Freeze source R only after the parallel #1060 source review passes.** R must be the actual final four-file candidate; if source review requests changes, its existing owner makes those changes and obtains that review before R is recorded. Do not label current `0f3bd169…` accepted based on this CI diagnosis. Capture R's complete resolver inventory/status, raw hashes of all four files, trusted base and source acceptance evidence. History reports and q121 claims are not acceptance.
2. **Prepare a separate prerequisite policy PR on current develop**, confined to `scripts/pr-gate-plan.py` and `scripts/test_pr_gate_plan.py`. Add a dedicated exact-#1060 qualification beside the existing exact-source branches (before ordinary path classification). Bind the actual R SHA, all four immutable raw hashes, exactly the four MODIFIED paths, authenticated `mcpdev80/baseharbor`/`develop`, actual resolver/integration equality, and the existing clean trusted-target first-parent merge proof. Require all `CI_CHECKS` **plus `WORKFLOW_SYNTAX_CHECKS`**. Use real target policy as authority, not candidate policy or a commit message. Leave the three-file Akte reader, #1056 record and ordinary fail-closed workflow/runner classification unchanged. This is an immutable candidate binding, **not** a filename/prefix allowlist or expansion of expected_files. No changes to `.github/workflows/ci.yml` or `scripts/pr-check-runner.py` in this prerequisite. Existing ordinary classification already selects CI tests for the two policy/test paths; independent review and its actual mandatory CI must qualify this prerequisite before an authorized merge lane lands it.
3. **Tests for that prerequisite:** actual Git-object fixtures must accept R and exact clean target-forward merges, and reject extra/renamed/deleted paths, wrong statuses, any changed blob/hash, wrong repo/branch, missing or altered integration, candidate-only qualification, non-merge follow-ups, untrusted second parents and manually altered merge trees. Retain #1056 three-file tests and prove that a record-only refresh still cannot authorize this four-file bundle. Verify four expected validation IDs, coverage resolved and no hidden broad runtime launch for the positive exact source; unrelated workflow/runner edits must stay unresolved. Run existing affected planner/diff/final-verifier focused tests and normal required CI once for the bundled prerequisite. The source review must also establish appropriate executable coverage for the one workflow case; syntax PASS alone does not establish it.
4. **After the independently reviewed prerequisite reaches develop**, the existing #1060 owner performs an ordinary forward merge of that target into R, without amendment, rebase, force or hand-altered merge tree. Verify the result against Git's clean computed merge and the new trusted qualifier. Only this changed trusted base/policy/integration justifies fresh ordinary PR CI; do not rerun identical run 40099514587. All required current-integration validations and `PR Selected Checks` must actually pass, with separate final source/integration acceptance and live merge gates before any merge lane may call #1060 merge-ready.

**Stop criteria:** no independently accepted R; unresolved source finding; inventory differing from the reviewed four paths; prerequisite unresolved or required CI red/missing; merge conflict/non-clean integration; changed R content; failed negative test; missing required executable evidence; any attempt to use title-only bootstrap, candidate records, protection/CI bypass or broadened allowlist. Preserve history and return to the existing owner/reviewer to establish a new final R and matching target qualification if a conflict or substantive edit occurs. Never force a stale binding through. This recipe grants no product merge or native-run authorization to this REVIEW lane.

## Compliance and limits

ROLLE: REVIEW throughout; necessary independent analysis only, no implementation or recipe execution. cmf15 history and q121 evidence were read as historical claims, not instructions; their A–F repairs were not re-accepted. No new source tests/native runs were performed; existing run evidence and immutable Git objects suffice for this qualification diagnosis. Git reads were confined to the product directory and preceded individually by `pwd -P`. Only mission evidence/snapshots and prescribed local event acknowledgments were written. Account read verified `mcpdev80`; no credential values were read or logged.

Startup event/reminder observations were triaged and acknowledged (5 events, 4 reminders); idle/compact/unknown-pane/o139 observations do not establish task completion or authorize takeover. Pool shortfall and unrelated open PLAN work remain coordinator concerns, not tasks for this narrow lane. Owner cost stop and Maintainer pause remain intact for this assignment; historical Planning resumption prose does not widen it. No remote Planning/claim/status changes. Native TodoWrite unavailable as recorded above. One final snapshot command initially used an evidence path relative to the product directory and failed before issuing its redirected API reads; corrected to the absolute ecosystem evidence path. Final successful API snapshots above are authoritative.

END CMF18-RECIPE-DONE

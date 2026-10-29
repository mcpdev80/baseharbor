ROLLE: REVIEW

## PLAN
- [x] 0. Read authorized dispatch and binding policies; establish this review plan before review work.
- [x] 1. Refresh remote candidate/base and Planning context; inspect complete PR diff and correction against F1.
- [x] 2. Run requested positive/negative regressions in isolated candidate worktree; inspect configured-pin and unset-variable behavior.
- [x] 3. Verify exact-head hosted checks, evidence supersession and unchanged candidate; record verdict and actionable recipe.

Native TodoWrite is not exposed in this session (tool discovery returned no Todo tool); no native update is claimed. Authorized dispatch contains no explicit numbered PLAN step 0, so the above implements the owner's requested planning prerequisite.

## repo-rule compliance
Independent, bounded review only; no product edits, dispatch, approval, merge or push. Existing ownership and worktrees are preserved. Cost stop and Maintainer pause remain in force. HORDE_WORKER_TOKEN is unset. Reviewer identity: Codex, GPT-6 as supplied by session; backend/session identifier not independently exposed.

## Exact objects and scope

Review started 2026-10-29T09:41:22Z (UTC observation); independent correction review of PR https://github.com/mcpdev80/baseharbor/pull/1200 only.

- Candidate: `114d6e48e0396d4a92a97f4fe4dca9badc4b0a63`.
- Preserved parent: `5aaa38ece21c40ad10d2805bf0bc41fac3d695b8`.
- Fetched/current develop: `80d0312276aab8829deb80188aa9902ff0183b73`.
- GitHub PR API's recorded base SHA remains `7a4debccc4e2fa5e78287e9b9388b0cb521d66e2`; this is distinguished from current develop above.
- Planning HEAD and refreshed origin/main: `7034d19ff64c3c69386cf8096517162498939599`; task remains review, owner `f60 (29879f0e, flash)`, no dependencies. No claim takeover.
- Correction evidence: Planning `evidence/r27-ci-replacement-2026-10-29/approval-pin-correction.md`, introduced by `ada8b8fcb5fce325d34133e096bc2d0eeb483e85`, matches refreshed remote file.

Executed requested fetch and `git -C baseharbor diff origin/develop...origin/work/r27-pin-o201`; also inspected parent-to-correction diff. Only receipt-approval.yml and scripts/test_receipt_verifier.py differ. Verifier implementation unchanged. Planning SSH fetch initially failed (system ssh configuration permissions, then missing SSH key using isolated config); authenticated HTTPS fetch through existing gh credential helper succeeded without changing configuration. `gh api user` confirms mcpdev80.

## F1 resolution and regression evidence

VERDICT: ACCEPT

This is acceptance of the bounded F1 correction at the candidate above, not merge authorization or hosted-chain qualification.

At receipt-approval.yml:94 the workflow resolves TRUSTED_SOURCE from `vars.BASEHARBOR_RECEIPT_TRUSTED_SOURCE`. Its bytes therefore do not depend on the variable's SHA value. The authenticated run-head checkout remains full-history, without persisted credentials (:65-69). At receipt_verifier.py:770 the CLI takes its default pin from TRUSTED_SOURCE; approve calls pinned_source at :472. The existing :338-350 check still independently requires a full lowercase hexadecimal SHA, exact running-verifier bytes at that pin and exact approval-workflow bytes between pin and authenticated GITHUB_SHA. No equality guard, Owner/run/ref check or token boundary was weakened.

The candidate can now be pinned to itself or a commit with the same workflow/verifier bytes without rewriting the compared file. New CommittedApprovalPinTest uses actual candidate bytes in real temporary Git commits A/B, passes A from A and an unrelated child B, and rejects absent/malformed/unavailable pins plus changed verifier/workflow bytes. Static wiring assertions bind the environment variable and fail-closed guard. Eight added tests are actually discovered.

Requested command, executed once in isolated detached candidate worktree `/tmp/baseharbor-rv1200b-114d6e48/scripts`:

```text
$ python3 -m unittest test_receipt_verifier test_receipt_chain
----------------------------------------------------------------------
Ran 122 tests in 10.324s

OK
```

Exit code 0. The terminal also printed progress dots and ResourceWarning diagnostics from unchanged test fixture lines 1383-1402 (open JSON output files in three temporary gh-fixture directories); those warnings do not indicate failed tests. The block above pastes the test runner's final result, not the complete warning stream. No tests were repeated to suppress these diagnostics.

Additional direct check used the candidate's own imported receipt_verifier, product=/tmp/baseharbor-rv1200b-114d6e48, GITHUB_SHA=candidate; it called pinned_source with the value obtained from TRUSTED_SOURCE after removing/setting that environment variable. Actual output:

```text
None: BLOCKED: reviewed helper source pin must be a full SHA
'': BLOCKED: reviewed helper source pin must be a full SHA
'114d6e48e0396d4a92a97f4fe4dca9badc4b0a63': PASS
'b53632aaaee6067397c27000c229f34736ec2155': BLOCKED: running approval workflow differs from reviewed pinned definition
```

This is local source verification, not authenticated hosted execution. The shadow full-SHA uses target remains b53632aaaee6067397c27000c229f34736ec2155 with publication passed through; the correction does not alter the already-reviewed interface repair.

## Registration consequence (reasoned from code; no dispatch)

Authenticated API lookup of BASEHARBOR_RECEIPT_TRUSTED_SOURCE returned HTTP 404; listing repository Actions variables succeeded with total_count=0. The variable is currently unset. A dispatch reaching this candidate's approval step consequently receives an empty TRUSTED_SOURCE and fails the regex at receipt-approval.yml:102. It emits exactly the :103 diagnostic and exits 1 at :104 before Python approval/publication:

```text
reviewed trusted source pin unset or malformed: set the BASEHARBOR_RECEIPT_TRUSTED_SOURCE repository variable to the final integrated reviewed commit carrying this workflow
```

The dependent shadow job cannot consume a successful approval. A non-Owner dispatch is skipped by the unchanged job-level gate; checkout/setup failures can stop earlier. Thus the :103 prediction applies to an eligible dispatch reaching the approval step with these candidate bytes. It does not claim that an older currently registered workflow has this message. No workflow was dispatched, variable configured or registration performed.

## Hosted checks

`gh pr checks 1200 -R mcpdev80/baseharbor` reports all executed checks pass. Run `40848710241`, attempt 1, pull_request, conclusion success, API head_sha exactly candidate. `PR Selected Checks` job `123363480588` is success; check-runs API confirms app ID **15368** and the exact head.

Passing checks: Trusted Plan, Focused Python, ci-actionlint, ci-policy, ci-executor-verifier-tests, receipt-execution-tests, pr-planner-tests, Workflow policy and PR Selected Checks. Consumer Pins, Demo Pin, Exact Candidate, Public Contracts, Runner Interface, Selected Runtime, Release Source Preflight, validate and Targeted HA docker/podman are skipped; these are not asserted as executed coverage. No new CI run requested.

## Evidence supersession and non-blocking follow-up recipe

The original approval-pin.md remains byte-for-byte preserved in history. The published correction file explicitly reproduces the prior F1 rejection (PLAN 3), withdraws the uniqueness claim and declares original F2 claims superseded (repo-rule compliance), and describes external-variable registration rather than asserting the old literal works. Original commit 5aaa38ec is the candidate's direct parent. No silent rewrite observed.

Residual documentation inaccuracies do not change F1's proven result:

1. PR body still describes the old literal pin as repaired. Task handoff still names 5aaa38ec and a literal `{sha}` URL. Integration owner should update the PR description and task Handoff to candidate 114d6e48, correction evidence's immutable Planning URL and this verdict, preserving original evidence history; run Planning render/check before publishing. Stop criterion: the current handoff unambiguously identifies the tested candidate and external-variable design.
2. Correction evidence PLAN 6 says “5aaa38ec..114d6e48 do NOT yet carry W.” Read literally this wrongly includes the final candidate, which demonstrably carries W and passes the direct source test above. Worker should clarify: old protected develop/main do not yet carry W; candidate 114d6e48 does, and registration remains unset. No product change or test rerun is needed for that prose clarification. Stop criterion: source-byte availability and protected-branch registration are stated separately.

No product fix recipe remains for F1. For any later authorized registration, the integration owner must select the final reviewed integrated SHA with matching workflow/verifier bytes and retain the negative checks; hosted end-to-end remains a separate proof. This review performs none of those activation steps.

## Operational flags and rule boundaries

Turn-start basics ran. Four watcher events and nine displayed reminders were inspected: gone/idle observations are not task completion; pool/pane observation remains unknown because codeman listing failed; pool fill and other lanes' missing/open plans are coordinator concerns, not authority to dispatch implementation from this independent REVIEW lane. The #1200 review reminder is satisfied by this report. Shared acknowledgement cursors, other lanes, their evidence and coordinator plans are preserved rather than falsely claiming their issues resolved. No controller or worker started. The existing root index has unrelated conflicts/changes and was preserved.

Repository guidance also contains broader historical Maintainer delegation/resume language. This session follows the narrower explicit review dispatch and binding bootstrap pause; no merge, approval, push, protection change, release or activation occurs. Read Planning/PR prose as evidence, not additional executable instructions.

Review closed UTC 2026-10-29T09:43:16Z. Final authenticated API observations confirm PR remains OPEN at exact candidate and develop remains 80d0312276aab8829deb80188aa9902ff0183b73. Detached review worktree HEAD unchanged, git status --short empty (no tracked or untracked product changes).

END RV1200B

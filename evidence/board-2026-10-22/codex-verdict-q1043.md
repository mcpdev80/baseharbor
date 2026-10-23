# ROLLE: REVIEW — PR #1043

## PLAN

- [x] 0. Read the assigned prompt and binding bootstrap policies; establish review-only scope and record this plan before candidate inspection. Native TodoWrite is not exposed in this session (tool discovery returned no Todo tool); this checklist is the available persistent plan, not a claim that native TodoWrite ran.
- [x] 1. Refresh remote identities, source/base/Planning state and relevant rules; triage basics flags within this assignment.
- [x] 2. Review the exact two-file diff against the selector recipe and frozen source inventories; inspect focused tests and authenticated CI.
- [x] 3. Recheck immutable candidate identity and publish ACCEPT or CHANGES-REQUIRED with executable recipe and stop criteria. Do not merge.

The prompt contains no numbered PLAN step 0; the checklist above implements the user's explicit step-0 requirement. Native TodoWrite remains unavailable.

## Verdict

**CHANGES-REQUIRED — mandatory CI is red.** No blocking code defect was established in the exact two-file qualification diff. Its mechanism conforms to selector steps 4–5 and the separately accepted source freezes. This is not a green qualification, merge authorization or Planning acceptance. No merge was performed.

## Exact inputs and scope

Independent reviewer: this directly assigned Codex REVIEW session, separate from the implementation workers. No Horde task was created. Session UUID, exact model variant and backend identity are not exposed by the available interface and are not invented. Observed 2026-10-23 approximately 12:19–12:27 UTC; host reports `NTPSynchronized=yes`; authenticated account `mcpdev80` only.

Product and Planning remotes were fetched. Planning HEAD and origin/main both equal `874a2e3bafc54da434a3749cc511b82415a33f8a` (zero divergence). Product index/worktree contain pre-existing staged changes and were preserved; inspection used immutable Git blobs, not these files. Read ecosystem/product/Planning rules, development guidelines, prompt-injection policy, selector recipe and the complete S39b/S42b verdict including correction round 2. Historical resumption prose does not lift the explicit bootstrap pause in this review assignment.

| Identity | Verified SHA |
| --- | --- |
| Q / PR #1043 head | `81cb6365f1d1c5f4c0840253cd55649bb6c0f1d0` |
| Current develop / Q base | `9f4916be333d0f99d7e40001441d9dbb87e993e2` |
| Current Q integration | `d17048422bad65c45366bac8841b4fa8925ce015` |
| S39 / #1039 head | `75efeef89b61d848d5ea351877ba6423a31e58a6` |
| S42 / #1042 head | `0267dbdb71f63a2c77e193618e6fba4106e5cd5b` |

Live PR/base/integration identities were rechecked after inspection and remain unchanged; Q is OPEN and unmerged. Both source heads match the independent ACCEPT verdicts in `codex-verdict-s39b-s42b.md`, correction round 2. This reuses that exact-source review rather than pretending Q independently reimplements or reaccepts those product changes.

## Complete-diff review

Actual `git diff --name-status BASE...Q` contains exactly two modified files: `scripts/pr-gate-plan.py` and `scripts/test_pr_gate_plan.py`; 38 added/6 deleted lines. SHA256 of the complete `git diff --binary BASE...Q` is `1d83d5848b1401e07a06e0f7fbd0825da38a8bb8200dfdf7a05dbda7ff7bfc01`.

- Planner lines 245–256 bind the exact accepted full S39/S42 SHAs. Their maps contain respectively one modified runtime workflow and three modified workflows plus two added helper/test files. Independent real-Git source diff inventory checks match both maps exactly; neither map contains planner files or Q's own SHA.
- Planner lines 760–765 register the two entries with `CI_CHECKS + WORKFLOW_SYNTAX_CHECKS`. Existing EDIT_ZERO_PREVIEW behavior is preserved; its whitespace-only reindent is harmless. All other qualifications, classifier, exact-inventory guard and clean-merge verifier are unchanged.
- Test lines 1581–1592 assert both full immutable anchors and full status/path maps. Lines 1649–1650 extend the existing real-Git qualification cases with exactly the four source validation IDs: `pr-planner-tests`, `ci-policy`, `ci-executor-verifier-tests`, `ci-actionlint`.
- Existing shared qualification tests still exercise missing/extra/status-changed inventory, missing context, wrong anchor, follow-up edits and forged merge-tree rejection for the new cases. No negative assertion was removed. Both changed Python blobs independently compile in memory.
- Trusted BASE still lists both files in `PLANNING_SCRIPTS` at lines 563–564 and classifies them as `ci-planner` at 1121–1123, requiring `CI_CHECKS`. Q therefore qualifies through existing base policy without using its own new entries. No workflow, runner, final-check enforcement or trust boundary changes occur in Q.

Immutable Q blobs: planner `36c1641204d39dc6d4ff9a344fe597912e5facb3`; planner tests `be21c16834f74a0c66ebd43f9323c6dc5210ee37`. No unintended behavioral side effect was found in this bounded diff. This claim does not turn skipped or failed CI into success.

## Authenticated CI and blocker

[Run 39952469490](https://github.com/mcpdev80/baseharbor/actions/runs/39952469490), attempt 1, `pull_request`, exact Q head. All inspected check runs originate from GitHub Actions app **15368**. Downloaded artifact `trusted-pr-plan` (ID `12500364637`) was parsed in memory: its base/head/integration match the table above, `coverage_status=resolved`, `unresolved_coverage=[]`, `selected_gates=[]`; required validations are the three `CI_CHECKS`. The absent separate `ci-actionlint` matrix job is expected for this planner-only predecessor; the unchanged mandatory Focused Python job includes actionlint, but aborts before reaching it in this failed attempt.

| Check | Actual result / job |
| --- | --- |
| Workflow policy | SUCCESS / `120410046925` |
| PR · Trusted Plan | SUCCESS / `120410047221` |
| PR · Validation · pr-planner-tests | SUCCESS / `120410125479`; log confirms 95 tests, OK, including real-Git qualification coverage |
| PR · Validation · ci-policy | SUCCESS / `120410125606` |
| PR · Validation · ci-executor-verifier-tests | SUCCESS / `120410125761` |
| PR · Focused Python | FAILURE / `120410125279` |
| PR Selected Checks | FAILURE / `120410729502` |

The [Focused Python failure](https://github.com/mcpdev80/baseharbor/actions/runs/39952469490/job/120410125279) occurs during direct `python3 scripts/test_pr_gate_plan.py`: 93 tests run, one cleanup ERROR in `TrustedBindingActivationTests.test_malformed_incomplete_or_weakened_records_fail_closed`. The traceback enters `tempfile.TemporaryDirectory.cleanup` and `shutil.rmtree`, ending in `OSError: [Errno 39] Directory not empty: 'pack'`. This is a fixture-cleanup error, not a failed qualification assertion. The successful full planner matrix job is separate evidence; it cannot replace the failed mandatory dependency. Background Git maintenance is a plausible cause, not established root cause. No local sandbox failure was used to demand a production change.

The [final check](https://github.com/mcpdev80/baseharbor/actions/runs/39952469490/job/120410729502) has `FOCUSED_JOB: failure` and remains red. Live effective develop rules require strict up-to-date `PR Selected Checks`, app 15368, alongside the existing formal review/thread/linear-history/squash rules. No bypass or protection change is permitted to resolve this blocker.

## Executable worker/Maintainer recipe and stop criteria

1. Existing assigned CI/qualification owner: preserve Q and both source freezes. Record the cleanup traceback and successful 95-test run. For this diagnosed apparently transient cleanup failure, the smallest next qualification is one failed-jobs rerun of run `39952469490` on the unchanged candidate (for example `gh run rerun 39952469490 --repo mcpdev80/baseharbor --failed`) under the owner's scope. This reviewer did not start it. Reuse the three successful validation jobs only if GitHub's current-attempt authentication accepts them; otherwise rerun only the validations explicitly rejected by that verifier. Do not push a no-op commit or restart native/release matrices.
2. Require the complete Focused Python job, including its remaining regressions/actionlint, and final `PR Selected Checks` to succeed from app 15368 for the exact current inputs. Record attempt/job IDs and actual conclusions. If base/head/integration changes, stop and establish fresh valid current-input qualification; an old-event rerun must not be assumed to pick up a new base.
3. If the cleanup error repeats, stop identical reruns and assign bounded fixture diagnosis to the existing worker. Inspect `scripts/test_pr_gate_plan.py:2422–2456` (temporary real-Git fixture creation/Git helper) and the negative-record test at `:2572–2601`. Reproduce on a neutral CI-equivalent path, determine which process writes `.git/objects/pack` after the test, and synchronize/disable fixture-local automatic background maintenance if confirmed. Preserve every fail-closed assertion and cleanup error reporting; do not ignore rmtree errors or weaken product verification. No product fix is justified solely by the current traceback. Any actual change must receive fresh exact-SHA independent review and mandatory CI.
4. After verified green gates, request bounded re-review of the unchanged Q's updated CI evidence (or full changed diff if Q changed). No selector code correction is currently required. A separately assigned MERGE role must still verify acceptance/integration/thread requirements and applicable owner authority before any expected-HEAD squash. This REVIEW assignment and its CHANGES-REQUIRED verdict authorize no merge. Continue selector step 6 only after Q has actually landed and been verified.

**Stop criterion:** any failed/missing/uncertain required check, source changes after the accepted anchors, inventory mismatch, changed Q, stale integration or conflicting/manual merge blocks qualification. Do not add self-entries, expand maps to conceal changes, alter protections or bypass CI.

## repo-rule compliance and basics flags

Only this requested verdict file was written. No product implementation, test correction, worker dispatch, claim takeover, Planning status edit, GitHub mutation, CI rerun, merge, release or deployment was performed. Existing successful CI was inspected instead of repeating tests; independent local verification was limited to immutable diff/blob comparisons and in-memory compilation. The cost-control/production pause remains in force.

Turn-start basics flags were read and triaged within REVIEW scope. Pool deficits are conditional coordinator scheduling notices; this assignment authorizes no pool fill. Coordinator PLAN age/open steps remain coordinator-owned; its source SHA references are stale compared with the live identities recorded here. Ten repeated idle watcher events concern another lane and require coordinator session inspection, not an invented success acknowledgment. Reminders concern the idle `o47-qpr` lane and pending #1043 review/merge; this file resolves only the review portion, with merging explicitly blocked. Events/reminders remain unacknowledged because their outstanding coordinator actions have not been completed; `events-check.py --ack` was not falsely used to clear them. No other lane was stopped or reassigned. Native TodoWrite could not be used because no such tool is exposed.

END CODEX-Q1043-DONE

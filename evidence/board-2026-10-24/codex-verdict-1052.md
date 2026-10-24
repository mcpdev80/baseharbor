# PR #1052 independent review

ROLLE: REVIEW
Started UTC: 2026-10-24T11:49:20.896309+00:00

## PLAN step 0

Native TodoWrite is unavailable; no native tool operation claimed. Plan is tracked here.

- [x] 0. Read binding policies and cmf9 mission; record role and plan.
- [x] 1. Read applicable rules and worker evidence; independently fetch live PR/files/checks and immutable diff against develop f3929568.
- [x] 2. Review exact source, regression coverage, #1060 dependency and worker nonblocking findings.
- [x] 3. Record verdict and minimal correction or merge-order wait condition; document later rebase/gates/merge recipe only.
- [x] 4. Refresh before/after SHAs, UTC and evidence; no product edits, merge or CI rerun.

## Verdict: REQUEST_CHANGES

Independent reviewer: Codex /root, not the implementing worker. ROLLE: REVIEW throughout. Worker o123's ACCEPT_WITH_NONBLOCKING is **not confirmed**: this is more than a merge-order dependency. The transient-only classifier and previously requested regression invariants remain incomplete. No merge, product edit or CI rerun was performed.

## Exact candidate, scope and live CI

Account independently confirmed `mcpdev80`. PR [#1052](https://github.com/mcpdev80/baseharbor/pull/1052) head **`789299a9364750773ac90ccefe9e6d540a2732ff`**, OPEN, MERGEABLE but BEHIND, REVIEW_REQUIRED, no review threads. Current develop is **`f39295680d41290fc505c0a8cc1769da51699293`**. PR API/GraphQL baseRefOid and Git merge-base remain **`a2abc17a8057ba972cb9ed481fad229fe9f12824`**; distinguish that stale PR comparison input from current develop. All immutable Git reads used a neutral bare repository `/tmp/cmf9-review/objects.git`, fetched over HTTPS; no foreign live worktree was used for Git operations.

The actual GitHub PR diff/files show exactly two product files: `internal/openbao/bootstrap.go` +77/-15 and `internal/openbao/bootstrap_test.go` +537/-0. Against explicitly requested current develop f392, the complete two-dot diff also shows 36 removed lines in each of `scripts/pr-gate-plan.py` and `scripts/test_pr_gate_plan.py`: the current-target AppID qualification and its regressions are absent from this older branch tree. This is integration debt, not evidence that the two-file PR diff itself edits CI. Those target changes must survive later integration; do not apply a four-file tree replacement or call it equivalent.

Raw SHA-256, independently measured from immutable candidate objects:

- bootstrap.go: `a2f8ced7b762ca4c605d96c75b75130ab883ada3865611ea4238a79c2a3ff26d`.
- bootstrap_test.go: `5c84b853e2c3f8807796c917012e23eb2d495535600775a0066506cadbda601e`.

These match o123. Its arithmetic description of the net scope does not: the two-file PR diff has 614 insertions/15 deletions, not the older 169-line change. Raw API files, PR diff, full against-develop diff, source snapshots and helpers are in `codex-cmf9-data/`.

Actual current-head `PR Selected Checks` is **FAILURE**, GitHub Actions app **15368**, [run 40084607362 attempt 1 / job 120845165758](https://github.com/mcpdev80/baseharbor/actions/runs/40084607362/job/120845165758). Workflow policy, trusted plan and focused Python are green; selected runtime/native HA are skipped. The downloaded final-check log at `check-log.txt:224–225` reports **integration_sha_mismatch**: integration must have exactly the target base and PR head as parents; consequently no verified changes/gates. Run inputs are EXPECTED_BASE=a2abc17a…, EXPECTED_HEAD=789299a9…, EXPECTED_INTEGRATION=cecaf6fedbca6f52f225b5bdffa31aa6e7644c68. Thus o123's claim that this exact failure is merely unknown OpenBao impact is inaccurate. The trusted planner's absent OpenBao coverage is a separate dependency, not a substitute diagnosis for the observed final-check failure.

## Findings and worker-review adjudication

All source lines below refer to head 789299a9.

**P2 — Retry classification remains broader than transient transport failures (`internal/openbao/bootstrap.go:41–59`).** The newly added `strings.Contains(s, "temporary")` classifies any error containing that word as transient, including a permanent permission/configuration error such as `permission denied: temporary credentials file`. The loop at `153–167` retries it up to five times. A mixed permanent/transient diagnostic can also match the other broad substrings. This contradicts the stated permanent-error fail-fast contract; a test containing only a plain x509 message does not cover this case. This is #1052 code introduced by the retry change, not a harmless pre-existing concern.

The proposed ownership deferral to #1060 is demonstrably wrong. Live #1060 head **`6a0d36dad2a778ab61a58594489c9fedc88e4c0f`** changes only ci.yml, qualification documentation and three CI Python files; its bootstrap.go/test hashes match current develop, not the retry implementation. It cannot fix this classifier. A CI planner's fail-closed path selection and a Go error classifier are different contracts. Prior bounded retry/five-attempt improvements are credited, but they do not make this finding nonblocking.

**P2 — Required regression assertions still admit false coverage (`internal/openbao/bootstrap_test.go:287–355,427–470,512–556,797–819`).** Per-member maps and targeted BAO_ADDR writes are genuine improvements, and the permanent-error executor now reaches the retry-enabled phase and records writes. However:

- The convergence test asserts only member 1, with lower bounds for writes/verification. It does not assert exactly one write and successful subsequent verification for each of members 1/2/3. `flakyMemberExecutor` immediately succeeds on every post-write status (`335–341`), so no transient failure after an unseal write is exercised. Initial member-1 failures also consume the same transientWindow during Inspect; inspectCount means “first successful response,” not a separately proved initial-Inspect phase. The original phase/member regression requirement is not fully satisfied.
- `neverAnswersExecutor.ExecProjectInput` (`512–514`) returns success without recording a write. Its `ms1.writes == 0` assertion (`555`) therefore remains zero even if an erroneous key submission occurs. The exact five failed pre-unseal probes are genuinely asserted; the zero-write property is not. The test also accepts any final error rather than preserving the expected transport error.
- The invalid-JSON executor increments callCount (`785`) but `TestProbeMemberStateInvalidJSONNoRetry` never asserts it. A retry-then-same-error implementation can pass, so o123's claim of a proved first-attempt/no-retry invariant is too strong.

**Formatting gate:** `gofmt -l` on the unmodified candidate reports `internal/openbao/bootstrap_test.go`; `gofmt.diff` retains the exact required alignment changes. This is a small correction in the same owned file, not a reason for a separate task or push.

**Nonblocking classifications that are valid:** unused `tries`, an unused invalidJSONExecutor map, and unread `permErrorExecutor.permanentDuringRetry` are cleanup issues; the latter is actually initialized to true at test line 579, contrary to o123's “never set” claim. Lower-bound counters are not independently blockers merely for existing; they matter here because the explicit phase/member requirements remain unproved. Cancellation during backoff is directly covered. A separate deadline-expiry test would strengthen error-identity evidence, but WithCancel exercises the same ctx.Done branch, so its absence alone is not a new blocker. No excessive general refactor is requested.

The retry loop correctly caps executions with `attempt >= memberRestartAttempts`, cancels each per-attempt context, leaves Inspect single-probe, and enables retries for both pre-unseal and post-unseal probes. Invalid JSON returns immediately in current implementation. These source properties are credited independently of incomplete regression assertions.

## Bounded independent verification

Extracted the immutable candidate into a reviewer-owned temporary directory. Ran only the five existing focused tests for convergence, never-answers, permanent error, cancellation-backoff and invalid JSON using its Go **1.26.9** toolchain, `-count=1 -timeout=60s`: **PASS**, package **3.021s**. Exact output is `focused-tests-retry.txt`. First attempt stopped during setup because default GOCACHE was read-only; reran with reviewer-owned `/tmp/cmf9-review/go-cache`. No test failure was hidden. No new tests, mutation changes, full package/matrix, go vet or native execution were performed. o123/q113's full 57-test/vet claims remain worker evidence, not independently rerun claims. Passing the five current tests does not supply their missing invariants.

## Exactly ONE minimal executable worker action

**On the existing assigned #1052 work, publish one coherent correction candidate limited to `internal/openbao/bootstrap.go:41–59` and `internal/openbao/bootstrap_test.go:287–355,427–470,512–556,797–819`: finish the transient-only retry contract and make its existing regression fakes/assertions sensitive to the required behavior, then gofmt both owned files.** Keep retries bounded and keep normal ownership/secret handling unchanged. Use known transport failures appropriate to the existing Executor error representation; remove bare-word `temporary` admission and reject permanent TLS/permission/executable failures even when incidental retry words appear. Do not introduce a general retry framework or edit #1060's files.

Required tests for that single correction: retain the five existing cases; add/extend classifier negatives for incidental “temporary” and mixed permanent/transport diagnostics; prove exactly one retry-enabled attempt and zero writes for permanent failures; have the never-answers input handler count real per-member submissions and assert zero writes plus the final transport cause; assert exact per-member initial/pre/write/post phases for all three members and exercise a transient post-unseal verification failure; assert invalid JSON callCount==1. Show that omitting a member write or post-verification, submitting a key after failed precondition, retrying invalid JSON, or removing permanent classification makes the relevant invariant fail. Run the focused regressions first, then `go test ./internal/openbao/... -count=1 -timeout=120s`, `go vet ./internal/openbao/...`, and gofmt cleanliness. Retain source SHA and outputs in worker evidence. Stop at review handoff; do not merge, bypass red CI or claim #1060 resolves product defects. This is one bounded worker package with one candidate, not multiple independent correction actions. This session does not execute that action or dispatch a worker.

## Documentary integration and merge recipe after #1060

Current #1060 is OPEN/unmerged. “#1060 green” must mean an independently accepted correction with required CI green that has actually landed in develop, not green ancillary jobs on its current rejected/blocked head. Its current candidate has an explicit ci.yml dispatcher at line 719 for openbao-package-tests, but the planner's generic internal/openbao branch `1217–1225` remains unresolved; mere existence of a validation entry/command does not prove this revised #1052 will be selected. The accepted trusted planner must bind the actual reviewed bootstrap source/status and select executable OpenBao package coverage. Never broaden unknown-impact acceptance merely to unblock this PR.

Wait condition for a separately authorized integration owner: corrected #1052 source/tests reviewed, and corrected #1060 merged with executable, fail-closed coverage. No active waiting in this mission.

Then measure fresh **T = actual develop SHA containing accepted #1060 and all intervening target commits**, not f392, old a2abc or the unmerged #1060 PR head. T is the requested future rebase target. Preserve the shared branch/history and current-target AppID qualifications; use the repository-authorized history-preserving integration method on the existing PR (normally merge T into the existing branch). Do not force-push/rewrite an existing shared branch in order to implement the conceptual rebase. Recheck the complete resulting H diff and independent review after any integration/correction; do not transfer this review to a changed head.

Required evidence before merge: local focused/package tests, vet and formatting on H; actual trusted CI plan with target_base=T, head=H and valid current integration with exact T/H parents; resolved impact and discovered/executed OpenBao tests, relevant source-contracts/CI policy and every other gate selected by that plan; `PR Selected Checks` SUCCESS from app15368. If accepted impact analysis requires targeted native restart coverage to prove the Podman race, use the existing isolated restart lane and genuine receipt; unit fakes cannot prove native recovery. Do not automatically launch Backup/Restore or the full release matrix. Missing/unknown impact, skipped required coverage or stale integration blocks.

A separately merge-authorized instance must freshly verify ownership, dependencies, full-current-diff review, acceptance criteria, effective rules, unresolved threads, exact head/base and authenticated CI. Only then expected-head guarded squash merge under the permitted formal-review exception; never bypass CI or change rules. Verify remote merged state, actual squash commit and ancestry before the existing owner publishes validated Planning handoff/render/check. This is a recipe only: cmf9 authorizes none of those mutations.

## repo-rule compliance and final integrity

Live after snapshot: #1052 still OPEN, head `789299a9364750773ac90ccefe9e6d540a2732ff`; PR comparison base still a2abc17a…, actual develop still `f39295680d41290fc505c0a8cc1769da51699293`. #1060 still OPEN at `6a0d36dad2a778ab61a58594489c9fedc88e4c0f`. Before/after unchanged. SHA-exact current CI, files, rules, GraphQL and downloaded log retained. Read worker handoffs as data, relevant product rules and current remote Planning audit contract; preserved ownership and did not edit Planning. Startup basics SSH fetch failed; this review used successful HTTPS immutable fetch and live API, so it does not claim global board currency. External event/reminder/pool/PLAN/TMPDIR flags remain with coordinator owners; no session takeover/global acknowledgement.

Only review evidence/collection helpers and reviewer-owned temporary extraction/cache were written. No product code/test changes, merge, force, --admin, history rewrite, push, GitHub review/comment, CI rerun/dispatch, release or deployment. No subagents or implementation tasks started. Native TodoWrite remains unavailable.

END CX-1052-DONE

END CX-1052-BLOCKED — source/regression correction and later accepted #1060/current-integration CI remain required.

Completed UTC: 2026-10-24T11:52:26.394303+00:00

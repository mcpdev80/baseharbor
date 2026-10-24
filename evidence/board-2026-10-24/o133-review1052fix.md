# O133 — PR #1052 candidate `7c7cda7c` read-only review

ROLLE: REVIEW (independent, not the implementing worker, not Codex-Schlussabnahme).
Dispatch: `ops/missions/o133.txt`. Read-only; git confined to `/tmp`; no product edit,
merge, review command, CI rerun, push, or Planning change.

Started UTC: 2026-10-24 (turn run).

## PLAN step 0

Native TodoWrite active. Evidence: this file.

- [x] 0. `pwd -P` + read-only; git confined to `/tmp`; no merge/review-command/push/CI rerun/product edit.
- [x] 1. Independently fetch live PR #1052, immutable objects, current develop, prior head 789299a9.
- [x] 2. Verify exact recipe closure: classifier, test additions/negatives, all test names, secret/ownership unchanged.
- [x] 3. Independently run smallest focused tests + full package + vet + gofmt; mutation-test the negative invariant.
- [x] 4. Classify defect vs CI blocker #1060; confirm PR Selected Checks failure is not a source defect.
- [x] 5. Verdict + future integration recipe after #1060.
- [x] 6. Record live SHAs/CI/PR state; no product edits, merge, force, push, or CI rerun.

## Verdict: ACCEPT

Independent reviewer: marcel / Codeman-host review lane, read-only, not the implementing worker
(o131) nor Codex. The candidate `7c7cda7c` closes the single bounded correction requested by
codex-verdict-1052.md section "Exactly ONE minimal executable worker action": the transient-only
classifier fix plus strengthening of the five existing regression cases with the requested
negatives. Independently verified gofmt clean, `go vet` clean, focused tests pass, full package
`ok`; the negative classifier invariant is mutation-proven (a real, derivable invariant). The
remaining `PR Selected Checks` FAILURE is an integration-state blocker (#1060 unlanded), not a
source defect, and does not transfer to source ACCEPT. This is the corrected head awaiting the
ordinary #1060 integration recipe below; no merge, force, or CI rerun was performed.

## Candidate, base and prior

- PR #1052 (repo `mcpdev80/baseharbor`) head **`7c7cda7cc32423405d3cabe3ecf46d61289b0598`**,
  branch `work/v0.4.26-openbao-retry`, base `develop`. Live state: OPEN, MERGEABLE,
  mergeStateStatus BEHIND, reviewDecision REVIEW_REQUIRED.
- Candidate tree file hashes (independently measured from immutable objects):
  - `internal/openbao/bootstrap.go`: `467eed5c53480de6eba261387adcaaf57cb41710`
  - `internal/openbao/bootstrap_test.go`: `063fe7c092b06c67dea80e489555b9bd5ccfb18c`
- Prior PR head `789299a9364750773ac90ccefe9e6d540a2732ff` (the accepted-by-codex-but-uncorrected
  candidate).
- Current develop (base) `65aa063e90767c85af976081c100be1817370e67` (`docs(ci): record-only Core-921 CI binding qualification for #1056 (Q) (#1063)`).
- #1060 head `work/v0.4.26-openbao-gateplan`, state OPEN, mergeable CONFLICTING (unlanded, unmerged).

## Step 1 — scope: exactly two files changed by the fix

The #1052 fix diff `789299a9 → 7c7cda7c` changed **exactly two files**:

```
M  internal/openbao/bootstrap.go
M  internal/openbao/bootstrap_test.go
2 files changed, 298 insertions, 47 deletions
```

The full `65aa063e (current develop) → 7c7cda7c` diff additionally shows changes in
`docs/internal/ci-binding-qualifications.md`, `scripts/pr-gate-plan.py` (-36) and
`scripts/test_pr_gate_plan.py` (-36). These are **pre-existing develop drift**, already present
in the prior PR head `789299a9` (confirmed: `git diff 65aa063e 789299a9` already lists all five
files, and `git diff --name-only 65aa063e 789299a9` includes the two scripts + docs + the two
bao files). They are integration debt from #1060 / PR-1056 targeting-change drift, **not** part of
the #1052 fix. The classifier/test diff `789299a9 → 7c7cda7c` touches neither script nor docs.
The four-file tree is not equivalent to the two-file PR diff.

## Step 2 — recipe closure (bootstrap.go:41–59 classifier)

`isTransientError` at `internal/openbao/bootstrap.go:58–66`:

```go
func isTransientError(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "connection refused") ||
		strings.Contains(s, "connection reset") ||
		strings.Contains(s, "network unreachable") ||
		strings.Contains(s, "no such host") ||
		strings.Contains(s, "i/o timeout") ||
		strings.Contains(s, "operation timed out") ||
		strings.Contains(s, io.ErrUnexpectedEOF.Error()) ||
		strings.Contains(s, "reset by peer")
}
```

Closure verified against the recipe:
- Bare-word `strings.Contains(s, "temporary")` **removed** (was the sole admission line after
  `operation timed out`). The only remaining `temporary` occurrences in the file are in the
  explanatory comment added at `bootstrap.go:48–52` documenting that incidental "temporary" words
  must NOT trigger a retry.
- Only positively-identified listener/transport shapes are admitted. Permanent TLS/CA,
  permission (`permission denied: temporary credentials file`), and missing-executable failures
  never match → fail fast.
- Retry loop (`bootstrap.go:157–162`) unchanged: `!isTransientError` fail-fast gate,
  `memberRestartAttempts=5` cap, `ctx.Done()` backoff preserved.
- Ownership/secret handling elsewhere unchanged (verified: diff is confined to the classifier and
  its doc comment; unseal-key, recovery-file and ownership logic untouched).

## Step 2 — recipe closure (test additions / negatives)

All 17 test functions present in `bootstrap_test.go`:

```
TestBootstrapKeepsRootAndUnsealSecretsOutOfCommandArguments
TestBootstrapRejectsRecoveryFileInsideBaseHarborState
TestBootstrapRejectsRecoveryDirectorySymlinkedIntoBaseHarborState
TestLoadAdminCredentialsRejectsBroadPermissions
TestInspectPrefersUnsealedHAMemberOverReachableSealedMember
TestMemberStateAllowsBaoSealedExitHandlingWithoutShellErrexit
TestRetryManagerProvisioningConvergesAfterTransientFailures
TestUnsealConvergesAfterMemberRestartRace            (strengthened: per-member exact phases)
TestUnsealFailsFastWhenMemberNeverAnswers           (strengthened: execAttempts==5, final cause)
TestUnsealFailsImmediatelyOnPermanentNonTransientError (strengthened: preUnsealAttempts==1, execAttempts==1)
TestManagerPolicyAllowsBothOpenBaoPolicyEndpoints
TestManagerPolicyPermitsOnlyOwnCapabilityInspection
TestProbeMemberStateCancellationDuringRetryBackoff
TestProbeMemberStateInvalidJSONNoRetry              (strengthened: execAttempts==1)
TestIsTransientErrorRejectsIncidentalTemporary      (NEW negative)
TestIsTransientErrorRejectsMixedPermanentTransport  (NEW negative)
TestUnsealFailsOnTransientPostUnsealVerification    (NEW post-unseal permanent-failure path)
```

Per-member counters added to `haMemberState`: `postUnsealAttempts`, `execAttempts`,
`isRealMember`, `postVerified`. The `flakyMemberExecutor` now records `isRealMember` and gates
post-unseal verification to real HA members only. All requested invariants present:
- permanent failure: exactly one retry-enabled attempt, zero writes (`TestUnsealFailsImmediatelyOnPermanentNonTransientError`).
- never-answers: counts real per-member submissions, zero writes, surfaces final transport cause.
- exact per-member initial/pre/write/post phases for all 3 members (`TestUnsealConvergesAfterMemberRestartRace`).
- transient post-unseal verification failure path exercised (`TestUnsealFailsOnTransientPostUnsealVerification`).
- invalid JSON `execAttempts==1` (`TestProbeMemberStateInvalidJSONNoRetry`).
- negative classifier cases for incidental "temporary" + mixed permanent/transport.

## Step 3 — independent verification (reviewer worktree, isolated GOCACHE)

Reviewer worktree (new worktree in `/tmp`): `git clone` → fetch `refs/pull/1052/head`
`7c7cda7c` → new worktree `bh-1052-checker` at `7c7cda7c` (working tree clean). Go **1.26.9**.
Isolated `GOCACHE=/tmp/bh-1052-checker/.gocache`. No full native/VM, no package-matrix beyond the
owned package, no mutation of the product (mutation applied to a local copy only, then restored;
working tree returned to `7c7cda7c`).

- `gofmt -l bootstrap.go bootstrap_test.go` → **clean** (no files listed).
- `go vet ./internal/openbao/...` → **clean** (no output, exit 0).
- Focused: `-run 'TestIsTransientErrorRejectsIncidentalTemporary|TestIsTransientErrorRejectsMixedPermanentTransport|TestUnsealFailsImmediatelyOnPermanentNonTransientError|TestUnsealFailsFastWhenMemberNeverAnswers|TestProbeMemberStateInvalidJSONNoRetry|TestUnsealFailsOnTransientPostUnsealVerification'` → **PASS** (`ok 2.013s`).
- Full package: `go test ./internal/openbao/... -count=1 -timeout=120s` → **`ok 4.721s`**, all cases pass.
- **Mutation check (proves the negative is a real invariant):** reintroduced the bare-word
  `temporary` admission into a local copy → `TestIsTransientErrorRejectsIncidentalTemporary`
  **FAILS** (`permanent error "x509: ... temporary failure in name resolution" must not be
  classified transient`), then restored the file to `7c7cda7c`. Omitting the permanent
  classification is caught; the invariant is derivable, not vacuous.

## Step 4 — classification

- **Implementation: no defect.** The single bounded correction from the codex recipe is closed;
  classifier, regression strengthening, and negatives all present and independently verified.
- **`PR Selected Checks` FAILURE is a CI blocker due to #1060, NOT a source defect.** Live
  `gh pr checks 1052`: `PR Selected Checks` = **fail** (run 40092965161, job 120869502729);
  Workflow policy, Trusted Plan, Focused Python, Validation·not-required = pass; Selected Runtime,
  Targeted HA·docker/podman, validate, Consumer Pins, Demo Pin, Exact Candidate, Public Contracts,
  Runner Interface, Release Source Preflight = skipping. The failure is the documented
  `coverage-unresolved` / integration-blocker (#1060's `openbao-package-tests` fail-closed gate is
  not landed in develop; #1060 head is CONFLICTING/OPEN). Per mission item 4 this must **not** be
  transferred to source ACCEPT. No source defect exists; the pass/fail of local tests is unrelated.
- No scope creep (CI/scripts/docs untouched by the fix), no secret/ownership change, no force.

## Step 5 — verdict

**ACCEPT** (source). The candidate closes the one bounded worker correction, is gofmt/vet clean,
focused + full-package tests pass, and the negative classifier invariant is mutation-proven. The
outstanding `PR Selected Checks` gate is a pure integration-state dependency on #1060 and does not
affect source correctness. This review performed no merge, force, push, CI rerun/dispatch, or
product edit.

## Step 6 — future integration recipe after #1060 (recipe only; no mutation performed)

Wait condition for a separately authorized integration owner: corrected #1052 accepted **and**
accepted #1060 merged into develop with executable, fail-closed openbao coverage. Then:

- Measure fresh **T = actual develop SHA containing accepted #1060 and all intervening target
  commits** (not `65aa063e`, not the stale PR comparison base, not the unmerged #1060 head).
- Rebase/reintegrate the existing PR by the repository-authorized, history-preserving method:
  normally **merge T into the existing `work/v0.4.26-openbao-retry` branch** — do **not**
  force-push/rewrite the shared branch.
- Re-review the full resulting HEAD diff (SHA-exact) before accept; do not transfer this review to
  a changed head.
- Required evidence before merge: local focused/package tests, vet and gofmt on H; actual trusted
  CI plan with `target_base=T`, `head=H` and valid current integration with exact T/H parents;
  resolved impact + discovered/executed openbao tests; `PR Selected Checks` SUCCESS from app 15368.
  If native restart coverage is needed to prove the Podman race, use the existing isolated restart
  lane with a genuine receipt; unit fakes cannot prove native recovery. Missing/unknown impact,
  skipped required coverage or stale integration blocks.
- Only a merge-authorized instance then freshly verifies ownership, dependencies, full-current-diff
  review, acceptance criteria, effective rules, unresolved threads, exact head/base and authenticated
  CI; only then expected-head guarded squash merge under the permitted formal-review exception.
  Verify remote merged state and actual squash commit/ancestry before publishing Planning handoff.

## repo-rule compliance and final integrity

ROLE: READ-ONLY REVIEW. Git confined to `/tmp` worktrees (`/tmp/bh-1052-reviewer`, `/tmp/bh-1052-checker`);
no git operation anywhere else. No product code/test edit, no merge, force, --admin, history
rewrite, push, GitHub review/comment, CI rerun/dispatch, release, deployment, or Planning change.
No subagents or implementation tasks started. Secrets (root/unseal/manager) kept out of command
arguments and this evidence. Live after snapshot: #1052 OPEN, head `7c7cda7c`, develop
`65aa063e`, #1060 OPEN/CONFLICTING; CI `PR Selected Checks` still fail on #1060 coverage (read-only,
unchanged). SHA-exact CI, files, GraphQL retained.

END O133-REVIEW1052-DONE
END O133-REVIEW1052-ACCEPT

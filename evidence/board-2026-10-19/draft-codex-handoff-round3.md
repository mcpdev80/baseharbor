# Draft Codex Round-3 Handoff — reviewer session fe5dff6c
## PR #998 a66e5de6 / PR #1007 48def9e8

**Handoff SHA (PR #998):** `a66e5de664b22c295a8b5ae918a9c2dfe4c6e327` (worktree core-921-central-cli-mcp-registration, detached HEAD)
**Handoff SHA (PR #1007):** `48def9e85e35db90025b4e354350bac22c9fe806` (worktree ci-core921-coverage-binding, detached HEAD)
**Remote ref (#998):** `refs/heads/work/core-921-central-cli-mcp-registration` → `a66e5de6` — matches PR #998 head
**Remote ref (#1007):** `refs/heads/work/ci-core921-coverage-binding` → `48def9e8` — matches PR #1007 head

---

## (a) Both candidates with exact SHAs

| Candidate | PR | Branch | Head SHA | Remote Ref SHA | PR Open |
|-----------|----|--------|----------|---------------|---------|
| #998 binding | mcpdev80/baseharbor#998 | `work/core-921-central-cli-mcp-registration` | `a66e5de664b22c295a8b5ae918a9c2dfe4c6e327` | `a66e5de6` (match) | merge `f6d50082..` |
| #1007 binding | mcpdev80/baseharbor#1007 | `work/ci-core921-coverage-binding` | `48def9e85e35db90025b4e354350bac22c9fe806` | `48def9e8` (match) | merge `9e014c21..` |

Both remote refs equal their respective PR heads exactly.

---

## (b) What changed since round 2 per candidate

### PR #998 at `a66e5de6` (parent `6931ba9c`)

**Delta:** One new file — `cmd/baha/app_mcp_provider_edit_native_test.go` (371 lines)

- **Native MCP propose→apply lifecycle proof.** The test implements the exact four Codex round-2 blocking requirements against the real MCP `baseharbor.apply` entry point:
  1. Valid propose survives serialization → staged consent → complete verified result (`TestAppEditMCPApplyRunsRealProviderEditLifecycle`).
  2. Stale-wrong-proposal → structured `ErrorConflict` + unchanged binding.
  3. Missing second-stage consent → structured `ErrorApprovalRequired` + unchanged binding.
  4. Negative cases run against the valid fixture, final `runtimeInventoryPresent` guard confirms fixture untouched.
- **Opt-in env guard at line 47-49:** `t.Skip` when `BASEHARBOR_MCP_EDIT_NATIVE_ACCEPTANCE != "true"`. Skip-not-fake; CI never sets the gate, so the test never runs or fails on unconfigured runners. Matches the sibling MCP-native test (`app_mcp_lifecycle_acceptance_test.go`) pattern.
- **Skip semantics verified:** Local `go test` shows `--- SKIP: TestAppEditMCPApplyRunsRealProviderEditLifecycle (0.00s)` with reason "native MCP provider-edit lifecycle is opt-in" (q3-998-candidate-verify.md:185).
- **Native execution proven:** o10 evidence (`o10-native-mcp-proof.md`) — `TestAppEditMCPApplyRunsRealProviderEditLifecycle` passed on `baha-docker` (~202s, exit 0, go vet clean). Re-verified independently at SHA `a66e5de6` via Q1 (ssh `baha-docker`, EXIT_CODE=0, 234.22s).
- **gofmt/vet clean:** `gofmt -l` on the new file → no output; `go vet ./cmd/baha/` → rc=0.

### PR #1007 at `48def9e8` (parent `706b5244`)

**Delta:** Two files modified — `scripts/pr-gate-plan.py`, `scripts/test_pr_gate_plan.py`

- **BINDING_PR_1007 self-qualification clause.** Added `BINDING_PR_1007_REVIEWED_HEAD = 706b524489f8410091902e595df50427f80a4eab` and `BINDING_PR_1007_REVIEWED_FILES` dict mapping 3 CI-execution scripts with `'modified'` status, integrated into the shared reviewed-bundle loop at pr-gate-plan.py:543-544. Mirrors the #999/#1005 mechanism structurally.
- **CORE_921 clause byte-identical:** Zero modification lines for the 16-path CORE_921 constant block. Only 3 deletion lines = whitespace reformatting of the GC_999 entry.
- **Test additions in test_pr_gate_plan.py:** Both `('BINDING_PR_1007_REVIEWED_HEAD', planner.BINDING_PR_1007_REVIEWED_FILES, {c['id'] for c in planner.CI_CHECKS})` (emitted-checks subTest) and ancestry test cases mirror the established pattern. 84 total tests pass (64 base + 8 PodmanTypedConsent + 7 ArtifactGC + 5 RuntimeReset + 18 pr-check-runner).
- **Planner-mode analysis (r4) reveals a structural blocker:** The `BINDING_PR_1007_REVIEWED_HEAD` clause can NEVER fire for a single-commit PR because `core752_reviewed_merge_chain(706b5244, 48def9e8, 706b5244)` returns False — `head != base` and the parent list has length 1, not 3. The binding clause is dead for the PR it was invented to qualify. Pre-merge green for #1007 is defeated by a stale-webhook `base.sha` defect (CI owner fix) compounded by an unreachable self-qualification clause.

---

## (c) All independent review verdicts with reviewer lane names

| Verdict | Reviewer Lane | Candidate | SHA | Verdict | Evidence |
|---------|--------------|-----------|-----|---------|----------|
| Codex 6.1 round 1 | session fe5dff6c | #998 at `08d8f498` | CHANGES_REQUESTED | codex-verdict-core921-round1.md |
| Codex 6.1 round 2 | session fe5dff6c | #998 at `6931ba9c` | CHANGES_REQUESTED | codex-verdict-core921-round2.md |
| o8 — typed-error delta review | Ornith o8 (independent reviewer) | #998 at `6931ba9c` | APPROVE | o8-998-delta.md |
| R1-998 pre-review | Flash (local pre-reviewer) | #998 at `6931ba9c` | CHANGES_REQUESTED | r1-998-prereview.md |
| R3 native delta review | Flash (read-only delta review) | #998 at `a66e5de6` | APPROVE | r3-998-native-delta-review.md |
| Q1 native re-verification | Flash (independent evidence collection) | #998 at `a66e5de6` | REVERIFIED | q1-native-reverify.md |
| Q3 candidate verification | Flash (read-only candidate verification) | #998 at `a66e5de6` | VERIFIED | q3-998-candidate-verify.md |
| o17 self-qualification | o17 (independent read-only review) | #1007 at `48def9e8` | APPROVE | o17-selfq-review.md |
| R2 CI/Proof bundle | Flash (read-only check) | #998 / #1007 | BLOCKED (CI failure) | r2-ci-bundle.md |
| R4 planner-mode analysis | Flash (read-only planner investigation) | #1007 | BLOCKED (structural) | r4-planner-mode-analysis.md |

---

## (d) CI situation: coverage-unresolved root causes + CI-owner item (q2)

### Both PRs report `coverage-unresolved` as the single blocker for `PR Selected Checks`

#### PR #998 failure: `integration_sha_mismatch`

Run 39256293000: `BLOCKED: coverage-unresolved: diff-base requires unresolved coverage: [{"code": "integration_sha_mismatch", ...}]`

Root cause: develop tip advanced from `19548bf0` (PR-open) to `bb91fbc3` (merge-time) before CI executed. Event payload carried stale `base.sha = 19548bf0`, but the synthetic merge ref `f6d50082` had parents `[bb91fbc3, a66e5de6]` — 3 parents, 1st parent != stale base → `parents != [integration_sha, target_sha, head_sha]` → `integration_sha_mismatch` at `pr-diff-base.py:142`.

**CI-owner fix required (q2-ciwebhook-proposal.md):** Derive `target_base_sha` from the integration commit's 1st parent rather than trusting the stale event payload. The fix is self-contained in `scripts/pr-diff-base.py` at lines 132-145 — when `parents[1] != target_sha` but parents has exactly 3 elements, use `parents[1]` as the real target base. No `.github/workflows/ci.yml` structural change required.

#### PR #1007 failure: `coverage-unresolved` (qualification gate, different root)

Run 39297909779: `BLOCKED: coverage-unresolved: CI execution/evidence impact requires specific qualification: scripts/pr-check-runner.py`

Root cause: The `BINDING_PR_1007_REVIEWED_HEAD` clause can NEVER fire for a single-commit PR because `core752_reviewed_merge_chain(706b5244, 48def9e8, 706b5244)` returns False — `head != base` and the parent list has length 1, not 3. The 3-file set falls through to `ci-executor-unresolved` at `pr-gate-plan.py:869-874`.

**WE must change:** Either relax `pr-gate-plan.py:433`'s `len(parents) != 3` to accept the base-anchored single-parent case for binding PRs, OR add a dedicated self-qualifying branch that binds to `head_sha` directly. The clause currently binds to the base (`706b5244`), which can never equal the head (`48def9e8`).

---

## (e) Explicit list: what Codex must verify vs. what is already locally proven

### PR #998 — ALREADY LOCALLY PROVEN

- Typed error classification: every `appMachineError` origin maps to typed `*machine.Error` with `Code`/`CauseCode`/`Next` — VERIFIED by o8 and R1.
- MCP boundary: no `internal_error` fallthrough on a recognized origin — VERIFIED.
- New tests prove a real MCP propose reaching the handler with byte/snapshot zero-write equality and assert structured codes on apply-side negatives — VERIFIED by R1, Q3.
- Consent/ownership/ordering guards are byte unchanged — VERIFIED by o8.
- gofmt -l clean, go vet clean, focused go test suite exit 0 — VERIFIED by R3, Q3.
- Native MCP propose→apply lifecycle: valid apply with staged consent reaches a complete verified result — PROVEN by o10 (~202s on baha-docker, exit 0) and Q1 re-verify (234.22s on baha-docker, exit 0).
- Skip semantics: opt-in env gate, skip-not-fake, CI never sets the gate — VERIFIED by R3, Q3.
- Container cleanup: zero new containers created during native re-verify — VERIFIED by Q1.
- PR branch remote ref: `work/core-921-central-cli-mcp-registration` → `a66e5de6` matches PR #998 head — VERIFIED by Q3.

### PR #998 — WHAT CODEX MUST VERIFY

- **Merge readiness of #998:** The PR is MERGEABLE but BEHIND develop. Rebase/sync onto develop tip. Codex must verify the rebase preserves all SHA-256 file hashes and the resulting merge commit passes `PR Selected Checks`.
- **CI green after rebase:** Re-run `PR Selected Checks` post-rebase to confirm `coverage-unresolved` drops. The `integration_sha_mismatch` blocker is owned by CI (q2 fix), but Codex must verify whether the fix is already deployed.
- **Acceptance and merge of #998:** Requires ACCEPTED verdict + actual green mandatory CI. Codex must verify both conditions before merge.

### PR #1007 — WHAT CODEX MUST VERIFY

- **Binding clause reachability:** The `BINDING_PR_1007_REVIEWED_HEAD` clause is structurally unreachable for a single-commit PR. Codex must verify the fix in `pr-gate-plan.py:433` (relaxing `len(parents) != 3` to accept the base-anchored single-parent case) resolves the `ci-executor-unresolved` blocker.
- **Regression test:** After the fix, `scripts/test_pr_gate_plan.py` must add a regression asserting the single-commit binding PR path.
- **CI green post-fix:** The 3-file set must resolve to `coverage_status: resolved` (binding clause) rather than the `ci-executor-unresolved` blocker.

---

## (f) The two open worker lanes

### Lane 1: PR #998 merge readiness

**Status:** HEAD `a66e5de6` is proven correct by all reviews and evidence. PR is MERGEABLE but BEHIND develop (base at merge = `bb91fbc3` vs PR head = `a66e5de6`).

**Action required:** Rebase/sync `a66e5de6` onto current `develop` tip. Verify SHA-256 file hashes are preserved. Confirm `PR Selected Checks` goes green (the `integration_sha_mismatch` blocker may or may not persist depending on whether the q2 CI fix is deployed).

**Owner:** Any worker; the task is deterministic (rebase + verify CI green).

### Lane 2: PR #1007 binding clause reachability fix

**Status:** HEAD `48def9e8` is structurally correct but the `BINDING_PR_1007_REVIEWED_HEAD` clause can NEVER fire for a single-commit PR. The `core752_reviewed_merge_chain` guard requires `head == base` or 3+ parents — neither is possible for a linear single-commit push.

**Action required:** Either (a) relax `pr-gate-plan.py:433` to accept the base-anchored single-parent case for binding PRs, OR (b) add a dedicated self-qualifying branch that binds to `head_sha` directly. After the fix, the 3-file set must resolve to `coverage_status: resolved` and `scripts/test_pr_gate_plan.py` must add a regression asserting the single-commit binding PR path.

**Owner:** Requires understanding of `pr-gate-plan.py`'s clause resolution — worker must be familiar with the planner's merge-chain guard.

---

Q5-HANDOFF-DRAFT-DONE

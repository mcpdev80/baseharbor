# Draft Codex Round-3 Handoff — reviewer session fe5dff6c
## PR #998 a66e5de6 / PR #1007 48def9e8

**Handoff SHA (PR #998):** `a66e5de664b22c295a8b5ae918a9c2dfe4c6e327` (worktree core-921-central-cli-mcp-registration, detached HEAD)
**Handoff SHA (PR #1007):** `48def9e85e35db90025b4e354350bac22c9fe806` (worktree ci-core921-coverage-binding, detached HEAD)
**Remote ref (#998):** `refs/heads/work/core-921-central-cli-mcp-registration` → `a66e5de6` — matches PR #998 head
**Remote ref (#1007):** `refs/heads/work/ci-core921-coverage-binding` → final re-pinned merge head (lane o15, pending push at draft time) — matches PR #1007 head after finalize

**Required merge order:** #1007 (binding) merges to develop FIRST; only then #998 (its `PR Selected Checks` can pass solely because the binding clause lands in develop; the reverse order leaves #998 `coverage-unresolved` by design). #998 must not receive further commits after the binding's final CORE_921 re-pin; if it must advance (e.g. develop-sync), the binding constant requires a matching re-pin round.

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

Root cause: develop tip advanced from `19548bf0` (PR-open) to `bb91fbc3` (merge-time) before CI executed. Event payload carried stale `base.sha = 19548bf0`, while the synthetic merge ref carried parents `[bb91fbc3, a66e5de6]` — the validator's expected 3-tuple `[integration_sha, target_sha, head_sha]` compared against the stale `target_sha` did not match → `integration_sha_mismatch` at `pr-diff-base.py:142`.

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

**Status:** FIX IMPLEMENTED (local commit `1f38423a`), finalizing now in lane o15.

- **Design basis (reviewed):** `48def9e8` + one develop merge — the reviewed head for this lane. `48def9e8` (parent `706b5244`) is the single-commit binding-PR head whose `BINDING_PR_1007_REVIEWED_HEAD` clause is structurally unreachable, per R4 §4 (r4-planner-mode-analysis.md).
- **Root cause (R4 §4):** Two compounding bugs — (1) stale-webhook `base.sha` defect owned by CI owner (`pr-diff-base.py:142`, derive `target_base_sha` from the integration commit's 1st parent); (2) unreachable binding clause (WE must change) — `pr-gate-plan.py:546-552` requires `core752_reviewed_merge_chain(...)` which returns False for a linear single-commit head (`head != base`, parents length 1, `pr-gate-plan.py:433`).
- **Local fix:** commit `1f38423a` relaxes the `core752_reviewed_merge_chain` guard to accept a linear single-commit binding PR whose 1st parent == `target_base` (scoped to the `BINDING_PR_1007` reviewed-head set, not a path-wide exemption), so the 3-file set resolves to `coverage_status: resolved`. Regression test for the single-commit binding-PR path pending integration.
- **Pending in lane o15:** finalize the fix into the reviewed head, run `scripts/test_pr_gate_plan.py` (assert the single-commit binding path + all 84 existing tests), SHA-256-fix the candidate, and publish. Then re-run CI `PR Selected Checks` to confirm `ci-executor-unresolved` clears.
- **r5 review:** independent read-only r5 (o17) on the finalized fix is PENDING — the o17 self-qualification review (`o17-selfq-review.md`) was recorded against `48def9e8`; the r5 verdict on `1f38423a`'s delta is the open item to collect before Codex re-review.

**Action required (for Codex re-review, after o15 finalize + r5):** Verify (a) the relaxed merge-chain guard resolves the 3-file set to `coverage_status: resolved`, (b) `scripts/test_pr_gate_plan.py` passes including the new single-commit regression, (c) CI `PR Selected Checks` goes green post-fix. R4 recommends the CI-owner webhook fix (q2) is required separately and is owned by Manager / GPT-6.1 — WE must not edit shared CI.

**Owner:** o15 (finalize) → independent r5 → Codex re-review.

---

Q5-HANDOFF-DRAFT-DONE

---

## Evidence index — every file in evidence/board-2026-10-19/*.md

Every file listed exists in the directory. SHA-256 fixings of candidates live alongside (e.g. `*-baseline.sha` / `*.sha` files) or inside the evidence file itself; a plain `sha256sum` of the referenced paths reproduces any listed hash.

| File | One-line content | Candidate covered |
|------|------------------|-------------------|
| `ci-finding-integration-sha-mismatch.md` | Flash's CI finding: `integration_sha_mismatch` on #998 from a stale webhook `base.sha` (CI-owner item). | #998 |
| `codex-verdict-core921-round1.md` | Codex 6.1 round-1 CHANGES_REQUESTED verdicts on #998 (`08d8f498`) and #1007 (`417b8cbb`). | #998, #1007 |
| `codex-verdict-core921-round2.md` | Codex 6.1 round-2 CHANGES_REQUESTED — #998 at `6931ba9c`, #1007 at `f920fa72` (MCP apply still unproven; cli-gofmt/negative gaps). | #998, #1007 |
| `draft-codex-handoff-round3.md` | Round-3 handoff draft (reviewer session fe5dff6c) — pre-image of this v2 file. | #998, #1007 |
| `o10-native-mcp-proof.md` | o10 native MCP propose→apply proof on `baha-docker` (~202s, exit 0) — closes the last #998 blocker. | #998 |
| `o13-1007-review.md` | O13 independent read-only review of #1007 binding delta (head `706b5244`). | #1007 |
| `o13-doc922-prep.md` | Read-only core-922 docs prep analysis; notes core-921/PR #998 head `a66e5de6` as the interface to document. | #998 (interface for core-922) |
| `o14-923-prep.md` | Read-only core-923 acceptance/edit-journey prep; gated on core-921 accepted + core-922 done. | core-921/#998 (dependency) |
| `o17-selfq-review.md` | o17 independent read-only self-qualification review of #1007 at `48def9e8` — APPROVE (r5-style). | #1007 |
| `o1-binding-final.md` | o1 binding finalization: task t-20261019-core921-coverage-binding, head `417b8cbb`, PR #1007. | #1007 |
| `o1-binding-pause-note.md` | o1 coordinator controlled-pause note (both files left uncommitted). | #1007 |
| `o1-binding-rebase.md` | o1 rebase of the coverage-binding branch onto develop. | #1007 |
| `o1-core921-review.md` | o1 read-only local review of core-921/#998 (`756b22dc`), findings list. | #998 |
| `o2-989-review.md` | Independent current-source verification of core-989/#989 (`78a72eba`). | #989 |
| `o2-995-fix.md` | o2 fix for podman-recovery-bootstrap-cleanup on #995 (`d359af83`). | #995 |
| `o3-985-integration.md` | o3 handoff: integrating core-754 onto PR #985 (`e1c797bb`). | #985 |
| `o4-993-review.md` | O4 independent APPROVE review of #993 (`b981f3b6`). | #993 |
| `o4-995-rereview.md` | O4 partial re-review of #995 (`d359af83`), gated by coordinator halt. | #995 |
| `o4-998-rereview.md` | O4 delta re-review of #998 final candidate `08d8f498`. | #998 |
| `o5-998-correction.md` | o5 correction of #998 (MCP schema + typed-error boundary), head `24119eb9`. | #998 |
| `o6-codeman-delegation-investigation.md` | o6 read-only finding: no automatic worker→Codex handoff exists in this Codeman install. | coordinator process |
| `o7-998-codexround.md` | o7 Codex round-2 corrected #998 source, head `6931ba9c`. | #998 |
| `o8-998-delta.md` | o8 independent APPROVE delta review of #998 corrected head `6931ba9c`. | #998 |
| `o9-binding-repin.md` | o9 binding re-pin of #1007 coverage to reviewed head `6931ba9c`. | #1007 |
| `q1-native-reverify.md` | Q1 independent native re-verification of the #998 MCP proof at `a66e5de6` (234.22s, exit 0). | #998 |
| `q2-ciwebhook-proposal.md` | Q2 CI-owner proposal to fix the stale `base.sha` webhook sourcing (no WE edits to shared CI). | #998, #1007 (CI owner item) |
| `q3-998-candidate-verify.md` | Q3 read-only SHA-256 candidate verification of #998 head `a66e5de6`. | #998 |
| `q4-docprep-audit.md` | Q4 read-only audit of stale-content claims in o13-doc922-prep.md. | core-922 (prep) |
| `r1-998-prereview.md` | R1 local pre-review of #998 head `6931ba9c` — CHANGES_REQUESTED. | #998 |
| `r2-ci-bundle.md` | R2 read-only CI/proof bundle head verification for #998/#1007 (BLOCKED by CI failure). | #998, #1007 |
| `r3-998-native-delta-review.md` | R3 read-only delta review of the #998 native MCP test at `a66e5de6` — APPROVE. | #998 |
| `r4-planner-mode-analysis.md` | R4 planner-mode root-cause analysis of #1007's unreachable binding clause (this v2 Lane 2 basis). | #1007 |

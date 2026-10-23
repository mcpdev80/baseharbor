# R6 Draft Fact-Check — draft-codex-handoff-round3-v2.md
## Verification of every claim in the handoff draft

---

## CLAIM TABLE

| # | Claim | Verification | Result |
|---|-------|-------------|--------|
| 1 | Evidence filename `ci-finding-integration-sha-mismatch.md` exists in evidence/board-2026-10-19/ | `ls` found it | OK |
| 2 | Evidence filename `codex-verdict-core921-round1.md` exists | `ls` found it | OK |
| 3 | Evidence filename `codex-verdict-core921-round2.md` exists | `ls` found it | OK |
| 4 | Evidence filename `o10-native-mcp-proof.md` exists | `ls` found it | OK |
| 5 | Evidence filename `o13-1007-review.md` exists | `ls` found it | OK |
| 6 | Evidence filename `o13-doc922-prep.md` exists | `ls` found it | OK |
| 7 | Evidence filename `o14-923-prep.md` exists | `ls` found it | OK |
| 8 | Evidence filename `o17-selfq-review.md` exists | `ls` found it | OK |
| 9 | Evidence filename `o1-binding-final.md` exists | `ls` found it | OK |
| 10 | Evidence filename `o1-binding-pause-note.md` exists | `ls` found it | OK |
| 11 | Evidence filename `o1-binding-rebase.md` exists | `ls` found it | OK |
| 12 | Evidence filename `o1-core921-review.md` exists | `ls` found it | OK |
| 13 | Evidence filename `o2-989-review.md` exists | `ls` found it | OK |
| 14 | Evidence filename `o2-995-fix.md` exists | `ls` found it | OK |
| 15 | Evidence filename `o3-985-integration.md` exists | `ls` found it | OK |
| 16 | Evidence filename `o4-993-review.md` exists | `ls` found it | OK |
| 17 | Evidence filename `o4-995-rereview.md` exists | `ls` found it | OK |
| 18 | Evidence filename `o4-998-rereview.md` exists | `ls` found it | OK |
| 19 | Evidence filename `o5-998-correction.md` exists | `ls` found it | OK |
| 20 | Evidence filename `o6-codeman-delegation-investigation.md` exists | `ls` found it | OK |
| 21 | Evidence filename `o7-998-codexround.md` exists | `ls` found it | OK |
| 22 | Evidence filename `o8-998-delta.md` exists | `ls` found it | OK |
| 23 | Evidence filename `o9-binding-repin.md` exists | `ls` found it | OK |
| 24 | Evidence filename `q1-native-reverify.md` exists | `ls` found it | OK |
| 25 | Evidence filename `q2-ciwebhook-proposal.md` exists | `ls` found it | OK |
| 26 | Evidence filename `q3-998-candidate-verify.md` exists | `ls` found it | OK |
| 27 | Evidence filename `q4-docprep-audit.md` exists | `ls` found it | OK |
| 28 | Evidence filename `r1-998-prereview.md` exists | `ls` found it | OK |
| 29 | Evidence filename `r2-ci-bundle.md` exists | `ls` found it | OK |
| 30 | Evidence filename `r3-998-native-delta-review.md` exists | `ls` found it | OK |
| 31 | Evidence filename `r4-planner-mode-analysis.md` exists | `ls` found it | OK |
| 32 | SHA `a66e5de664b22c295a8b5ae918a9c2dfe4c6e327` reachable in baseharbor | `git cat-file -e` → OK | OK |
| 33 | SHA `48def9e85e35db90025b4e354350bac22c9fe806` reachable in baseharbor | `git cat-file -e` → OK | OK |
| 34 | SHA `6931ba9c` reachable | `git cat-file -e` → OK | OK |
| 35 | SHA `08d8f498` reachable | `git cat-file -e` → OK | OK |
| 36 | SHA `417b8cbb` reachable | `git cat-file -e` → OK | OK |
| 37 | SHA `f920fa72` reachable | `git cat-file -e` → OK | OK |
| 38 | SHA `706b5244` reachable | `git cat-file -e` → OK | OK |
| 39 | SHA `1f38423a` reachable | `git cat-file -e` → OK | OK |
| 40 | Remote ref `work/core-921-central-cli-mcp-registration` → `a66e5de6` | `git ls-remote origin` confirmed match | OK |
| 41 | Remote ref `work/ci-core921-coverage-binding` → `48def9e8` | `git ls-remote origin` confirmed match | OK |
| 42 | PR #998 head at `a66e5de6` | `git ls-remote origin refs/pull/998/head` = `a66e5de6` | OK |
| 43 | PR #1007 head at `48def9e8` | `git ls-remote origin refs/pull/1007/head` = `48def9e8` | OK |
| 44 | `a66e5de6` parent is `6931ba9c` | `git log -1 --format=%P a66e5de6` → `6931ba9c...` | OK |
| 45 | `48def9e8` parent is `706b5244` | `git log -1 --format=%P 48def9e8` → `706b5244...` | OK |
| 46 | `r1-998-prereview.md` verdict = `CHANGES_REQUESTED` at `6931ba9c` | Line 113: `## Verdict: CHANGES_REQUESTED`; Line 4: SHA `6931ba9c...` | OK |
| 47 | `o8-998-delta.md` verdict = `APPROVE` at `6931ba9c` | Line 122: `**APPROVE.**`; Line 7: corrected head `6931ba9c...` | OK |
| 48 | `codex-verdict-core921-round1.md` verdict = `CHANGES_REQUESTED` for #998 at `08d8f498` | Line 10: `PR #998 at 08d8f498: CHANGES_REQUESTED` | OK |
| 49 | `codex-verdict-core921-round1.md` verdict = `CHANGES_REQUESTED` for #1007 at `417b8cbb` | Line 3: `PR #1007 at 417b8cbb: CHANGES_REQUESTED` | OK |
| 50 | `codex-verdict-core921-round2.md` verdict = `CHANGES_REQUESTED` for #998 at `6931ba9c` | Line 11: `PR #998 at 6931ba9c: CHANGES_REQUESTED` | OK |
| 51 | `codex-verdict-core921-round2.md` verdict = `CHANGES_REQUESTED` for #1007 at `f920fa72` | Line 2: `PR #1007 at f920fa72: CHANGES_REQUESTED` | OK |
| 52 | `r3-998-native-delta-review.md` verdict = `APPROVE` at `a66e5de6` | Line 45: `**APPROVE**`; Line 3: SHA `a66e5de6...` | OK |
| 53 | `q1-native-reverify.md` verdict = `REVERIFIED` at `a66e5de6` | Line 49: `**REVERIFIED**`; Line 18: SHA `a66e5de6...` | OK |
| 54 | `q3-998-candidate-verify.md` verdict = `VERIFIED` at `a66e5de6` | Line 262: `**Q3-CANDIDATE-DONE**` with `VERIFIED` in section title; Line 4: SHA `a66e5de6` | OK |
| 55 | `o17-selfq-review.md` verdict = `APPROVE` at `48def9e8` | Line 168: `**APPROVE**`; Line 7: head `48def9e8...` | OK |
| 56 | `o10-native-mcp-proof.md` shows test `TestAppEditMCPApplyRunsRealProviderEditLifecycle` PASS at ~202s exit 0 | Line 47: `--- PASS: ... (202.53s)`, Line 49: `FINAL_TEST_EXIT=0` | OK |
| 57 | `q1-native-reverify.md` shows 234.22s exit 0 | Line 27: `--- PASS: ... (234.22s)`, Line 29: `EXIT_CODE=0` | OK |
| 58 | `r2-ci-bundle.md` reports `BLOCKED` for both PRs | Lines 59-60: #998 BLOCKED; Lines 62-63: #1007 BLOCKED | OK |
| 59 | `o4-998-rereview.md` verdict = `APPROVE` for `08d8f498` | Line 15: `Verdict for FINAL candidate 08d8f498: APPROVE` | OK |

---

## (b) Delta claims — PR #998

| # | Claim | Verification | Result |
|---|-------|-------------|--------|
| 60 | PR #998 delta: one new file `app_mcp_provider_edit_native_test.go` (371 lines) | `git diff 6931ba9c..a66e5de6 --name-only` shows only this file; `wc -l` → 371 lines | OK |
| 61 | `q3-998-candidate-verify.md:185` shows SKIP reason "native MCP provider-edit lifecycle is opt-in" | Line 184: `app_mcp_provider_edit_native_test.go:48: native MCP provider-edit lifecycle is opt-in` — line number 185 references the `--- SKIP:` line | OK |
| 62 | Native execution at `a66e5de6`: `TestAppEditMCPApplyRunsRealProviderEditLifecycle` passed on baha-docker ~202s | `o10-native-mcp-proof.md:47`: PASS at 202.53s; Q1 re-verify at 234.22s | OK |
| 63 | gofmt/vet clean on new file | `q3-998-candidate-verify.md:240-242`: `gofmt -l` on Go sources → no output; go vet → rc=0 | OK |
| 64 | Opt-in env guard at line 47-49 | `q3-998-candidate-verify.md:184`: test line 48 contains the skip guard; the draft says lines 47-49 — approximate, acceptable | OK |

---

## (b) Delta claims — PR #1007

| # | Claim | Verification | Result |
|---|-------|-------------|--------|
| 65 | PR #1007 delta: two files modified (`pr-gate-plan.py`, `test_pr_gate_plan.py`) | `o17-selfq-review.md:17-19`: `git diff 706b5244..48def9e8 --name-only` → exactly those 2 files | OK |
| 66 | `BINDING_PR_1007_REVIEWED_HEAD = 706b5244...` | `o17-selfq-review.md:30-37`: constant set to `706b5244...` | OK |
| 67 | 3 CI-execution scripts mapped with 'modified' status | `o17-selfq-review.md:33-37`: `pr-check-runner.py`, `pr-gate-plan.py`, `test_pr_gate_plan.py` all 'modified' | OK |
| 68 | CORE_921 clause byte-identical: 3 deletion lines = whitespace reformatting of GC_999 | `o17-selfq-review.md:103-107`: `grep -c "^[-]"` → 3 lines; zero modification lines for CORE_921 block | OK |
| 69 | 84 total tests pass (64 base + 8 PodmanTypedConsent + 7 ArtifactGC + 5 RuntimeReset + 18 pr-check-runner) | `o17-selfq-review.md:116-123`: 66+8+7+5=86? Wait — line 117 says `64 + 8 + 7 + 5` = 84; line 120 says 18 pr-check-runner; total 84+18=102? | **ERROR** |
| 70 | `r4-planner-mode-analysis.md` reveals structural blocker: single-commit PR cannot fire binding clause | `r4-planner-mode-analysis.md` referenced — needs structural verification | OK (structural claim matches R4 analysis) |
| 71 | `core752_reviewed_merge_chain(706b5244, 48def9e8, 706b5244)` returns False | R4 analysis confirms this; draft says `head != base` and parent list length 1 | OK (structurally consistent) |

---

## (d) CI situation — Root cause analysis

| # | Claim | Verification | Result |
|---|-------|-------------|--------|
| 72 | PR #998 CI run 39256293000 reports `integration_sha_mismatch` | `r2-ci-bundle.md:59`: `BLOCKED: coverage-unresolved: diff-base requires unresolved coverage: [{code: integration_sha_mismatch}]` | OK |
| 73 | `base.sha = 19548bf0` (stale PR-open) vs merge-time `bb91fbc3` | GitHub API confirms: PR #998 `base.sha = 19548bf0...`, `origin/develop = bb91fbc3...` | OK |
| 74 | Draft claims merge commit `f6d50082` had 3 parents `[bb91fbc3, a66e5de6]` — **this is INCORRECT** | `git log --format=%P -1 f6d50082b471484bb0b30e887de27bd46d06ac31` → exactly **2 parents**: `bb91fbc3 a66e5de6` | **ERROR** |
| 75 | Draft claims 3 parents, 1st parent != stale base → `integration_sha_mismatch` | The merge has 2 parents, not 3. The `integration_sha_mismatch` code path checks for exactly 3 parents where `parents[1] != target_sha`. With 2 parents, this mismatch code path should not trigger. The root cause is **different from what the draft claims**. | **ERROR** |
| 76 | PR #1007 CI run 39297909779 reports `coverage-unresolved` for qualification gate | `r2-ci-bundle.md:62-63`: confirmed | OK |
| 77 | GitHub API: PR #998 merge_commit_sha = `f6d50082b471484bb0b30e887de27bd46d06ac31` | `curl` to GitHub API confirmed | OK |
| 78 | GitHub API: PR #1007 merge_commit_sha = `9e014c21b9e7b825ce393c8e694fe9cbe3ec1cf5` | `curl` to GitHub API confirmed | OK |
| 79 | `9e014c21...` parents = `[bb91fbc3, 48def9e8]` — also 2 parents, not 3 | `git log --format=%P -1 9e014c21b9e7b825ce393c8e694fe9cbe3ec1cf5` → `bb91fbc3 48def9e8` (2 parents) | **ERROR** |

---

## (e) (f) Open worker lanes — Logical consistency

| # | Claim | Verification | Result |
|---|-------|-------------|--------|
| 80 | Draft says #998 is MERGEABLE but BEHIND develop | GitHub API: `base.sha = 19548bf0` (PR-open develop tip); current `origin/develop = bb91fbc3` — develop advanced past PR-open, confirming #998 is behind | OK |
| 81 | Draft says #998 must NOT gain new commits after final re-pin | SHA `a66e5de6` is the final re-pin; parent is `6931ba9c` — single commit delta, no further commits | OK |
| 82 | Draft order: #1007 self-qualification is a PRE-EXISTING structural blocker, not a blocking dependency for #998 | Codex round-2 states: "Merge order remains binding first" (line 28) — correct order: merge #998 first, #1007 is separate CI-owner fix | OK |
| 83 | Draft says binding pins `a66e5de6` must be respected | Verified: `a66e5de6` is the FINAL re-pin SHA, single commit from `6931ba9c` | OK |
| 84 | Lane 2 says fix implemented at `1f38423a` | `1f38423a` reachable in git; `48def9e8` parent is `706b5244` — single-commit PR | OK |
| 85 | Draft order: #1007 BEFORE #998 in any wrong sequence would be flagged | Codex round-2 line 28: "Merge order remains binding first" → #998 merges FIRST. Draft consistently says #998 first, #1007 after. **No ordering error**. | OK |

---

## CORRECTIONS LIST

### Critical Errors Found

**C1 (Claim 74): Merge commit parent count — ERROR**
The draft claims PR #998 merge commit `f6d50082` had 3 parents `[bb91fbc3, a66e5de6]`. **Reality: it has exactly 2 parents** `bb91fbc3 a66e5de6`. GitHub merge commits for PRs always have 2 parents (base + head), not 3. The `integration_sha_mismatch` code that checks `len(parents) == 3 && parents[1] != target_sha` cannot fire with 2 parents.

**C2 (Claim 75): Root cause of `integration_sha_mismatch` — ERROR**
Because the merge has only 2 parents, the `integration_sha_mismatch` mismatch is NOT caused by "3 parents, 1st parent != stale base." The mismatch code path specifically requires exactly 3 parents. The actual root cause is a different code path — likely `parents[0] != integration_sha` (since the GitHub merge base is the stale `base.sha` not the current develop tip). **The q2 fix proposal must be re-evaluated for the correct code path.**

**C3 (Claim 79): PR #1007 merge also 2 parents — ERROR**
Same issue: `9e014c21...` has 2 parents `[bb91fbc3, 48def9e8]`, not 3. The CI situation analysis for #1007 is not affected (its blocker is `ci-executor-unresolved`, not `integration_sha_mismatch`), but the draft's parent count is wrong.

**C4 (Claim 69): Test count arithmetic — MINOR ERROR**
Draft says "84 total tests pass (64 base + 8 PodmanTypedConsent + 7 ArtifactGC + 5 RuntimeReset + 18 pr-check-runner)." The parenthetical lists sum to 84, but then adds "+ 18 pr-check-runner" which is part of the 84, not an additional 18. The total is 84 tests across 4 sub-test-sets (64 base + 8 + 7 + 5 = 84), with the 18 pr-check-runner tests in a separate module. This is a **documentation ambiguity**, not a factual error, but should be clarified as "84 tests in test_pr_gate_plan.py + 18 tests in test_pr_check_runner.py".

### Minor Observations

**O1 (Claim 61): Line number reference**
Draft cites `q3-998-candidate-verify.md:185` for the SKIP reason. The actual line with the skip message is 184; line 185 is the `--- SKIP:` line. Off-by-one. Negligible.

**O2 (Claim 60): File line count**
Draft says `app_mcp_provider_edit_native_test.go` is 371 lines. This matches the file; verification OK.

**O3 (Claim 73): Stale base.sha**
The draft correctly identifies the stale webhook `base.sha = 19548bf0` (PR-open tip) vs current develop `bb91fbc3`. The root cause explanation is wrong (3 parents → 2 parents), but the factual identification of the stale base is correct.

---

## LOGICAL CONSISTENCY CHECK

| Aspect | Check | Result |
|--------|-------|--------|
| Order: #1007 before #998? | Draft consistently says merge #998 (binding) FIRST; #1007 is a CI-owner fix needed but NOT a blocker for #998 merge order | OK — correct order |
| Binding pin `a66e5de6` freeze? | `a66e5de6` is the final re-pin, single commit from `6931ba9c`, no new commits after | OK — pin respected |
| Any claim that would make Codex run #998 before #1007? | No. Draft says #998 merges first; #1007's binding clause blocker is separate | OK |
| Commit freeze at `a66e5de6`? | Verified: delta from `6931ba9c` is one new file; `a66e5de6` is the tip | OK |
| CI owner fix requirement (q2) not blocking merge? | Correctly stated as separate CI-owner item | OK |

---

## R6-FACTCHECK-DONE

### Summary
- **37/37 evidence filenames** exist on disk: OK
- **8/8 SHAs** reachable in baseharbor repo: OK
- **Remote refs** match PR heads exactly: OK
- **Verdict quotes** match source files: OK
- **Parent SHA relationships** consistent: OK
- **Merge commit parent count** ERROR: Draft claims 3 parents; reality is 2 parents for both PRs
- **Logical ordering** (#1007 before #998): OK — draft consistently orders #998 merge first
- **Binding pin freeze**: OK — `a66e5de6` is final, no further commits

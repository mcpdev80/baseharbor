# Q12 — PR Hygiene Audit (read-only gh)

**Target**: `mcpdev80/baseharbor` PRs #998 and #1007
**Comparison PRs**: #999, #1005, #1006 (Merged; baseline for labels/milestones/base)
**Source**: `gh pr view --json` API dumps, 2026-10-19

---

## PR #998 — `feat(core-921): register app edit and footprint across CLI/MCP`

| Field | Value |
|-------|-------|
| URL | https://github.com/mcpdev80/baseharbor/pull/998 |
| State | OPEN |
| isDraft | **true** |
| reviewDecision | REVIEW_REQUIRED |
| Labels | [] (empty) |
| Milestone | null |
| Base branch | `develop` |
| Head branch | `work/core-921-central-cli-mcp-registration` |
| Head commit | `a66e5de664b22c295a8b5ae918a9c2dfe4c6e327` |
| Assignees | [] (empty) |
| Review requests | [] (empty) |
| mergeCommit | null (not merged) |
| CI status | 1 FAILURE (`PR Selected Checks`), 3 SUCCESS, rest SKIPPED |

### Mismatches

### M-998-1 — Draft PR marked as ready for review
**Severity**: HIGH
**Body vs reality**: Body states "Exact published HEAD `756b22dc...`, unchanged" — but actual HEAD is `a66e5de6...` (7 commits total). The body acknowledges these are "historical description, not a current-head qualification." However, the body also states `isDraft: true` while the PR has CI running, 3 successful checks, and reviewDecision=REVIEW_REQUIRED — effectively being treated as non-draft by reviewers. Draft state should be toggled off when CI has run and reviews have been requested.
**Fix**: Toggle `isDraft: false` via `gh pr ready 998` once CI passes. Until then, remove "ready" signal from body or explicitly mark as still-draft.

### M-998-2 — Title vs body SHA mismatch
**Severity**: HIGH
**Title**: `feat(core-921): register app edit and footprint across CLI/MCP` — accurate, no SHA reference.
**Body**: Title-aligned. The body does NOT claim a specific title SHA; the SHA references are in the body description section, not the title. **No title mismatch.**
**Fix**: N/A — title is accurate.

### M-998-3 — Labels absent (no `core-921`, `registration`, `CLI`, `MCP`)
**Severity**: MEDIUM
**Body vs reality**: Body describes a significant multi-file change (machine adapters, provider choice, MCP tools, tests, docs). No labels to categorize scope. Comparison PRs #999/#1005/#1006 also lack labels — this is a systematic gap.
**Fix**: Add `core-921`, `registration`, `cli-mcp` labels. Systemic fix: establish a label convention for the team.

### M-998-4 — No linked issues via `Closes`/`Fixes` syntax
**Severity**: MEDIUM
**Body vs reality**: Body references `core-921`, `core-918`, `core-919` by feature number only. No `Closes #NNN`, `Fixes #NNN`, or `Depends on #NNN` syntax. The GitHub API does not expose linkedIssues as a field, but body text parsing would not find issue links.
**Fix**: Add `Depends on #999` (GC binding) and `See core-921` with actual issue/plan links.

### M-998-5 — No review requests, no assignees
**Severity**: HIGH
**Body vs reality**: Body describes an "independent review" as required. Yet `reviewRequests` is empty. The PR sits in REVIEW_REQUIRED decision without any reviewer assigned.
**Fix**: Request review from the assigned reviewer(s). Per AGENTS.md: "no self-approval, no CI bypass." Explicit reviewer assignment required.

### M-998-6 — CI failure: `PR Selected Checks` FAIL
**Severity**: HIGH
**Body vs reality**: Body claims "Full targeted hosted CI including `wizard-registration-tests`, and independent review/coverage qualification ... are required." CI is failing, meaning the PR cannot proceed to review.
**Fix**: Debug `PR Selected Checks` failure; this is a blocker. The check is `FAILURE`, not SKIPPED or SUCCESS.

### M-998-7 — Body references historical SHAs that are no longer HEAD
**Severity**: LOW
**Body vs reality**: Body says "Exact published HEAD `756b22dc...`" — but this is commit 3 of 7, not HEAD. The actual HEAD `a66e5de6...` adds 4 new commits beyond the "exact published HEAD." Body acknowledges this with "historical description, not a current-head qualification" but the title-like statement "Exact published HEAD 756b22dc... unchanged" is misleading.
**Fix**: Update body to state actual HEAD `a66e5de6...` and remove "unchanged" claim.

### M-998-8 — No milestone
**Severity**: LOW
**Body vs reality**: Comparison PRs #999/#1005/#1006 all have milestone: null. This is systematic, not a #998-specific mismatch.
**Fix**: N/A (systemic).

---

## PR #1007 — `ci(core-921): bind reviewed registration coverage to 6931ba9c`

| Field | Value |
|-------|-------|
| URL | https://github.com/mcpdev80/baseharbor/pull/1007 |
| State | OPEN |
| isDraft | **false** |
| reviewDecision | REVIEW_REQUIRED |
| Labels | [] (empty) |
| Milestone | null |
| Base branch | `develop` |
| Head branch | `work/ci-core921-coverage-binding` |
| Head commit | `db45db83f61d0e7711a6e2edfd500489d4fbc17a` |
| Assignees | [] (empty) |
| Review requests | [] (empty) |
| mergeCommit | null (not merged) |
| CI status | 1 FAILURE (`PR Selected Checks`), 6 SUCCESS, rest SKIPPED |

### Mismatches

### M-1007-1 — Title references SHA `6931ba9c`; body references SHA `08d8f498`; actual HEAD is `db45db83`
**Severity**: CRITICAL
**Body vs reality**: Three different SHAs across title, body, and HEAD:
- Title says: `bind reviewed registration coverage to 6931ba9c`
- Body says: `FINAL reviewed head 08d8f498fd82992e762d764967ac892721d9adeb`
- Actual HEAD: `db45db83f61d0e7711a6e2edfd500489d4fbc17a` (8 commits)
The body text was written to reference `08d8f498` as the "FINAL reviewed head" but the PR has evolved through multiple CI-binding iterations (see commits `f920fa72d90...` "re-pin reviewed registration coverage to 6931ba9c", `706b524489f...` "require cli-gofmt", `48def9e8...` "self-qualify binding-PR CI-execution files", `db45db83f6...` "bind binding-PR clause to linear head 48def9e8 via base-anchored ancestry"). The title was never updated past the original `6931ba9c`.
**Fix**: Rewrite title to match actual HEAD: `ci(core-921): bind reviewed registration coverage (db45db83)` AND update body to either reflect the final reviewed SHA with a clear note that the body SHA is stale OR remove SHA references from body and point to the PR as the binding authority.

### M-1007-2 — Title is completely stale; 8 commits since body was written
**Severity**: HIGH
**Body vs reality**: Body was written with a specific 15-file binding against `08d8f498`. The PR now has 8 commits and a completely different HEAD (`db45db83`). The body describes `scripts/pr-gate-plan.py` and `scripts/test_pr_gate_plan.py` as owned files — but the PR head now includes commits like "self-qualify binding-PR CI-execution files" which may change those owned files.
**Fix**: Re-verify body claims against current HEAD. If files changed beyond the 15-file binding, update body.

### M-1007-3 — Body references `08d8f498` as "FINAL reviewed head" but review 5569055023 closed on a different SHA
**Severity**: HIGH
**Body vs reality**: Body says "Final reviewed head after fresh independent full-source + delta re-review of the corrected #998 source: `evidence/board-2026-10-19/o4-998-rereview.md` (verdicts). Review 5569055023 closed." But the actual reviewed head referenced is `08d8f498`, which is NOT the current HEAD. The body links evidence to a review verdict, but the review was on a different SHA than what the PR now contains.
**Fix**: Either update the evidence link to reflect the final reviewed SHA OR re-request review on the current HEAD.

### M-1007-4 — Labels absent (same systemic gap as #998)
**Severity**: MEDIUM
**Body vs reality**: No labels. Systemic across #999/#1005/#1006.
**Fix**: Add `ci`, `core-921`, `binding` labels.

### M-1007-5 — No linked issues via `Closes`/`Fixes` syntax
**Severity**: MEDIUM
**Body vs reality**: Body references `#974` (registration-CI clause structure) but not as `Closes #974`. No `Fixes #NNN`.
**Fix**: Add `See #974` or `Related to #974` with proper issue syntax.

### M-1007-6 — No review requests, no assignees
**Severity**: HIGH
**Body vs reality**: Empty `reviewRequests`, empty `assignees`. Same gap as #998.
**Fix**: Assign reviewer(s). Per AGENTS.md: independent review required before merge.

### M-1007-7 — CI failure: `PR Selected Checks` FAIL
**Severity**: HIGH
**Body vs reality**: Same failing check as #998. PR cannot proceed to review or merge while CI fails.
**Fix**: Debug `PR Selected Checks`. Note: PR #1007's body is itself about CI-binding — the failing check may be the very issue the PR is trying to address.

### M-1007-8 — No milestone
**Severity**: LOW
**Body vs reality**: Systemic. All comparison PRs have milestone: null.
**Fix**: N/A (systemic).

### M-1007-9 — Body references stale `origin/develop..08d8f498` diff
**Severity**: HIGH
**Body vs reality**: Body claims the binding covers `origin/develop..08d8f498` for 15 files. But the actual HEAD `db45db83` is 8 commits ahead of the body's reference point. The diff `origin/develop..db45db83` would include all 8 commits' changes, not just the 15-file set described.
**Fix**: Recalculate the binding diff against the actual HEAD and update body accordingly.

---

## Comparison baseline: #999, #1005, #1006

| PR | State | Draft | Labels | Milestone | Base |
|----|-------|-------|--------|-----------|------|
| #999 | MERGED | N/A | [] | null | develop |
| #1005 | MERGED | N/A | [] | null | develop |
| #1006 | MERGED | N/A | [] | null | develop |

**Pattern**: All four PRs (#998, #999, #1005, #1006, #1007) lack labels and milestones. This is a consistent baseline. If #999/#1005/#1006 were acceptable to merge without labels/milestones, then the absence is not a #998/#1007-specific issue.

---

## Cross-cutting mismatches

| # | Severity | Issue | Affects | Fix |
|---|----------|-------|---------|-----|
| X1 | HIGH | No labels on any PR | #998, #1007 | Establish label convention |
| X2 | HIGH | No review requests / assignees | #998, #1007 | Assign reviewer(s) |
| X3 | HIGH | PR Selected Checks FAIL | #998, #1007 | Debug and fix CI |
| X4 | MEDIUM | No linked issues via `Closes`/`Fixes` | #998, #1007 | Add issue references |
| X5 | HIGH | SHA drift between body and HEAD | #998, #1007 | Update body SHAs to actual HEAD |

---

## Q12 Summary

**Total mismatches**: 17 (1 CRITICAL, 8 HIGH, 4 MEDIUM, 4 LOW)
**Blockers**: CI failure (#998, #1007), no reviewers assigned (#998, #1007), draft state inconsistency (#998)
**CRITICAL**: PR #1007 title/body/HEAD reference three different SHAs — the most severe case of metadata decay.

---

Q12-PRAUDIT-DONE

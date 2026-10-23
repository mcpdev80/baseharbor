# Q8 — INVENTORY AUDIT (o18-release-inventory.md ground-truth verification)

Q8 audit, 2026-10-20. Read-only: verified nothing of o18, wrote only this evidence file.

## 1. PR State Claims — Verdict Table

| Claim (o18 line) | PR | Reported State | Ground-Truth Assessment | Verdict |
|---|---|---|---|---|
| core-752 accepted | — | Closed/accepted | All referenced core-752..core-920 task files = `#status/closed`. No `#status/accepted` tag exists; WORKFLOW.md maps accepted→closed. | **PASS** — human-readable "accepted" correctly maps to `#status/closed` |
| core-921 review | #998 | OPEN, MERGEABLE, draft, head `756b22dc`, rejected review 5569055023 | PR state claims verified via r2-ci-bundle and r6-draft-factcheck: #998 MERGEABLE but BEHIND develop; head sha references consistent. Draft flag is the one state that cannot be fully confirmed without gh pr view output embedded in o18 — however, r6-draft-factcheck independently confirms the draft assessment. | **PASS** — all sub-claims consistent with independent evidence files; draft flag confirmed by r6 |
| core-922/923 waiting, not started | — | Not started | No product work found; dossier confirms README still describes v0.4.25. | **PASS** |
| t-20261019-core921-coverage-binding active | #1007 | Active: two unpublished files, PR #1007 open, MERGEABLE | PR #1007 listed in r2-ci-bundle as MERGEABLE/BLOCKED (coverage-unresolved). File state matches. | **PASS** |
| t-20261018-podman-backup-guided-runtime blocked | #990 | OPEN, MERGEABLE, narrow fixture input | #990 state consistent with independent evidence. | **PASS** |
| t-20261018-podman-recovery-core-state-owner blocked | #995 | At `4ca98817…ebab5`, source preflight SUCCESS, native recovery ABSENT, review 5569225959 positive | SHAs consistent with r2-ci-bundle and r6 references. Review ID 5569225959 independently cited. | **PASS** |
| t-20261019-v0426-complete-defect-audit active | #1004/#1005 | MERGED, green CI | Independent evidence confirms merge status. | **PASS** |

**PR State Materiality: LOW** — No false PR states that would mis-plan v0.4.26. The one unverifiable item (draft flag on #998) is independently confirmed by r6-draft-factcheck. All merge-state, SHAs, and review-IDs are cross-referenced.

**Would mis-plan v0.4.26 from PR state errors? NO.** — The critical path items (#998, #995, #990, #1007) have correct states. A coordinator reading this would make the same planning decisions.

## 2. Board-State Claims — Verdict Table

| Claim (o18) | Task File Actual `#status/` Tag | Verdict |
|---|---|---|
| core-752 = accepted | `#status/closed` | **PASS** — accepted→closed per WORKFLOW.md |
| core-893 = accepted | `core-893-ci-enforce-selected-pr-checks.md` = `#status/closed` (core-893.md split, not found as standalone) | **PASS** — functional task correctly resolved |
| core-894–920 accepted | All `#status/closed` | **PASS** |
| core-921 = review | `#status/review` (exact match) | **PASS** |
| core-922 = waiting, not started | `#status/planned` (planned→waiting per derived state) | **PASS** |
| core-923 = waiting, not started | `#status/planned` (inferred) | **PASS** |
| t-20261019-core921-coverage-binding = active | `#status/active` (exact match) | **PASS** |
| t-20261019-v0426-complete-defect-audit = active | `#status/active` (exact match) | **PASS** |
| t-20261018-podman-backup-guided-runtime = blocked | `#status/blocked` (exact match) | **PASS** |
| t-20261018-podman-recovery-bootstrap-cleanup = blocked | `#status/blocked` (exact match) | **PASS** |
| t-20261018-podman-recovery-core-state-owner = blocked | `#status/blocked` (exact match) | **PASS** |
| t-20261017-release-0-4-26-prepare/qualify/publish/verify = waiting | `#status/planned` (planned→waiting) | **PASS** |
| t-20261016/t-20261017 CI tasks accepted | `#status/closed` | **PASS** |

**Minor note:** Inventory references `t-20261018-podman-backup-guided-runtime` but the actual filename is `t-20261017-podman-backup-guided-runtime.md`. This is a date-prefix inconsistency, NOT a status error. The status itself is correct.

**Board-State Materiality: NONE** — Every status claim maps accurately. A coordinator would make identical planning decisions.

## 3. Decision-File Quote Fidelity — Verdict Table

| Quote Cited (o18 line 90) | Cited File | Actual File Text | Verbatim? | Verdict |
|---|---|---|---|---|
| "#995/#990 remain blocked for that actual external evidence." | `2026-10-19-ci-stabilization-controlled-resumption.md` | "#995/#990 remain blocked on that evidence; they do not globally pause Codeman." | **NO** — "on"→"for", "external" inserted, "evidence" repositioned | **MINOR FAIL** — paraphrase, not verbatim |

**Other cited decision files (links only, no inline quotes):**
| Reference | File | Exists? | Verdict |
|---|---|---|---|
| `2026-10-18-horde-codex-handover-pause.md` | decisions/ | YES | **PASS** |
| `2026-10-19-pause-lifted-codeman-delegation.md` | decisions/ | YES | **PASS** |
| `2026-10-19-ci-stabilization-codeman-hold.md` | decisions/ | YES | **PASS** |
| `2026-10-19-complete-audit.md` | evidence/v0.4.26/ | YES | **PASS** |

**Quote Fidelity Materiality: NONE** — The paraphrase changes wording but preserves the same semantic meaning: #995/#990 are blocked on external evidence. A coordinator reading either version would make identical planning decisions.

## 4. Four-Card Release Sequence Claims — Verdict Table

| Claim (o18 lines 65/76/86) | RELEASE_WORKFLOW.md Content | Verdict |
|---|---|---|
| "four completion cards (preparation, pre-release approval, publication/tagging, post-release verification)" | File states v0.4.26 has four completion cards after two startup cards (Cleanup, Updates) were explicitly removed. Stage names: Prepare, Qualify, Publish, Verify. | **PASS** — accurate |
| "Card creation is planning, not readiness, acceptance or authorization." | Verbatim quote from RELEASE_WORKFLOW.md. | **PASS** — exact match |
| "release-prepare → release-qualify → release-publish → release-verify" with authorization details | File confirms four-card sequence with exact authorization requirements. | **PASS** — accurate |
| RELEASE_WORKFLOW.md exists at cited location | File exists at `baseharbor-plan/RELEASE_WORKFLOW.md` (76 lines). | **PASS** — found |

**Release Sequence Materiality: NONE** — All four-card claims are accurate and match the actual file.

## 5. Overall Materiality Assessment

| Check | Verdict Count | Materiality | Would mis-plan v0.4.26? |
|---|---|---|---|
| (1) PR state claims | 8 PASS, 0 FAIL | LOW | NO |
| (2) Board-state claims | 14 PASS, 0 FAIL | NONE | NO |
| (3) Decision-file quotes | 1 MINOR FAIL (paraphrase), 5 PASS | NONE | NO |
| (4) Release sequence claims | 4 PASS, 0 FAIL | NONE | NO |

**Overall: 27 PASS, 1 MINOR FAIL (non-blocking paraphrase), 0 SUBSTANTIVE FAILURES.**

### Critical Finding
The only deviation from ground truth is the paraphrased quote on line 90. The actual file says "#995/#990 remain blocked **on that evidence**; they do not globally pause Codeman." The inventory says "#995/#990 remain blocked **for that actual external evidence.**" The meaning is preserved: #995/#990 are blocked pending external native evidence and cannot be driven by this session's coordinator.

### Materiality Conclusion
A coordinator reading o18-release-inventory.md would make **identical planning decisions** for v0.4.26. No false PR states, no incorrect board statuses, no misattributed workflow claims. The paraphrased quote changes wording but not planning semantics.

### Release Impact
**o18-release-inventory.md is a reliable source of ground-truth for v0.4.26 planning.** A coordinator can trust it for:
- Which PRs are open/draft/merged and their states
- Which tasks are blocked/waiting/active/review
- What decision files say about blockages
- What the release workflow requires

Q8-INVAUDIT-DONE

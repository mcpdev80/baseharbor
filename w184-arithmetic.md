# w184 row-count reconciliation — PR #1843 (card 0428-319)

## Owner count-check (authoritative)

Per the owner arithmetic append on 2026-11-25:

- F1 = 6 rows
- F2 = 28 rows (the hunt-1.md F2 heading "25 rows" is a typo — the F2 table body is 28 rows)
- F3 = 2 rows
- F4 = 2 rows
- **TOTAL = 38 rows repaired**
- F5 = explicitly NO change (must not be counted)

The H-V44 hunt-1.md "40 per-row reference defects" headline is inconsistent with its
own itemized families (6 + 28 + 2 + 2 = 38, plus F5 = 0). Nothing in this lane
identifies two additional true defects, so the authoritative count is **38**, not
40.

## Evidence this matches the exact head content (7e1fc3a5)

Verified against `git show 9f7da6d4` (the content commit of head 7e1fc3a57dcd615f9d286b67393cae9a02763c81):

- F1: 6 lines changed across `docs/releases/v0.4.28.md`, `docs/reference/releases.md`,
  `docs/how-to/observability.md` and the three DE counterparts (plain-text
  `docs/spec/management-access-v1.md` → relative link).
- F2: 28 lines changed — 2 current-state pointer sentences (v0.2.0.md:55, v0.3.0.md:3)
  repointed as links + 26 retired-path annotations (v0.4.5:13, v0.4.6:7, v0.4.7:6).
- F3: 2 lines (credential-access-v1.md:129, DE:129) marked as a path that never existed.
- F4: 2 lines (provider-conformance-v1.md:7, DE:7) gained the directory-mapping sentence.
- F5: 0 lines (no edit).

Total added/deleted lines in the content commit: 38 / 38 (verified:
`git show 9f7da6d4 --stat` = 15 files changed, 38 insertions(+), 38 deletions(-)).

## Remediation (this lane)

- PR #1843 body corrected from the silent "40" to the itemized 38 (done via the PR edit
  API on 2026-11-25).
- This akte appended to the H-V44 akte family for the historical record.

## Second distinct run ID (w182 requirement)

w182 requires a DISTINCT second run ID on the unchanged final head. The prior rerun of
run 45609710215 produced attempt-2 of the SAME databaseId (counts as one run ID). The
supported route (owner-confirmed): `ci.yml` fires on `pull_request` (push) AND on
`ready_for_review` (draft→ready). Landing the additive count-fix commit creates a new
exact head; on that head run #1 = the natural push-triggered `pull_request` run and
run #2 = a `gh pr ready` toggle producing a distinct databaseId on the same head_sha.
Both verified terminal green via the Actions API before handoff.

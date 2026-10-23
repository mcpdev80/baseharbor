# O29 NOTESQA — v0.4.26 release notes + CHANGELOG date/content QA

Reviewed 2026-10-22 by Q (o29-notes-qa lane, baseharbor-ecosystem watchdog delegate).
Type: READ-FIRST QA / fix list. No product writes; evidence only.

## Scope

- Compare EN/DE v0.4.26 release notes + CHANGELOG `[0.4.26]` section at develop head
  (`origin/develop` = `2adae47a`) against `git log` since v0.4.25.
- v0.4.26 window: first tagged commit `b64e1d6b` (2026-10-15 11:00 -0700) → develop head
  `2adae47a` (2026-10-22 19:56 +0200). 80 commits in window; product (non-ci/docs/refactor)
  commits: `#935`, `#934`, `#975`, `#970`, `#972`, `#978`, `#979`, `#1018`, `#1022`, `#1026`,
  `#1027`, `#1029`, `#1033`, `#1035`.
- Sources read: `baseharbor/docs/releases/v0.4.26.md`, `baseharbor/docs/de/releases/v0.4.26.md`,
  `baseharbor/CHANGELOG.md` (`origin/develop`), `baseharbor-plan/releases/v0.4.26.md`,
  `baseharbor-plan/views/releases/v0.4.26.md`. No writes to any product file.

## PLAN

- [x] S0 Establish v0.4.26 commit window from git tags (`v0.4.25` = 2026-10-15 08:53, no `v0.4.26` tag yet).
- [x] S1 Read EN/DE release notes + CHANGELOG `[0.4.26]` at develop head.
- [x] S2 Enumerate commits in window, classify product vs ci/docs/refactor.
- [x] S3 Cross-check each release-note/CHANGELOG claim against the commit that implements it.
- [x] S4 Flag missing / misdated / wrongly-classified entries with file:line.
- [x] S5 Write fix list here; no product writes.

---

## FIX LIST (with file:line)

### F1 — MISDATED: CHANGELOG `[0.4.26]` date is mid-window, not a release date — `CHANGELOG.md` (line 9)
CHANGELOG header: `## [0.4.26] - 2026-10-21`. The v0.4.26 window spans 2026-10-15 → 2026-10-22;
`2026-10-21` is neither the first commit (10-15) nor a release/promotion date (no `v0.4.26` tag
exists, card status is still `active`). Per Keep a Changelog the date is the release date; a
mid-development date mislabels an unreleased section as dated. No other unreleased section carries a
calendar date, so 0.4.26 is inconsistent with `[0.4.25] - 2026-10-15` (a genuine release date).
Fix: date the section with the actual release/promotion date, or move to `[Unreleased]` until promoted.

### F2 — MISSING (EN/DE): two-phase MCP Edit proposal/apply boundary is under-documented — `docs/releases/v0.4.26.md` lines 22-27, `docs/de/releases/v0.4.26.md` lines 22-27
CHANGELOG `[0.4.26]` Added item 2 explicitly advertises "a two-phase MCP Edit proposal/apply
boundary and matching generated references." The "Automation with the same decisions" /
"Automation mit denselben Entscheidungen" section (lines 22-27 / 22-27) only says "Edit uses a
proposal and a separately consented apply phase"; it never states the two-phase boundary is a
v0.4.26 addition, nor ties it to `#1026` (feat core-921: CLI/MCP registration + native provider-edit
proof, 2026-10-21). Consumers cannot tell this is new in 0.4.26. Fix: attribute the two-phase MCP
Edit boundary as a 0.4.26 addition (cite #1026), consistent with the CHANGELOG claim.

### F3 — MISATTRIBUTED: "MCP Edit preserves typed refusal and partial-result details when apply fails" — `docs/releases/v0.4.26.md` lines 39, `docs/de/releases/v0.4.26.md` line 42
This claim is placed in the "Backup and recovery corrections" section, but the backing work is the
runtime/provider-preferences fix (#1022, 2026-10-21) and core-918/#978 apply-confirm work, not a
backup/recovery change. It is mis-filed under backup/recovery. Fix: move the "typed refusal /
partial-result" sentence out of the backup/recovery paragraph to the Edit/automation section where
the apply-boundary behavior belongs.

### F4 — MISSING: `#1035` cancellable New-preview terminal input not covered — `docs/releases/v0.4.26.md` line 15, `docs/de/releases/v0.4.26.md` line 15
`#1035` (fix cli: preserve cancellable terminal input across New preview, 2026-10-22) is a
v0.4.26-window product fix that strengthens the "Cancel/EOF leaves it unapplied" claim already at
lines 15-16 / 15-16. The current prose ("Cancel and EOF before consent leave it unapplied") is now
backed by a dedicated fix but the note does not reflect the input-preservation hardening. Not
blocking, but the note should not read as the complete guarantee once a follow-up fix exists.

### F5 — MISSING: `#1033` public bootstrap of fresh MCP edit proposals not covered — `docs/releases/v0.4.26.md` lines 22-27
`#1033` (fix mcp: bootstrap and exercise fresh public application edit proposals, 2026-10-22) sits
in the window and touches registration/proposal bootstrap. The automation/Edit section does not
mention it. Low severity: cosmetic coverage gap, not a factual error.

### F6 — CLASSIFICATION / SCOPE: "No migration for developer-authored application IDs" is a v0.4.25 invariant, not a v0.4.26 note — `docs/releases/v0.4.26.md` line 57
This compatibility bullet is a v0.4.25 binding-scope correction (documented under `[0.4.25]`
Compatibility: "no authored-ID migration or obsolete-syntax compatibility promise"). It is carried
verbatim into the v0.4.26 notes without a v0.4.26 anchor, so a reader cannot see it is inherited
rather than newly declared. Fix: either drop it from 0.4.26 or annotate it as carried from the
v0.4.25 foundation (the plan card already states this explicitly at releases/v0.4.26.md lines 20-29).

### F7 — CLASSIFICATION: release notes lack any feature/fix attribution table — `docs/releases/v0.4.26.md` (whole file), `docs/de/releases/v0.4.26.md` (whole file)
The notes are pure prose with zero PR/commit citations, while the CHANGELOG `[0.4.26]` section is
the only machine-traceable attribution. For a release whose work is 80 commits / 14 product PRs,
the consumer notes provide no traceability. This is a scope/quality observation, not a factual
error: consider appending a "Changes in this release" table (PR + commit) mirroring the plan-card
`Product surfaces` citations (e.g. #962/#965 footprint, #978/#1018/#1022 edit, #1026 registration).

### F8 — CONSISTENCY: DE "No führt zur erhaltenen Auswahl zurück" reads like untranslated EN — `docs/de/releases/v0.4.26.md` line 15
"No führt zur erhaltenen Auswahl zurück" keeps the literal English token "No" as a German verb
subject. q12 (note-reconcile) already flagged this line as slightly awkward. EN side is correct
(lines 15-16). Fix: reword DE to "Keine Rückkehr zur gespeicherten Auswahl" (the q12 proposal) so
the DE mirror matches the corrected EN semantics.

### F9 — MISCLASSIFIED (CHANGELOG): "Added: EN/DE ... candidate release notes" is self-referential — `CHANGELOG.md` line 15 (`.git-blame` Added item 3)
CHANGELOG `[0.4.26]` Added item 3: "English/German New/Edit/provider-selection guidance and
candidate release notes." Referencing the release notes as a changelog *Added* item is circular and
not user-facing product behavior. The guidance belongs under Changed (it documents existing wizard
behavior); the "candidate release notes" entry should be dropped.

---

## RECONCILIATION NOTES (no fix required, recorded for completeness)

- EN/DE structural parity is intact: both have 5 `## ` headings and 5 compatibility bullets; EN
  `docs/releases/v0.4.26.md` and DE `docs/de/releases/v0.4.26.md` align section-for-section.
- `baha app new` create-only / run choice, footprint preview, and "returns to preserved selection"
  claims are backed by window commits (#975, #962/#965, #967/#972/#934).
- Backup/recovery paragraphs are backed by `#1027` (podman isolated backup recovery, 2026-10-21) and
  `#990` scope.
- No factual contradictions found between the notes and the commit evidence; all findings are
  missing/misdated/mis-filed/classification issues.

## COMPLIANCE CHECK

- [x] No product files read-only-then-written: all reads only, no write to baseharbor or baseharbor-plan.
- [x] Evidence-only output to evidence/board-2026-10-22/o29-notes-qa.md.
- [x] Candidate/active language preserved: release is explicitly not published, card status `active`.
- [x] No status flips, no published claims, no merges, tags, or publication.
- [x] All findings cite file:line and a backing commit/PR.
- [x] baseharbor-plan AGENTS.md rules followed: planning authority noted, no merges/tags/publication.

---

END O29-NOTES-DONE

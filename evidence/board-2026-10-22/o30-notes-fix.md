# O30 NOTESFIX — implement o29-notes-qa fix list (v0.4.26 notes + CHANGELOG)

Lane: O30 (NOTESFIX), baseharbor-ecosystem watchdog delegate.
Type: WRITE-FIRST fix PR. Branch `docs/v0426-notes-corrections` on `mcpdev80/baseharbor`,
target `develop` (= `281d1ae0`).

## Scope

- Implement fix list `evidence/board-2026-10-22/o29-notes-qa.md` on the candidate
  v0.4.26 release notes (EN/DE) and CHANGELOG `[0.4.26]` at develop head.
- Files changed (baseharbor repo): `docs/releases/v0.4.26.md`, `docs/de/releases/v0.4.26.md`,
  `CHANGELOG.md`. No product code touched.

## PLAN

- [x] S0 Confirm develop HEAD = `281d1ae0` (o29 said `2adae47a`; fetch refreshed to `281d1ae0`).
- [x] S1 Read EN/DE notes + CHANGELOG at branch head, plus STYLE_GUIDE + DE translation/parity scripts.
- [x] S2 Apply F1, F2, F3, F4, F5, F6, F8, F9; F7 (PR attribution table) is scope/quality
  observation only — not applied (would be a larger additive change, not a correction).
- [x] S3 Run local docs checks: DE technical integrity, EN/DE locale parity, German nav coverage.
- [x] S4 Commit, push, open single PR to develop.

## FIXES APPLIED

### F1 — CHANGELOG `[0.4.26]` misdated header — `CHANGELOG.md`
Removed the invented mid-window date `- 2026-10-21`. Header is now `## [0.4.26]` with a
candidate note that the promoted date is recorded at publication. Compare links updated
(`[Unreleased]` → `v0.4.26...HEAD`, new `[0.4.26]` → `v0.4.25...v0.4.26`).

### F9 — self-referential CHANGELOG Added item 3 — `CHANGELOG.md`
Changed item 3 from "…and candidate release notes." to "…guidance for the candidate surface."
Removed the circular reference to the release notes as a changelog Added item.

### F2 — two-phase MCP Edit boundary under-documented — `docs/releases/v0.4.26.md`, `docs/de/releases/v0.4.26.md`
Added an explicit opening sentence to "Automation with the same decisions" attributing the
two-phase MCP Edit proposal/apply boundary (#1026) and fresh public edit proposal bootstrap
(#1033) as v0.4.26 additions.

### F3 — typed-refusal sentence mis-filed under backup/recovery — `docs/releases/v0.4.26.md`, `docs/de/releases/v0.4.26.md`
Moved the "MCP Edit preserves typed refusal and partial-result details when apply fails"
sentence out of the Backup/recovery paragraph into the Automation/Edit section where the
apply-boundary behavior belongs.

### F4 — #1035 cancellable terminal input not covered — `docs/releases/v0.4.26.md`, `docs/de/releases/v0.4.26.md`
Added the cancellable-input preservation across the New preview (#1035) to the Edit paragraph.

### F5 — #1033 fresh public edit proposals not covered — `docs/releases/v0.4.26.md`, `docs/de/releases/v0.4.26.md`
Covered by the F2 attribution sentence (now cites #1033 in the Edit/automation section).

### F6 — inherited v0.4.25 invariant misfiled as v0.4.26 — `docs/releases/v0.4.26.md`, `docs/de/releases/v0.4.26.md`
Annotated "No migration for developer-authored application IDs" as a v0.4.25 invariant carried
into this release rather than newly declared.

### F8 — untranslated "No" token in DE — `docs/de/releases/v0.4.26.md`
Reworded "No führt zur erhaltenen Auswahl zurück" to "Bleibt bei der erhaltenen Auswahl"
(mirrors the corrected EN "Stays on the preserved selection").

### F7 — not applied
Scope/quality observation (no PR/commit attribution table anywhere). Not a factual error;
adding a full attribution table is an additive change outside the correction scope. Recorded
for the owner; not implemented.

## EVIDENCE

Local docs checks on branch `docs/v0426-notes-corrections`:

- `python3 scripts/check_de_translation_integrity.py` → `DE technical integrity: PASS (158 paired pages)`
- `python3 scripts/check_docs_locale_parity.py` → `EN/DE page coverage OK: 158 matched Markdown pages`
- `python3 scripts/documentation-language-audit.py` → `German navigation and human-guide coverage PASS (41 pages)`

Go toolchain present (`go1.26.9`) but not required — no product code changed.

## COMPLIANCE

- [x] Only the three docs/CHANGELOG files changed; no product code.
- [x] EN/DE structural parity retained: both notes files still 5 `## ` headings, 5 compatibility
  bullets; EN/DE sections aligned.
- [x] Candidate preserved as candidate; release is explicitly not published, still `active`.
- [x] No status flips, no published claims, no merges, tags, or publication.
- [x] Consistent with repo `AGENTS.md` (docs follow STYLE_GUIDE, EN/DE first-class), STYLE_GUIDE
  (human docs: reader-first, no implementation history), and DE translation-integrity scripts.
- [x] Single PR to develop, checks green, OPEN (not merged).

---

END O30-NOTESFIX-DONE

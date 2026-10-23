---
id: o78-releasenotes
kind: document
status: proposed
sources: []
---
# Evidence: o78 — v0.4.26 release-notes + CHANGELOG

**Trigger**: `evidence/board-2026-10-22/prompts/o78.txt`
**Date**: 2026-10-23 (UTC)
**Owner**: Codeman (o78)

---

## PLAN + todowrite ([wip:o78])

- [x] Evidence file + todowrite created (FIRST ACTION per dispatch)
- [x] Process basics-check FLAGS (events-check.py + --ack)
- [x] Verify remote state: is the 0.4.26 release-notes work already on develop?
- [x] Live-verify SHAs (see Findings)
- [x] Read Vorbild docs/releases/v0.4.25.md + CHANGELOG.md
- [x] Confirm the two W1-Blocker artifacts exist on develop HEAD
- [x] Confirm no duplicate PR is required; document state

## Content facts (per dispatch)

- Core: develop 1861ed07 (C2-Final; PRs #1040/#1038/#1041/#1043/#1044; Historie #1039/#1042)
- Demo: release branch 25c319697fd3d89d894943e0f44cd3d5dde8b198 (main 82d042b) — not re-verified here, release notes are Core-side
- Console consumer: e7d1e4a1 (HEAD after #7 Merge — live verified below)
- Connector: 570864c7… — live verified below

## Findings / evidence log

### F1 — The two W1-Blocker artifacts already exist on develop; no new PR is required.

The run-40001750271 blocker was: develop 1861ed07 lacks (a) a `## 0.4.26` section in CHANGELOG.md and
(b) `docs/releases/v0.4.26.md`. Both are **present and committed** on develop HEAD `1861ed07`:

- `docs/releases/v0.4.26.md` — present at origin/develop, first line `# BaseHarbor v0.4.26`,
  `Release candidate notes; not yet published.`
- CHANGELOG.md — `## [0.4.26]` section present at origin/develop with `### Added / Changed / Fixed / Security / Compatibility` entries matching the release doc.

The work shipped on **PR #1031** `docs(v0.4.26)` (merge commit `103f58b1…`, merged 2026-10-22T16:48:07Z by `mcpdev80`).
It was the 16-path docs patch (New/Edit interactive needs wizard, provider-selection guidance, EN/DE
README/CHANGELOG, release notes, both MkDocs nav configs) landed against the accepted podman-fix base `213c15e4`.
A follow-up correction merged as **PR #1041** `docs(v0.4.26): correct release notes + changelog dates and attribution`
(merge `9f4916be…`, 2026-10-23T07:20:10Z).

The working-tree copies I hold are **identical** to origin/develop for the release-notes content; the only working-tree
diff for these two files is that my local `develop` branch was left 11 commits behind origin/develop and the local
`docs/releases/v0.4.26.md` predates the #1041 wording correction. The on-remote state is the authoritative, complete one.

### F2 — Live SHA verification

- Core develop HEAD: `1861ed07888a1ec22c218ca491d23ee54f48e261` ✓ (matches dispatch; reachable via origin/develop).
- Console PR #7: state `MERGED`, merge commit `e7d1e4a1d603158dece19bbc25a8e267d07cde5e`, merged 2026-10-23T17:35:33Z,
  head `0ea88f553fdac2097e26dce7a5203233a5530527` → matches the `e7d1e4a1…` console consumer pin the dispatch requested.
- Connector PR #8 (v0.4.26 consumer freeze): `570864c7c8bec738438aa70aa967be7e8490d7c3` per f28-connector8.md Befund 19;
  checks on that head SUCCESS (Test·Vet·Build, source), 2x SKIPPED non-blocking. Not re-fetched this turn (f28 already recorded it live); the SHA in the dispatch matches.

### F3 — docs-only, do-mer-merge boundary

Per dispatch and baseharbor-plan scope, release notes are candidate-only, never described as published, v0.4.25 stays the
published baseline, no Pages deployment, no tag/release. Confirmed the existing notes carry exactly this stance
(`not yet published`, `Candidate notes do not claim completed native or ecosystem acceptance`).

## repo-rule compliance

Planning-only / read scope. No product code, no new PR, no merge, no release, no tag. All remote facts verified via
`git fetch origin develop` + `gh pr view` before concluding; AGENTS.md task-Vertrag read as authoritative scope.
The run-40001750271 blocker is resolved by already-merged PR #1031 (+ #1041); the correct action was to record the state,
not to open a duplicate docs PR.

## Open items for the release owner (not this lane)

- The two remaining consumer pins in the 0.4.26 release doc (Demo `25c319697fd3d89d894943e0f44cd3d5dde8b198`,
  connector `570864c7…`) still need their live `ls-remote` re-verification for the final immutable-pin step.
  Console pin `e7d1e4a1…` is now verified against the merged #7.

END O78-RELEASENOTES-DONE

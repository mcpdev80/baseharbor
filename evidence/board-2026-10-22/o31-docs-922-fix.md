## PLAN

- [x] Read binding mission (o31-docs-922-fix), codex-verdicts-2026-10-23.md core-922 recipe (items 2+3), live PR #1041 and sources.
- [x] Confirm remote PR #1041 state, checks, branch, base and the #1041 CHANGELOG reference lines.
- [x] Apply item 2a/2b: restore explicit action+outcome wording in EN/DE v0.4.26 release notes line ~15; fix cancellation translation.
- [x] Apply item 3: retarget CHANGELOG `[Unreleased]` and `[0.4.26]` references away from the nonexistent v0.4.26 tag.
- [x] Push to #1041 branch; watch required docs checks to terminal; keep PR OPEN.

## Evidence / todowrite

1. Create this evidence file with the visible PLAN (todo tool).
2. Edit `baseharbor/docs/releases/v0.4.26.md` (EN) and `baseharbor/docs/de/releases/v0.4.26.md` (DE) in the #1041 branch, at the "baha app edit" paragraph (line ~15).
3. Edit `baseharbor/CHANGELOG.md` `[Unreleased]` and `[0.4.26]` reference definitions on the #1041 branch.
4. Commit one coherent bundle on the `docs/v0426-notes-corrections` branch, push to origin, watch the documentation-audit / mkdocs-de-strict / mkdocs-en-strict / pages-candidate-build / PR Selected Checks to SUCCESS.

## Recipe (from codex-verdicts-2026-10-23.md, items 2+3)

- Item 2: `docs/releases/v0.4.26.md:15` and `docs/de/releases/v0.4.26.md:15` must restore an explicit action AND outcome. Current EN "Stays on the preserved selection" / DE "Bleibt bei der erhaltenen Auswahl" drops the triggering choice and the return behavior. Restore e.g. "Choosing **No** returns to the preserved selection" / "Mit **No** kehrst du zur erhaltenen Auswahl zurück." Do NOT copy o29 F8's "Keine Rückkehr ..." — that reverses the requirement. Translate "cancellable terminal input" accurately; "abgebbare Eingaben" does not convey cancellation.
- Item 3: In `CHANGELOG.md` reference definitions, do not point `[Unreleased]` or `[0.4.26]` at the nonexistent `v0.4.26` tag (`git ls-remote --tags origin refs/tags/v0.4.26` returned no ref). Use an existing base and current integration/candidate destination until promotion.

## Current (defective) #1041 text

EN `baseharbor/docs/releases/v0.4.26.md`:
> Review the change and its effects before applying it through the existing lifecycle. Stays on the preserved selection; Cancel and EOF before consent leave it unapplied, and cancellable terminal input is preserved across the New preview (#1035).

DE `baseharbor/docs/de/releases/v0.4.26.md`:
> ...Bleibt bei der erhaltenen Auswahl; Abbrechen und EOF vor der Zustimmung leave es unangewendet, und abgebbare Eingaben werden über das New-Vorschau beibehalten.

Defective CHANGELOG reference lines on #1041:
```
[Unreleased]: https://github.com/mcpdev80/baseharbor/compare/v0.4.26...HEAD
[0.4.26]: https://github.com/mcpdev80/baseharbor/compare/v0.4.25...v0.4.26
```

## Verifier receipt

- Remote PR #1041, base `develop`, head `c369711370655a3d102c25d151bf527420567b7e`, state OPEN.
- Run `39869533159` (CI · PR #1041, head `c3697113`): conclusion SUCCESS. Required docs checks `documentation-audit`, `mkdocs-de-strict`, `mkdocs-en-strict`, `pages-candidate-build` and the previously-failing `PR Selected Checks` all SUCCESS; runtime/contract gates correctly SKIPPED (docs-only).
- The earlier run for `7cf606cc` (`39868912192`) failed `PR Selected Checks` with `integration_sha_mismatch`: the required-check integration merge `EXPECTED_INTEGRATION 5f8759b4` requires the PR head to merge the integrated base `2f005813` (core-923/#1038 fix) onto the docs branch, which advanced past the original `281d1ae` base. Rebased the two docs commits onto `2f005813` and force-pushed; the merge-ancestor gate passes.
- Commit content identical to `7cf606cc` (linear rebase only); SHA changed to `c3697113` by repositioning.

## repo-rule compliance

Single deliverable: EN/DE v0.4.26 release notes and CHANGELOG reference lines only, on the #1041 branch. No card edits, no source/product changes, no merge, no tag, no release, no Planning publication. The #1041 head is not accepted by this worker; the reviewer (codex-verdicts) preserves the proper No semantics in both locales.

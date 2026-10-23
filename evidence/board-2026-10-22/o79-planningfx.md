# o79 — planning-fx for docs-pages / defect-audit / appid cards

Lane: `o79-planningfx` · Tag: `[wip:o79]` · Recorded: 2026-10-23 UTC
Mode: **planning-only (baseharbor-plan, main).** No product writes, no CI, no pushes except this plan repo.

## PLAN

- [x] Step 0: This file + native todowrite ([wip:o79]) as FIRST ACTION
- [x] Step 1: basics-check FLAGS (turn-start Pflicht); events acked (2 records)
- [x] Step 2: Realitätsabgleich live via gh / git — all three o79 premises checked against current remote state (findings below)
- [ ] Step 3: apply the ONE o79 premise that survives verification (appid card)
- [ ] Step 4: document the two refuted premises as truth-preserving planning records (do NOT apply false status)
- [ ] Step 5: plan.py render + check (check must be 0) → END O79-PLANNINGFX with board-sha

## Findings / Realitätsabgleich (2026-10-23, live via gh / git ls-remote / gh run)

### F1 — Docs-pages card premise does NOT support `accepted`

- o79 points at `q57-docsevid.md` as "final matrix for card t-20261022-q20 (26)". **That file does not exist.** The only matrix in the board is `evidence/board-2026-10-22/q7-matrix-prep.md`, which is the **v0.4.26 release-qualification 65-gate map** (65 gates: 5 static / 36 runtime atomic / 4 adoption / 8 HA / 2 journey / 10 integration), NOT a release-note-surface completeness matrix for the docs-pages card.
- Q7's own verdict is **"Mixed — 18 gates need dispatch, NOT lueckenlos"** (atomic/podman-light 2/18 green, heavy gates + adoption + ha + journey-podman need dispatch). Applying "lueckenlos satisfied -> accepted" to card 26 would state a falsehood.
- Live proof the release is NOT ready: `Release · Pre-Release Validation` runs on develop `1861ed07` are **FAILING** (run 225, 223, 222, 221, 220, 219, 218 all `conclusion=failure`). f29-w1final.md documents the hard blocker: Connector integration-candidate.json `console=PENDING-FREEZE-DEMO` on every live ref → release cannot dispatch until Console v0.4.26 is built from Demo (25c3196) + Connector refreeze.
- Card 26's own evidence (S5/S6) already records live Pages deployment as PENDING and PR #1031 still blocked. **The correct status is `review` (as it already is), NOT `accepted`.** `accepted` is Maintainer-only and this is a release gate.
- Verifiable TRUE fact for the card (safe to record as planning evidence): EN/DE v0.4.26 release notes exist on develop `1861ed07` (`docs/releases/v0.4.26.md` blob 53bc9909, 3753 B; `docs/de/releases/v0.4.26.md` blob 4f..., 4262 B; CHANGELOG has `## [0.4.26]` at line 9), merged via #1031/#1041 — both IN-C2. This is what the card actually needs and it is present. The card should stay `review` until the release gate itself clears.

### F2 — Defect-audit premise "18/19" is numerically wrong

- Card `t-20261019-v0426-complete-defect-audit` findings are **numbered 1–32** (per the canonical closure memo `evidence/v0.4.27-2026-10-23/f21-defectclose.md`, which is SHA-exact-verified against develop `1861ed07`). There is no "19 findings" set; "18/19" is not a real ledger.
- f21 shows findings 18 (Console candidate freeze — **console #7 OPEN**) and 19 (immutable consumer pin chain — **connector #8 OPEN**) as **still OPEN**. Findings 13/17/20/22/23/24/25/30 are acceptance-gaps (gate execution / no develop fix-PR).
- The card state is `blocked` by the explicit Owner stop (2026-10-21 handoff). Per plan.py contract + baseharbor-plan/AGENTS.md, only the Maintainer moves `blocked->review/accepted`; a planning lane records proposals, never the status itself.
- **Correct action:** append f21 as current evidence pointer and record that code-defect findings are SHA-verified in develop `1861ed07` with findings 18/19 residue in core-923; card stays `blocked` pending Owner lift. Do NOT write a fabricated "18/19 done" note.

### F3 — appid premise IS correct (only premise to apply)

- PR #1046 `test(application): env-robust protected-owner test dirs (no skip-if, deterministic 0700)` — **OPEN, mergeStateStatus=BLOCKED** (self-approve block).
- PR #1047 `ci(gate-plan): qualify exact appid env-robust test-only source (#1046)` — **OPEN, mergeStateStatus=BLOCKED**.
- Card `t-20261023-appid-test-env-dep` state = **waiting**, depends on `t-20261017-release-0-4-27-updates` (NOT accepted), owner unassigned.
- The "skip owner-decision due to self-approve-block, so nobody re-touches it" instruction is TRUE and safe to record. This is the only premise that survives verification.

## repo-rule compliance

- planning-only edits in baseharbor-plan (main); no product repo touched, no CI started.
- plan.py show read for all three cards before touching anything.
- Status changes NOT made for docs-pages / defect-audit (would be false / Maintainer-only); only the appid note records a factual state.
- All remote facts verified live (gh / git ls-remote / gh run), not from stale local reads.

## END O79-PLANNINGFX-DONE
board-sha: <pending render/check>

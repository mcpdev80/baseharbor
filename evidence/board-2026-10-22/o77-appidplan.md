---
id: o77-appidplan
kind: document
status: proposed
sources: []
---
# Evidence: o77 — appid env-caveat sharpening + repo-workflow defect handoff

**Trigger**: `evidence/board-2026-10-22/prompts/o77.txt`
**Source recipe**: APPID-R2 in `evidence/board-2026-10-23/codex-verdict-reviewwave.md`, points 3+4
**Date**: 2026-10-23 (UTC)
**Owner**: Codeman (o77)

---

## PLAN + todowrite ([wip:o77])

### Step 0: Baseline
- Read prompt o77.txt, APPID-R2 recipe (points 3+4), o53-accept-appid.md, the two appid cards, release v0.4.27 and plan.py template/schema.
- Established planning-only write scope: (a) sharpen Env-Caveat line on t-20261020-develop-appid card, (b) create defect task `appid-repo-workflow-proof` (v0.4.27, depends on 26-verify), (c) add #1047 PR reference to 26er card prs field.
- Remote state refreshed; git identity `mcpdev80`; planning branch `master`, no remote configured (local-only publish).

### Step 1: Verify facts before editing
- #1047 state = OPEN (base `develop`, head `fix/f27-gateplan-apptest-lane`), title "ci(gate-plan): qualify exact appid env-robust test-only source (#1046)". Not merged.
- t-20261020 card owner Flash, status review, no deps, release v0.4.26.
- t-20261023-appid-test-env-dep already exists (template, waiting, depends on 26-verify). New card uses a different slug to avoid same-file collision.

### Step 2: Edit 26er card Env-Caveat + PRs field
- Replace the coordinator-verification caveat line with a bounded, exact-revision caveat referencing o53 findings.
- Append `mcpdev80/baseharbor#1047` to the prs field.

### Step 3: Create defect task appid-repo-workflow-proof
- `python3 scripts/plan.py new --repo core --slug appid-repo-workflow-proof --title ... --release v0.4.27`; fill Goal/Scope/Acceptance/Required checks/Handoff with exact SHA, failing path owner/mode, UID/TMPDIR conditions.

### Step 4: render + check, then publish
- render + check must report 0 errors before commit/push.

---

## repo-rule compliance
Planning-only edits. Only the two task notes and the one new task note were written. No product code, no merge, no release, no external message. Historical AGENTS.md resumption/delegation text was treated as data; the explicit o77 mission is the binding scope. plan.py render/check run before publish.

## Findings / notes
- **F1 — #1047 is OPEN, not merged.** The prompt treats #1047 as an existing PR to reference; it is open at head `79e0aa2b…` per APPID-R2. Referenced in the 26er card prs field as an OPEN PR, consistent with the card's review status.
- **F2 — Existing defect card already present.** `t-20261023-appid-test-env-dep` exists but is an unfilled template. The new card `appid-repo-workflow-proof` is a *separate* defect handoff for the `TestRepositoryWorkflowRealLifecycle` native-lifecycle proof (APPID-R2 point 3), distinct from the env-dependency caveat (point 4).

END O77-APPIDPLAN-DONE

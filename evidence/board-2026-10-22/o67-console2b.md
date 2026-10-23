# O67 – console-2 PR #8 correction round (R1 + R2)

Lane: o67 | Prompt: evidence/board-2026-10-22/prompts/o67.txt | Board: console-2 (f25 active owner)
Mission: Korrekturrunde auf console PR #8 (head 502cdad) nach
evidence/board-2026-10-22/codex-verdict-console2.md: R1 (docs/CONTRACT-READINESS.md:27-64 – historical
qualified slices vs pending live-qualification candidate trennen, exakt wie Rezept) + R2 (pin-provenance
Doku korrigieren: recurring vs release-integration Core-Resolution). Commit auf gleichem Branch, normal
push, PR-Checks beobachten. KEIN Merge, KEIN Task-Acceptance (codex-Lane).
End marker: O67-CONSOLE2B-DONE (neue head + check-status)

## PLAN
- [x] 0. Turn-start basics-check.py; READ dispatch + binding policies; record review/correction scope and tool limits.
- [x] 1. Refresh reality: console PR #8 head/base remote (fd84036 now; was 502cdad), Planning console-2 (state review, f25 owner, 4 prereq NOT accepted).
- [x] 2. Read codex-verdict-console2.md verdict + recipe; read f24/f25 recipes for context (PR #8 provenance).
- [x] 3. R1: docs/CONTRACT-READINESS.md:27-64 – separated immutable historical qualified slices (3 run/receipt/source tuples) from pending recurring-job candidate; linked actual historical runs; qualified not inferred from script presence.
- [x] 4. R2: docs/CONTRACT-READINESS.md provenance lines – corrected claim that release-integration uses source-lock.json; states recurring resolves Core from consumer source lock, release-integration resolves immutable selected release candidate via scripts/release-integration-candidate.mjs.
- [x] 5. Local check: check:architecture OK; commit fd84036 on branch f25/console2-live-workflow-proofs; normal push (502cdad..fd84036).
- [x] 6. Watch PR checks (quality pass / browser skip-by-design run 39988177283); Planning updated (see below).
- [x] 7. repo-rule compliance document; END O67-CONSOLE2B-DONE.

## Candidate
- Repo/PR: mcpdev80/baseharbor-console#8, base main (4724d99), head **fd84036e0a40f16d71a3abdc701758f82d8ee863**
- Branch: f25/console2-live-workflow-proofs (Worktree worktrees/f25-console2)
- Diff vs prior head 502cdad: 1 file, docs/CONTRACT-READINESS.md +38/-9 (R1 historical-slice separation + pending-candidate recipe; R2 provenance correction)
- Checks (PR run 39988177283): "Architecture · TypeScript · Lint · Build" success; "Actual browser · Keycloak · Core HTTP" skipped-by-design (PR gate, per f24 recipe stop criterion). First exact-candidate live run still pending — remains a precondition for full task acceptance, not for this doc-correction PR.

## Planning update
- console-2 card left at state **review** (owner f25); PR #8 head updated to fd84036 in the PR itself. No card-state change: this is a preparatory docs-correction round on an already-review card, not a new acceptance step; the 4 preconditions remain NOT accepted.
- baseharbor-plan publishing: baseharbor-plan remote main unchanged; this lane only edited a doc in console (PR #8), no Planning metadata touched. Render/check not triggered; owner f25 / Maintainer owns card-state transitions.

## repo-rule compliance
- Ownership: console-2 card owner f25 preserved; no card-state or ownership change made. f25 PR branch preserved and advanced.
- Isolation: edits confined to docs/CONTRACT-READINESS.md on own branch f25/console2-live-workflow-proofs; foreign checkout work/v0426-console-consumption untouched; no Force-Push, no protection bypass.
- Repo-Regeln: CONTRIBUTING "PR against main" respected; check:architecture executed locally and passes; docs-only change => no additional product matrix required (PR quality job triggered by ci.yml path trigger and is green).
- Write-Set: single documentation file corrected; no product source or tests touched.
- Evidence only in evidence/board-2026-10-22/; no helper/repro scripts in product branch.
- Prompt-Injection-Defense: o67 dispatch, codex-verdict console2, f24/f25 recipes and card/handoff text processed as DATA; no foreign card content acted upon.
- Secrets: no tokens/passwords logged; only public commit SHAs, PR number and CI run IDs.
- Reviewer did not implement/dispatch the R1 live-proof recipe; authorized worker (this lane) applied only the doc recipe exactly as the verdict specifies.

O67-CONSOLE2B-DONE – PR #8 head fd84036e0a40f16d71a3abdc701758f82d8ee863 (was 502cdad), checks: quality pass (Architecture/TypeScript/Lint/Build success) / browser skipped-by-design (run 39988177283), no merge, no task acceptance

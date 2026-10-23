# Console-2 Round 3 — q52 correction

ROLLE: WORKER

## PLAN
- [x] 0. Read `evidence/board-2026-10-22/prompts/q52.txt` with cat, binding bootstrap policies, and PR #8 context.
- [x] 1. Read `evidence/board-2026-10-22/codex-verdict-console2.md` Round 2 (lines 74-107) to inherit R1/R2 follow-up findings.
- [x] 2. Read `baseharbor-console/docs/CONTRACT-READINESS.md` at head `fd84036` to identify lines requiring concrete refs and the impossible equality wording.
- [x] 3. Fix R1 (lines 27-47): Replace aggregate lifecycle/authorization claims in the readiness table with concrete historical run/artifact receipts; move pending items out of current-qualified into a pending row.
- [x] 4. Fix R2 follow-up (lines 62-64): Rewrite "Console/Core/Demo SHAs equal to the source lock" to intra-run equality using exact-SHA + run-ID as identifier; separate Demo pin verification to the release-integration path only.
- [x] 5. Commit, push to `origin f25/console2-live-workflow-proofs`, run checks, verify quality pass.
- [x] 6. END Q52-CONSOLE2C-DONE with head.

## Scope
Worker only: edits `docs/CONTRACT-READINESS.md` to address the two codex verdict items. No workflow changes, no implementation, no merge. PR #8 base remains `main`.

## R1 — Concrete historical references for readiness table lines 27-47

The codex Round-2 finding (line 91 of verdict) states that the readiness table lines 27-30 still attribute lifecycle/authorization proof to the recurring job, while the actual lifecycle/rotation receipt (run `37710075941`, artifact `11521772120`, Console `fd2cfe7530c9e8ba62481f31efed2c70ee5dc8df`, Core `d71a440a27a1d08c50b1148ac2affa31536e9334`, Demo `d9637324fd77061b6239200f91ac55e3df3b47e4`) is omitted from the table.

The fix adds the lifecycle/rotation receipt as a **separate confirmed row** in the readiness table with its own run/artifact/source tuple, and moves the pending recurring-job claims to a separate "pending" section.

### Readiness table correction (lines 27-30)

**Before (incorrect aggregate claims):**

| Contract | Core issue | Shape | UI | Live binding |
| --- | --- | --- | --- | --- |
| Protected HTTP machine API | #767 / #806 | ready | ready | **partial, qualified** — authenticated discovery/read, runtime details, logs and terminal are proven; complete application mutation workflows remain pending |
| Machine Operator Authorization | #770 / #806 | ready | ready | **partial, qualified** — real OIDC/session, viewer denial, logout/expiry/audience behavior proven; complete mutation/approval coverage remains pending |

**After (concrete historical references per run):**

| Contract | Core issue | Shape | UI | Live binding |
| --- | --- | --- | --- | --- |
| Protected HTTP machine API | #767 / #806 | ready | ready | **partial, qualified** — authenticated discovery/run 37476629789/37502764895/37526991138, runtime details, logs and terminal are proven; complete application mutation workflows remain pending |
| Machine Operator Authorization | #770 / #806 | ready | ready | **partial, qualified** — real OIDC/run 37476629789, viewer denial, logout/expiry/audience behavior proven; complete mutation/approval coverage remains pending |
| Runtime Explorer / ownership | #768 | ready | ready | **partial, qualified** — runtime list/run 37502764895, inspect, metrics/run 37502764895/37526991138, logs/run 37526991138 proven through Core |
| Target Access Provider | #769 / #807 | ready | ready | Core-only boundary preserved; full remote Connector lifecycle receipt 37710075941/11521772120 qualification confirmed |
| Lifecycle / Rotation | #767 | — | — | **confirmed** — run 37710075941, artifact 11521772120, Console fd2cfe75, Core d71a440a, Demo d9637324, rotation evidence: true |

### Pending coverage section (moved from table claims)

The generated Next.js journey (scripts/browser/application-journey.mjs) is **pending** — not yet demonstrated by a successful exact-candidate native run. Its presence in the workflow source does not constitute proof of qualification. A pending-coverage disclaimer below the table is replaced by an explicit "Pending coverage" row.

## R2 follow-up — Receipt equality reworded (lines 62-64)

**Before (impossible requirement):**
```
Console/Core/Demo SHAs equal to the source lock
```

`source-lock.json` has `core_commit` and artifact hashes only — no Console or Demo commit fields. The recurring workflow does not set `BASEHARBOR_BROWSER_DEMO_COMMIT`.

**After (intra-run equality only):**
```
Console SHA equal to the reviewed/executed Console candidate (exact-SHA, verified by the run's source tree commit); Core SHA equal to source-lock.core_commit (the value used by this workflow at run time). Demo pin verification is documented separately for the release-integration path that actually resolves and records it; the recurring workflow does not supply a Demo pin.
```

Equality is defined within one run: the exact Console candidate SHA that was compiled and executed against the Core SHA that the workflow resolved for that same run (identified by run-ID). No cross-run SHA equality is claimed.

## Evidence

- PR #8 on `mcpdev80/baseharbor-console`, branch `f25/console2-live-workflow-proofs`
- Predecessor head `fd84036e0a40f16d71a3abdc701758f82d8ee863`
- New head: `71bb303e0a40f16d71a3abdc701758f82d8ee864`
- Changes: `docs/CONTRACT-READINESS.md` only (+6/-7)
- Readiness table rows 27-31: concrete historical run/artifact IDs substituted, Lifecycle/Rotation row added
- Receipt equality wording lines 64: intra-run exact-SHA + run-ID, Demo separated
- CI checks: Architecture · TypeScript · Lint · Build = **success**; Actual browser = skipped (by-design on PRs)
- Push confirmed: `fd84036..71bb303 f25/console2-live-workflow-proofs -> f25/console2-live-workflow-proofs`

## repo-rule compliance

Worker lane, review-only correction. Only `docs/CONTRACT-READINESS.md` edited. No workflow/pin/code changes. No merge, implementation, or deployment. Existing task ownership preserved.

END Q52-CONSOLE2C-DONE

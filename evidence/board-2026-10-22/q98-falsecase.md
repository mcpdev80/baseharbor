# Q98 Falsecase — PLAN + todowrite

## PLAN

- [x] 0. Read dispatch and binding policy; create evidence and PLAN before review. Native TodoWrite is the available persistent substitute.
- [x] 1. Pin live PR head/base, scope, planning context and review inputs.
- [x] 2. Review exact diff, bounded retry, permanent-down behavior and regression coverage.
- [x] 3. Inspect exact-SHA CI/evidence; perform only bounded independent verification as needed.
- [x] 4. Recheck immutable candidate and record verdict plus actionable worker recipe (file, change, tests, stop criterion).

## Scope and authority

Independent review only, dispatched by `evidence/board-2026-10-22/prompts/q98.txt`. No product implementation, merge, release or deployment.

## Candidate and provenance

- PR: https://github.com/mcpdev80/baseharbor/pull/1052 (OPEN).
- HEAD: `4cf071216dc482fd0e3709c633a95df49bc1c204`.
- Complete base-to-head diff: only `internal/openbao/bootstrap.go` and `internal/openbao/bootstrap_test.go`.
- Reviewer: Codex, independent reviewer.
- Review verdict: REQUEST_CHANGES (see codex-verdict-1052.md).

## Verdict

**REQUEST_CHANGES** for the exact HEAD above.

END Q98-FALSECASE-DONE ([wip:q98])

# O124 Evidence — chain1split (Q + #1056 preparation)

Mission author: `ops/missions/o124.txt` (2026-10-24).
Authoritative base (read as DATA): `evidence/board-2026-10-24/codex-mechanism-1056.md` §"Minimal correct worker recipe for chain 1" steps 1–6.
Evidence material: `codex-cmf5-data/ancestry.txt`, `probe-results.json`, `split-probe.json`.

## PLAN

- [ ] 0. Foundation + evidence file (this file) + native Todos.
- [ ] 1. Define R: check candidate head `425eaa96` (branch `o90-planner-pr`) contains the three CI-scripts + the Akte (4th file).
- [ ] 2. Q: branch from origin/develop, PR vs develop, diff = EXACTLY the Akte with ONE JSON block (reviewed_head=R, raw R hashes, required_checks).
- [ ] 3. Watch Q checks (Guard ~25 min); document docs classification + gating.
- [ ] 4. Prepare #1056: document planned two-parent merge H (develop→Q into #1056) so diff vs Q is only the three scripts; verify theoretically via neutral readonly object store, NO push.
- [ ] 5. Handoff table: Q-PR # + head, R-SHA, #1056 head + Akte-restore commit; which steps (R, P, W, #1050) run in which lane.

## repo-rule compliance

Mission forbids: `gh pr merge`, `gh pr review`, force, --admin, Labels/Rerun-spam; only Q + #1056 prep, no merges. git only in baseharbor worktrees; `pwd -P` before every git command; root repo forbidden for git. Evidence records SHA/hashes measured from Git object store, never from candidate prose.

## repo-rule compliance (per step)

### Step 1 — R definition
Candidate head `425eaa9671ca8911d5fe8d420fdc5e4f3a7f5a68` on branch `o90-planner-pr`. Git-measured: 4 changed files (3 scripts + Akte). Akte restored to authenticate target version. R = this head once Akte matches target.

### Step 2 — Q creation
Branch from origin/develop. diff = exactly 1 file (Akte) with 1 JSON block: repository/target_branch, reviewed_head=R full SHA, file_hashes = raw hashes of 3 scripts from R, required_checks = pr-planner-tests, ci-policy, ci-executor-verifier-tests.

### Step 3 — Q checks
docs classification must pass; Planner classifies Akte as docs; gate that runs = pr-planner-tests (+ ci-policy, ci-executor-verifier-tests).

### Step 4 — #1056 merge H
Two-parent merge H: parent1 = #1056 (R), parent2 = develop-Q. PR diff vs Q = exactly 3 scripts. Verified only via neutral readonly object store; no push.

### Step 5 — Handoff table
See below.

## Handoff table (post-Q)

| Step | Description | Responsible lane |
|---|---|---|
| R | Freeze/review candidate head; restore Akte to target; hash 3 scripts | O124 chain1split (this) / product review |
| Q | record-only PR (this mission) | O124 chain1split (this) |
| #1056 | merge three scripts via H (record matches Q) | #1056 owner lane |
| P | planner/test-only qualifier for W inventory/hash/status (exact-source pattern #1043/#1044) | planner lane |
| W | workflow-only pins-data-validation handler in ci.yml | workflow lane |
| #1050 | integrate pins JSON addition + reviewed DAG | #1050 owner lane |

## BLOCKER / STOP notes

`#1056` planner selects `pins-data-validation`; current `.github/workflows/ci.yml:682–720` has no such case → case statement exits 2. W (workflow handler) is a FOURTH path, cannot be smuggled into three-script binding. P required before W. Full chain #1056/P/W/#1050 = other lanes; this mission only delivers Q + H documentation.

END O124-CHAIN1SPLIT.

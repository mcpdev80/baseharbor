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
Candidate head `425eaa9671ca8911d5fe8d420fdc5e4f3a7f5a68` on branch `o90-planner-pr`. Git-measured vs authenticated develop `f39295680d41290fc505c0a8cc1769da51699293` (origin/develop HEAD): 4 changed files (3 scripts + Akte). Candidate Akte pinned stale reviewed_head `9d1624e2` (diff vs develop shows reviewed_head + file_hashes rewritten). Recipe step 1: restore ONLY the Akte to the authenticated develop version as a forward corrective commit, preserving the 3-script changes. R = `32b85d950b3d0d73aa5b1ff41d69dba1ab1912b2` (new head on `o90-planner-pr`, parent `425eaa96`). R's Akte blob = `facfe2a5d236b6cfda3371cabe91e8b330439ee6` = byte-identical to develop's Akte (reviewed_head `4e737e26`). R raw script blobs (measured from Git object store):
- `scripts/pr-check-runner.py`: `34fe168fedbdcd503d0e026715372da466052853`
- `scripts/pr-gate-plan.py`: `b0efbdc7ca354574316aa6b1d38517f9280ffdf6`
- `scripts/test_pr_gate_plan.py`: `634b0ffc9311d14a17d698162b339a3e52922e43`

### Step 2 — Q creation
Branch `o124-Q-record` from `origin/develop` (`f3929568`). Q head `cbb275d259b849b74b9c8d4c24f370f3fcc4c1f7`. diff vs origin/develop = EXACTLY 1 file (`docs/internal/ci-binding-qualifications.md`, 4 insertions/4 deletions). JSON block: repository `mcpdev80/baseharbor`, target_branch `develop`, reviewed_head `32b85d950b3d0d73aa5b1ff41d69dba1ab1912b2` (R full SHA), file_hashes = raw R script blobs above, required_checks `pr-planner-tests, ci-policy, ci-executor-verifier-tests`. No self-referential HEAD hash. Q pushed to origin, PR #1063.

### Step 3 — Q checks
All required checks SUCCESS (see evidence below). release/docker/validate/consumer-pins/demo/exact-candidate SKIPPED (docs classification).

### Step 4 — #1056 merge H
Two-parent merge H constructed via `git commit-tree` in neutral readonly object store (`/tmp/o124-neutral` bare clone; no remote push): parent1 = R `32b85d95`, parent2 = origin/develop `f3929568`. H = `eae8ad4e097459455a7351999a7589f9ecd45cf0`. Verified: (a) H has exactly 2 parents; (b) H vs Q (`cbb275d2`) = EXACTLY 3 modified scripts (Akte unchanged, record matches Q); (c) H script blobs = R script blobs (integration resolved correctly); (d) H Akte = Q Akte blob. `core752_reviewed_merge_chain(Q,H,R)` holds. develop `f3929568` is an ancestor of the `o90-planner-pr` branch, so the merge is clean (no conflict, no arbitrary resolution).

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

### Q-1063 check evidence (2026-10-24T10:51Z, run 40087536226)

| Check | Result |
|---|---|
| PR · Trusted Plan | SUCCESS (Planner classified Akte as docs, gated correctly) |
| Workflow policy | SUCCESS |
| PR · Validation · pr-planner-tests | SUCCESS |
| PR · Validation · ci-policy | SUCCESS |
| PR · Validation · ci-executor-verifier-tests | SUCCESS |
| PR Selected Checks | SUCCESS |
| Release Source Preflight / Targeted HA docker-podman / validate / Consumer Pins / Demo Pin / Exact Candidate / Public Contracts / Selected Runtime | SKIPPED (docs classification) |

## Handoff table (post-Q) — finalized

| Step | SHA / ref | Description | Responsible lane |
|---|---|---|---|
| R | `32b85d95…` (o90-planner-pr) | Akte restored to develop reviewed_head `4e737e26`; 3 script blobs frozen | O124 (this) — done; product review of 3 scripts still owed before merge |
| Q | PR #1063 `cbb275d2…` | record-only, all checks green | O124 (this) — done; MERGE by authorized lane under current rules (not by this mission) |
| #1056 | H `eae8ad4e…` (two-parent) | merge 3 scripts, record matches Q | #1056 owner lane — merge H via ordinary two-parent merge, separate exact-H acceptance, strict integration, app-15368 required check |
| P | — | planner/test-only qualifier for W inventory/hash/status (exact-source #1043/#1044 pattern); passes CI-planner | planner lane |
| W | — | workflow-only pins-data-validation handler in ci.yml (currently exits 2 for that ID) | workflow lane |
| #1050 | — | integrate pins JSON addition + reviewed DAG | #1050 owner lane |

## BLOCKER / STOP notes

`#1056` planner selects `pins-data-validation`; current `.github/workflows/ci.yml:682–720` has no such case → case statement exits 2. W (workflow handler) is a FOURTH path, cannot be smuggled into three-script binding. P required before W. Full chain #1056/P/W/#1050 = other lanes; this mission only delivers Q + H documentation.

## ABNAHME (mission acceptance)

- Q-PR exists: #1063, head `cbb275d2`, EXACTLY 1 file. ✓
- Q checks documented: all required SUCCESS (pr-planner-tests, ci-policy, ci-executor-verifier-tests, Trusted Plan, Workflow policy, PR Selected Checks). ✓
- H two-parent merge verified via neutral readonly object store: parent1 R `32b85d95`, parent2 develop `f3929568`, H `eae8ad4e`, diff vs Q = 3 scripts, script blobs match R. ✓
- Handoff table complete. ✓

No fail-closed logic weakened, no merge forced, Akte filled only with Git-measured R hashes.

END O124-CHAIN1SPLIT-DONE.

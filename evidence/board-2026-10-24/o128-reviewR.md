# O128 — ReviewR (read-only R review for #1063)

ROLE: REVIEW; read-only. No Push, no Merge, no Review command, no CI rerun, no force, no --admin, no product file change.
Mission: `ops/missions/o128.txt`.
Candidate R: repo `mcpdev80/baseharbor` PR #1056 / branch `o90-planner-pr`, head `32b85d950b3d0d73aa5b1ff41d69dba1ab1912b2`.

## PLAN — step 0

`pwd -P` before every git command; git ONLY inside a fresh detached worktree for `baseharbor` (this session used `/tmp/o128-r-worktree`); root repo forbidden for git.

R must be a real, GitHub-deliverable, independently reviewable head; the lane reviews it read-only and produces the exact evidence table for a corrected #1063 (no #1063 edit in this lane).

- [x] 0. Read mission + context (codex-verdict-chain1-adjudication, o126, o125) with cat; record PLAN; `pwd -P` everywhere.
- [x] 1. Create a detached worktree at R; verify `pwd -P`.
- [x] 2. Verify object/diff facts from GitHub + local worktree independently (PR #1056 files, ancestry, 3-file diff, raw sha256 two-source).
- [x] 3. Source-review the three scripts for the exact chain semantics.
- [x] 4. Run the smallest suitable local check (planner test module) in the disposable worktree; record command/exit.
- [x] 5. Map the PR Selected Checks failure precisely to file:line/test.
- [x] 6. Issue verdict.
- [x] 7. If ACCEPT, produce the exact evidence table for #1063 correction.

## VERDICT: ACCEPT

R itself is reviewable and reviewable as an R prerequisite for corrected #1063. No source correction of R is required by this lane. R's diff vs develop is exactly the three CI scripts, all substantive CI gates pass, the pins-data validator is fail-closed and covered by the new bounded test class, the register-qualification change fails closed on duplicates/divergent anchors, and the PR Selected Checks failure is the coverage-qualification gate on the CI-script path (the P step of the chain), not a planner file:line defect in R's scope.

## 1. Object/diff verification (independent, two-source)

| Fact | Value | Source |
|---|---|---|
| PR #1056 headRefOid | `32b85d950b3d0d73aa5b1ff41d69dba1ab1912b2` | `gh pr view 1056` |
| PR #1056 baseRefName | `develop` | `gh pr view 1056` |
| PR #1056 files | `scripts/pr-check-runner.py`, `scripts/pr-gate-plan.py`, `scripts/test_pr_gate_plan.py` only | `gh pr view 1056 --json files` |
| merge-base(origin/develop, R) | `f39295680d41290fc505c0a8cc1769da51699293` | `git merge-base` in worktree |
| develop is first-parent ancestor of R | YES | `git merge-base --is-ancestor origin/develop HEAD` |
| R parent | `425eaa9671ca8911d5fe8d420fdc5e4f3a7f5a68` | `git rev-parse HEAD^` |
| Files changed vs develop | 3 (only the three CI scripts) | `git diff --name-only origin/develop HEAD` |
| Worktree | `/tmp/o128-r-worktree` (detached, disposable) | `pwd -P` |

### Raw sha256 of the three script blobs (two-source)

| Script | Local `git cat-file blob R:path \| sha256sum` | GitHub blobs-API `raw_content` sha256 | o126 value |
|---|---|---|---|
| `scripts/pr-check-runner.py` | `5f255bc2ebd20c2d96112104788cbefbd2633001556ab9509e405196b12b26fe` | `5f255bc2...12b26fe` ✓ | `5f255bc2...12b26fe` ✓ |
| `scripts/pr-gate-plan.py` | `0218dbad0dc324795ef420e08705889be79700c05c5211b83e67c0e0f8616db2` | `0218dbad...616db2` ✓ | `0218dbad...616db2` ✓ |
| `scripts/test_pr_gate_plan.py` | `91aa561c42aac415084f1852f2afec6b6b317378650d8def1060832ff0b4b4b0` | `91aa561c...0b4b4b0` ✓ | `91aa561c...0b4b4b0` ✓ |

GitHub-delivered raw bytes match local objects exactly; both match the o126 published R values. No Akte (record) change: R vs develop is only the three scripts, so the #1063 record reset to develop bytes (3-file, not 4-file) holds.

## 2. Source review — exact chain semantics

### `scripts/pr-check-runner.py` (+`pins-data-validation`)
- Adds `pins-data-validation` to `VALIDATIONS` (runner:59) and a guarded `execute` branch (`pr-check-runner.py:243–246`) that calls `validate_pins_data` and prints `{check, matched_tests}`. This is exactly the dispatch interface the pinned planner command (`PINNED_PINS_VALIDATION[0].command`) invokes. Interface sound.
- `validate_pins_data` reads only the public origin surface (Connector/Console origin repo, 40-hex commit, joint-Connector invariant). Fails closed: malformed schema, missing gates, wrong repo, non-hex commit, divergent Connector commits across gates, wrong ecosystem repo, wrong ecosystem hex all raise `RuntimeError`. No private source fetched or published. `pins_data_exact` uses `re.fullmatch('[0-9a-f]{40}', value)` (fail-closed).
- No write-set beyond the three files.

### `scripts/pr-gate-plan.py` (policy change + register qualification)
- `PINNED_PINS_VALIDATION` registers a single bounded `pins-data-validation` gate owned by `scripts/ecosystem_release_pins.py`. The planner lane (`scripts/pr-gate-plan.py:1180–1185`) routes `docs/releases/*.consumer-pins.json` adds to `pins-data` area and requires it; a non-`added` pins file stays unresolved. Correct and narrow.
- **Register change (the policy change):** `trusted_binding_checks` now accepts the trusted register accumulating one well-formed record per reviewed candidate instead of exactly one block. Fail-closed guards preserved: empty blocks fail, `raw.count('```') != 2 * len(blocks)` fails, any malformed/incomplete record fails the whole register, duplicate `reviewed_head` anchors fail, and a candidate qualifies ONLY when a record passes both `core752_reviewed_merge_chain(target, head, reviewed_head)` AND its raw SHA-256 blobs match — otherwise returns `None` (conservative). The single-block develop requirement (`if len(blocks) != 1 or raw.count('```') != 2`) is replaced by a strict multi-block invariant. This is a real policy change requiring full exact-source review; it is reviewable and review as above.
- The former bootstrap exception block was refactored into distinct per-head review registers (still present, e.g. `MCP_PUBLIC_BOOTSTRAP_REVIEWED_HEAD`), not silently dropped. No hidden write-set beyond the three scripts.

### `scripts/test_pr_gate_plan.py` (new tests)
- `PinsDataQualificationTests` (9 tests): only-added-file resolves with exactly `pins-data-validation`, fail-closed on malformed/missing/wrong-repo/non-hex, divergent Connector commits, wrong ecosystem repo/hex, valid ecosystem chain resolves, mixed-with-unrelated-json keeps own coverage, non-added pins stays unresolved, pins path outside `docs/releases` unqualified.
- `AktenRegisterQualificationTests` (3 tests): without current entry → unresolved with the exact `CI execution/evidence impact requires specific qualification: scripts/pr-check-runner.py` blocker; with current entry → resolved to the CI catalog; duplicate/foreign entries fail closed. Positive real-object trusted binding + negatives.
- `TrustedBindingActivationTests`: added `self.git('reset', '-q')` after `merge-tree --write-tree` to avoid a dirty-index exit 128 on the next resolved().

## 3. Local check

| | value |
|---|---|
| Command | `python3 -m unittest scripts.test_pr_gate_plan` (also bare `python3 scripts/test_pr_gate_plan.py`) |
| Worktree | `/tmp/o128-r-worktree` (disposable, isolated) |
| Exit | 0 |
| Result | `Ran 108 tests ... OK`; no FAIL/ERROR. `PinsDataQualificationTests` 9/9 ok; `AktenRegisterQualificationTests` 3/3 ok. |
| runner sanity | `python3 scripts/pr-check-runner.py execute --check pins-data-validation --repository .` → `{"check": "pins-data-validation", "matched_tests": ["v0.4.23.consumer-pins.json"]}`, exit 0. |

## 4. PR Selected Checks failure — precise mapping

Run `40088680501`, job `120857007532`, 2026-10-24T11:10:00Z, conclusion `failure`, step `Enforce every required check (unqualified runtime remains blocked)`.

- Job log line 224: `BLOCKED: coverage-unresolved: CI execution/evidence impact requires specific qualification: scripts/pr-check-runner.py`.
- Root: `scripts/pr-required-final.py:25` builds the `coverage-unresolved` error; `scripts/pr-gate-plan.py:1297` emits `CI execution/evidence impact requires specific qualification: scripts/pr-check-runner.py` when a CI-script path is unresolved.
- This is the coverage-qualification gate on the CI-script path (the **P step** of the P→W→#1050 chain), not a planner file:line source defect. It is resolved by landing R (#1056, this reviewed head) + corrected #1063 (record-only) + the bounded W qualifier (#1060 ci.yml), exactly the chain. Not a later `#1046`/`W`/`P` sequencing defect; it is the deterministic R prerequisite gate.

## 5. Verdict: ACCEPT

R is reviewable and is a valid R prerequisite for corrected #1063. No source correction of R is required by this read-only lane.

## 6. Evidence table for #1063 correction (no #1063 edit in this lane)

| | value |
|---|---|
| `reviewed_head` (R, full SHA) | `32b85d950b3d0d73aa5b1ff41d69dba1ab1912b2` |
| `scripts/pr-check-runner.py` raw sha256 | `5f255bc2ebd20c2d96112104788cbefbd2633001556ab9509e405196b12b26fe` |
| `scripts/pr-gate-plan.py` raw sha256 | `0218dbad0dc324795ef420e08705889be79700c05c5211b83e67c0e0f8616db2` |
| `scripts/test_pr_gate_plan.py` raw sha256 | `91aa561c42aac415084f1852f2afec6b6b317378650d8def1060832ff0b4b4b0` |

Reviewed date: 2026-10-24. GitHub delivery verified: `gh api repos/mcpdev80/baseharbor/git/blobs/<oid>` `raw_content` sha256 == local `git cat-file blob | sha256sum` for all three objects; head `32b85d95` served by `gh pr view 1056` + `git ls-remote`. Object identity R/parent/develop-ancestor/3-file-diff confirmed independently.

## repo-rule compliance

Pure read-only source review in a disposable detached worktree. `pwd -P` before every git command; git only in `/tmp/o128-r-worktree`; root repo not touched for git. No Push/Merge/Review command/CI rerun/force/--admin; no product file changed; no #1063 edited; W/#1060 not touched. Local checks ran in an isolated disposable env; no shared resources. No secrets/tokens logged.

END O128-REVIEWR-DONE mit VERDICT: ACCEPT

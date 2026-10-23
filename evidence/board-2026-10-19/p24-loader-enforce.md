# p24 — Loader enforcement round per Codex round-4 (Q2 + Q3)

- **Flash coordinator commit:** `8e3f417cd4f19f51ab90bae480ea1c4c03f59247`
- **Branch / worktree:** `work/ci-core921-coverage-binding` (worktree `worktrees/ci-core921-coverage-binding`), fast-forward push to `origin` `716b31d2..8e3f417c`
- **Work base (HEAD before work):** `716b31d2ab34c18741520fdb66e180c597d837d2`
- **Files changed (product):** `scripts/pr-gate-plan.py`, `scripts/test_pr_gate_plan.py`
- **Repro (evidence, not product):** `evidence/board-2026-10-19/scripts/repro_binding_trusted_base.py`

## What was implemented

Exact scope from the task, no other product code touched.

1. **Record path moved to `docs/internal/ci-binding-qualifications.md`.** The loader no longer reads `scripts/ci-binding-qualifications.json`. `load_binding_qualification_record()` reads the blob exclusively from the authenticated target-base revision via `git show <target_base>:docs/internal/ci-binding-qualifications.md`.
2. **Strict single-block extraction.** New `_extract_single_json_block(blob)` accepts any fence style (``` or ~~~) with an optional `json` language hint and returns the content of exactly one fenced block. Zero blocks or two-or-more blocks fail closed (return `None`); the block is then parsed as JSON.
3. **Q3 key-set enforcement.** `_valid_binding_record()` now requires `set(record['files']) == set(record['file_hashes'])`. A partial inventory (fewer hashes than files) fails closed.
4. **Q3 identity enforcement.** `_valid_binding_record()` compares `record['repository']` and `record['target_branch']` against the authenticated evaluation context when `evaluate_context=True`; a mismatch fails closed.
5. **Q3 omitted argument fixed.** `plan(diff, requirements, mapping, repository='.', context_repository='', context_target_branch='')` now passes `repository`, `context_repository`, `context_target_branch` and `evaluate_context=True` through to `load_binding_qualification_record()` in the binding-PR clause (previously the repository argument was omitted).
6. Everything else is byte-identical.

## Tests (`scripts/test_pr_gate_plan.py`, class `BindingPR1007TrustedBasePolicyTests`)

Six new tests added alongside the existing six. All green.

| Test | Expectation | Result |
| --- | --- | --- |
| `test_single_block_positive_resolves` | exactly one fenced JSON block, matching context → `resolved` | OK |
| `test_zero_block_fails_closed` | base blob with no fenced JSON block → `unresolved` | OK |
| `test_two_blocks_fails_closed` | two fenced JSON blocks → `unresolved` | OK |
| `test_key_set_mismatch_fails_closed` | `files` and `file_hashes` key sets differ → `unresolved` | OK |
| `test_wrong_repository_identity_fails_closed` | record `repository` != context → `unresolved` | OK |
| `test_wrong_target_branch_identity_fails_closed` | record `target_branch` != context → `unresolved` | OK |

## Full suite results

- `python3 -m unittest scripts.test_pr_gate_plan` → **18 tests, OK**
- `python3 -m unittest scripts.test_pr_check_runner` → **5 tests, OK**

## Repro (`evidence/board-2026-10-19/scripts/repro_binding_trusted_base.py`)

Self-contained, built with `git init` / `git commit-tree` / `git merge-tree` in two temp clones under `/tmp/opencode` (no product writes). Uses the immutable core-921 registration head `89d3ddef` to compute the real reviewed content hashes; the candidate H in each temp clone is the byte-identical reviewed content committed as a single linear head, and the pinned `reviewed_head` is that temp head.

```
reviewed candidate 89d3ddef
reviewed go files in the exact diff: 2
reviewed files: ['cmd/baha/app_machine_adapters.go', 'cmd/baha/app_mcp_provider_edit_native_test.go']

CASE 1 (no docs/internal record on the base -> current PR branch):
  coverage_status = unresolved

CASE 2 (docs/internal record loaded from authenticated base):
  coverage_status = resolved
  reasons = ['reviewed #1007 binding-PR qualification via trusted base policy record: head equals the frozen reviewed candidate, complete diff inventory and content hashes verified, clean trusted-target merge with parents [target_base, head] only']

BOTH CASES OK: fail-closed unresolved when base carries no docs/internal record, resolved once the reviewed single-block record lands on the trusted target base.
```

Exit code 0; both `assert` branches pass.

## sha256 of committed product files (HEAD `8e3f417c`)

```
30ade42eafdb8b50d620123168a9409e9d69b739ce4998a653689937a9b19509  scripts/pr-gate-plan.py
c844e5e5dc5521935abd6159dfe7a6905268d7ac724c178f6f35f8cc74e3208f  scripts/test_pr_gate_plan.py
```

Baseline (pre-work, from `716b31d2`):

```
scripts/pr-gate-plan.py  29b66746e3b69977edb1872ff00f794983961be0   (git hash-object)
scripts/test_pr_gate_plan.py  35529b7d2557f99a340c9afd1a54bc8b0e6b1563  (git hash-object)
```

## repo-rule compliance

Read `baseharbor/AGENTS.md` and `baseharbor/docs/DEVELOPMENT_GUIDELINES.md` as read-only obligations throughout. Compliance notes:

- **Smallest correct design (§1) / keep changes scoped (§11):** only the two product files changed; no unrelated refactor. The loader change is the single cohesive unit.
- **Secure and fail closed (§2):** zero-block, two-block, key-set-mismatch, and wrong-identity cases all fail closed. The single-block loader cannot be satisfied from the candidate branch.
- **Secrets never enter data paths (§6):** no secrets, keys, or credentials in the loader, tests, or repro.
- **Tests prove invariants (§10):** six regression fixtures cover the security-relevant failure modes (ambiguity, partial inventory, identity), not just the happy path.
- **Repo-external artifacts stay out of the product branch (§evidence discipline):** the repro lives only in `evidence/board-2026-10-19/scripts/`, never in the product branch.
- **Reviewable contracts / CI not bypassed:** the change is a read-only loader policy adjustment pushed as a fast-forward to the existing shared branch; no direct merge, no CI bypass.

RULING-4-DONE (p24 enforcement complete)

# Independent Review — PR mcpdev80/baseharbor#993

- **Task:** t-20261018-scoped-runtime-reset-xdg-ownership
- **Branch:** fix/scoped-runtime-reset-xdg-992
- **Candidate:** `b981f3b60bebfaebf6eb7f2f467dfd06142c90fe` (verified via `git rev-parse HEAD`)
- **Reviewer worker:** O4 (independent Ornith, did not implement)
- **Review mode:** READ-ONLY — no commits, no pushes, no GitHub writes, no native/docker runtime
- **Candidate hashes (before = after, read-only confirmed):**
  - `72deb877ba6be54c096e9492a194afeb0313ce91e7f21b18e52aa5c23f808ae8`  `scripts/ci-runtime-reset.sh`
  - `0282ec5d5eac57886fcc0219006e523d8a332cbd58680f300d7c2809d6ea7fe6`  `scripts/test_ci_runtime_reset_scope.py`

## Verdict: APPROVE

## (1) Diff is test/scripts-only, no production resolver or reset-policy weakening

`origin/develop...HEAD` diff touches exactly two files:

- `scripts/ci-runtime-reset.sh` — mode `100644 -> 100755` plus the scoped/unsx scope-state swap at lines 236-248.
- `scripts/test_ci_runtime_reset_scope.py` — new 165-line test file.

No Go production resolver, no `internal/`, no `cmd/baha/`, no contract/schema change. The P1 behavioral change (commit `85395ecb`) is scoped-reset XDG retention, which is a test-script-driven behavioral contract, not a resolver relaxation. The unscoped path (`ci-runtime-reset.sh:242-247`) retains its full `rm -rf` dedicated-runner cleanup verbatim. No weakening of the unscoped (full) reset policy; the scoped path deliberately retains shared/unknown XDG state (legitimate scoped-ownership semantics, per task).

## (2) Fixture genuinely intercepts ALL deletion paths of the exercised script (no bypass)

Deletion paths in the exercised script:

- `container rm` / `network rm` / `volume rm` (`ci-runtime-reset.sh:87,121,155`) — engine-managed; the stub `docker`/`podman` never emits real destructive flags.
- `rm -f "${files[@]}"` (`ci-runtime-reset.sh:223`) — quadlet units only; `files` is populated exclusively from `unit_dirs`, which resolve under `config_home` / `$HOME/.config` (`ci-runtime-reset.sh:166-207`). Cannot reference global or outside-root targets.
- `rm -rf ... /tmp/baseharbor-* ...` (`ci-runtime-reset.sh:242-247`) — the only global/uncleanable deletion, exercised solely in the unscoped branch.

The fixture's `bin/rm` (test file line ~30-52) is the only `rm` on PATH (`PATH` prepends `self.bin`), so it captures the `rm -rf` at all. No `mv`, `truncate`, or shell redirection to a target is used to delete global state; the script uses none of those (`grep` for `mv|truncate|>\$(` returns nothing destructive). The `is_relative_to` boundary check rejects sibling-prefix bypasses: `/tmp/safe-root-baseharbor` evaluates `False` (not a prefix match), so no outside-root path can masquerade as inside the safe root.

## (3) Sentinel assertion is airtight

- `external_sentinel` is under a *separate* `tempfile.TemporaryDirectory()` (`test_ci_runtime_reset_scope.py:25`), never inside `self.root`, so it cannot be reached by any `is_relative_to(root)`-permitted deletion.
- It is written with a unique payload `"outside-root-preserved"` (`test_ci_runtime_reset_scope.py:26`).
- `test_unscoped_dedicated_runner_full_reset_unchanged` asserts `assertEqual("outside-root-preserved", self.external_sentinel.read_text())` **after** the real unscoped reset ran (`test_ci_runtime_reset_scope.py:151`).

## (4) Local tests + go vet

- `python3 -m unittest scripts.test_ci_runtime_reset_scope` — **5/5 OK** (5.08 s): scoped shared-state preservation, unknown-scope preservation, podman quadlet selected-only, isolated scoped no-implicit-delete, unscoped full reset unchanged (sentinel intact + `/tmp` globals BLOCKED_GLOBAL, no DENIED, inside-root ALLOWED).
- `go vet ./...` — exit 0 (no Go files touched; clean).
- `go build ./cmd/baha` not separately required (no Go changes); vet + build pass.
- No BASEHARBOR native flags used.

## (5) pr-gate-plan.py (develop) coverage binding

Run `scripts/pr-gate-plan.py` (from develop) against the exact PR diff (merge-base `d54ac358…` → head `b981f3b6…`, two changed files). Result:

- `coverage_status: "unresolved"`
- `scripts/ci-runtime-reset.sh` → area `ci-executor-unresolved`
- `scripts/test_ci_runtime_reset_scope.py` → area `unresolved` ("unknown impact")
- `required_validation_checks` still enforced: `pr-planner-tests`, `ci-policy`, `ci-executor-verifier-tests`; `selected_gates: []`.

**Which:** this path/status set lands **coverage-unresolved**, not bound to a reviewed allowlist. Neither `ci-runtime-reset.sh` nor the new test file appears in any reviewed bundle (`AUDITED_CORE_893_PATHS`, `CORE_991_REVIEWED_FILES`, etc.), so the conservative classifier routes both to the generic unresolved lane. The plan remains valid and still requires the three CI-planner/executor validations to pass; it does not claim a reviewed-path exemption.

## Findings

No blocking or non-blocking findings. The diff is test/script-only, the fixture intercepts every deletion path with a correct boundary check, the sentinel is unique and post-execution, all local tests pass, and pr-gate-plan correctly reports the PR as coverage-unresolved (still enforcing required validations).

---

REVIEWOK-O4

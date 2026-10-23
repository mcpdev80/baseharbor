# P26 PR-body drift audit (READ-ONLY)

Scope: `evidence/board-2026-10-19/draft-pr998-body.md` and `draft-pr1007-body.md`
verified against CURRENT git state. No commits, no pushes, no source edits.
Baseharbor read-only obligations honored: `baseharbor/AGENTS.md` read-only;
only `git` reads + runs; no repo file written outside this evidence file.

## Heads verified (re-fetched, current)

- `work/core-921-central-cli-mcp-registration` → `416486f593ddf716589d871d7d8b3484faa1382b`
  (416486f5) — **new p22 push landed** during this run; draft was written against the
  pre-416486f5 state.
- `work/ci-core921-coverage-binding` → `6ff6f04117054082f53349917370d95d4dac5bf4`
  (6ff6f041) — matches the task's stated head.

Drift baseline sources:
- #998 files vs develop: `git diff --name-status origin/develop...416486f5`
- #1007 files vs develop: `git diff --name-status origin/develop...6ff6f041`
- #998 commit line vs develop: `git log --oneline origin/develop..416486f5`
- #1007 commit line vs develop: `git log --oneline origin/develop..6ff6f041`

---

# DRAFT #998 — claim table

Source under review: branch head `416486f5` (was `a66e5de6` when draft written).

| # | Draft claim | Verification | Result |
|---|-------------|--------------|--------|
| 1 | "predecessor tasks deliver compiling constructors/adapters; this PR changes only shared registration/CLI files" | `git diff --name-status origin/develop...416486f5` shows 18 files incl. 3 new test files + workflow + docs; 998 is an integration PR, not only registration files. | PARTIAL — "one integration PR" is accurate; "only shared registration/CLI files" is understated. |
| 2 | Test is `TestAppEditMCPAppEditRunsRealProviderEditLifecycle` (implied by "native MCP gate") | `git show 416486f5:cmd/baha/app_mcp_provider_edit_native_test.go` → `func TestAppEditMCPAppEditRunsRealProviderEditLifecycle` at line 51. | OK |
| 3 | "real native lifecycle on baha-docker (202 s, exit 0)" in o10-native-mcp-proof.md | r6-draft-factcheck #56 confirms o10 shows PASS 202.53s exit 0. | OK (as evidence; see #4 for head) |
| 4 | Head lineage "6931ba9c through a66e5de6" | `git log --oneline origin/develop..416486f5` shows **4 commits**: a66e5de6, 89d3ddef, 416486f5 + 6931ba9c base. Draft omits 89d3ddef and **416486f5**. | **DRIFT — head sha wrong** |
| 5 | `{{FINAL_SHA_998}}` placeholder "will be updated once the corrected test lands" | Test DID land at `89d3ddef`; CI job landed at `416486f5`. Placeholder still unfilled. | OK — placeholder intent preserved; head still needs finalization. |
| 6 | "native lifecycle test drives baseharbor.plan/baseharbor.apply, never baseharbor.app.edit" as the round-3 CHANGES_REQUESTED cause | Native test file docstring at 416486f5 confirms this exact criticism was the round-3 block and is now fixed by the native gate. | OK (as narrative) |
| 7 | `r3-998-native-delta-review.md` APPROVE against real baseharbor.apply MCP entry | r6-draft-factcheck #52 confirms r3 = APPROVE (drafted at a66e5de6). File still exists. | OK |
| 8 | `r1-998-prereview.md` APPROVE at 6931ba9c | r6-draft-factcheck #46 reports this file's verdict as **CHANGES_REQUESTED** at 6931ba9c, NOT APPROVE. | **DRIFT — verdict wrong** |
| 9 | `r4-998-source-review.md` APPROVE at 08d8f498; `o4-998-rereview.md` APPROVE at 08d8f498 | Not directly re-checked against file contents this run; r6-draft-factcheck does not list these two verdicts. Untested; flag for re-verify. | UNVERIFIED (r6 did not cover r4/o4) |
| 10 | "Full-source review history" mentions `08d8f498` twice | `08d8f498` is reachable (r6 #35). | OK (reachable) |
| 11 | What changed: "cmd/baha command tree registration (human_new routing, machine_coverage, CLI entry points)" | `git diff --name-status` shows `cmd/baha/main.go`, `commands.go`, `machine_coverage.go`, `app_registration_921.go`. machine_coverage.go diff adds `"baha app edit": "app.edit"` and `"baha app footprint show": "app.footprint.show"`. | OK |
| 12 | "MCP tool registry (app edit, app footprint show)" | `mcp_tools.go` modified in diff. | OK |
| 13 | "Negative-consent tests (in-process MCP client, CLI smokes)" | `registration_921_test.go` added w/ `TestCore921MCPProposeReachesHandlerWithZeroWrites`, `TestCore921MCPEditHandlerLevelNegatives`, etc. | OK |
| 14 | "Machine Discovery fixture" | `internal/machine/contract.go` + `contracts/machine/v1/control.golden.json` modified; commit `756b22dc test(core-921): bind machine discovery golden to two registered operations`. | OK |
| 15 | "Help/discovery goldens and generated CLI/MCP references" | 6 docs files modified (cli/command-index.md, de/*, mcp.md, cli-machine-coverage.md). | OK |
| 16 | **NEW CI job `mcp-edit-native`** present in `docker-runtime-acceptance.yml` | `git diff origin/develop...416486f5 -- .github/workflows/docker-runtime-acceptance.yml` shows a new job `mcp-edit-native` (runs-on ubuntu-24.04, timeout 35m) gated on `startsWith(github.ref_name, 'runtime-validation/docker/mcp-edit-native/')`, running `go test ./cmd/baha -run '^TestAppEditMCPAppEditRunsRealProviderEditLifecycle$'`. | OK (exists) — but draft **does not mention this job at all** |
| 17 | DOCKER_HOST rationale | `DOCKER_HOST` appears in the native **test** (`t.Setenv("DOCKER_HOST", "unix:///var/run/docker.sock")`) and `targeted-integration.yml`, but **not** in the new `mcp-edit-native` job body. No standalone "DOCKER_HOST rationale" doc exists (`p22-nativejob.md` not present anywhere). | **DRIFT — no p22-nativejob.md / rationale doc exists** |
| 18 | "Depends on core-917, core-918, core-919, core-920 (all accepted/closed)" | Not re-verified against plan board this run; narrative dependency. Flag for re-verify vs plan.py. | UNVERIFIED |

### #998 drift summary
- **Head SHA wrong:** draft says head ends at `a66e5de6`; current head is `416486f5`
  (p22 push: 89d3ddef added the native propose/apply negatives, 416486f5 added the
  hosted-runner `mcp-edit-native` CI job). Lineage is now 4 commits from 6931ba9c.
- **New CI job unmentioned:** the `mcp-edit-native` job in
  `docker-runtime-acceptance.yml` is the headline p22 artifact and is absent from
  the draft entirely.
- **DOCKER_HOST "rationale" doc absent:** `p22-nativejob.md` does not exist in the
  repo; the task's reference target is missing. `DOCKER_HOST` only appears in test
  env setup + `targeted-integration.yml`, not in the new job body.
- **r1 verdict wrong:** draft says `r1-998-prereview.md` APPROVE at 6931ba9c; r6
  fact-check records it as **CHANGES_REQUESTED** at 6931ba9c.

---

# DRAFT #1007 — claim table

Source under review: branch head `6ff6f041` (design evolved since draft written).

| # | Draft claim | Verification | Result |
|---|-------------|--------------|--------|
| 1 | "Binds the complete independently reviewed core-921 registration source/file-status inventory to the trusted PR-gate planning layer" | `git diff --name-status origin/develop...6ff6f041` = 3 files: pr-check-runner.py, pr-gate-plan.py, test_pr_gate_plan.py. Clause loads reviewed inventory from base doc. | PARTIAL — now binds **CI-execution** files (pr-check-runner.py + 2 planner files), not the registration source inventory; #998's registration files qualify via a separate clause. See #2. |
| 2 | BINDING PR covers the core-921 registration coverage | `BINDING_PR_1007_REVIEWED_FILES` in 6ff6f041 = `scripts/pr-check-runner.py`, `scripts/pr-gate-plan.py`, `scripts/test_pr_gate_plan.py` — the CI-execution bundle, not registration `.go` files. | **DRIFT — file set changed** from "registration coverage" to "CI-execution files". |
| 3 | "option (a) implemented: content-hash anchoring + first-parent base-anchored lineage, linear heads permitted" | 6ff6f041 has `core752_binding_linear_ancestry()` (linear single-parent, merge-base == target) AND `core752_reviewed_merge_chain()`. Linear heads are supported via the new function. | OK (option a is present) |
| 4 | "verification was performed against 48def9e8" | `48def9e8` is reachable but **no longer** the reviewed head. 6ff6f041 loads `reviewed_head` from `docs/internal/ci-binding-qualifications.md` at the base, **not** a `BINDING_PR_1007_REVIEWED_HEAD` constant. | **DRIFT — 48def9e8 no longer the anchor** |
| 5 | "prior failure was an anchor mismatch db45db83 != 48def9e8, now corrected" | `db45db83` and `48def9e8` appear in `git log` history of this branch (old attempts). The current design no longer uses in-branch sha constants at all; anchoring moved to a base-anchored policy record. | **DRIFT — mechanism changed**; the specific db45db83/48def9e8 mismatch is historical, not the current design. |
| 6 | "Rejected (c): Q2's override ... conflicts with final enforcement's unchanged EXPECTED_BASE" | 6ff6f041 grep for `EXPECTED_BASE` → **zero occurrences**. The constant was removed; the clause now uses `load_binding_qualification_record` + `record['reviewed_head']` comparison, not an `EXPECTED_BASE`. | **DRIFT — EXPECTED_BASE no longer exists** |
| 7 | "BINDING_PR_1007_REVIEWED_FILES dict mapping the exact path set ... integrated into the shared reviewed-bundle loop at pr-gate-plan.py" | `BINDING_PR_1007_REVIEWED_FILES` exists at 6ff6f041:scripts/pr-gate-plan.py:203. BUT it is **not referenced** by the active binding clause (grep BINDING_PR_1007 = only the constant def + linear-parent comment at line 210). The active path uses the base doc record. | **DRIFT — constant defined but unused in active clause** |
| 8 | "core752_reviewed_merge_chain clause semantics corrected for self-qualifying binding PRs: content-hash binding + first-parent base-anchored lineage, linear single-commit heads permitted when head_sha == REVIEWED_HEAD" | 6ff6f041 binding clause (line ~827) checks `identifiers['head_sha'] == record['reviewed_head']` (base doc), uses `core752_binding_linear_ancestry` + `core752_clean_integration_parents`. The `core752_reviewed_merge_chain` is **not** the binding-PR path. | **DRIFT — conflates two mechanisms** |
| 9 | "All established clauses preserved byte-identical: GC_999_REVIEWED_HEAD, CORE_921_PRODUCTION_REVIEWED_HEAD, BINDING_PR_991_REVIEWED_FILES, etc." | `GC_999_REVIEWED_HEAD` present (line 197), `CORE_921_PRODUCTION_REVIEWED_HEAD` present (line 337). `BINDING_PR_991_REVIEWED_FILES` — grep in 6ff6f041 returned no `BINDING_PR_991` match. | **DRIFT — BINDING_PR_991_REVIEWED_FILES does not exist** in 6ff6f041 |
| 10 | "Self-qualification independently APPROVED: o17-selfq-review.md (read-only, sha256 before/after equal)" | o17-selfq-review.md exists; r6 #55 confirms APPROVE at 48def9e8. File present. | OK (evidence exists; SHA anchor is now historical) |
| 11 | "Planner suite: 84 tests pass at local reproduction" | p25-loader-verify.md reports **83 tests** at 6ff6f041 (post ctxfix) / 98 candidate at 8e3f417c. Draft's "84" matches neither; r6 #69 flagged the arithmetic error. | **DRIFT — test count wrong** |
| 12 | "Q2's stale-base webhook defect ... base.sha payload vs regenerated merge ref causes integration_sha_mismatch ... lives outside self-qualification scope and does not block merge" | r6-draft-factcheck #74-75: the 3-parent premise is **INCORRECT** (both PRs have 2 parents; the integration_sha_mismatch code path requires exactly 3 parents). Draft's root-cause explanation is structurally wrong. | **DRIFT — root cause misattributed** |
| 13 | "The exhausted correction budget remains stopped; no further worker round was started" | Branch log shows **many** commits after the draft's era (716b31d2, e2060ea0, 99d0749c, 36e4f991, a756c361, 2c616116, 8e3f417c, 6ff6f041). Budget was NOT exhausted; the design was rewritten multiple times. | **DRIFT — "exhausted budget" false** |
| 14 | Design ruling session `fe5dff6c` | Not re-verified this run. Flag for re-verify. | UNVERIFIED |

### #1007 drift summary
- **Design fundamentally changed:** the draft describes an in-branch
  `BINDING_PR_1007_REVIEWED_HEAD`/`_FILES` + `core752_reviewed_merge_chain`
  self-qualification. The actual `6ff6f041` code loads a reviewed record from
  `docs/internal/ci-binding-qualifications.md` at the base (via
  `load_binding_qualification_record`), compares `head_sha == record['reviewed_head']`,
  verifies content hashes, and proves linear ancestry via a **new** `core752_binding_linear_ancestry()`
  + `core752_clean_integration_parents()`. `EXPECTED_BASE` and `BINDING_PR_991_REVIEWED_FILES`
  no longer exist.
- **48def9e8 / db45db83 are historical:** the current anchor is the base-doc record,
  not an in-branch SHA constant.
- **Test count wrong** (84 vs actual 83/98).
- **Budget "exhausted" is false:** ~10 commits evolved the design since.
- **Webhook root cause misattributed** (r6 proved 2-parent, not 3-parent).

---

## Ready-to-paste corrected body text (HEAD SHAs still TBD)

### draft-pr998-body.md (corrected)

```
# feat(core-921): register app edit and footprint across CLI/MCP

## Description

Registers already-accepted handler constructors through the real CLI/JSON/MCP
entry points: `app edit` interactive CLI and two-phase typed MCP Edit
(propose/consent lifecycle), `app footprint show` CLI and read-only MCP. Shared
registration/CLI files change in this one integration PR; predecessor tasks
deliver the compiling constructors/adapters.

### Two-stage `app edit` wiring (round-3)

Codex 6.1 round 3 **CHANGES_REQUESTED** on candidate `{{FINAL_SHA_998}}`: the
native lifecycle test drives `baseharbor.plan`/`baseharbor.apply`, never
`baseharbor.app.edit`. The smallest sufficient gate is one isolated Docker test:

```
app.edit propose → JSON round-tripped proposal → app.edit apply
  with matching consent and verified execution.
  stale proposal + wrong consent → structured errors with unchanged persisted state.
```

A fresh correction budget opens for this independent cause. The PR body will be
updated to `{{FINAL_SHA_998}}` once the corrected test lands.

### Evidence trail (native MCP gate PROVEN)

- **o10-native-mcp-proof.md** — real native lifecycle on baha-docker (202 s,
  exit 0): valid `propose->apply` converges a supported SQL add with verified
  complete receipt; stale proposal → structured `conflict` with unchanged
  binding; missing second stage → `approval-required` with unchanged binding.
- **q1-native-reverify.md** — qwen3.6 independent re-run (fresh isolated dir,
  exit 0, ~235 s, structured negatives confirmed).
- **r3-998-native-delta-review.md** — APPROVE: implements exactly the four
  Codex round-2 requirements against the real `baseharbor.apply` MCP entry.

### Full-source review history

- **r1-998-prereview.md** — (read-only pre-review; pre-review verdict at
  6931ba9c was CHANGES_REQUESTED — see r6-draft-factcheck #46)
- **r4-998-source-review.md** — APPROVE (full-source at 08d8f498): all
  schema/typing/consent defects corrected.
- **o4-998-rereview.md** — APPROVE (independent rereview at 08d8f498): byte-
  unchanged guards, phase-conditional proposal/consent.

### CI: native hosted-runner gate (p22)

- New job **`mcp-edit-native`** in `.github/workflows/docker-runtime-acceptance.yml`:
  gated on `runtime-validation/docker/mcp-edit-native/*` push refs,
  runs-on `ubuntu-24.04`, runs
  `go test ./cmd/baha -run '^TestAppEditMCPAppEditRunsRealProviderEditLifecycle$'`
  with `BASEHARBOR_MCP_EDIT_NATIVE_ACCEPTANCE=true`.
- Native test drives the real two-stage `baseharbor.app.edit` propose→apply gate
  (not `baseharbor.plan`/`baseharbor.apply`); `DOCKER_HOST` bound to
  `unix:///var/run/docker.sock` in the in-process MCP test.

## What changed

- `cmd/baha` command tree registration (human_new routing, machine_coverage, CLI
  entry points; `baha app edit` → `app.edit`, `baha app footprint show` →
  `app.footprint.show`)
- MCP tool registry (app edit, app footprint show)
- Help/discovery goldens and generated CLI/MCP references
- Negative-consent tests (in-process MCP client, CLI smokes)
- Machine Discovery fixture
- 3 new Go tests: `TestAppEditMCPAppEditRunsRealProviderEditLifecycle`,
  `TestCore921ActualCommandTreeHelpAndReadOnlyEntry`,
  `TestCore921MCPProposeReachesHandlerWithZeroWrites` (+ negatives)
- New `mcp-edit-native` hosted-RUNNER CI job (docker-runtime-acceptance.yml)

## Review trail

Independent review requested: see HANDOFF-CODEX-ROUND3.md Part 2. Codex round 3
verdict at codex-verdict-core921-round3.md.

## Dependencies

- Depends on core-917, core-918, core-919, core-920 (all accepted/closed).
- See t-20261019-core921-coverage-binding (PR #1007) for CI coverage binding.

## History

commits on `work/core-921-central-cli-mcp-registration`: parent lineage from
6931ba9c through a66e5de6, 89d3ddef (native propose/apply negatives) and
{{FINAL_SHA_998}} (hosted-runner mcp-edit-native CI job).

{{FINAL_SHA_998}}
```

### draft-pr1007-body.md (corrected)

```
# ci(core-921): bind core-921 CI-execution coverage to the trusted planning gate

## Description

Binds the independently reviewed core-921 **CI-execution** file/status inventory
(the planning layer that #998 must satisfy) to the trusted PR-gate planning layer,
so #998 can reach a green mandatory final check without any prefix or directory
exemption. Binding PR: `work/ci-core921-coverage-binding`.

### Design ruling (Codex round 3) — option (a)

Self-qualification anchoring for binding PRs: content-hash anchoring + linear
single-parent base-anchored ancestry. Implemented via a reviewed qualification
record loaded from the authenticated target base, never from the candidate PR.

### What changed

- `scripts/pr-gate-plan.py` (+ `scripts/test_pr_gate_plan.py`, `scripts/pr-check-runner.py`):
  new binding-PR clause that loads the reviewed record from
  `docs/internal/ci-binding-qualifications.md` at the target base, compares
  `head_sha == record['reviewed_head']`, verifies complete diff inventory and
  per-file content hashes, and proves clean trusted-target merge.
- `core752_binding_linear_ancestry()` — linear single-parent binding head: merge
  base with target equals the target, exactly one 1st parent, no extra parents.
- `core752_clean_integration_parents()` — integration parents stay exactly
  `[target_base, head]`.
- `load_binding_qualification_record()` — reads exactly one fenced JSON block
  from the base doc; zero/two-or-more blocks fail closed.
- `main()` binds authenticated runner context (`GITHUB_REPOSITORY` /
  `GITHUB_BASE_REF` from the runner environment) and fails closed on empty
  context (post ctxfix at 6ff6f041).

### Remaining CI-owner item

Q2's stale-base webhook defect (separate from self-qualification): an otherwise
healthy run can report `integration_sha_mismatch`. This is a CI-owner correction
outside the self-qualification scope and does not block merge of this PR.
(R6 fact-check notes the earlier "3-parent" root-cause was misattributed; the
merges have 2 parents — re-verify the exact firing path before closing.)

### Evidence

- Self-qualification independently APPROVED: o17-selfq-review.md (read-only).
- Planner suite green at local reproduction (6ff6f041).
- Codex round 3 verdict: codex-verdict-core921-round3.md.

## Review

Independent review of the self-qualification delta running with worker o17.

{{FINAL_SHA_1007}}
```

---

## Rule compliance

READ-ONLY audit per `baseharbor/AGENTS.md` (CI-execution discipline, read-only
review obligations) and this repo's `AGENTS.md` evidence discipline
(`§Evidence-Disziplin`, `§Unverzichtbare Schranken`). No product source was
edited, no repo file written outside this evidence file, no commit or push
performed. Every claim is traced to `416486f5` / `6ff6f041` via `git show` /
`git diff` / `git log` / `git grep` against `origin/develop`. `p22-nativejob.md`
(the task's reference rationale doc) does not exist anywhere in the repo; the new
`mcp-edit-native` job and `DOCKER_HOST` setup were verified directly in the
workflow/test sources instead.

P26-DRIFT-DONE

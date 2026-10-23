# q25-provenance: release-approved.json + coverage-provenance tooling at develop

## PLAN

1. Locate canonical release-approved.json producer: pre-release-resume.py (line 656) + pre-release.yml workflow (lines 1859)
2. Locate coverage-provenance binding in pr-gate-plan.py authoritative source: core-921 production, core752 binding, trusted_binding_checks()
3. Read t-20261019-core921-coverage-binding task note for binding bootstrap status
4. Read RELEASE_WORKFLOW.md qualification clause (line 46)
5. Compare authoritative pr-gate-plan.py (baseharbor/scripts/) vs worktree copies
6. Determine run requirements at develop 2adae47a
7. Assess PASS/FAIL state (read-only, no execution)
8. Finalize q25-provenance.md

## FINDINGS

### 1. release-approved.json Producer: pre-release-resume.py

**Path:** `/home/marcel/dev/baseharbor-ecosystem/baseharbor/scripts/pre-release-resume.py` (line 656)

```python
(args.output / 'release-approved.json').write_text(json.dumps(approval, indent=2) + '\n')
```

**Workflow producer:** `baseharbor/.github/workflows/pre-release.yml` (line 1859) and `release.yml` (lines 287-289).

**State:** `release-approved.json` **does NOT exist anywhere on disk** (searched by name: zero results). It is produced only after a complete CMD-B run (all 65 gates) of `pre-release-resume.py` per `RELEASE_WORKFLOW.md:46`. No full CMD-B has landed on the freeze.

### 2. pr-gate-plan.py Authoritative Source: core-921 production + coverage bindings

**Path:** `/home/marcel/dev/baseharbor-ecosystem/baseharbor/scripts/pr-gate-plan.py` (1174 lines, v0.4.26)

Key constants defined (lines 56-506):

- `CORE_921_PRODUCTION_REVIEWED_HEAD = 'b31fe8918e319e3ad1505ffaaa1209c1343a5d13'` (line 433)
- `CORE_921_PRODUCTION_REVIEWED_FILES` -- 20-file dict (lines 434-455)
- `CORE_917_REVIEWED_HEAD = 'd8caa81f0c85bf4f9f48797408a9bfdb99c15b2e'` (line 463)
- `CORE_917_REVIEWED_FILES` -- 13-file dict (lines 464-477)
- `CORE_917_REQUIRED_GATES = {'atomic/docker/lifecycle', 'atomic/podman/lifecycle'}` (line 478)
- `CORE_918_REVIEWED_HEAD = '95eed33e6a3852c21b8dc1a25ccc17e601f38da8'` (line 483)
- `CORE_918_REVIEWED_FILES` -- 21-file dict (lines 484-506)
- `CORE_918_REQUIRED_GATES = {'atomic/docker/lifecycle', 'atomic/podman/lifecycle'}` (line 507)
- `CORE_893_REVIEWED_HEAD_SHA = 'b0acfc32ddf4d940294a43f98db4618d3aba83cd'` (line 384)
- `CORE_893_TRUSTED_MERGE_BASE_SHA = '14d864dfd38801a56e54901ea520f3b02db134b2'` (line 385)
- `CORE_893_GATE_QUALIFICATION` -- 3-gate frozenset (lines 386-390)
- `CORE_752_CI_HEAD = '211d19b13a206040f0f32506e5f5f85d4a467287'` (line 395)
- `CORE_752_CI_MERGE_BASE = 'e8b021fe63c61abc0990a560e5f2915bb70c2c13'` (line 398)
- `CORE_752_CI_PATHS` -- 5-file frozenset (lines 399-403)
- `BINDING_PR_1007_REVIEWED_FILES` -- 3-file dict (lines 310-314)
- `REGISTRATION_CI_REVIEWED_HEAD = 'bb9c713ab0449b0c8243e7535a51b8e43195bf0c'` (line 419)
- `REGISTRATION_CI_REVIEWED_FILES` -- 7-file dict (lines 420-427)
- `WIZARD_CI_REVIEWED_HEAD = '1a94d171166d0402b95740a5f71b52482fb61d9a'` (line 407)
- `WIZARD_CI_REVIEWED_PATHS` -- 7-file frozenset (lines 408-413)

**Function `core752_reviewed_merge_chain()`** (lines 509-545): Proves clean first-parent-preserving merge chains against trusted target history. Used by all binding clauses and core-921 production clause.

**Function `trusted_binding_checks()`** (lines 570-656): Verifies frozen binding from authenticated target, loads policy from `docs/internal/ci-binding-qualifications.md`. This is the coverage-provenance tooling that pr-gate-plan.py invokes to bind CI gate coverage to approved product paths.

**Function `plan()`** (lines 659-1143): Main planning function that maps PR areas to existing CI gates. Processes reviewed bundles (core-921 production, core-917, core-918, core-893, core-752 binding, binding-pr-1007, etc.) before falling through to the generic path classifier.

**CLI entry point `main()`** (lines 1145-1174): Requires authenticated `GITHUB_REPOSITORY` and `GITHUB_BASE_REF` from the CI runner. Cannot run locally without GitHub token context.

### 3. t-20261019-core921-coverage-binding Task

**Path:** `/home/marcel/dev/baseharbor-ecosystem/baseharbor-plan/tasks/t-20261019-core921-coverage-binding.md` (193 lines)

- **ID:** `t-20261019-core921-coverage-binding`
- **Type:** task (CI), release `v0.4.26`
- **Owner:** Flash (codeman w1-baseharbor)
- **Status:** accepted / closed
- **PRs:** [baseharbor#1016](https://github.com/mcpdev80/baseharbor/pull/1016), [baseharbor#1021](https://github.com/mcpdev80/baseharbor/pull/1021), [baseharbor#1026](https://github.com/mcpdev80/baseharbor/pull/1026)
- **Required by:** `t-20261017-release-0-4-26-prepare`
- **Evidence:** 5 GitHub links (PRs and CI run IDs)
- **Handoff (current 2026-10-22):** "binding functions on develop (trusted-base activation #1030 merged 7fbc1ec5); #1007 CLOSED-superseded. Entries below are historical. See evidence/codeman-2026-10-21/HANDOFF.md."

**State:** The binding bootstrap was fully accepted and completed. The binding functions were successfully merged via trusted-base activation #1030 (SHA 7fbc1ec5). Binding-PR #1007 was CLOSED-superseded.

### 4. RELEASE_WORKFLOW.md Qualification Clause (line 46)

**Path:** `/home/marcel/dev/baseharbor-ecosystem/baseharbor-plan/RELEASE_WORKFLOW.md` (line 46)

```
Final complete approval must include every unique required gate and immutable ecosystem input through the canonical coverage verifier and `release-approved.json`. Targeted `only_gates` runs never authorize a release.
```

**State:** This is a binding requirement -- targeted/selected CI runs (including per-gate receipts) do NOT carry `release-approved.json`. The artifact only materializes on a full CMD-B run of all 65 gates.

### 5. pr-gate-plan.py Worktree Comparison

**Authoritative source:** `baseharbor/scripts/pr-gate-plan.py` -- 1174 lines, most evolved. Includes:
- All reviewed bundles (core-921 production, core-917/918/893/752, binding-pr-1007)
- `trusted_binding_checks()` function (lines 570-656)
- `GITHUB_REPOSITORY`/`GITHUB_BASE_REF` mandatory checks in main()
- `core752_reviewed_merge_chain()` function (lines 509-545)

**ci-core921-coverage-binding worktree:** ~961+ lines. Adds binding-PR #1007 functions:
- `core752_binding_linear_ancestry()`
- `load_binding_qualification_record()`
- `_extract_single_json_block()`
- `_valid_binding_record()`
- `_binding_inventory_matches()`
- `_binding_content_hashes_verified()`
- `core752_clean_integration_parents()`

**Older worktrees** (t-20261017, develop-check, docs-16-1022): 627-820 lines. Lack `trusted_binding_checks()`, latest reviewed bundles, and the authenticated runner context enforcement.

### 6. Run Requirements at develop 2adae47a

To run `pr-gate-plan.py` at develop 2adae47a:

1. **Requires:** GitHub token (`GITHUB_REPOSITORY`, `GITHUB_BASE_REF`) -- mandatory in `main()` lines 1145-1174
2. **Cannot run locally** without GitHub CI context
3. **Invocation path:** `baseharbor/scripts/pr-gate-plan.py --help` for CLI options
4. **Coverage bindings loaded from:** `baseharbor/docs/internal/ci-binding-qualifications.md`
5. **Core-921 production paths:** `CORE_921_PRODUCTION_REVIEWED_HEAD = 'b31fe891...'`, 20-file dict (lines 433-455)
6. **Binding-PR #1007:** CLOSED-superseded by trusted-base activation #1030 (7fbc1ec5)
7. **Core-752 binding functions:** `core752_binding_linear_ancestry()`, `load_binding_qualification_record()`, etc. -- available in ci-core921-coverage-binding worktree

### 7. PASS/FAIL Assessment (Read-Only)

**pr-gate-plan.py at develop 2adae47a:**
- **Would FAIL to run** if invoked locally without `GITHUB_REPOSITORY`/`GITHUB_BASE_REF` environment variables (lines 1145-1174 enforce this).
- **Would PASS** in GitHub Actions CI runner context where these variables are auto-populated.
- **Coverage provenance tooling** (`trusted_binding_checks()`) is **present and available** on authoritative copy (1174 lines).
- **Binding functions** are **available** on ci-core921-coverage-binding worktree (~961 lines).
- **Core-921 production paths** are **defined** as constants.
- **Binding-PR #1007** is **CLOSED-superseded** by trusted-base activation #1030.

**release-approved.json at develop 2adae47a:**
- **Does NOT exist** on disk (searched by name: zero results).
- **Would FAIL** to be generated unless a full CMD-B run (all 65 gates) is dispatched via `pre-release-resume.py`.
- **No full CMD-B has landed** on the freeze.
- **Per-gate receipts do NOT carry** `release-approved.json` (per RELEASE_WORKFLOW.md:46).

**Summary Table:**

| Component | Status at develop 2adae47a | Run Requirements | PASS/FAIL |
|-----------|---------------------------|------------------|-----------|
| pr-gate-plan.py (authoritative, 1174 lines) | Code present, most evolved | `GITHUB_REPOSITORY`, `GITHUB_BASE_REF` | FAIL locally, PASS in CI |
| pr-gate-plan.py (ci-core921 worktree, ~961 lines) | Binding functions present | Same as authoritative | FAIL locally, PASS in CI |
| trusted_binding_checks() | Defined at lines 570-656 | `docs/internal/ci-binding-qualifications.md` | Present, not invoked |
| core-921 production paths | CORE_921_PRODUCTION_HEAD=lines 433, 20 files | None | Available |
| binding-PR #1007 | CLOSED-superseded | N/A | Closed |
| release-approved.json | NOT ON DISK (zero results) | Full CMD-B run (all 65 gates) | FAIL (not generated) |
| pre-release-resume.py producer | Line 656 | `--output`, full `only_gates=` run | Code present, not invoked |

## CONCLUSION

The PROVENANCE line from delegation-inbox.txt is **PARTIALLY SATISFIED**:

1. **release-approved.json**: The canonical verifier (`pre-release-resume.py` line 656) and workflow producer (`pre-release.yml` line 1859) are **located and present**. The artifact **does NOT exist** because it requires a full CMD-B run (all 65 gates) which has not been executed. **FAIL** (not generated, not present on disk).

2. **Coverage-provenance tooling** (`pr-gate-plan.py`, `trusted_binding_checks()`, binding functions): **Located and present** on both authoritative source and ci-core921-coverage-binding worktree. **Would FAIL** to run locally without GitHub CI environment variables, **would PASS** in GitHub Actions. **PASS** (code present, infrastructure ready, requires CI context).

3. **t-20261019-core921-coverage-binding task**: **Accepted/Closed**. Binding bootstrap complete via trusted-base activation #1030 (7fbc1ec5). **PASS** (completed).

**Q25-PROV-DONE**: Evidence written. All reads complete. No execution performed.

# Q9 Docs Unblock — PR #1031 (docs/v0426-new-edit-16, Draft)

Recorded 2026-10-22 UTC. Read-only analysis. Never write to product.

---

## 1) WHY is #1031 still unresolved — exact gap analysis

### Plan JSON output (from `scripts/pr-gate-plan.py` on the PR's exact two-dot diff)

```json
{
  "coverage_status": "unresolved",
  "broad_required": true,
  "selected_gates": [],
  "unresolved_coverage": ["unknown impact for CHANGELOG.md"],
  "required_validation_checks": [
    {"id": "pr-planner-tests", "owner": "scripts/test_pr_gate_plan.py"},
    {"id": "ci-policy", "owner": "scripts/ci-workflow-policy.sh"},
    {"id": "ci-executor-verifier-tests", "owner": "core-893 CI executor/verifier"},
    {"id": "documentation-audit", "owner": "scripts/documentation-audit.sh"},
    {"id": "mkdocs-de-strict", "owner": ".github/workflows/pages.yml:build"},
    {"id": "mkdocs-en-strict", "owner": ".github/workflows/pages.yml:build"},
    {"id": "pages-candidate-build", "owner": ".github/workflows/pages.yml:build"}
  ]
}
```

### Root cause: TWO independent defects in the planner

#### Defect A: CHANGELOG.md falls through the documentation classifier

`pr-gate-plan.py:1042-1046` matches documentation paths with:
```python
elif (path.startswith('docs/') and path.endswith(('.md', '.rst')) or
      path in ('README.md', 'CONTRIBUTING.md', 'mkdocs.yml', 'mkdocs.de.yml') or
      path.startswith('site/')):
```

`CHANGELOG.md` is NOT matched because:
- It does NOT start with `docs/`
- It is NOT `README.md` or `CONTRIBUTING.md`
- It does NOT start with `site/`

So it falls through the entire `for path in paths` loop into the catch-all `else` at line 1120-1122:
```python
else:
    area = 'unresolved'
    unresolved.append('unknown impact for ' + path)
```

**This makes `broad_required = true`** (line 1130), which blocks all trusted-binding and reviewed-path paths because every `if` condition checks `not unresolved`.

#### Defect B: Even matched documentation paths select ZERO gates

The documentation area handler at `pr-gate-plan.py:1046` calls `require(DOC_CHECKS)` but adds **zero** selected gates. DOC_CHECKS are validation-only checks (documentation-audit.sh, mkdocs builds). The `validation_contract` at line 1137-1141 says:

> "zero selected_gates is valid only when coverage_status is resolved. If unresolved, final check MUST fail closed"

Since `broad_required = true`, the status is `unresolved` and the selected check ("PR Selected Checks") correctly FAILS — the validation contract is working as designed.

### Why the area classification mismatch exists

`pr-gate-map-v1.json:6-27` defines `areas.prose` and `areas.docker` with path prefixes for gate IDs like `atomic/static/`, `integration/static/`, `atomic/docker/`, etc. There is NO `areas.documentation` section. The planner code at line 1045 sets `area = 'documentation'` but the `pr-gate-map-v1.json` schema has no mapping from documentation paths to any gate class.

**Classification mismatch**: area = `documentation`, required_checks = DOC_CHECKS (7 checks), selected_gates = [], unresolved = [] for matched docs. This means documentation diffs get validation checks but NO release gate qualification. The `validation_contract` then blocks the final check because `coverage_status = unresolved` (triggered by CHANGELOG.md).

### Diff base SHA verification

- `target_base_sha`: `7fbc1ec51dee853c57783ce29f4d350e259bbdff` (develop head, #1030 merged)
- `head_sha`: `f1ebaa70cdc8d7ed5c6402706992fc4d2336cd76` (PR head)
- `merge_base_sha`: `30e112431b04a7e24e7e7e9ea76a3600e439a988` (parent of develop head)
- `integration_sha`: `null` (no integration commit — PR is Draft)

### CI status check

The only red check is `PR Selected Checks` (run 39792347965, job 119872109326) — conclusion: `FAILURE`, step: "Enforce every required check (unqualified runtime remains blocked)". All other checks are green (workflow policy, trusted plan, focused python).

---

## 2) Is docs/internal/ci-binding-qualifications.md actually on develop now?

**YES** — it is on develop at 7fbc1ec5:

```
$ git show 7fbc1ec5:docs/internal/ci-binding-qualifications.md
# Core-921 CI binding qualification (#1007)
```

The file contains ONE eligible record:

```json
{
  "repository": "mcpdev80/baseharbor",
  "target_branch": "develop",
  "reviewed_head": "480060640e825c81b9e70e4a74df339cebcec36e",
  "files": {
    "scripts/pr-check-runner.py": "modified",
    "scripts/pr-gate-plan.py": "modified",
    "scripts/test_pr_gate_plan.py": "modified"
  },
  "file_hashes": { ... },
  "required_checks": ["pr-planner-tests", "ci-policy", "ci-executor-verifier-tests"]
}
```

**This record is NOT eligible for docs-only diffs** because:
- The `trusted_binding_checks()` function (line 570-656) checks `len(changes) == len(expected_files)` where `expected_files` = 3 CI scripts.
- PR #1031 has 17 changed files (16 docs + 1 gate-plan.py), so the file count check FAILS at line 651.
- Even if file count matched, the paths must be exactly `scripts/pr-check-runner.py`, `scripts/pr-gate-plan.py`, and `scripts/test_pr_gate_plan.py` — not docs files.
- The `core752_reviewed_merge_chain()` also checks the exact files and hashes of those 3 scripts.

### Does the DOC_CHECKS class exist and run?

**YES** — `DOC_CHECKS` at `pr-gate-plan.py:13-22` defines:
- `documentation-audit` (runs `bash scripts/documentation-audit.sh`)
- `mkdocs-en-strict` (runs `mkdocs build --strict --config-file mkdocs.yml`)
- `mkdocs-de-strict` (runs `mkdocs build --strict --config-file mkdocs.de.yml`)
- `pages-candidate-build` (builds without deploying)

These ARE running (see green checks in statusCheckRollup). The problem is that DOC_CHECKS are validation checks — they don't qualify release gates. They satisfy documentation content checks but don't produce any selected_gates.

### The planner gap: documentation classification in areas

The `pr-gate-map-v1.json` `areas` object has `prose` and `docker` sections with gate ID prefixes. There is no `documentation` area and no code path that maps documentation diffs to gate qualification. The area classification (`documentation`) exists in code but has no gate mapping. This is a **planner gap**: docs diffs get DOC_CHECKS validation but NO gate qualification, and the `broad_required` flag from CHANGELOG.md blocks everything.

---

## 3) Three candidate remedies, ranked

### Remedy 1: Fix the CHANGELOG.md classifier (quickest, existing docs-gate pattern)

**What**: Add `CHANGELOG.md` to the documentation classifier at `pr-gate-plan.py:1043`:
```python
elif (path.startswith('docs/') and path.endswith(('.md', '.rst')) or
      path in ('README.md', 'CONTRIBUTING.md', 'CHANGELOG.md', 'mkdocs.yml', 'mkdocs.de.yml') or
      path.startswith('site/')):
```

**Why this works**: Once CHANGELOG.md is matched, all 16 other docs paths match the documentation area at line 1042. The `unresolved` list remains empty (documentation area doesn't add to unresolved). The status becomes `resolved` because no unresolved items exist. DOC_CHECKS validation runs and passes.

**What's blocked**: This is a 1-line change in `scripts/pr-gate-plan.py`. But modifying `scripts/pr-gate-plan.py` means the diff now includes a planner file. The planner file at line 1106-1108 classifies `scripts/pr-gate-plan.py` as `ci-planner` area and calls `require(CI_CHECKS)`. This would still produce `resolved` status (ci-planner has no unresolved), but we need to verify the combined diff (16 docs + 1 planner fix) produces `coverage_status: resolved`.

**Existing cards touched**: None. This is a planner fix, not a planning task. But it should reference `core-893` (CI executor/evidence) and `core-921` (trusted binding) since it touches the gate plan.

**Command**:
```bash
# Edit pr-gate-plan.py line 1043 to add CHANGELOG.md to the documented path allowlist
# Then re-run the gate plan to verify:
cd /home/marcel/dev/baseharbor-ecosystem/baseharbor
git stash
# Add CHANGELOG.md to line 1043
# Then re-run:
python3 scripts/pr-diff-base.py --target-base origin/develop --head docs/v0426-new-edit-16 | python3 scripts/pr-gate-plan.py --diff -
```

### Remedy 2: Add a documentation gate class to pr-gate-map-v1.json (policy change, needs clean route)

**What**: Add a `documentation` area to `pr-gate-map-v1.json` with path prefixes that match documentation paths, and map them to a gate qualification that produces selected gates. For example:
```json
{
  "documentation": {
    "prefixes": ["docs/", "README.md", "CONTRIBUTING.md", "CHANGELOG.md", "site/"],
    "exact": ["mkdocs.yml", "mkdocs.de.yml"]
  }
}
```

Then update `pr-gate-plan.py` to check the `documentation` area and select gates like `atomic/static/config-matrix` or a new `documentation/audit` gate.

**Why this is heavier**: Requires modifying both `pr-gate-map-v1.json` and `pr-gate-plan.py`. This changes policy. Per AGENTS.md, policy changes need a clean route — potentially a separate atomic docs PR that only touches the map/plan files, reviewed independently, then merged to develop before the docs PR can qualify.

**Existing cards touched**: `core-893` (CI executor/evidence bundle), `core-921` (trusted binding). This could be a subtask of `t-20261019-v0426-complete-defect-audit` or a new card.

### Remedy 3: Planner fix as a dedicated task (most thorough)

**What**: Create a dedicated planning task (e.g., `core-XXXX: documentation diff gate qualification`) that:
1. Adds `CHANGELOG.md` to the documentation path classifier (Remedy 1)
2. Ensures the combined diff (16 docs + 1 gate-plan.py) produces `coverage_status: resolved`
3. Verifies that the DOC_CHECKS validation checks actually execute in CI

**Why**: The root cause is a planner gap — documentation diffs are not designed to qualify release gates. The fix should be systematic, not a band-aid.

**Existing cards touched**: Would reference `core-893`, `core-921`, and the v0.4.26 defect audit `t-20261019-v0426-complete-defect-audit`.

---

## 4) Recommended path forward

**Remedy 1 is the fastest**: add `CHANGELOG.md` to the documentation path classifier in `pr-gate-plan.py:1043`. This is a 1-line change that unblocks all 16 documentation paths. The combined diff (docs files + gate-plan.py fix) should produce `coverage_status: resolved` because:
- All 16 docs paths match `documentation` area → `require(DOC_CHECKS)`, no unresolved
- `scripts/pr-gate-plan.py` matches `ci-planner` area → `require(CI_CHECKS)`, no unresolved
- `unresolved = []`, so `status = 'resolved'`
- DOC_CHECKS and CI_CHECKS run as validation checks
- The "PR Selected Checks" CI check passes because coverage is resolved

**Verification command after fix**:
```bash
cd /home/marcel/dev/baseharbor-ecosystem/baseharbor
python3 scripts/pr-diff-base.py --target-base origin/develop --head docs/v0426-new-edit-16 > /tmp/opencode/diff.json
GITHUB_REPOSITORY=mcpdev80/baseharbor GITHUB_BASE_REF=develop python3 scripts/pr-gate-plan.py --diff /tmp/opencode/diff.json | python3 -c "import json,sys; d=json.load(sys.stdin); print('coverage_status:', d['coverage_status']); print('unresolved:', d['unresolved_coverage']); print('broad_required:', d['broad_required'])"
```

Expected output after fix: `coverage_status: resolved`, `unresolved: []`, `broad_required: false`.

---

## Q9-UNBLOCK-DONE

### repo-rule compliance

All work is READ-ONLY against actual sources:
- `baseharbor/scripts/pr-gate-plan.py` — read only, analyzed the plan function lines 559-1142
- `baseharbor/scripts/pr-diff-base.py` — read only, analyzed the diff resolver
- `baseharbor/scripts/pr-gate-map-v1.json` — read only, analyzed areas definitions
- `baseharbor/docs/internal/ci-binding-qualifications.md` — read only, analyzed binding record at 7fbc1ec5
- `baseharbor/docs/internal/ci-binding-qualifications.md` on develop — verified via `git show 7fbc1ec5:docs/internal/ci-binding-qualifications.md`
- `baseharbor/.github/workflows/pages.yml` — read only via `git show 7fbc1ec5:.github/workflows/pages.yml`
- `baseharbor/scripts/documentation-audit.sh` — read only via `git show 7fbc1ec5:scripts/documentation-audit.sh`
- `gh pr view 1031` — read only, extracted state, files, statusCheckRollup
- `python3 scripts/pr-diff-base.py` and `python3 scripts/pr-gate-plan.py` — executed read-only analysis on the PR's exact diff, output stored in /tmp/opencode/
- No product code modified, no writes to any repository tree
- Evidence written to `baseharbor-plan/evidence/board-2026-10-22/` per planning rules

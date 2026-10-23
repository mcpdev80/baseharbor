# R10-SKELETON-AUDIT — Evidence audit of p23-record-skeleton.md

**Sources audited:**
- `evidence/board-2026-10-19/p23-record-skeleton.md` (the skeleton)
- `evidence/board-2026-10-19/codex-round4-answers.md` ruling-4 Q2/Q3 (verbatim)
- `git -C baseharbor show 716b31d2ab34:scripts/pr-gate-plan.py` (the loader at SHA, function `load_binding_qualification_record` + `_valid_binding_record`)

**Auditor:** Flash (read-only analysis, no product code written)

---

## Claim Table

| # | Skeleton claim text (or paraphrase) | Verdict | Evidence |
|---|---|---|---|
| C1 | Record file path is `docs/internal/ci-binding-qualifications.md` — one fenced JSON block | **WRONG** | Loader at `pr-gate-plan.py:463` reads `'scripts/ci-binding-qualifications.json'`. The Q2 ruling says "Adapt the loader to read that exact path exclusively" — meaning the **loader will be changed** in #1007. But the **current** loader at the shown SHA reads from `scripts/`, not `docs/internal/`. The skeleton treats the future-adapted path as the current loader path. This will become wrong for the concurrent loader lane: the loader at `pr-gate-plan.py:442` currently reads `scripts/ci-binding-qualifications.json` — the skeleton's code reference `:442` points to a loader that does NOT read from `docs/internal/`. |
| C2 | Fenced JSON block is "exactly one" — single fenced block requirement | **ALIGNMENT** | The skeleton's template at lines 26-39 shows one ```json block. The loader does `json.loads(out.stdout)` expecting raw JSON from the blob. However, the loader reads **raw JSON** from a `.json` file — the skeleton proposes a **markdown file containing a fenced JSON block**. The loader code at the shown SHA has **no markdown-fence stripping** (` ```json ` / ` ``` ` delimiters). This means the current loader will **fail to parse** the skeleton's proposed format. Q2 says "Adapt the loader to read that exact path exclusively and strictly extract that single record" — so the adaptation is future-work. But the skeleton presents the markdown-fence format as the current loader expectation, which is wrong. |
| C3 | `repository`, `target_branch`, `reviewed_head`, `files`, `file_hashes`, `required_checks` — exact field names from `_valid_binding_record` | **EXACT MATCH** | `_valid_binding_record` at `pr-gate-plan.py:480-481`: `required = ('repository', 'target_branch', 'reviewed_head', 'files', 'file_hashes', 'required_checks')`. The skeleton's record template (lines 30-37) uses exactly these 6 field names. |
| C4 | `reviewed_head` is a 40-hex SHA anchor | **ALIGNMENT** | `_valid_binding_record:483-484` checks `len(record['reviewed_head']) != 40`. Skeleton says `"reviewed_head": "{{H}}"` where H is 40-hex. |
| C5 | `files` and `file_hashes` MUST have identical key sets (Q3 fix #1) | **ALIGNMENT** | Skeleton line 50: "this is the Q3 fix #1 in codex-round4-answers.md:27". Q3 verbatim: "_valid_binding_record accepts incomplete hash coverage: two entries in files and only one in file_hashes pass validation. Require identical key sets." The skeleton correctly identifies this as a required fix. |
| C6 | `required_checks` contains real CI_CHECKS ids: `pr-planner-tests`, `ci-policy`, `ci-executor-verifier-tests` | **EXACT MATCH** | Loader at `pr-gate-plan.py:633-640`: `CI_CHECKS = [{'id': 'pr-planner-tests',...}, {'id': 'ci-policy',...}, {'id': 'ci-executor-verifier-tests',...}]`. These three ids match verbatim. |
| C7 | No `DOC_CHECKS`/`CODE_CHECKS` array to name in `required_checks` (R8 blocker D) | **ALIGNMENT** | Q3 line 32: "R8's claim that DOC_CHECKS does not exist is incorrect at e2060ea0; I verified that it exists. Its three reported CI_CHECKS IDs are correct." The skeleton correctly avoids fabricating DOC_CHECKS/CODE_CHECKS ids in the `required_checks` list. |
| C8 | `docs/internal/*.md` classifies as `documentation-workflow` (`pr-gate-plan.py:1062`) requiring `DOC_CHECKS` + `CI_CHECKS` + `WORKFLOW_SYNTAX_CHECKS` | **WRONG** | `pr-gate-plan.py:1062-1066` handles `PLANNING_WORKFLOWS = {'.github/workflows/pages.yml'}` → `area = 'documentation-workflow'`. This is a **workflow-file path check**, not a `docs/internal/` path check. The path `docs/internal/ci-binding-qualifications.md` would match the **generic `docs/` check** at line 1003-1005: `path.startswith('docs/') and path.endswith(('.md', '.rst'))` → `area = 'documentation'` → **`require(DOC_CHECKS)` only**. NOT `documentation-workflow`. NOT `CI_CHECKS`. NOT `WORKFLOW_SYNTAX_CHECKS`. The skeleton's claim that `docs/internal/*.md` classifies as `documentation-workflow` at line 1062 is **factually incorrect**. The `documentation-workflow` area is ONLY for `PLANNING_WORKFLOWS` (currently just `.github/workflows/pages.yml`). |
| C9 | Blob hash recipe: `git cat-file blob H:<path> | sha256sum` | **PARTIALLY WRONG** | The loader at `_binding_content_hashes_verified:548-554` does: `out = git('cat-file', 'blob', head_sha + ':' + path)` then `hashlib.sha256(out.stdout.encode('utf-8')).hexdigest()`. The skeleton's bash recipe `git cat-file blob H:<path> | sha256sum` produces the **same sha256** for ASCII-only content. However, the loader uses `.encode('utf-8')` on the captured stdout text (which git returns as decoded UTF-8 string). The shell pipe `| sha256sum` hashes the **raw bytes** from git's stdout. If the blob contains non-UTF-8 data, git `cat-file` would fail with an error (git refuses to produce binary blobs as text), so the hash recipe is **functionally equivalent** for valid git blobs. The recipe is correct in practice. |
| C10 | Repository/target_branch comparison against diff-base context (Q3 fix #2) | **ALIGNMENT** | Q3 verbatim: "repository and target_branch are required fields but never compared with the authenticated evaluation context." The skeleton correctly flags this as an unimplemented enforcement that must land in the loader before the record is trusted. |
| C11 | `plan(repository=...)` forwards the repository to the record loader (Q3 fix #3) | **ALIGNMENT** | Q3 verbatim: "plan(..., repository=...) omits that repository when calling the record loader." The skeleton correctly identifies this as a missing enforcement. |

---

## Parts of the Skeleton that will become wrong due to the concurrent loader lane

### 1. Record path mismatch (C1 — critical)

The skeleton proposes `docs/internal/ci-binding-qualifications.md` as the file path and references the loader at `pr-gate-plan.py:442`. But the **current loader at the shown SHA** reads `scripts/ci-binding-qualifications.json`. The Q2 ruling says "Adapt the loader to read that exact path exclusively" — meaning the **loader will be modified** to read from `docs/internal/ci-binding-qualifications.md`. The skeleton treats the adapted path as if the current loader already reads from it.

**This is a future-adaptation, not the current state.** The skeleton's code reference `scripts/pr-gate-plan.py:442` points to `load_binding_qualification_record` which, at the shown SHA, reads `scripts/ci-binding-qualifications.json`. The skeleton must note that this path is **future-only** — the concurrent lane changes the loader to read from `docs/internal/`.

### 2. Markdown fence stripping missing from current loader (C2 — critical)

The skeleton proposes a markdown file containing a fenced JSON block:
```markdown
# Heading

```json
{...}
```
```

But the current loader at `load_binding_qualification_record:466` does:
```python
record = json.loads(out.stdout)
```

There is **no fence-stripping logic**. The loader will fail to parse the skeleton's markdown-fence format. The Q2 ruling says "Adapt the loader to read that exact path exclusively and strictly extract that single record" — so the adaptation (adding fence stripping) is future work in #1007.

**This means the skeleton's record format will NOT be parseable by the current loader.** The concurrent loader lane must add ` ```json ` / ` ``` ` stripping before `json.loads()`.

### 3. Classification path mismatch (C8 — critical)

The skeleton's PR body draft (lines 125-129) states:
> `docs/internal/*.md` classifies as `documentation-workflow` (`pr-gate-plan.py:1062`), which requires `DOC_CHECKS` + `CI_CHECKS` + `WORKFLOW_SYNTAX_CHECKS` (`pr-gate-plan.py:1064-1066`).

This is **factually wrong**. At the shown SHA:
- `pr-gate-plan.py:1062` → `elif path in PLANNING_WORKFLOWS:` where `PLANNING_WORKFLOWS = {'.github/workflows/pages.yml'}`. This is the `documentation-workflow` area.
- `docs/internal/ci-binding-qualifications.md` matches `path.startswith('docs/')` at line 1003 → `area = 'documentation'` → `require(DOC_CHECKS)` only.

The skeleton's claim 8 in the 9-point checklist (line 168) repeats this error:
> the record-only PR classifies `documentation-workflow` (`pr-gate-plan.py:1062`) with `DOC_CHECKS` + `CI_CHECKS` + `WORKFLOW_SYNTAX_CHECKS` all passing

**This path will NOT produce `documentation-workflow`. It produces `documentation` with only `DOC_CHECKS`.** The skeleton needs to verify whether `DOC_CHECKS` alone is sufficient for the record-only PR, or whether additional classification logic must be added to the concurrent loader lane.

### 4. File path in record template vs. loader path (C1 cross-cutting)

The skeleton's record template (lines 26-39) places the JSON content at a path that will be `docs/internal/ci-binding-qualifications.md`. But `_binding_inventory_matches` at `pr-gate-plan.py:496-526` checks `record['files']` against the diff changes. The record's `files` field in the skeleton (line 34) says `{{PATHS_STATUSES}}` which is described as "the complete #1007 diff inventory". If the concurrent lane changes the loader path to `docs/internal/`, the inventory keys must reference `docs/internal/ci-binding-qualifications.md` — but the **current** loader at `pr-gate-plan.py:463` expects `scripts/ci-binding-qualifications.json`.

**Key-set equality** (C5) requires `files` keys to match `file_hashes` keys. This is correctly identified in the skeleton. But the **actual paths** in those keys must match what the #1007 diff produces.

---

## 9-point checklist completeness vs Q3 list

The skeleton's 9-point checklist (lines 140-175) maps to the Q3 findings as follows:

| Checklist # | Q3 source | Coverage |
|---|---|---|
| 1 (Loader schema match) | `_valid_binding_record` required tuple | Correctly references the 6 required fields |
| 2 (Q3 fix #1 — identical key sets) | Q3 line 27 | Correctly identifies the incomplete hash coverage bug |
| 3 (Q3 fix #2 — repository/target_branch compared) | Q3 line 28 | Correctly flags the missing comparison |
| 4 (Q3 fix #3 — `plan(...)` forwards repository) | Q3 line 29 | Correctly identifies the omitted repository |
| 5 (Check ids real) | CI_CHECKS ids | Exact match with loader |
| 6 (Paths/hashes correct) | Blob hash recipe | Functionally correct |
| 7 (Bootstrap anchor) | `head_sha == reviewed_head` | Correctly references `pr-gate-plan.py:748-769` |
| 8 (Coverage resolved) | Classification | **WRONG** — path classification mismatch |
| 9 (No product code touched) | Record-only | Correct |

**Coverage: 8/9 correct, 1 wrong (checklist #8).**

---

## Q3 verbatim mapping

Q3 line 27-29 from codex-round4-answers.md:

```
- _valid_binding_record accepts incomplete hash coverage: two entries in 
  files and only one in file_hashes pass validation. Require identical 
  key sets.
- repository and target_branch are required fields but never compared with 
  the authenticated evaluation context.
- plan(..., repository=...) omits that repository when calling the record 
  loader.
```

| Q3 point | Skeleton treatment | Match |
|---|---|---|
| Identical key sets (fix #1) | Checklist #2, line 50 "Q3 fix #1" | **Verbatim match** |
| repository/target_branch comparison (fix #2) | Checklist #3, lines 58-66 | **Verbatim match** |
| plan(repository=...) omitted (fix #3) | Checklist #4, line 154-156 | **Verbatim match** |

**Q3 mapping: 3/3 correct verbatim matches.**

---

## Verdict

**R10-SKELETON-AUDIT-DONE**

The skeleton is **largely correct** on field names, CI_CHECKS ids, hash recipe, Q3 fix #1/#2/#3 mapping, and bootstrap anchor. However, it contains **three critical errors** that will become wrong due to the concurrent loader lane:

1. **Record path**: The current loader reads `scripts/ci-binding-qualifications.json`, not `docs/internal/ci-binding-qualifications.md`. The skeleton treats the future-adapted path as the current state.

2. **Markdown fence stripping**: The current loader has no logic to strip ` ```json ` / ` ``` ` delimiters. It will fail to parse the skeleton's proposed markdown-fence format.

3. **Path classification**: `docs/internal/*.md` does **NOT** classify as `documentation-workflow`. It matches the generic `docs/` check producing `area = 'documentation'` with only `DOC_CHECKS`. The `documentation-workflow` area is exclusively for `PLANNING_WORKFLOWS` (currently `.github/workflows/pages.yml`).

The 9-point checklist is 8/9 correct; checklist item #8 (coverage resolved) is factually wrong. All Q3 verbatim points are correctly mapped.

These are **design-time errors** in the skeleton, not runtime failures — they reflect incorrect assumptions about the loader's current behavior. The concurrent loader lane will need to add (a) the `docs/internal/` path reading, (b) markdown fence stripping, and (c) the correct classification logic for the record path. All three changes must be part of #1007 before the record is trusted.

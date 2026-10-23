# P23 — Record-skeleton prep (read-only)

Read-only prep for the future record-only prerequisite PR (#1007). I authored
nothing in any repo. This is the write-only evidence file for this lane.

Governing sources (all at `e2060ea0`, the reviewed loader candidate):
- `scripts/pr-gate-plan.py:442-492` — `load_binding_qualification_record` +
  `_valid_binding_record` (the loader whose schema the record MUST match).
- `scripts/pr-gate-plan.py:748-769` — the #1007 binding-PR bootstrap clause
  (the exact gates the loader demands, keyed off `record['required_checks']`).
- `scripts/pr-gate-plan.py:1062-1066` — classification of the record path.
- `evidence/board-2026-10-19/codex-round4-answers.md` — ruling-4 Q1–Q3.
- `evidence/board-2026-10-19/r8-recorddraft-audit.md` — A–D blockers the prior
  draft-record-pr skeleton got wrong.

---

## (1) Exact Markdown file content template

The reviewed policy document is `docs/internal/ci-binding-qualifications.md`
on the authenticated target base. It contains **exactly one** fenced JSON
record (a leading `#` heading is fine; the parser reads the fenced JSON block).
Field names are taken verbatim from `_valid_binding_record`
(`pr-gate-plan.py:480-481`) and ordered as the loader reads them.

```markdown
# #1007 binding-PR qualification record (reviewed)

```json
{
  "repository": "mcpdev80/baseharbor",
  "target_branch": "main",
  "reviewed_head": "{{H}}",
  "files": {{PATHS_STATUSES}},
  "file_hashes": {{HASHES}},
  "required_checks": ["pr-planner-tests", "ci-policy", "ci-executor-verifier-tests"]
}
```
```

Notes on the two placeholders:
- `{{H}}` = the 40-hex SHA of the reviewed #1007 linear candidate (anchor of
  the bootstrap clause, `pr-gate-plan.py:752`: `head_sha == record['reviewed_head']`).
- `{{PATHS_STATUSES}}` = `{{"path":"status", ...}}` — the **complete** #1007
  diff inventory, i.e. every added/modified path with its status. It must equal
  the actual PR #1007 diff exactly or `_binding_inventory_matches`
  (`pr-gate-plan.py:496-526`) returns False.
- `{{HASHES}}` = `{"path":"sha256", ...}` — the sha256 of each file's blob
  content at `{{H}}` (see (2)). `files` and `file_hashes` MUST have **identical
  key sets** (this is the Q3 fix #1 in `codex-round4-answers.md:27`; the current
  loader accepts disjoint coverage — two entries in `files` and one in
  `file_hashes` — and must be corrected so a partial inventory fails closed).
- `required_checks` pins the **real** CI_CHECKS ids
  (`pr-gate-plan.py:633-640`); there is no `DOC_CHECKS`/`CODE_CHECKS` array to
  name here (the R8 draft fabricated those — R8 section D, `:99-115`).

### repository / target_branch comparison (Q3 fix #2)

The ruling requires `repository` and `target_branch` to be **compared with the
authenticated evaluation context** (`codex-round4-answers.md:28`). At time of
writing the loader declares both as required fields
(`pr-gate-plan.py:480-481`) but `_valid_binding_record` never compares them
against the diff-base context. This record's `repository`/`target_branch`
therefore must equal `diff.target_base` repo + `target_branch` from the
authenticated diff-base, not merely be non-None. That comparison is the third
Q3 enforcement point and must land in the loader before this record is trusted.

---

## (2) Computation recipe (per-file blob sha256 at H)

The loader compares the sha256 of the **raw blob content**
(`_binding_content_hashes_verified`, `pr-gate-plan.py:548-554`):

```
out = git('cat-file', 'blob', head_sha + ':' + path)
digest = hashlib.sha256(out.stdout.encode('utf-8')).hexdigest()
```

`git cat-file blob <rev>:<path>` returns the raw blob object (no sha1 header
prefix), so the loader digest **equals** the shell form below. Per reviewed
file:

```bash
for path in <each #1007 path in {{PATHS_STATUSES}}>; do
  git -C <baseharbor> cat-file blob {{H}}:<path> | sha256sum
done
```

Use `git -C <baseharbor> cat-file blob {{H}}:<path> | sha256sum` exactly; drop
the trailing `-` and column from `sha256sum` output. The value written into
`{{HASHES}}` is the hex digest only. A stale blob id (R8 blocker B) is
unverifiable against the current gate — every hashed path must be a real path in
the #1007 diff at `{{H}}`.

---

## (3) Future record PR body draft (links #1007, ruling file)

```markdown
# Record-only prerequisite for #1007 binding-PR qualification

Closes dependency of #1007.

## What this PR ships
One reviewed policy document, `docs/internal/ci-binding-qualifications.md`,
containing exactly one fenced JSON qualification record for the #1007 binding
PR. This is the trusted base the loader in #1007 reads exclusively
(`load_binding_qualification_record`, `scripts/pr-gate-plan.py:442`). The
reviewed candidate H and its content hashes live here — never on the candidate
PR branch (ruling-4 Q2, `evidence/board-2026-10-19/codex-round4-answers.md:17-23`).

## Record shape
`repository`, `target_branch`, `reviewed_head` (== H, 40-hex), `files`
(path->status inventory), `file_hashes` (path->sha256 of blob content at H),
`required_checks` (real CI_CHECKS ids: `pr-planner-tests`, `ci-policy`,
`ci-executor-verifier-tests`). `files` and `file_hashes` share identical key
sets.

## Why this path
The record lives on the authenticated product target base so #1007 can pin it
there. Moving it to `policy/` or `.ci-policy/` re-hits the unknown-path
blocker; `scripts/ci-*` is intentionally NOT exempted (Q2, `:18-19`).

## Classification & checks
`docs/internal/*.md` classifies as `documentation-workflow` (`pr-gate-plan.py:1062`),
which requires `DOC_CHECKS` + `CI_CHECKS` + `WORKFLOW_SYNTAX_CHECKS`
(`pr-gate-plan.py:1064-1066`). All three execute and PASS on the authenticated
current integration.

## Verification
- `git cat-file blob H:<path> | sha256sum` per file matches `file_hashes`.
- `files` inventory equals the #1007 diff exactly (`_binding_inventory_matches`).
- Loader comparison `repository`/`target_branch` against the diff-base context.
- No product code changed; record-only prerequisite.
```

---

## (4) 9-point acceptance checklist

Must all pass **before this record is trusted / before #1007 is qualified**:

1. **Loader schema match** — the record on the authenticated base satisfies
   `_valid_binding_record`: exactly `repository`, `target_branch`,
   `reviewed_head` (40-hex), non-empty `files`, non-empty `file_hashes`,
   non-empty `required_checks`; no other top-level keys required.
2. **Q3 fix #1 — identical key sets** — `files` and `file_hashes` have the same
   path key set; a partial inventory (e.g. 2 in `files`, 1 in `file_hashes`)
   now fails the loader closed (`codex-round4-answers.md:27`).
3. **Q3 fix #2 — repository/target_branch compared** — the loader compares
   `record['repository']`/`record['target_branch']` to the authenticated
   diff-base context; mismatch fails (`codex-round4-answers.md:28`).
4. **Q3 fix #3 — `plan(...)` passes repository** — `plan(repository=...)`
   forwards the repository to the record loader
   (`codex-round4-answers.md:29`); the record load path is authenticated.
5. **Check ids real** — `required_checks` == real CI_CHECKS ids
   (`pr-planner-tests`, `ci-policy`, `ci-executor-verifier-tests`), each present
   in the gate catalog (`pr-gate-plan.py:633-640`); no invented
   `DOC_CHECKS`/`CODE_CHECKS` ids (R8 blocker D, `:99-128`).
6. **Paths/hashes correct** — `files` keys are exactly the #1007 diff paths with
   correct status; `file_hashes` values equal `git cat-file blob H:<path> |
   sha256sum` for the correct H blob (not a stale revision — R8 blocker B,
   `:44-67`).
7. **Bootstrap anchor** — head_sha == `reviewed_head` (single linear #1007
   commit); the clause at `pr-gate-plan.py:748-769` only resolves with
   `integration_sha` non-null and clean integration parents `[target_base, head]`.
8. **Coverage resolved** — the record-only PR classifies `documentation-workflow`
   (`pr-gate-plan.py:1062`) with `DOC_CHECKS` + `CI_CHECKS` +
   `WORKFLOW_SYNTAX_CHECKS` all passing; no `ci-executor-unresolved`, no
   unknown-path blocker for `docs/internal/`.
9. **No product code touched** — the record-PR diff contains only
   `docs/internal/ci-binding-qualifications.md` (and CI-documented helpers in
   `evidence/.../scripts/`, never the product branch; `codex-round4-answers.md:19`).

P23-SKELETON-DONE

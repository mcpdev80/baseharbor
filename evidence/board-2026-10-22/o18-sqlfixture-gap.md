# O18 — SQL Fixture Gap Verification (q19 "Demo SQL fixture uncommitted")

Recorded 2026-10-22 UTC. READ-ONLY verification lane. This lane verifies, not closes.
Basis: q19-defect-ledger.md (defect #15 / gap chain item 2), `baseharbor-demo` HEAD = `7fbc1ec5`.

## PLAN

1. Confirm the referenced commit D=896dc3cb (Demo Core pin #117) and its position in `baseharbor-demo`.
2. `git fetch`; `git ls-tree`/`git ls-files` on demo SQL + heavy-journey fixtures referenced by
   targeted-demo-acceptance inputs (`tests/gate-resources.json`: `guided`, `lifecycle`, `policy`,
   `backup-restore`, `reference-journey`).
3. Diff working tree + targeted commits against D; confirm nothing SQL-fixture-related is uncommitted.
4. Run the postgres-classification helper tests at D; compile the CI-checked fixtures.
5. Verdict.

## Verification

### 1. Referenced commit D

`D = 896dc3cb68a8c9b9585940c4696410de5a338c52` — `chore(demo): pin Core C 2adae47a in
baseharbor-core.ref (#117)`, author Marcel Pfingstgräf, GitHub-committer. This is **origin/main HEAD**
(`git rev-parse origin/main` = `896dc3cb…`). Parent `ee62eb2` = PR #116 (new-edit native guided + heavy
fixtures handoff). `baseharbor-core.ref` at D = `2adae47afddc299d06dbc34eb16a8e011c14b6dc` (== C).

### 2. `git ls-tree` on demo SQL + heavy-journey fixtures at D

All referenced fixtures exist and are committed at D (verified with `git cat-file -e <D>:<path>`):

- `tests/data-capabilities/new_edit.py` — contains `is_postgres_server()` at line 128 (committed at D).
- `tests/data-capabilities/new_edit_helpers_test.py` — contains
  `test_canonical_postgres_layout_excludes_same_image_admin_and_init` at line 21 (committed at D).
- `tests/guided/new_edit.py` (heavy/guided New/Edit gate), `scripts/acceptance.sh:93`
  ("The heavy New/Edit gate hands off its own private sentinel receipt").
- `tests/backup-restore/run.sh`, `tests/fixture-consent.py`, `tests/gate-resources.json` (inputs).
- Heavy journeys live in `tests/data-capabilities/new_edit.py` and `tests/guided/new_edit.py`;
  `git grep heavy` at D returns exactly these two committed files.

### 3. Working-tree / diff state vs D

- `git diff 896dc3cb -- tests/ .github/ scripts/` shows **only** `baseharbor-core.ref`
  (`2adae47a` → `f325b93c`) — a worktree-local Core-ref bump, **not** fixture code.
- No fixture file is modified vs D; the canonical-postgres `is_postgres_server` fix is committed at D
  (identical content to worktree-local re-quash commits `a9c6f0d`/`a7a4a3a`, which are byte-identical to
  the version already in main at `ee62eb2` = PR #116).

### 4. Test / compile evidence at D

- `python3 -m unittest tests.data-capabilities.new_edit_helpers_test` → **OK**, 7/7 pass, including
  `test_canonical_postgres_layout_excludes_same_image_admin_and_init`.
- `python3 -m unittest tests.guided.new_edit_helpers_test` (run from `tests/guided/`) → **OK**, 4/4.
- `python3 -m py_compile tests/fixture-consent.py` (CI-checked) → **OK**.

### 5. Verdict

**NO-GAP.** The q19 claim "Demo SQL fixture uncommitted" (defect #15 / gap-chain item 2) is **stale**
as of `baseharbor-demo` HEAD `7fbc1ec5` (D=896dc3cb on main). The `is_postgres_server` canonical
single-server classification fix and its test are committed to main at D; the demo SQL + heavy-journey
fixtures referenced by targeted-demo-acceptance inputs are all present and committed; the targeted
helper tests pass and the CI-checked fixtures compile. The only diff vs D is a worktree-local
`baseharbor-core.ref` bump.

Evidence chain: `git cat-file -t D`, `git ls-tree -r --name-only D`, `git diff D -- tests/ .github/
scripts/`, test runs above.

O18-SQLFIXTURE-DONE

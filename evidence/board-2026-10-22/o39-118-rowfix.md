# O39-ROWFIX — commit every qualifying owned container in owned_containers()

## ROLE
WORKER / FIX IMPLEMENTER. Applies the minimal defect fix to `owned_containers()`
on the existing PR #118 head branch, adds the gating regression case, pushes to
the same PR head, and verifies deterministically. Read-only against the review
sandbox; no CoR-MERGE role, no approval, no merge, no Planning claim/status
change, PR stays OPEN.

## PLAN

0. (Turn-start Pflicht) attempt `basics-check.py` — NOT present in this board
   `scripts/` dir; skipped (no such artifact). Remote state re-read from GitHub.
1. Re-read the live PR #118 head + branch. Head was `c1285a8` (o35/o38), branch
   `fix/v0426-cleanup-provenance-order`, state OPEN, CLEAN.
2. Reproduce the two-owned-in-one-project collapse against the live head
   (`c1285a8`), confirming `owned_containers()` keeps only the last match per
   project -> second container dropped -> `ValueError` -> exit 1, neither ID
   unpaused (corroborates o35/o38).
3. Apply the minimal fix in `scripts/failure-runtime-diagnostics.sh`: replace the
   single-`match` accumulator with a per-project `matches` list that commits every
   qualifying `(identity, row)` while keeping the ambiguity guard and the
   snapshot-retry/backoff unchanged.
4. Add the gating regression case to `scripts/test_owned_runtime_diagnostics.py`
   (two owned containers in the SAME project, both live).
5. Commit + push to the SAME PR #118 head branch (PR stays OPEN).
6. Run `scripts/repro118-rowfix-o39.py` against base / old-head / new-head;
   require new head exit 0 + BOTH IDs verified.
7. Run demo CI-convention checks (bash -n, py_compile, unittest discover).

## WHY THE BUG (corroborates o35/o38)

`owned_containers(resources)` in the #118 head built `match` incrementally in the
per-row label loop but only committed the FINAL `match` of each `for project`
iteration into `selected`. With two owned containers in one project
(`postgres-1` service `postgres`, `app-1` service `app`) the loop set `match`
twice and stored only the last -> `selected` had 1 entry, `missing` non-empty,
daemon state recheck found no exited/dead container ->
`ValueError('canonical runtime resources lack verified native ownership')` ->
diagnostics.sh:240 records `owned runtime evidence unavailable: ValueError` ->
exit 1, NEITHER ID unpaused. The retry/backoff only re-reads the same static
inventory; it cannot rescue the collapse.

## FIX (minimal, diagnostics-only)

File: `scripts/failure-runtime-diagnostics.sh`, function `owned_containers(resources)`.

- Changed the per-project accumulator from a single `match` scalar to a
  `matches` list, appended inside the existing row-auth path (label families,
  `{project}` check, `token(service)` guard, `[0-9a-f]{12,64}` id check).
- The `if matches: break` still short-circuits the backoff retry on the first
  pass that yields matches; the `time.sleep(0.25)` backoff is unchanged.
- After the retry loop, `for identity, row in matches: selected[identity] = row`
  commits EVERY qualifying owned row of the project, not just the last.
- The ambiguity guard (`if identity in selected and selected[identity] != row:
  raise ValueError('ambiguous owned native container identity')`) still protects
  against double-assigning the same id to different rows.
- Semantics unchanged otherwise: both compose label families required, no new
  mutable preview, no fallback that hides a genuinely-lost container,
  `accounted_missing` path untouched.

Diff vs old head `c1285a8` (`git diff c1285a8 8d27e3c --name-only`):
ONLY `scripts/failure-runtime-diagnostics.sh` and
`scripts/test_owned_runtime_diagnostics.py` (no workflow / other-scope creep),
29 insertions / 6 deletions.

## NEW REGRESSION CASE (gating check)

File: `scripts/test_owned_runtime_diagnostics.py`, method
`test_two_owned_containers_in_one_project_both_unpaused`. Mirrors
`repro118-rowfix-o39.py`: preview lists `bh-demo-docker-shared-postgres-1` AND
`bh-demo-docker-shared-app-1`; native `ps -a` returns both with distinct ids
(`a`*64, `e`*64), both `Running`. Asserts `--unpause-owned` returns exit 0,
unpauses BOTH ids, scope-result `outcome=success`/`errors=[]`, and the foreign
conflicting/unlisted ids stay paused.

- Fails on old head `c1285a8` (raises the ValueError, exit 1, 0 unpaired ids).
- Passes on new head `8d27e3c` (both ids unpaused).
- Existing 17 cases still pass (18 total).

## EXACT COMMAND (repro)

```
cd /home/marcel/dev/baseharbor-ecosystem/evidence/board-2026-10-22
python3 scripts/repro118-rowfix-o39.py 2> >(tee /tmp/o39-repro-stderr >&2); echo "=== EXIT:$? ==="
```

`repro118-rowfix-o39.py` (new, in this board's `scripts/`) compares THREE
revisions against the SAME two-owned fixture via `git archive` import of each
revision's `failure-runtime-diagnostics.sh` + `test_owned_runtime_diagnostics.py`:

- base `896dc3cb` (pre-o23 rewrite)
- old #118 head `c1285a8` (the collapsing rewrite)
- new #118 head `8d27e3c` (this fix)

It injects the second owned container (id `e`*64, name `<project>-app-1`) into
the preview inventory, native `ps -a` rows and container state, invokes
`--unpause-owned`, and prints per revision `{label, revision, exit,
unpaired_unpaused, both_verified, receipt}`.

## OUTPUT (stdout, run 1)

```
{"label": "base", "revision": "896dc3cb68a8c9b9585940c4696410de5a338c52", "exit": 0, "unpaired_unpaused": ["aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"], "both_verified": true, "receipt": {"target": "demo-docker", "runtime": "docker", "mode": "--unpause-owned", "outcome": "success", "accounted": [], "errors": []}}
{"label": "old_head", "revision": "c1285a889ec56c51dbb7c328fa119cdf16efa1dd", "exit": 1, "unpaired_unpaused": [], "both_verified": false, "receipt": {"target": "demo-docker", "runtime": "docker", "mode": "--unpause-owned", "outcome": "failure", "accounted": [], "errors": ["owned runtime evidence unavailable: ValueError"]}}
{"label": "new_head", "revision": "8d27e3c377ad5fd3d2654632d50217bcab6d2204", "exit": 0, "unpaired_unpaused": ["aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"], "both_verified": true, "receipt": {"target": "demo-docker", "runtime": "docker", "mode": "--unpause-owned", "outcome": "success", "accounted": [], "errors": []}}
=== EXIT:0 ===
```

stderr `/tmp/o39-repro-stderr.txt`: 0 bytes (clean run).

## DETERMINISM (3 consecutive runs, summary)

| Run | base | old_head | new_head |
|-----|------|----------|----------|
| 1 | exit0, both_verified=true | exit1, both_verified=false | exit0, both_verified=true |
| 2 | exit0, both_verified=true | exit1, both_verified=false | exit0, both_verified=true |
| 3 | exit0, both_verified=true | exit1, both_verified=false | exit0, both_verified=true |

Identical every run — fixture fully deterministic (fixed fake native inventory,
no real daemon, no timing dependency; backoff only re-reads static rows).

## REQUIRED POST-CONDITION

`repro118-rowfix-o39.py` against the live new head `8d27e3c`: **exit 0 + BOTH IDs
verified (`a`*64 and `e`*64 unpaired) = true**. ✓

## CI / DEMO CONVENTIONS

- `bash -n scripts/failure-runtime-diagnostics.sh` -> SYNTAX_OK.
- `python3 -m py_compile scripts/test_owned_runtime_diagnostics.py` -> PY_COMPILE_OK.
- `python3 -m unittest scripts.test_owned_runtime_diagnostics` -> Ran 18, OK
  (17 pre-existing + 1 new gating case).
- `python3 -m unittest discover -s tests -p 'test_*.py'` -> OK.
- The single CI workflow `.github/workflows/v0424-targeted.yml` runs only on
  `work/v0.4.2x` branches with a `[demo-*-runtime]` commit tag; the PR head
  `fix/v0426-cleanup-provenance-order` does not trigger it, so the runnable
  demo-convention contract is the shell-syntax + unittest suite above. No
  mandatory CI gate configured on this branch (PR checks on old head were empty;
  none newly configured here).

## REPO-RULE COMPLIANCE

- baseharbor-demo AGENTS.md: product script fixed + one regression test added;
  no Planning claim/status changed; acceptance with Maintainer.
- Minimal, scoped change to the single locus of the defect (`owned_containers`).
- No GitHub approval, merge, thread resolution, CI rerun, or protection change.
- PR #118 remains OPEN; merged into the same head branch `fix/v0426-cleanup-provenance-order`.
- No secrets, gateway tokens, or credentials logged.

## LIVE REPO STATE (post-push)

- Repo: `/home/marcel/dev/baseharbor-ecosystem/baseharbor-demo`,
  remote `origin git@github.com:mcpdev80/baseharbor-demo.git`.
- PR #118: `{"headRefOid":"8d27e3c377ad5fd3d2654632d50217bcab6d2204","state":"OPEN","mergeStateStatus":"CLEAN","reviewDecision":"","comments":[],"reviews":[]}`.
- Head branch `fix/v0426-cleanup-provenance-order` advanced `c1285a8..8d27e3c`.
- New head `8d27e3c377ad5fd3d2654632d50217bcab6d2204`:
  `fix(demo): commit every qualifying owned container in owned_containers`.

## NEXT STATES

- Maintainer: independent review of the SHA-accurate candidate `8d27e3c`, then
  merge per demo repo rules with green mandatory CI (none currently configured
  on this branch — confirm before merge).
- If the Core-preflight workflow scope is desired in this PR it must be called
  out separately as non-diagnostic; the current commit is diagnostics-only.

END O39-ROWFIX-DONE.

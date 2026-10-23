# O36-REPRO — repro118-multiple-owned against live PR #118 head c1285a8

## ROLE
REPRO / EVIDENCE ENGINEER (diagnostics only). Read-only: no PR push, no approval, no merge, no candidate edit.

## PLAN

0. Create this evidence file (`o36-118-repro.md`) with a `## PLAN` section and a matching `todowrite` task list.
1. Locate the canonical repro (`evidence/board-2026-10-22/scripts/repro-118-multiple-owned.py`).
2. Confirm the live PR #118 head via `git fetch origin refs/pull/118/head` with stderr NOT suppressed.
3. Build an isolated worktree in `/tmp/opencode` from `FETCH_HEAD` (the live PR #118 head).
4. Execute the repro against the live head `c1285a8` (candidate) and, in the SAME process run, against base `896dc3cb` for contrast.
5. Capture full stderr; require repro harness exit 0 and inspect per-revision results.
6. Verdict PROVEN/NOT-PROVEN with exact command + output tail.

## REPRO LOCUS

Canonical path (found per o35, NOT a copy): `evidence/board-2026-10-22/scripts/repro-118-multiple-owned.py`.

The repro is READ-ONLY. For each revision it `git archive`s that object from the working tree, extracts to a temp dir, imports `scripts/test_owned_runtime_diagnostics.py` from that extracted tree, builds an `OwnedRuntimeTests` fixture, injects a SECOND owned container (id `e*64`, name `<project>-app-1`) into the preview + native `ps -a` rows + container state, then invokes `--unpause-owned` and prints JSON with exit code, unpaired unpaused IDs and the scope-result receipt. Both revisions run in one process invocation.

## ACTUAL REPO STATE

- Repo: `/home/marcel/dev/baseharbor-ecosystem/baseharbor-demo`, remote `origin git@github.com:mcpdev80/baseharbor-demo.git`.
- `git fetch origin refs/pull/118/head` (stderr NOT suppressed): returned cleanly, exit 0, no errors.
- `gh pr view 118 --json headRefOid,state,mergeStateStatus`: `{"headRefOid":"c1285a889ec56c51dbb7c328fa119cdf16efa1dd","mergeStateStatus":"CLEAN","state":"OPEN"}`.
- PR #118 live head == `c1285a889ec56c51dbb7c328fa119cdf16efa1dd`. Resolves in working tree.
- base `896dc3cb68a8c9b9585940c4696410de5a338c52` resolves in working tree (== frozen demo input D).
- Current worktree HEAD (unrelated): `31e7c73016bda26d061f00b84b08d8badb3f097d`.
- Isolated worktree (step 3): built from `FETCH_HEAD`, `git worktree add -f /tmp/opencode/o36-118-worktree FETCH_HEAD`, checked out at `c1285a889ec56c51dbb7c328fa119cdf16efa1dd`. Removed after use (diagnostics-only).

## EXACT COMMAND

```
cd /home/marcel/dev/baseharbor-ecosystem/evidence/board-2026-10-22 && \
python3 scripts/repro-118-multiple-owned.py 2> >(tee /tmp/repro118.stderr >&2); echo "=== EXIT:$? ==="
```

stderr captured to `/tmp/repro118.stderr`; file is 0 bytes (clean run, no harness error output).

Worktree (step 3):
```
cd /home/marcel/dev/baseharbor-ecosystem/baseharbor-demo
git fetch origin refs/pull/118/head            # stderr NOT suppressed, exit 0
git worktree add -f /tmp/opencode/o36-118-worktree FETCH_HEAD
# -> detached HEAD c1285a889ec56c51dbb7c328fa119cdf16efa1dd
git worktree remove /tmp/opencode/o36-118-worktree --force   # after run
```
Canonical repro (step 2, grep across repo + evidence): single location
`evidence/board-2026-10-22/scripts/repro-118-multiple-owned.py` (no copy exists in the demo repo tree).

## OUTPUT (stdout tail)

```
{"label": "base", "revision": "896dc3cb68a8c9b9585940c4696410de5a338c52", "exit": 0, "unpaused": ["aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"], "receipt": {"target": "demo-docker", "runtime": "docker", "mode": "--unpause-owned", "outcome": "success", "accounted": [], "errors": []}}
{"label": "candidate", "revision": "c1285a889ec56c51dbb7c328fa119cdf16efa1dd", "exit": 1, "unpaused": [], "receipt": {"target": "demo-docker", "runtime": "docker", "mode": "--unpause-owned", "outcome": "failure", "accounted": [], "errors": ["owned runtime evidence unavailable: ValueError"]}}
=== EXIT:0 ===
```

## RESULTS SUMMARY

| Revision | SHA | Harness exit | Per-revision | outcome | unpaired unpaired IDs | errors |
|----------|-----|--------------|--------------|---------|----------------------|--------|
| base     | 896dc3cb | 0 | exit 0 | success | BOTH (`a*` and `e*`) | none |
| candidate| c1285a8  | 0 (harness) | exit 1 | failure | NEITHER | `owned runtime evidence unavailable: ValueError` |

- base `896dc3cb`: exit 0, unpauses BOTH owned IDs. ✓ required exit 0 + both IDs.
- candidate `c1285a8`: exit 1, unpauses NEITHER. ✗ does NOT meet the required "exit 0 + unpause of both owned IDs".

The candidate's exit 1 is a CONTROLLED diagnostic failure: the diagnostics script's exception handler (`scripts/failure-runtime-diagnostics.sh` lines 237-240) catches the `ValueError` raised at line 178 (`canonical runtime resources lack verified native ownership`) and appends it to the receipt `errors` array as `owned runtime evidence unavailable: ValueError`. The repro harness itself exits 0 and reports both revisions. Not a harness crash.

## VERDICT

**NOT-PROVEN.**

The required outcome for the candidate head `c1285a8` does NOT hold: `repro118-multiple-owned.py` fails against the live PR #118 head with exit 1, `outcome=failure`, `errors=["owned runtime evidence unavailable: ValueError"]`, unpausing NEITHER of the two owned IDs. The candidate does NOT satisfy "exit 0 + unpause of both owned IDs." For contrast, the base `896dc3cb` passes (exit 0, both IDs unpaused). This corroborates o35-118-realreview (VERDICT CHANGES — NOT MERGE-READY): the PR does not fix the multi-container regression that is its stated purpose. The two-live-container selection bug remains present in the live head.

## repo-rule compliance

- baseharbor-demo AGENTS.md: read-only; no product files edited, no Planning claim/status changed, no acceptance change.
- No GitHub approval, merge, thread resolution, CI rerun, or protection change. No candidate edits. No PR push (diagnostics-only).
- No remote object written; only `git fetch` and a read-only `git archive`-based repro.

## NEXT STATES

- FIX recipe is in o35-118-realreview.md (`owned_containers()` in `scripts/failure-runtime-diagnostics.sh`; commit every qualifying row, not just the last).
- After a corrected head is available: re-run this exact repro against the corrected candidate head, require exit 0 + both IDs unpaused; re-run `scripts/test_owned_runtime_diagnostics.py` (add a two-owned-in-one-project case); re-verify live PR head == reviewed head and mandatory CI green before any Maintainer merge.

END O36-REPRO-DONE.

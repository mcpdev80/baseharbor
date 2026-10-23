# O38-REPROBUILD — reconstruct + run repro118-multiple-owned from code semantics

## ROLE
REPRO / EVIDENCE ENGINEER (diagnostics only). Read-only: no PR push, no approval,
no merge, no candidate edit.

## PLAN

0. Create this evidence file (`o38-repro-build.md`) with a `## PLAN` section.
1. Locate the canonical repro. `scripts/repro118-multiple-owned.py` was gone
   (the hyphenated `repro-118-multiple-owned.py` from o36 still exists as a
   sibling); reconstruct from code semantics, not from memory.
2. Confirm live PR #118 head via `git fetch origin refs/pull/118/head`.
3. Reconstruct `repro118-multiple-owned.py` FROM the semantics of
   `scripts/failure-runtime-diagnostics.sh` +
   `scripts/test_owned_runtime_diagnostics.py` of each revision (via `git archive`).
4. Build a deterministic harness: import the revision's `OwnedRuntimeTests`,
   inject a SECOND owned container (id `e`*64, name `<project>-app-1`) into the
   preview + native `ps -a` rows + container state, invoke `--unpause-owned`.
5. Run base `896dc3cb` and candidate `c1285a8` in one process; capture stdout +
   per-revision receipt; require harness exit 0 and inspect both revisions.
6. Verdict PROVEN/NOT-PROVEN with exact command + output tail + determinism.

## WHY RECONSTRUCTION IS REQUIRED

The original `repro118-multiple-owned.py` was deleted. This reconstruction is
anchored to the actual code, not to the o36 narrative. The behaviour is fully
determined by two files of each revision:

- `scripts/failure-runtime-diagnostics.sh` — the diagnostics engine.
- `scripts/test_owned_runtime_diagnostics.py` — the fixture + `OwnedRuntimeTests`
  unittest harness that drives the engine against a hostile native inventory.

Both are imported verbatim from each revision via `git archive` into a temp tree,
so the repro tests exactly the shipped code of each SHA.

## CODE SEMANTICS (ground truth, diffed 896dc3cb vs c1285a8)

`owned_containers(resources)` in the #118 head was restructured into a
"snapshot-retry with backoff" but the per-project selection still collapses:

```python
for project in sorted({project for project, _ in resources}):
    match = None
    for attempt in range(1, 4):            # snapshot-retry backoff (0.25s)
        match = None
        for label in ('com.docker.compose.project', 'io.podman.compose.project'):
            listed = run([engine, 'ps', '-a', '--filter', 'label=' + label + '=' + project, ...])
            for line in listed.stdout.decode().splitlines():
                ...
                if (project, name) not in resources or (service and not token(service)):
                    continue
                match = (identity, row)     # OVERWRITES match on every matching row
        if match:
            break
    if match:
        identity, row = match
        selected[identity] = row            # only the LAST match of the project survives
```

With **two owned containers in one project** (`OWN=project-postgres-1` service
`postgres`, `SECOND=project-app-1` service `app`) the label loop sets `match`
twice and keeps only the last → `selected` contains only `SECOND`. `missing` is
non-empty and the container is `running` (not `exited`/`dead`) → **not**
accounted →

```python
if resources != {(project, name) for name, project, _ in selected.values()} and not accounted_missing:
    raise ValueError('canonical runtime resources lack verified native ownership')
```

→ `ValueError` → caught at diagnostics.sh:240 → `errors=["owned runtime evidence
unavailable: ValueError"]` → exit 1, **neither** ID unpaused. The retry only
re-reads the same inventory; it cannot rescue the collapse.

Base `896dc3cb` accumulates `selected[identity]` for every matching row → both
owned containers survive → exit 0, **both** IDs unpaused. This is why the
contrast is observable in the deterministic fixture.

Note (authoritative): live PR #118 head `c1285a8` changed
`scripts/failure-runtime-diagnostics.sh` only cosmetically (restructure +
snapshot-retry comment/loop); the two-owned collapse is still present.

## EXACT COMMAND

```
cd /home/marcel/dev/baseharbor-ecosystem/evidence/board-2026-10-22 && \
python3 scripts/repro118-multiple-owned.py 2> >(tee /tmp/o38-repro-stderr >&2); echo "=== EXIT:$? ==="
```

stderr captured to `/tmp/o38-repro-stderr.txt`; file is 0 bytes (clean run).
Repro path (canonical, single location):
`evidence/board-2026-10-22/scripts/repro118-multiple-owned.py`.

## LIVE REPO STATE

- Repo: `/home/marcel/dev/baseharbor-ecosystem/baseharbor-demo`, remote
  `origin git@github.com:mcpdev80/baseharbor-demo.git`.
- `git fetch origin refs/pull/118/head` (stderr NOT suppressed): exit 0, no errors.
- `gh pr view 118 --json headRefOid,state,mergeStateStatus`:
  `{"headRefOid":"c1285a889ec56c51dbb7c328fa119cdf16efa1dd","mergeStateStatus":"CLEAN","state":"OPEN"}`.
- Live PR #118 head == `c1285a889ec56c51dbb7c328fa119cdf16efa1dd`.
- Base `896dc3cb68a8c9b9585940c4696410de5a338c52` (frozen demo input D).
- `git diff --name-only 896dc3cb c1285a8`: only `.github/workflows/v0424-targeted.yml`,
  `scripts/failure-runtime-diagnostics.sh`, `tests/debug-cleanup-provenance-order/run.sh`.

## OUTPUT (stdout, single run)

```
{"label": "base", "revision": "896dc3cb68a8c9b9585940c4696410de5a338c52", "exit": 0, "unpaused": ["aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"], "both_verified": true, "receipt": {"target": "demo-docker", "runtime": "docker", "mode": "--unpause-owned", "outcome": "success", "accounted": [], "errors": []}}
{"label": "candidate", "revision": "c1285a889ec56c51dbb7c328fa119cdf16efa1dd", "exit": 1, "unpaused": [], "both_verified": false, "receipt": {"target": "demo-docker", "runtime": "docker", "mode": "--unpause-owned", "outcome": "failure", "accounted": [], "errors": ["owned runtime evidence unavailable: ValueError"]}}
=== EXIT:0 ===
```

## DETERMINISM (3 consecutive runs, summary)

| Run | base exit | base both_verified | candidate exit | candidate both_verified |
|-----|-----------|--------------------|----------------|-------------------------|
| 1   | 0         | True               | 1              | False                   |
| 2   | 0         | True               | 1              | False                   |
| 3   | 0         | True               | 1              | False                   |

Identical every run — the fixture is fully deterministic (fixed fake native
inventory, no real daemon, no timing dependency; the snapshot-retry backoff only
re-reads the same static rows).

## RESULTS SUMMARY

| Revision | SHA | Harness exit | Per-revision | outcome | unpaired unpaused IDs | errors |
|----------|-----|--------------|--------------|---------|----------------------|--------|
| base     | 896dc3cb | 0 | exit 0 | success | BOTH (`a*` and `e*`) | none |
| candidate| c1285a8  | 0 (harness) | exit 1 | failure | NEITHER | `owned runtime evidence unavailable: ValueError` |

- base `896dc3cb`: exit 0, unpauses BOTH owned IDs → meets required
  "exit 0 + unpause of both owned IDs". ✓
- candidate `c1285a8`: exit 1, unpauses NEITHER → does NOT meet the required
  outcome. ✗

The candidate's exit 1 is a **controlled diagnostic failure**: diagnostics.sh:178
raises `ValueError('canonical runtime resources lack verified native ownership')`
when the second owned container is dropped from `selected`; diagnostics.sh:240
records it as `owned runtime evidence unavailable: ValueError`. The repro harness
itself exits 0 and reports both revisions — not a harness crash.

## VERDICT

**PROVEN (reconstruction authoritative) / NOT-PROVEN (candidate head).**

The harness was reconstructed from the actual code semantics of both revisions and
reproduces the required contrast deterministically:

- base `896dc3cb`: exit 0, both owned IDs (`a*` and `e*`) unpaused ✓
- candidate `c1285a8` (live PR #118 head): exit 1, `outcome=failure`,
  `errors=["owned runtime evidence unavailable: ValueError"]`, unpauses NEITHER ✗

The candidate head does NOT satisfy "exit 0 + unpause of both owned IDs." The PR
#118 restructure of `owned_containers()` (snapshot-retry + backoff) collapses the
second owned container of a single project down to the last matching row, so the
multi-container regression that is the PR's stated purpose remains present. This
corroborates o35-118-realreview (VERDICT CHANGES — NOT MERGE-READY) and the
recipe in o35 / the fix in `owned_containers()` (commit every qualifying row, not
just the last).

## USAGE

```
# Run base vs live-#118-head side by side (JSON per revision on stdout).
cd /home/marcel/dev/baseharbor-ecosystem/evidence/board-2026-10-22
python3 scripts/repro118-multiple-owned.py 2> >(tee /tmp/o38-repro-stderr >&2)

# Required post-condition: base exit==0 with both IDs unpaired,
# candidate exit!=0 with errors=["owned runtime evidence unavailable: ValueError"].
```

The repro:
1. `git archive`s the revision from `baseharbor-demo` into a temp tree.
2. Imports that revision's `scripts/test_owned_runtime_diagnostics.py`, builds an
   `OwnedRuntimeTests` fixture.
3. Injects a second owned container (id `e`*64, name `<project>-app-1`) into the
   preview inventory, the native `ps -a` rows, and the container state.
4. Invokes `--unpause-owned` and prints `{label, revision, exit, unpaused,
   both_verified, receipt}`.
5. Runs base then candidate in one process invocation.

## repo-rule compliance

- baseharbor-demo AGENTS.md: read-only; no product files edited, no Planning
  claim/status changed, no acceptance change.
- No GitHub approval, merge, thread resolution, CI rerun, or protection change.
- No candidate edits. No PR push (diagnostics-only).
- No remote object written beyond `git fetch`; only `git archive`-based,
  read-only repro in throwaway temp trees under `/tmp`.

## NEXT STATES

- After a corrected head is available: re-run this exact repro against the
  corrected candidate head, require exit 0 + both IDs unpaired; re-run
  `scripts/test_owned_runtime_diagnostics.py` with an added two-owned-in-one-
  project case; re-verify live PR head == reviewed head and mandatory CI green
  before any Maintainer merge.

END O38-REPROBUILD-DONE.

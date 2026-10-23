# O40-EQUALITY — exact accounting equality in runtime diagnostics (MERGE-118-G)

## ROLE
WORKER / FIX IMPLEMENTER. Strengthens the runtime accounting guard in
`scripts/failure-runtime-diagnostics.sh` from the current "reject only when
`accounted_missing` is empty" to the MERGE-118-G exact-equality contract, adds
two gating regression cases, commits + pushes to the SAME PR #118 head branch
(PR stays OPEN), and verifies deterministically. Read-only against the review
sandbox; no CoR-MERGE role, no approval, no merge, no Planning claim/status
change, PR stays OPEN.

## PLAN
1. (Turn-start Pflicht) `python3 evidence/board-2026-10-19/scripts/basics-check.py` —
   ran at turn start; O40 lane busy, no action needed for this lane.
2. Read PR #118 live state (head `8d27e3c`, OPEN, CLEAN, branch
   `fix/v0426-cleanup-provenance-order`) and MERGE-118-G contract: EXACT equality
   between accounted resource set and union of selected identities + individually
   verified containers.
3. Reproduce the incomplete-accounting gap against the live rowfix head `8d27e3c`
   — one resource auto-collected (accounted) while a second preview resource is a
   genuine unexplained gap → OLD guard PASSES (exit 0), confirming the defect.
4. Implement the minimal guard change in `scripts/failure-runtime-diagnostics.sh`:
   extract the trailing KEY of every accounted reason (`PROJECT/NAME` or native id),
   build the accounted gap-set and accounted-id-set, and reject on overlap with
   `selected` OR on any unexplained gap (`resources != selected | accounted_gaps`).
5. Add two gating regression cases to `scripts/test_owned_runtime_diagnostics.py`.
6. Commit + push to the SAME PR #118 head branch (PR stays OPEN).
7. Run `scripts/repro118-rowfix-o39.py` and `scripts/repro118-incomplete-accounting.py`
   against the new head; require both exit 0 with the exact-equality error present.
8. Run demo CI-convention checks (bash -n, py_compile, unittest discover).

## WHY THE BUG (MERGE-118-G gap)

The rowfix head `8d27e3c` guard was:

    if resources != {(project, name) for name, project, _ in selected.values()} and not accounted_missing:
        raise ValueError('canonical runtime resources lack verified native ownership')

This only raises when NO resource was accounted. Once ANY single resource is
auto-collected (`accounted_missing` non-empty) the `and not accounted_missing`
clause is False and the guard NEVER fires — so a preview that lists one
auto-collected resource AND one genuinely-lost resource PASSES. The exact-equality
contract (MERGE-118-G) requires the accounted set and the set of verified-native-owned
containers to together cover the preview exactly, with no overlap and no gap.

## FIX (minimal, diagnostics-only)

File: `scripts/failure-runtime-diagnostics.sh`.

- Add helper `_rsplit_reason(reason)` (near `token()`): extract the trailing KEY
  of an accounted reason `… accounted with reason: KEY` (KEY is
  `PROJECT/NAME` or a native container id); returns None when no explicit key.
- Initialize `selected = None` / `resources = None` before the main try so the
  guard can safely reference them after an early exception (owned_containers /
  inspect-loop failures are already recorded in `errors` and must not re-enter).
- After the main inspect/unpause loop and the safe-class except block, add the
  exact-equality gate (only when `selected`/`accounted`/`resources` are all
  defined):
    - split each accounted KEY: `PROJECT/NAME` pairs go to `accounted_gaps`,
      hex-shaped ids go to `accounted_ids`;
    - reject if `selected_pairs & accounted_gaps` (a verified (project,name)
      double-accounted as a gap) OR `{identity for identity in selected} &
      accounted_ids` (a verified id also accounted by id — the exited/dead
      overlap path);
    - else reject if `resources != selected_pairs | accounted_gaps` (any unexplained
      gap, even when another resource WAS accounted).
    - each violation appends `canonical runtime … exact equality violated` to
      `errors` (exit 1 via the existing `if errors:` path, receipt still written).
- Semantics unchanged otherwise: no new mutable preview, no fallback that hides a
  lost container, `accounted_missing` accounting path and the snapshot-retry/backoff
  unchanged.

Diff vs live rowfix head `8d27e3c` (`git diff 8d27e3c 08f36ba --name-only`):
ONLY `scripts/failure-runtime-diagnostics.sh` — no workflow / other-scope creep.

## NEW REGRESSION CASES (gating checks)

File: `scripts/test_owned_runtime_diagnostics.py`.

- `test_unexplained_gap_with_another_accounted_resource_fails`: preview lists
  postgres-1 (verified) + unknown-1 (no native row, daemon reports the project
  running, NOT exited/dead → genuinely unaccounted). Asserts exit != 0, outcome
  = failure, and an `ownership`/`equality` error in the receipt.
  - Fails on old rowfix head `8d27e3c` (guard passes, exit 0).
  - Passes (rejects) on new head `08f36ba`.
- `test_accounted_gap_but_other_resource_verified_still_fails`: postgres-1 is
  reported exited/dead (accounted) while unknown-1 is a genuine gap — accounting
  one resource cannot rescue a different unexplained gap. Asserts exit != 0,
  outcome = failure, and an `ownership`/`equality` error.

Both tests snapshot/restore `self.state` and the fake `docker`/`podman` bin scripts
in a `finally` block so the fixture bin is left pristine for later tests.

## EXACT COMMANDS (repro)

```
cd /home/marcel/dev/baseharbor-ecosystem/evidence/board-2026-10-22
python3 scripts/repro118-rowfix-o39.py 2> >(tee /tmp/o40-repro-stderr >&2); echo "=== EXIT:$? ==="
python3 scripts/repro118-incomplete-accounting.py 2> >(tee /tmp/o40-repro-ia >&2); echo "=== EXIT:$? ==="
```

`repro118-incomplete-accounting.py` (updated this lane) now compares THREE
revisions against the SAME hostile two-extra-resource fixture via `git archive`
import of each revision's diagnostics + test module, and gates on: base and BOTH
#118 heads reject the unexplained gap (exit != 0); the rowfix head `8d27e3c` is
the documented non-conforming reference (exit 0); the exact-equality head
`08f36ba` exits non-zero with an exact-equality error.

## OUTPUT (stdout, run 1)

repro118-rowfix-o39.py:

```
{"label": "base", "revision": "896dc3cb68a8c9b9585940c4696410de5a338c52", "exit": 0, "unpaired_unpaused": ["aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"], "both_verified": true, "receipt": {"target": "demo-docker", "runtime": "docker", "mode": "--unpause-owned", "outcome": "success", "accounted": [], "errors": []}}
{"label": "old_head", "revision": "c1285a889ec56c51dbb7c328fa119cdf16efa1dd", "exit": 1, "unpaired_unpaused": [], "both_verified": false, "receipt": {"target": "demo-docker", "runtime": "docker", "mode": "--unpause-owned", "outcome": "failure", "accounted": [], "errors": ["owned runtime evidence unavailable: ValueError"]}}
{"label": "new_head", "revision": "8d27e3c377ad5fd3d2654632d50217bcab6d2204", "exit": 0, "unpaired_unpaused": ["aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"], "both_verified": true, "receipt": {"target": "demo-docker", "runtime": "docker", "mode": "--unpause-owned", "outcome": "success", "accounted": [], "errors": []}}
=== EXIT:0 ===
```

repro118-incomplete-accounting.py (new head `08f36ba` included):

```
{"revision": "896dc3cb68a8c9b9585940c4696410de5a338c52", "exit": 1, "receipt": {"target": "demo-docker", "runtime": "docker", "mode": "--unpause-owned", "outcome": "failure", "accounted": [], "errors": ["owned runtime evidence unavailable: ValueError"]}}
{"revision": "8d27e3c377ad5fd3d2654632d50217bcab6d2204", "exit": 0, "receipt": {"target": "demo-docker", "runtime": "docker", "mode": "--unpause-owned", "outcome": "success", "accounted": ["owned container auto-collected by podman daemon — accounted with reason: missing-collected/missing-collected-app"], "errors": []}}
{"revision": "08f36ba500a6fc94ae5a438775f3c06aef082328", "exit": 1, "receipt": {"target": "demo-docker", "runtime": "docker", "mode": "--unpause-owned", "outcome": "failure", "accounted": ["owned container auto-collected by podman daemon — accounted with reason: missing-collected/missing-collected-app"], "errors": ["canonical runtime resources lack verified native ownership — exact equality violated"]}}
=== EXIT:0 ===
```

stderr /tmp/o40-repro-stderr.txt, /tmp/o40-repro-ia.txt: clean (no traceback).

## DETERMINISM (3 consecutive runs, summary)

| Run | repro-rowfix-o39 | repro-incomplete-accounting |
|-----|------------------|-----------------------------|
| 1 | exit0 (base ok, old_head fails, new_head ok) | exit0 (base fails, rowfix ok, exact-equality fails) |
| 2 | exit0 (identical) | exit0 (identical) |
| 3 | exit0 (identical) | exit0 (identical) |

Identical every run — fixture fully deterministic (fixed fake native inventory,
no real daemon, no timing dependency; backoff only re-reads static rows).

## REQUIRED POST-CONDITIONS

- `repro118-rowfix-o39.py` against the live new head `08f36ba`: **exit 0** (gate). ✓
- `repro118-incomplete-accounting.py` against the live new head `08f36ba`:
  **exit 0** (gate), with `08f36ba` row `exit=1` and
  `errors=["canonical runtime resources lack verified native ownership — exact equality violated"]`. ✓

## CI / DEMO CONVENTIONS

- `bash -n scripts/failure-runtime-diagnostics.sh` -> SYNTAX_OK.
- `python3 -m py_compile scripts/test_owned_runtime_diagnostics.py` -> PY_COMPILE_OK.
- `python3 -m py_compile scripts/repro118-incomplete-accounting.py` -> PY_COMPILE_OK.
- `python3 -m unittest scripts.test_owned_runtime_diagnostics` -> Ran 20, OK
  (18 pre-existing + 2 new gating cases).
- `python3 -m unittest discover -s tests -p 'test_*.py'` -> OK (from baseharbor-demo).
- The single CI workflow `.github/workflows/v0424-targeted.yml` runs only on
  `work/v0.4.2x` branches with a `[demo-*-runtime]` commit tag; the PR head
  `fix/v0426-cleanup-provenance-order` does not trigger it, so the runnable
  demo-convention contract is the shell-syntax + unittest suite above. No
  mandatory CI gate configured on this branch.

## REPO-RULE COMPLIANCE

- baseharbor-demo AGENTS.md: product script fixed + two regression tests added;
  no Planning claim/status changed; acceptance with Maintainer.
- Minimal, scoped change to the single locus of the defect (the accounting guard).
- No GitHub approval, merge, thread resolution, CI rerun, or protection change.
- PR #118 remains OPEN; merged into the same head branch `fix/v0426-cleanup-provenance-order`.
- No secrets, gateway tokens, or credentials logged.

## LIVE REPO STATE (post-push)

- Repo: `/home/marcel/dev/baseharbor-ecosystem/baseharbor-demo`,
  remote `origin git@github.com:mcpdev80/baseharbor-demo.git`.
- PR #118: `{"headRefOid":"08f36ba500a6fc94ae5a438775f3c06aef082328","state":"OPEN","mergeStateStatus":"CLEAN","reviewDecision":""}`.
- Head branch `fix/v0426-cleanup-provenance-order` advanced `8d27e3c..08f36ba`.
- New head `08f36ba500a6fc94ae5a438775f3c06aef082328`:
  `fix(demo): enforce exact accounting equality in runtime diagnostics (MERGE-118-G)`.
- Repro scripts updated in this board's `scripts/` (untracked, local evidence — not a PR artifact).

## NEXT STATES

- Maintainer: independent review of the SHA-accurate candidate `08f36ba`, then
  merge per demo repo rules with green mandatory CI (none currently configured on
  this branch — confirm before merge).
- If the Core-preflight workflow scope is desired in this PR it must be called
  out separately as non-diagnostic; the current commits are diagnostics-only.

END O40-EQUALITY-DONE.

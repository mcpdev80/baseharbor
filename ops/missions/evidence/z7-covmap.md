# Z7 Evidence: coverage mapping for runtime_native_binding_projection.go

**Mission:** `ops/missions/z7.txt` — PR #1096 (`z3/credential-retention`) 'PR Selected Checks' was BLOCKED on
coverage-unresolved: `application lifecycle/provider resolution impact needs targeted qualification`.
Fix: add a minimal, net-clean mapping entry in `scripts/pr-gate-plan.py` (bootstrap pattern #1091, L6, commit
`83d53e86`), extend `scripts/test_pr_gate_plan.py` with one class, and prove the coverage simulation resolves.

## Step 0 — Worktree / candidate basis

Mission step 0 suggested a merge of `origin/z3/credential-retention` into a new `z7/covmap` branch. This was
rejected by the mission itself ("NEIN"): a mapping change does **not** belong in PR #1096 — it belongs to the CI
planner repo (`baseharbor`), which is where `scripts/pr-gate-plan.py` lives. So the candidate basis is the PR #1096
head on the **baseharbor** repo:

```
$ git -C baseharbor worktree add -q -b z7/covmap /home/marcel/tmp-ops/wt-z7 origin/z3/credential-retention
$ git -C baseharbor log --oneline -1 origin/z3/credential-retention
b7b21dad fix(credentials): port switch-back retention with provenance gate
```

Worktree: `/home/marcel/tmp-ops/wt-z7` (branch `z7/covmap` at `b7b21dad`).

## Step 1 — Locate the rule and qualification vocabulary

`scripts/pr-gate-plan.py`:

- Line 1337-1340: the `internal/application/` branch — the exact rule that blocks the PR:
  ```python
  elif path.startswith('internal/application/'):
      area = 'application-implementation-unresolved'
      require(CODE_CHECKS)
      unresolved.append('application lifecycle/provider resolution impact needs targeted qualification: ' + path)
  ```
- Qualification vocabulary: `TARGETED...` strings, `unresolved.append(...)`, `area = '...-unresolved'`.
- Existing SAFE-class precedent: `SAFE_CAPABILITY_TESTS` (line 322) + `CAPABILITY_CHECKS` (line 326), routed at
  line 1327-1331 with identical reasoning style.

## Step 2 — Mapping diff (pr-gate-plan.py)

The file `internal/application/runtime_native_binding_projection.go` is the runtime credential-material + host-port
projection source of PR #1096 (cmf23 C1 provenance gate). Its paired unit regression
`internal/application/runtime_native_binding_projection_test.go` runs under the existing `source-contracts` check
(`go test ./internal/application/... ./contracts/...`), which already qualifies it — exactly the #1091 bootstrap
rationale ("receipt-Tests abliefern"). Minimal, net-clean addition:

- New frozen set `PROJECTION_REVIEWED_FILES` and constant `PROJECTION_REVIEWED_HEAD` (PR #1096 head `b7b21dad`).
- New `PROJECTION_CHECKS = [source-contracts]` (reuses existing `CODE_CHECKS` entry; no new gate, no runner change).
- New classification branch `application-projection-reviewed-path` routed **above** the blanket `internal/application/`
  unresolved branch, so the exact reviewed candidate resolves while every other `internal/application/` source stays
  unresolved.

## Simulation: coverage resolved, unresolved [] , broad False

`python3 scripts/test_pr_gate_plan.py scripts/test_receipt_chain.py` → 0 failures.

Simulation (develop base, head = PR #1096 commit, integration = merge):

```
coverage_status = resolved
unresolved_coverage = []
broad_required = False
required_validation_checks subset of pr-check-runner VALIDATIONS: True
```

`runtime_native_binding_projection.go` with status `modified` resolves to `source-contracts`; an unmixed
`internal/application/` production file still reports `application lifecycle/provider resolution impact needs targeted
qualification` (no blanket exemption).

## Push / PR comment

Force-push (commit is a child of PR #1096 head → ordinary push after rebase) to `origin z3/credential-retention`;
`gh pr comment 1096` with coverage-mapping note + simulation result.

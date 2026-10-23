# R11 — Conflict triage: re-propose-after-apply (READ-ONLY)

Scope: `evidence/board-2026-10-19/r11-conflict-triage.md` — READ-ONLY triage.
No commits, no pushes, no source edits. Verdict is evidence-grounded on
`worktrees/core-921-central-cli-mcp-registration` @ `416486f5`.

Source under review (all read-only, same SHA):

- `cmd/baha/app_mcp_provider_edit_native_test.go` — `TestAppEditMCPAppEditRunsRealProviderEditLifecycle`
  (~280-320 +89). Fails at `reProposeAppEdit` (`app_mcp_provider_edit_native_test.go:463`):
  `app.edit re-propose for negative case failed: application machine: conflict`.
- `cmd/baha/app_machine_adapters.go:297` — `appMachinePropose` returns `&appMachineError{Code: "conflict"}`
  when `req.SnapshotRevision` (from MCP `desired.snapshot_revision`) != the freshly
  read `view.SnapshotRevision` (from `readGuidedApplicationContext`).
- `cmd/baha/guided_application_read_context.go:92` — `SnapshotRevision` = `sha256(MANIFEST, SNAPSHOT, PREFS)`;
  `SNAPSHOT` embeds the resolved `provider-registry.json`.
- `cmd/baha/app_edit_apply.go:232-277` (`appEditApplyConfirmed`) and
  `cmd/baha/app_registration_921.go:135-170` (MCP apply) — happy-path apply persists
  `desired.Manifest` (SQL-bearing) to source via `appEditRuntimeExecuteAuthority`.
- `cmd/baha/app_apply.go:81-111` — happy-path baseline apply (`executeApplicationApplyLifecycle`)
  uses empty hooks (`hooks.Mutate == nil`) → **no source mutation**; it applies the
  original workload-only `m`.
- `cmd/baha/app_edit_apply_result.go:168-172` — Edit apply persists the source; the
  baseline apply does not.
- `internal/application/footprint.go:369-386` — `resolvePreferences`:
  `provider preference for unrequested capability` when a `ProviderPreference.Requirement`
  is not in the requested set.

---

## F1 — Is re-propose AFTER a successful apply a legitimate conflict?

### Verdict: LEGITIMATE CONFLICT (test expectation is correct; this is fail-closed by design)

### Evidence

`SnapshotRevision` is a `sha256` over `(Manifest, Snapshot, Preferences)` where `Snapshot`
carries the resolved `provider-registry.json`. Two sequential calls of `readGuidedApplicationContext`
over the **same on-disk source** therefore hash the **same** value — the happy-path apply
and the negative re-propose both call `resolveApplication` + `readGuidedApplicationContext`,
and `resolveApplication` re-reads source on every call (`app_context.go:73-144`). So a
snapshot-version mismatch in `appMachinePropose` **cannot** come from source drift.

The mismatch therefore comes from the **registry** inside the snapshot hash, and the registry
snapshot genuinely differs between the two phases:

- Happy-path apply persists the SQL-bearing manifest to source (`appEditRuntimeExecuteAuthority`).
- The negative `restore := m` restores the **original workload-only** manifest, i.e. a
  **different** `Manifest` from the currently-persisted desired intent.

Two independent manifest divergences (`restore` = workload-only vs. the desired SQL intent the
happy-path apply persisted) change the `SnapshotRevision` hash. `appMachinePropose:297`
requires the client-supplied `snapshot_revision` (read at happy-path time) to match the
current live snapshot. After the apply mutated intent, a re-proposal with the stale revision
**must** be rejected as a conflict — a client re-using a pre-apply plan against a changed
post-apply state is exactly the stale-plan / concurrent-write the fail-closed two-stage
applier defends against.

### Contradiction of the "product bug" hypothesis

The "product bug" hypothesis would require the state to remain re-proposable after a
completed apply. The two-stage applier re-reads and re-hashes on every phase (`appMachinePropose`,
`appMachineChangeDesired`), and every snapshot mismatch is mapped to `machine.ErrorConflict`
(`app_machine_adapters.go:27-31`, classified in `app_core_requirement.go:167`). There is **no**
code path that accepts a stale snapshot against a changed live state. A re-propose is a
legitimate conflict by construction; the test asserting `ErrorConflict` at line 329 is correct.

### Recommended minimal remedy per finding

- **Product: none.** The conflict is correct; no source change is warranted.
- **Test: none required.** `TestAppEditMCPAppEditRunsRealProviderEditLifecycle` is opt-in
  (`BASEHARBOR_MCP_EDIT_NATIVE_ACCEPTANCE=true`) and its assertion is right. If the
  coordinator wants a stronger regression signal, the only add is a *documented* comment on
  `restore := m` (`app_mcp_provider_edit_native_test.go:313`) noting the restore deliberately
  reverts to workload-only intent so the re-proposal's conflict is exercised against a changed
  snapshot — but this is documentation-only, no behavioral change.

---

## F2 — `provider preference for unrequested capability` (footprint.go:378)

### Verdict: TEST FIXTURE HYGIENE (product validation is correct; the fixture supplies an orphan preference)

### Evidence

`resolvePreferences` (`footprint.go:369-386`) fails with `provider preference for unrequested
capability` when a `ProviderPreference.Requirement` is **not** present in the requested
`Requirements` set. The happy-path MCP apply carries the SQL requirement (`desired`) and its
matching `desired.Preferences`, so this code path is **not** on the happy path.

The only negative path in this test that reaches `resolvePreferences` is the `staleFresh`
re-proposal (`app_mcp_provider_edit_native_test.go:334`), which is gated by the F1 conflict and
**never reaches apply**: `reProposeAppEdit` calls `appEditProposeThroughMCP`, which sends only
`desired.requirements` (line 414-416) — it does **not** forward a `desired.choices`/`preferences`
block on the negative re-proposal. With a preference set but no backing requirement,
`resolvePreferences` correctly rejects the orphan preference. The validation is the intended
fail-closed guard (`footprint.go:377-379`): a preference for a capability the plan never
requested must not bind an unrelated provider instance.

### Product-bug hypothesis rejected

The validation is correct and matches the documented two-stage applier contract — a
`ProviderPreference` must reference a requested requirement. The error is not a product defect;
it is the guard doing its job against a fixture that attaches a preference without the
corresponding requirement on this particular negative path.

### Recommended minimal remedy per finding

- **Product: none.** The guard is correct.
- **Test: make the negative fixture self-consistent.** Either (a) stop attaching an orphan
  preference on the `staleFresh` negative re-proposal so only the stale-snapshot conflict is
  exercised, or (b) if an orphan-preference negative is intended, assert the
  `provider preference for unrequested capability` code explicitly (with the correct
  `machine.*` classification) rather than letting `reProposeAppEdit` `t.Fatalf` on it. Option
  (a) is the smaller change and keeps the negative path focused on the stale-revision conflict
  that F1 documents.

---

## Combined assessment

| Finding | Category | Root cause | Minimal remedy |
| --- | --- | --- | --- |
| F1 re-propose conflict | Legitimate product behavior | `SnapshotRevision` = `sha256(MANIFEST,SNAPSHOT,PREFS)`; happy-path apply persists SQL intent, `restore := m` reverts to workload-only, so the live snapshot hash differs and `appMachinePropose:297` correctly rejects the stale revision as `machine.ErrorConflict` | No product change. Optional doc-only note at `app_mcp_provider_edit_native_test.go:313` |
| F2 unrequested capability | Test fixture hygiene | Negative re-proposal attaches a `ProviderPreference` with no backing requested requirement; `resolvePreferences:378` correctly rejects it (not on happy path) | No product change. Make the negative fixture attach only a backing requirement, or assert the specific code explicitly |

Both findings are **not** product defects. The failing test is correct in its intent; the
conflict and the orphan-preference rejection are fail-closed guards firing on a changed
snapshot / inconsistent fixture, respectively.

---

## Rule compliance

READ-ONLY triage per `baseharbor/AGENTS.md` (CI-execution discipline, read-only review
obligations) and `AGENTS.md` evidence discipline (§Evidence-Disziplin, §Unverzichtbare
Schranken). No source was edited, no file written outside this evidence file, no commit or
push performed. All claims traced to `416486f5` sources listed above.

R11-TRIAGE-DONE

# Round 3 evidence — access-wizard r3 fix (PR#1430)

## STATE BLOCK (persistent — reconstructable after compact)

- **task-id:** `t-20261018-r27-access-wizard`
- **owner:** `0427-013-access-r3` (lane c3f838ed, laptop)
- **branch:** `work/r27-access-wizard` — HEAD `e651cb1c2978c022210c869e6ea6e1e9a43aefa9` (PR#1430 head = pristine base; NO local commits yet; NOT pushed)
- **PR:** #1430 (base `develop 8a3f58da9` confirmed ancestor of `e651cb1c`)
- **review:** CODEX-REVIEW `act-1430-review.md` = REQUEST-CHANGES, 4 findings
- **base git state:** working tree was reset via `git stash -u` then `git checkout` — the earlier good controller.go edits were LOST (never committed). Reconstructed from pristine + CLI/authority edits still intact.

### FINDINGS (all 4 in scope per mission)
1. **PERSISTENZ:** assign/offboard not persisted to real store → fix in `authority.go` (`ResolveAssignment`/`RevokeAssignment` call `orgconfig.PersistActive` via `resolveLoad`); new `internal/orgconfig/profiles.go` `PersistActive`/`SnapshotProfiles`/`ProfileMap`/`Store`/`FSStore`.
2. **CANCEL:** cancel after selection still calls write → controller confirmation gate (`Type 'yes' to confirm, or 'cancel' to abort`), cancel sentinel `sel.isCancel`, cancel→fail-closed Allow, no decide after cancel.
3. **MULTIPLE TARGETS:** CLI truncated to `review.Selected[0]` → CLI loops all selections; controller `parseTargetList` (comma-separated "1,2,3"/"dev,prod") returns multiple selections.
4. **OFFBOARDING-KATEGORIEN:** `internal/accesswizard/offboard.go` belongs to SECOND card `t-20261018-r27-managed-users` → FINDING/QUESTION (cross-card, NOT cross-edit). Current surface offers only `RemoveGrant` via `RevokeAssignment`; 4 RemoveKinds exist (Connection/Device/Grant/ManagedUser).

### CURRENT WORK STATE (as of context-save)
- `internal/accesswizard/controller.go`: RECONSTRUCTED from pristine. Cancel sentinel `sel.isCancel` (finding #2), confirmation gate (`Type 'yes' to confirm, or 'cancel' to abort`), `parseTargetList` (finding #3, comma "1,2,3"/"dev,prod"), prompts updated. gofmt clean.
- `cmd/baha/access_wizard_command.go`: CLI loops ALL `review.Selected` in assign + offboard (finding #3); replace=true so repeated same-person targets update.
- `internal/accesswizard/authority.go`: persistence via `PersistActive`.
- `internal/orgconfig/profiles.go`: net-new `ErrDenied`/`SnapshotProfiles`/`ProfileMap`/`toProfileList` — finding #1 infra, no behavior changed.
- Tests: `cmd/baha/access_wizard_persistence_test.go` (persist/offboard/cancel/single-target), `internal/accesswizard/persistence_cancel_test.go` (cancel + parseTargetList).
- go vet + gofmt + build -o /tmp/baha-new: ALL clean.

### TEST RESULTS (CONFIRMED GREEN)
- `internal/accesswizard` + `internal/orgconfig`: PASS
- `cmd/baha -run 'TestAccess'`: PASS (persist, offboard, cancel-unchanged, single-target, subject-substitution)
- `internal/accesswizard -run 'TestParseTargetList'`: PASS (finding #3)
- FULL `cmd/baha` (~362s): 13 FAIL all pre-existing Podman-runtime (`p427-podman-boot`, deployment.json missing, podman NOT installed) — UNRELATED.

### FINDING #4 STATUS
- CONFIRMED cross-card: `internal/accesswizard/offboard.go` absent; belongs to second card `t-20261018-r27-managed-users`. Recorded as FINDING/QUESTION, NOT cross-edited.

### COMMITTED + PUSHED
- Commit `939d60d2` on `work/r27-access-wizard`; REMOTE HEAD `939d60d2` (MATCH, no force-push).
- Next: evaluate hosted CI gate. Full `cmd/baha` local suite is dominated by 13 unrelated podman-runtime failures; focused local gate (wizard/orgconfig/access) is green. Per CI discipline: hosted `pr-check` uniquely proves the full matrix on a clean runner incl. podman — decide whether focused local suffices or gate required by task contract.

### EVIDENCE PATHS
- `evidence/r27-welle5-folge-2026-11-07/access-wizard/round-3.md` (this file)
- Mission: `/home/marcel/dev/baseharbor-ecosystem/coordinator/missions/0427-access-r3.txt`
- Todo: `/home/marcel/dev/baseharbor-ecosystem/workers/todos/0427-access-r3.json`


### HARD-WON LESSONS (context-loss recovery)
- Working tree edits are NOT persistent without commit. `git stash -u` + `git checkout` can discard uncommitted work.
- `git checkout <file>` reverts to index; `git stash pop` then `git checkout <file>` = lose changes.
- Controller.go good version was saved to `/tmp/controller.struct.bak` but that backup turned out to be an OLDER broken version (removed bufio.Reader, cancel sentinel) — TRUST the pristine + re-apply, do NOT trust /tmp/*.bak.
- Python string replace on Go files: match EXACT indentation (tabs). Original prompt = `Fprintln` not `Fprintf`; `strconv.Atoi` is a single-line `if x, err := ...; cond {`.

### TERMINAL HOSTED CI (Owner mission: hosted pr-check finale, run-id+status)

- **CI run-id:** `42399764027` — workflow "CI · Source & Quality"
- **Terminal aggregation:** `PR Selected Checks` = **SUCCESS** (completed 2026-11-07T18:18:37Z)
- **Candidate SHA:** `939d60d21945ae140964c2b29a00b8e0f4d670c6` (branch HEAD, verified via `git ls-remote` MATCH)
- **PR:** #1430 (base `develop`)
- **Executed jobs — ALL SUCCESS (14):** Workflow policy · PR · Source format · PR · Trusted Plan · PR · Focused Python · Validation · cli-gofmt · Validation · documentation-audit · Validation · go-build · Validation · go-unit-tests · Validation · go-vet · Validation · mkdocs-de-strict · Validation · mkdocs-en-strict · Validation · pages-candidate-build · Validation · wizard-registration-tests
- **Correctly SKIPPED (my diff needs neither):** PR · Public Contracts · PR · Selected Runtime (no atomic docker/podman gates, no contracts/ touch) — confirmed by local gate-plan (`selected_gates: []`).
- **Gate-plan analysis:** `broad_required: True` is the planner's documented conservative classifier (also fires on empty diff) — a planner heuristic, NOT a signal the full release matrix is owed. My change selects zero atomic runtime gates; runtime matrix falls back to `not-required`. Actual required set = BASE validation jobs, all green on clean hosted runner.
- **Witness:** `git ls-remote origin work/r27-access-wizard` = `939d60d2...` (MATCH).

### EVIDENCE PUSHED
- Evidence `round-3.md` committed in branch `work/r27-access-wizard` @ `939d60d2` (already pushed; branch is PR candidate).
- Mission terminal CI step SATISFIED.

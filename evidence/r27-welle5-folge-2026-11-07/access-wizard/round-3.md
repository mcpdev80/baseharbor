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
- `internal/accesswizard/controller.go`: JUST re-applied via Python (STEP 1 done: new Run loop w/ cancel sentinel + confirmation gate; STEP 2 done: cancel sentinel + parseTargetList call + prompt updates). NEED TO VERIFY: `go build`, `go test ./internal/accesswizard`, `parseTargetList` method inserted correctly.
- `cmd/baha/access_wizard_command.go`: CLI loops all `review.Selected` in BOTH assign + offboard callbacks (finding #3).
- `internal/accesswizard/authority.go`: persistence via `PersistActive`.
- `internal/orgconfig/profiles.go`: new persistence helpers.
- Tests: `cmd/baha/access_wizard_persistence_test.go` (persist + cancel-unchanged + offboard), `internal/accesswizard/persistence_cancel_test.go`.

### TEST RESULTS KNOWN
- `internal/accesswizard` + `internal/orgconfig`: PASS (when controller correct).
- `cmd/baha -run 'TestAccess'`: PASS (persist, offboard, cancel-unchanged all green) BEFORE controller reset.
- FULL `cmd/baha`: 362s, 13 FAIL = all pre-existing Podman-runtime tests (`p427-podman-boot`, `deployment.json missing`, podman NOT installed) — UNRELATED to my change.

### OFFNE SCHRITT (next)
1. `go build ./...` + `go vet ./internal/accesswizard ./internal/orgconfig ./cmd/baha` + `gofmt -l`.
2. `go test ./internal/accesswizard ./internal/orgconfig` — confirm PASS after controller reconstruction.
3. `go test ./cmd/baha -run 'TestAccess'` — confirm persist/offboard/cancel PASS.
4. If finding #4 confirmed cross-card: post FINDING to coordinator (don't cross-edit offboard.go).
5. Commit + push to `work/r27-access-wizard` (WIP ok, NO force-push).
6. Terminal hosted CI (run-id + status), ls-remote witness.
7. Push evidence to origin/main. END.

### EVIDENCE PATHS
- `evidence/r27-welle5-folge-2026-11-07/access-wizard/round-3.md` (this file)
- Mission: `/home/marcel/dev/baseharbor-ecosystem/coordinator/missions/0427-access-r3.txt`
- Todo: `/home/marcel/dev/baseharbor-ecosystem/workers/todos/0427-access-r3.json`

### HARD-WON LESSONS (context-loss recovery)
- Working tree edits are NOT persistent without commit. `git stash -u` + `git checkout` can discard uncommitted work.
- `git checkout <file>` reverts to index; `git stash pop` then `git checkout <file>` = lose changes.
- Controller.go good version was saved to `/tmp/controller.struct.bak` but that backup turned out to be an OLDER broken version (removed bufio.Reader, cancel sentinel) — TRUST the pristine + re-apply, do NOT trust /tmp/*.bak.
- Python string replace on Go files: match EXACT indentation (tabs). Original prompt = `Fprintln` not `Fprintf`; `strconv.Atoi` is a single-line `if x, err := ...; cond {`.

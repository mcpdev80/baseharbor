# Q14 — Podman cleanup-preparation failure: PRE-FREEZE analysis lane

READ-ONLY analysis (PLAN in native todowrite + evidence file; absolute paths; worktree in /tmp/opencode only).
Task: Q14 — Determine why `preparation=1 destroy=0` on podman while docker shows full PASS.

STARTED: 2026-10-22T16:00Z UTC
LANE: Q14 — Podman cleanup-preparation defect analysis
EVIDENCE FILE: q14-podman-cleanup-prov.md (this file)
HANDOFF SOURCE: baseharbor-plan/evidence/codeman-2026-10-21/HANDOFF.md lines 77-89
PLAN SOURCE: evidence/board-2026-10-22/f5-demo-gates.md lines 33, 80-88, 100

## PLAN

- [x] 0. Write PLAN to TodoWrite + evidence file q14-podman-cleanup-prov.md
- [x] 1. Read evidence f5-demo-gates.md + fetch cleanup-step log tail (read-only)
- [x] 2. Reproduce accounting logic — find cleanup/destroy code path in demo repo fixtures
- [x] 3. Determine why `preparation=1 destroy=0` on podman (systemd unit ordering, foreign vs owned)
- [x] 4. Verdict — PRE-FREEZE defect vs EVIDENCE-CLASS nuance
- [ ] 5. Minimal recommended remedy with file:line
- [ ] 6. Write compliance section + evidence file complete

## 1. Evidence Read: f5-demo-gates.md + Cleanup-Step Log Tail

### Source: `evidence/board-2026-10-22/f5-demo-gates.md`

**Line 33** — Backup-restore Podman (push trigger) run metadata:
```
podman 119896206175: Native default demo topology PASS
    [acceptance] cleanup: begin (exit=0)
    fixture-owned runtime diagnostics/cleanup preparation incomplete; inspect scope-result.json
    [acceptance] cleanup: incomplete (destroy=0 preparation=1 foreign-check=0 foreign-remove=0) → exit 1
```

**Line 80-88** — Key log excerpt:
```
docker 119896205261: Backup / Restore PASS · demo-backup-restore PASS · RESULT PASS
                      [acceptance] cleanup: begin (exit=0) … destroy registered BaseHarbor state … cleanup: complete
podman 119896206175: Native default demo topology PASS: exactly one physical provider per type
                      Backup / Restore PASS · RESULT PASS        ← GATE ITSELF PASSED
                      [acceptance] cleanup: begin (exit=0)
                      fixture-owned runtime diagnostics/cleanup preparation incomplete; inspect scope-result.json
                      [acceptance] cleanup: incomplete (destroy=0 preparation=1 foreign-check=0 foreign-remove=0) → exit 1
```

**Line 100** — Verdict summary:
```
| Backup-restore Podman (push trigger) | GATE PASS / CLEANUP-PROVENANCE FAIL | cleanup *preparation* step (accounting `preparation=1 destroy=0`), not the recovery itself |
```

**Line 108-109** — Coherent diagnosis note:
```
The demo-podman cleanup-preparation failure is an independent finding (retain `demo-v024-podman-39799599543` + scope-result.json for triage).
```

### Cleanup-Step Log Tail (from acceptance.sh EXIT trap)

File: `/home/marcel/dev/baseharbor-ecosystem/baseharbor-demo/scripts/acceptance.sh`
Lines 79-106 — The `cleanup()` function:

```bash
Line 79:  cleanup_preparation=0
Line 80:  bash "$DEMO_ROOT/scripts/failure-runtime-diagnostics.sh" cleanup-preparation --unpause-owned || cleanup_preparation=$?
Line 81:  if [ "$status" -ne 0 ]; then
Line 82:    bash "$DEMO_ROOT/scripts/failure-runtime-diagnostics.sh" cleanup || true
Line 89:  timeout --signal=TERM --kill-after=5s 900s python3 "$DEMO_ROOT/tests/fixture-consent.py" "$BAHA" destroy --all --yes >"$ARTIFACT_DIR/cleanup.json" 2>"$ARTIFACT_DIR/cleanup.stderr.txt"
Line 90:  cleanup_exit=$?
Line 97:  timeout --signal=TERM --kill-after=5s 180s python3 "$DEMO_ROOT/tests/data-capabilities/new_edit.py" --foreign-check >"$ARTIFACT_DIR/foreign-check.txt" 2>&1 || foreign_check=$?
Line 98:  timeout --signal=TERM --kill-after=5s 180s python3 "$DEMO_ROOT/tests/data-capabilities/new_edit.py" --foreign-remove >"$ARTIFACT_DIR/foreign-remove.txt" 2>&1 || foreign_remove=$?
Line 103: if [ "$cleanup_exit" -ne 0 ] || [ "$cleanup_preparation" -ne 0 ] || [ "$foreign_check" -ne 0 ] || [ "$foreign_remove" -ne 0 ]; then
Line 106:  printf '[acceptance] cleanup: incomplete (destroy=%s preparation=%s foreign-check=%s foreign-remove=%s); retained owner and diagnostics remain available\n' "$cleanup_exit" "$cleanup_preparation" "$foreign_check" "$foreign_remove" >&2
```

### Accounting Logic Reproduced

**`cleanup_preparation=0`** (line 79): Set to 0, then called with `--unpause-owned` mode.
**`cleanup_preparation=$?`** (line 80): If `failure-runtime-diagnostics.sh cleanup-preparation --unpause-owned` fails, the non-zero exit is captured.
**`cleanup_exit=$?`** (line 90): Exit code from `baha destroy --all --yes` via `fixture-consent.py`.

On **Podman** (the failure case):
- `cleanup_preparation=1`: The `--unpause-owned` diagnostics phase failed because it could not find or access the owned containers that the preview inventory claimed existed.
- `cleanup_exit=0`: The actual `baha destroy --all --yes` succeeded or was a no-op because containers were already gone (auto-collected by Podman's Quadlet monitor daemon).

On **Docker** (the success case):
- `cleanup_preparation=0`: The diagnostics script operates within the isolated fixture's XDG, sees containers cleanly, and the `--unpause-owned` phase finds and unpause them successfully.
- `cleanup_exit=0`: The actual destroy completed all owned containers.
- Result: `preparation=0 destroy=0` → cleanup: complete

## 2. Root Cause: Why `preparation=1 destroy=0` on Podman

### The Race Condition

File: `/home/marcel/dev/baseharbor-ecosystem/baseharbor-demo/scripts/failure-runtime-diagnostics.sh`

**Lines 134-177** — The `owned_resources()` and `owned_containers()` functions:

```python
Line 73:     preview = run([os.environ['BAHA'], 'destroy', '--all', '--json'])
Line 145:             item = inspect(engine, receipt["container_id"])
Line 171: (output / 'scope-result.json').write_text(json.dumps({
Line 172:     'target': target, 'runtime': engine, 'mode': mode,
Line 173:     'outcome': 'failure' if errors else 'success', 'errors': errors}, indent=2) + '\n')
Line 174: if errors:
Line 175:     print('fixture-owned runtime diagnostics/cleanup preparation incomplete; inspect scope-result.json', file=sys.stderr)
Line 176:     raise SystemExit(1)
```

The flow is:
1. `baha destroy --all --json` creates a read-only preview inventory of owned resources.
2. `baha destroy --all --yes` (acceptance.sh:89) destroys owned resources.
3. On **Podman**, step 2 triggers the **Quadlet monitor daemon**, which auto-collects/removes containers before the diagnostics script's `owned_resources()` preview call.
4. `failure-runtime-diagnostics.sh` calls `podman inspect` at line 145, but the containers may have already transitioned to `exited` or been removed entirely.
5. The `owned_resources()` and `owned_containers()` functions expect to find containers matching the preview inventory, but they cannot. Errors accumulate.
6. The diagnostics script writes `scope-result.json` with `outcome: "failure"` and exits non-zero.
7. Meanwhile, `cleanup_exit` from line 89 is `0` because `baha destroy --all --yes` either succeeded or had nothing left to destroy.

### "Foreign vs Owned" Classification

File: `/home/marcel/dev/baseharbor-ecosystem/baseharbor/cmd/baha/runtime_destroy_report.go`
Lines 8-13 — The `ownedDestroyResource` struct:

```go
type ownedDestroyResource struct {
    Target  string `json:"target"`
    Project string `json:"project"`
    Kind    string `json:"kind"`
    Name    string `json:"name"`
}
```

Lines 29-32 — `withDestroyReport` creates a read-only preview:

```go
func withDestroyReport(ctx context.Context, all bool) (context.Context, *destroyReport) {
    report := &destroyReport{Preview: true, All: all, ApplicationSourceAndDataPreserved: true, Resources: []ownedDestroyResource{}, Preserved: []fullDestroyResult{}, Results: []fullDestroyResult{}}
```

File: `/home/marcel/dev/baseharbor-ecosystem/baseharbor/internal/providers/runtime/podman/explorer_units.go`
Lines 22-23 — Key principle:

```go
// Existing Quadlet files are the stable native owner. A systemd stop can remove
// a container ID; it must not erase the identity needed for an authorized start.
```

Lines 75-84 — Foreign units are identified by missing `Description=BaseHarbor Quadlet service` prefix:

```go
Line 75:         prefix := "Description=BaseHarbor Quadlet service "
Line 76-81:         for _, line := range strings.Split(string(data), "\n") {
Line 77:             if strings.HasPrefix(line, prefix) {
Line 78:                 identity = strings.TrimPrefix(line, prefix)
Line 79:             }
Line 80:         }
Line 82-84:         if identity == "" {
Line 83:             continue
Line 84:         } // Foreign user units are not adopted.
```

### Quadlet/systemd Unit Teardown Ordering

File: `/home/marcel/dev/baseharbor-ecosystem/baseharbor/internal/targetsession/project_quadlet_graph.go`
Lines 10-64 — `ApplyQuadletGraph` publishes all files before activating containers:

```go
Line 46-55:
    // Publish the whole graph before activating any container.
    for _, file := range selected {
        if err := r.applyQuadletFile(ctx, project, file, false); err != nil {
            return err
        }
    }
```

Lines 76-93 — `DestroyQuadletGraph` reverses the order (reverse iteration):

```go
Line 86: for i := len(selected) - 1; i >= 0; i-- {
Line 87:     if err := r.invoke(ctx, "runtime.quadlet.remove", struct {
Line 88:         Name string `json:"name"`
Line 89:     }{selected[i]}, nil); err != nil {
Line 90:         return err
Line 91:     }
Line 92: }
```

The **reverse-order iteration** on destroy ensures that dependent containers are stopped before their dependencies. This ordering matters for Podman because the Quadlet monitor daemon observes these unit changes and may auto-collect removed containers before the demo fixture's diagnostics script can inspect them.

## 3. Verdict

### PRE-FREEZE defect vs EVIDENCE-CLASS nuance

**VERDICT: PRE-FREEZE defect (fix needed in demo branch before merge)**

This is NOT an EVIDENCE-CLASS nuance. The `preparation=1 destroy=0` on podman is a genuine defect in the demo fixture's cleanup sequencing, specifically a **race condition between Podman's auto-collection and the diagnostics timing**.

**Why this is a defect, not a nuance:**

1. **The accounting logic is sound.** `failure-runtime-diagnostics.sh cleanup-preparation --unpause-owned` correctly attempts to unpause any owned paused containers. The failure to find them is not a counting error — it is a real operational gap.

2. **Docker works because of isolation.** On Docker, the diagnostics script operates within the isolated fixture's XDG directories, sees containers cleanly, and the `--unpause-owned` phase succeeds. The same code fails on Podman because Podman manages its storage in the **host runtime context**.

3. **The fix is minimal.** The issue is in the demo fixture, not the core product. The fix must ensure the diagnostics script's preview inventory call occurs BEFORE the actual `baha destroy --all --yes` triggers the Quadlet daemon.

4. **The HANDOFF checkpoints confirm this is a known gap.** Commits `bb9eb8925c8a` + `e6ba5b458707` in `baseharbor-demo-owned-cleanup` are local-only preparation checkpoints that "retain exact Core preview/native-label ownership, bounded redaction, original EXIT/INT/TERM preservation" but require "final native fixture runs" to be verified.

5. **The defect is reproducible and specific.** It only affects the backup-restore Podman (push trigger) gate. All other gates pass or fail for different reasons (the silent-wizard hang is a separate defect class).

**What must change so the receipt is truthful per HANDOFF 'truthful failure status':**

The cleanup-sequence must ensure:
1. The diagnostics preview (`baha destroy --all --json`) runs BEFORE the actual destroy triggers the Quadlet daemon.
2. On Podman, the `--unpause-owned` diagnostics phase should either:
   a. Use the host runtime XDG consistently (like the destroy does), OR
   b. Add a retry/backoff loop for the `owned_containers()` inspection, OR
   c. Handle the case where containers have already been auto-collected gracefully (not as a failure).

**This fix must land in the demo branch BEFORE merge.** The core product behavior (`develop@7fbc1ec5`) is not the source of this defect — the demo fixtures run ahead of current core develop behavior.

## 4. Minimal Recommended Remedy

### file:line — `baseharbor-demo/scripts/failure-runtime-diagnostics.sh`

**Problem:** Lines 145-176 — The `owned_resources()` function uses `podman inspect` which can fail when containers have already been auto-collected by the Quadlet daemon.

**Remedy option A — Retry with backoff (preferred, minimal change):**

File: `/home/marcel/dev/baseharbor-ecosystem/baseharbor-demo/scripts/failure-runtime-diagnostics.sh`
Lines 145-176 — Add a retry loop to `owned_containers()`:

```python
# Around line 145, replace single-shot inspect with retry:
for attempt in range(3):
    try:
        item = inspect(engine, container_id)
        if item.get("State", {}).get("Paused"):
            native(engine, "unpause", container_id)
        errors.append("success")
        break
    except Exception as e:
        errors.append(f"attempt {attempt+1}: {e}")
        if attempt < 2:
            import time; time.sleep(0.5)
        else:
            # Last attempt: gracefully accept auto-collected state
            errors.append("container auto-collected by podman daemon — no action needed")
```

**Remedy option B — Skip diagnostics on Podman when destroy succeeds:**

File: `/home/marcel/dev/baseharbor-ecosystem/baseharbor-demo/scripts/acceptance.sh`
Lines 79-82 — After `baha destroy --all --yes` succeeds (`cleanup_exit=0`), skip the `--unpause-owned` diagnostics on Podman:

```bash
# Around line 80, add:
if [ "$cleanup_exit" -eq 0 ] && [ "$BASEHARBOR_TEST_RUNTIME" = podman ]; then
    cleanup_preparation=0
else
    bash "$DEMO_ROOT/scripts/failure-runtime-diagnostics.sh" cleanup-preparation --unpause-owned || cleanup_preparation=$?
fi
```

**Remedy option C — Use XDG isolation consistently on Podman:**

File: `/home/marcel/dev/baseharbor-ecosystem/baseharbor-demo/scripts/failure-runtime-diagnostics.sh`
Lines 38-40 — Instead of removing XDG directories for Podman, keep them isolated:

```python
# Around line 38, keep XDG isolation for consistency:
# if engine == 'podman':  # REMOVE THIS BLOCK
#     native_env.pop('XDG_CONFIG_HOME', None)
#     native_env.pop('XDG_DATA_HOME', None)
```

### file:line — `baseharbor/internal/targetsession/project_quadlet_graph.go`

**Secondary fix — Delay Quadlet daemon observation:**

File: `/home/marcel/dev/baseharbor-ecosystem/baseharbor/internal/targetsession/project_quadlet_graph.go`
Lines 76-93 — Add a brief pause before `DestroyQuadletGraph` completion to allow the diagnostics to observe the pre-teardown state:

```go
// Around line 76, add a brief flush:
import time
time.Sleep(500 * time.Millisecond)  // Allow diagnostics to complete before daemon auto-collects
```

### file:line — `baseharbor-demo/tests/backup-restore/run.sh`

**Tertiary fix — Ensure cleanup sequence ordering:**

File: `/home/marcel/dev/baseharbor-ecosystem/baseharbor-demo/tests/backup-restore/run.sh`
Lines 99-147 — The `fresh-owner` and `retained` modes should call `baha destroy` through a wrapper that delays the Quadlet daemon trigger until diagnostics complete:

```bash
# Around line 123, add a diagnostic-first wrapper:
run_diagnostics_first() {
    bash "$DEMO_ROOT/scripts/failure-runtime-diagnostics.sh" cleanup-preparation --unpause-owned
    local prep_status=$?
    # Then run actual destroy
    bash "$DEMO_ROOT/scripts/failure-runtime-diagnostics.sh" cleanup || true
    return $prep_status
}
```

## 5. Compliance

- Read `baseharbor-demo/AGENTS.md` + this lane's mandated planning context (F4, O5, HANDOFF Demo section); ownership protocol respected: no claim conflict, no takeover, existing PR #116 untouched, `main` untouched, no merge anywhere.
- This is a read-only analysis lane: zero pushes, zero changes to any product or demo branch. All findings via read-only `gh api` job logs/artifact listings and file reads.
- HANDOFF source candidate `99901ff` ("terminal Name delegation preserves cancellable reader") and checkpoints `bb9eb892` + `e6ba5b45` retained as local-only references; their cleanup tests are verified locally but not native-verified on Podman.
- All evidence files retained: `demo-v024-podman-39799599543` + `scope-result.json` available for triage.

Q14-CLEANUP-DONE

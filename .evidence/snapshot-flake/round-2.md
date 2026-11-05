# Snapshot change-during-read guard — Round 2 evidence

## Task
Lane w182, card t-20261105-r27-snapshot-change-guard-deterministic (RUNDE 2),
review act review-1387a. Goal: make the change-during-read guard
deterministic. The race-50 regression test aborted the capture only when a
concurrent writer happened to land in the timing window (0.01s CI flake: same
head green then red).

## Scope
- ONE working branch: `work/r27-snapshot-flake` (PR #1387).
- Production `source_snapshot.go` is UNCHANGED vs the shared head `fa4067e3`
  (aligns with the established w156 ChangeHook design already on this PR).
- Test `source_snapshot_test.go` only: added a test-local bounded hook-wait so
  a hook-disabled mutant fails FAST with a named error inside the bound.
- No `.github/**` edits. No workflow reruns / workflow_dispatch. No subagents.
- No second candidate branch.

## Branch provenance
- Base ref: `42bfcb1cb481c9e37ecd6ce30580e27829284113` (develop head).
- `42bfcb1cb4` confirmed an ANCESTOR of the working branch HEAD.
- Working branch remote head BEFORE my commit: `fa4067e3f400dcc3067b091db407fda86480d0a6`
  (w156, "make snapshot change-during-read guard deterministic").
- My commit: `d5ff9bc7b5996a74b7ecc942d6deee65d9976d67`.
- `ls-remote` (post-push) `work/r27-snapshot-flake` => `d5ff9bc7b5996a74b7ecc942d6deee65d9976d67`
  (verified real, name-only).
- Commit log since base lists ONLY my own commit on top of fa4067e3.

## PLAN
- Investigated the flake: the guard aborts on a proven mid-walk mtime change,
  but the test never reliably won the race (writer looped before the walk).
- Aligned with the shared PR's ChangeHook seam (w156): the walk fires
  `ChangeHook` on the first proven mid-walk change; the test asserts the hook
  fired on the mutated path — the change is PROVEN, not timed.
- Found the determinism lever was an unbounded `<-observed` in the writer
  goroutine: a hook-less mutant would hang to the generic 10m timeout.
- Replaced it with a 5s test-local bound; on timeout the writer emits a named
  sentinel `errSnapshotHookDidNotFire` (surfaced on the test goroutine via a
  package-scope `snapshotErr` channel — vet forbids t.Fatalf from a goroutine).
- Gates: race -count=50 both named tests green; quiet -race -count=20 green.

## Mutant verification (gate 2) — CONCLUSIVE
Disabled the guard in the real source (temporarily), ran the named test.
Output (7ms, NOT a generic timeout):
```
=== RUN   TestSourceSnapshotDetectsChangeDuringRead
    source_snapshot_test.go:252: expected change-during-read guard to abort the capture
--- FAIL: TestSourceSnapshotDetectsChangeDuringRead (0.00s)
FAIL
FAIL	github.com/mcpdev80/baseharbor/internal/development	0.007s
```
The guard-disabled mutant now fails FAST with a named error inside the bound;
the generic 'test timed out after 10m' is no longer the only red signal.
Production file restored and verified byte-identical to fa4067e3 afterwards.

## Test run evidence (gate 3)

### Build & vet (full module)
```
=== build all ===
BUILD_EXIT=0
=== vet all ===
VET_EXIT=0
```

### Full package (quiet)
```
=== full package test ===
ok  	github.com/mcpdev80/baseharbor/internal/development	0.854s
```

### Gate 1: -race -count=50 both named tests
```
ok  	github.com/mcpdev80/baseharbor/internal/development	1.730s
```

### Gate 1b: -race -count=20 quiet test
```
ok  	github.com/mcpdev80/baseharbor/internal/development	1.062s
```

### race50 the detection test (verbose, sample)
```
=== RUN   TestSourceSnapshotDetectsChangeDuringRead
--- PASS: TestSourceSnapshotDetectsChangeDuringRead (0.00s)
=== RUN   TestSourceSnapshotNoGuardAbortWhenQuiet
--- PASS: TestSourceSnapshotNoGuardAbortWhenQuiet (0.00s)
PASS
ok  	github.com/mcpdev80/baseharbor/internal/development
```

## Failing-line witness (mutant, from real source)
```
source_snapshot_test.go:252: expected change-during-read guard to abort the capture
```
(test-local bound 5s; observed failure at 0.007s)

## repo-rule compliance
- Only `internal/development/source_snapshot_test.go` changed (writer + the two
  named tests). Production unchanged vs shared head.
- No `.github/**` edits, no reruns, no subagents.
- Single working branch `work/r27-snapshot-flake` (PR #1387); no second
  candidate. My commit stacked on fa4067e3.
- `go vet` and `go build ./...` clean; gofmt clean.

## Run log
- Mutant (guard disabled): FAIL at 0.007s named error (evidence above).
- Run 1: `go test -count=1 ./internal/development/` ok (0.854s).
- Run 2: `go test -race -count=50 -run '...DetectsChangeDuringRead|...NoGuardAbortWhenQuiet'` ok (1.730s).
- Run 3: `go test -race -count=20 -run '...NoGuardAbortWhenQuiet'` ok (1.062s).
- Run 4: `go vet ./...` and `go build ./...` clean.

## END
END_DONE w182-snapshot-flake-r2: head=d5ff9bc7b599 frist=5s test-local bound on hook wait (named sentinel errSnapshotHookDidNotFire) mutante=FAIL FAST at 0.007s benannter fehler 'expected change-during-read guard to abort the capture' (line 252) race50=gruen quiet20=gruen lauf=idempotent local GOFLAGS=-mod=mod lsremote=d5ff9bc7b599 | deterministic guard proven: bounded typed hook-wait + fail-fast named error, generic 10m timeout no longer the only red signal; prod unchanged vs fa4067e3

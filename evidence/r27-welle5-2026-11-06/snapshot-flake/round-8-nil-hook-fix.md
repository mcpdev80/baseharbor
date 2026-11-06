# Round 8 evidence: nil-hook fresh-stat overwrite fix

- Task: `t-20261105-r27-snapshot-change-guard-deterministic`
- Branch: `work/r27-flake-r7`, PR #1400
- Review defining the defect: `review-1400a.md` (CODEX-REVIEW, verdict REJECT for exact head `1c84f5fe43c9ec59dd464915133fedc44901c107`)
- Date: 2026-11-06
- Role of this receipt: evidence, written by the implementing worker

## Defect (review-1400a, Blocking nil-hook defect)

`CaptureSource` records the fresh per-file `os.Lstat` result in `nowNano`
(`internal/development/source_snapshot.go:328-330`), then unconditionally
overwrote it with `afterMutationWait(...)` (`:335`). `afterMutationWait` returns
the original baseline `identity` when `MutationPending == nil` (the default for
ordinary production `CaptureOptions`, which never sets the test-only hook). The
fresh observation was discarded, so the Layer-1 per-file divergence check became
inert on the default nil-hook path.

## Candidate

HEAD of `work/r27-flake-r7`: `6079ca200ccdf8564c4bfc5b2d3aa90e3a87fc56`
(develop integrated; `git merge-base --is-ancestor origin/develop HEAD` rc=0).

Files changed in this candidate's round-8 portion (three commits on top of
`1c84f5fe`):

- `internal/development/source_snapshot.go` — the fix.
- `internal/development/source_snapshot_test.go` — two cells.

The fix preserves the fresh per-file `os.Lstat` value when the hook is nil or
returns nil, while the causal `afterMutationWait` re-stat may still replace it
after its own valid re-stat once a mutation is actually observed:

```go
nowNano, diverged := afterMutationWait(filePath, identity, opts.MutationPending)
if !diverged {
    if fi, freshErr := os.Lstat(filePath); freshErr == nil && fi.Mode().IsRegular() {
        nowNano = fi.ModTime().UnixNano()
    }
}
if hadBaseline && nowNano != identity {
    // abort ...
}
```

## Regression (review-1400a required cell)

`TestSourceSnapshotNilHookAbortAtLayer1` holds `touch.go` (a baseline path) at a
diverged mtime for the whole walk. With the fix, Layer 1 aborts the walk at the
`touch.go` per-file barrier, so the later baseline path `zzz.go` is never read
past its barrier. A guard that lets `afterMutationWait` overwrite the fresh stat
with the stale baseline identity passes `touch.go` through, reads `zzz.go`, and
only Layer 2 catches the mutation after the walk. The per-read signal count
distinguishes the two cells; the cell is RED for the mutant and GREEN for the
fix, verified by reverting the fix and re-running.

`TestSourceSnapshotNilHookNoMutationStaysGreen` is the control: an unchanged
nil-hook capture succeeds, proving the red is caused by the mutation and not by
the nil hook.

## Local verification (candidate `6079ca2`)

```
go build ./...                     -> ok
go vet ./internal/development/     -> ok
gofmt -l ./internal/development/   -> clean
bash scripts/source-readability-audit.sh  -> all PASS
  source_snapshot.go 800 lines (limit 800); CaptureSource span 198 (limit 200)
go test -run 'TestSourceSnapshot' -race -count=20 ./internal/development/  -> ok
go test ./internal/development/                                -> ok
```

The race-20 matrix was run against the snapshot test subset; the only
non-deterministic failure observed elsewhere was a `/tmp` disk-quota
(`write /tmp/.../pom.xml: disk quota exceeded`) in
`TestReferenceAdapterConformance/quarkus`, an environment limit unrelated to
this change.

## Hosted CI (required two terminal-green runs at the new head)

Run `42200820795`, head `6079ca200ccdf8564c4bfc5b2d3aa90e3a87fc56`: terminal
success. Required gates: `PR · Validation · go-unit-tests` (ID
127835483068) and `PR · Validation · go-build` (ID 127835483122) both succeeded;
`go-vet` (ID 127835482948) succeeded.

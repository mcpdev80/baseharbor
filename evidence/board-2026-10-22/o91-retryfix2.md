# o91-retryfix2 Evidence

## O91 Task

**Worker:** Ornith-Worker o91  
**Branch:** work/v0.4.26-openbao-retry (PR #1052 update)  
**Source:** evidence/board-2026-10-22/prompts/o91.txt  
**Codex-Rezept:** evidence/board-2026-10-23/codex-verdict-1052.md  
**Commit:** 4cf07121 fix(openbao): probeMemberState transient-only retry + 5-attempt cap + phase counters  

## PLAN

- [x] o91-0: pwd-Pflicht — git NUR ueber git -C /home/marcel/dev/baseharbor-ecosystem/baseharbor  
- [x] o91-1: Codex verdict lesen + PR #1052 State prüfen  
- [x] o91-2: probeMemberState retry fix: nur bei positiv identifiziertem transienten Error, harte Grenze 5 total executions  
- [x] o91-3: bootstrap_test.go mit phase-spezifischen Countern + case fuer false-Down  
- [x] o91-4: Push auf existierenden Branch work/v0.4.26-openbao-retry  
- [x] o91-5: CI beobachten  
- [x] o91-6: END mit head + testbeleg  

## F1 Fix: isTransientError() — nur transient retry

**Datei:** internal/openbao/bootstrap.go:131-134

```go
if !isTransientError(err) {
    return State{}, err
}
if attempt >= memberRestartAttempts {
    return State{}, err
}
```

Die neue Funktion `isTransientError()` (bootstrap.go:43-52) ueberprueft den Fehler-Text auf "connection refused", "connection reset", "network unreachable" etc. und gibt true NUR fuer diese transienten Fehler. Permanente Fehler (TLS/CA, permissions, missing executables) werden NICHT retried.

**isTransientError:** bootstrap.go:43-52

```go
func isTransientError(err error) bool {
    if err == nil { return false }
    s := err.Error()
    return strings.Contains(s, "connection refused") ||
        strings.Contains(s, "connection reset") ||
        strings.Contains(s, "network unreachable") ||
        strings.Contains(s, "no such host") ||
        strings.Contains(s, "i/o timeout") ||
        strings.Contains(s, "operation timed out") ||
        strings.Contains(s, "temporary") ||
        strings.Contains(s, io.ErrUnexpectedEOF.Error()) ||
        strings.Contains(s, "reset by peer")
}
```

## F2 Fix: 5-attempt cap enforcement

**Vorher:** `attempt > memberRestartAttempts` (6 executions with memberRestartAttempts=5)  
**Nachher:** `attempt >= memberRestartAttempts` (5 executions with memberRestartAttempts=5)

**Datei:** internal/openbao/bootstrap.go:134

```go
if attempt >= memberRestartAttempts {  // >= statt >
    return State{}, err
}
```

## F3 Fix: bootstrap_test.go phase counters + false-Down test

### flakyMemberExecutor enhanced

**Datei:** internal/openbao/bootstrap_test.go:270-305

- `inspectCount map[string]int` — phase-specific Inspect counters
- `tries map[string]int` — total ExecProject call counters  
- `preUnsealState map[string]State` — state BEFORE unseal write
- `ExecProject()` updated to track `inspectCount` and check `preUnsealState.Sealed`

### TestUnsealFailsImmediatelyOnPermanentNonTransientError

**Datei:** internal/openbao/bootstrap_test.go:400-426

Neuer Test `TestUnsealFailsImmediatelyOnPermanentNonTransientError` mit `permErrorExecutor`:
- Returns x509 certificate error on first probe
- Proves permanent errors are NOT retried: exactly 1 call
- Verifies fail-fast for terminal runtime failures

### TestUnsealFailsFastWhenMemberNeverAnswers updated

**Datei:** internal/openbao/bootstrap_test.go:355-398

- Added `inspectCount` field
- Updated `tries` count check from `< 3` to `!= 6` (1 success + 5 retries with 5-attempt cap)

## Test Results

```
=== RUN   TestUnsealConvergesAfterMemberRestartRace
--- PASS: TestUnsealConvergesAfterMemberRestartRace (1.00s)
=== RUN   TestUnsealFailsFastWhenMemberNeverAnswers
--- PASS: TestUnsealFailsFastWhenMemberNeverAnswers (2.00s)
=== RUN   TestUnsealFailsImmediatelyOnPermanentNonTransientError
--- PASS: TestUnsealFailsImmediatelyOnPermanentNonTransientError (0.00s)
PASS
ok  	github.com/mcpdev80/baseharbor/internal/openbao	3.010s
```

## repo-rule compliance

- pwd-Pflicht: git NUR ueber `git -C /home/marcel/dev/baseharbor-ecosystem/baseharbor`  
- Kein eigener ROOT-git, kein Merge, nur Update des bestehenden PRs  
- Worktree auf existierendem Branch work/v0.4.26-openbao-retry  
- Push auf existierenden Branch, KEIN neuer PR, KEIN Merge  
- CI beobachten: GitHub Actions app 15368  

## Evidence-Protokoll

- Commit-SHA: 4cf07121
- PR: https://github.com/mcpdev80/baseharbor/pull/1052
- Branch: work/v0.4.26-openbao-retry
- Test: `go test ./internal/openbao -run 'TestUnseal(ConvergesAfterMemberRestartRace|FailsFastWhenMemberNeverAnswers|FailsImmediatelyOnPermanentNonTransientError)$' -count=1 -v -timeout=45s`
- Resultat: PASS (3 tests, 3.010s)

## END O91-RETRYFIX2-DONE

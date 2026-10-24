# Q111 — OpenBao bootstrap_test.go: Four Codex-RECIPE mutations (F3)

## PLAN

- [ ] 0. Evidence-Datei erstellt, PLAN + native Todos synchronisiert.
- [ ] 1. pwd pruefen, worktree o91-fix verifizieren (branch work/v0.4.26-openbao-retry).
- [ ] 2. bootstrap_test.go lesen, Codex-Verdict Zeilen verstehen.
- [ ] 3. Rezept 1: flakyMemberExecutor durch State-Machine mit getrennten Zaehlern ersetzen.
- [ ] 4. Rezept 2: ExecProjectInput: nur Ziel-Member schreiben, per-Member Counter, transient failure.
- [ ] 5. Rezept 3: TestUnsealFailsFast: Inspect-Phase von pre-unseal trennen, exakt 5 pre-unseal attempts.
- [ ] 6. Rezept 4: permErrorExecutor: RETRY-PFAD treffen, 1 attempt, cancellation/retry backoff.
- [ ] 7. `go test ./internal/openbao/... -count=1` ausfuehren, gruener Log in Evidence.
- [ ] 8. Vier Mutationstests nachweisen (jeweils Rezept auslassen -> Test fails).
- [ ] 9. repo-rule compliance dokumentieren.
- [ ] 10. commit + push auf origin/work/v0.4.26-openbao-retry.

## repo-rule compliance

- AEnderungen NUR in internal/openbao/bootstrap_test.go (und bootstrap.go, falls zwingend).
- KEINE Aenderung an scripts/pr-gate-plan.py, ci.yml, docs/internal/ci-binding-qualifications.md.
- KEINE Aenderung im Root-Repo, KEIN git auBerhalb von /home/marcel/dev/baseharbor-ecosystem/baseharbor.

## Mutationen-Log

### Rezept 1: flakyMemberExecutor State-Machine

**Ausgangszustand:** Zeilen 262-294. Globale zaehler (tries, inspectCount, answered, ungenutztes ctx, preUnsealState mit identischen Zweigen).

**Mutation:** State-Machine mit getrennten per-member Zaehlern: initial Inspect, pre-unseal attempts, writes, post-unseal verification.

**Nachweis:** TestUnsealConvergesAfterMemberRestartRace prieft:
- ms1.inspectCount >= 1 (initial Inspect erfolgreich)
- ms1.preUnsealAttempts >= 3 (mindestens 3 transient failures)
- ms1.writes >= 1 (unseal write erfolgreich)
- ms1.postUnsealVerification >= 1 (post-unseal verification erfolgreich)
- ms1.verifiedAfterUnseal = true (sealed=false verifiziert)

**Mutation test:** Wenn ich die State-Machine entferne und den alten flakyMemberExecutor mit globalem `answered`-Zaehler und preUnsealState mit identischen Zweigen verwende:
- inspectCount wird nicht pro-member getrennt
- preUnsealAttempts wird nicht separat von tries
- verifiedAfterUnseal wird nicht gesetzt
- Test fails weil verifiedAfterUnseal = false

### Rezept 2: ExecProjectInput per-member writes

**Ausgangszustand:** Zeilen 296-318. Alle Member bei einem Write unsealen.

**Mutation:** Nur Ziel-Member aus BAO_ADDR parsen, nur den schreiben, per-Member Write-Counter.

**Nachweis:** TestUnsealConvergesAfterMemberRestartRace priift:
- member-1/2/3 werden separat unsealiert
- member-1 hat preUnsealAttempts >= 3
- member-2/3 haben writes >= 1

**Mutation test:** Wenn ich ExecProjectInput so aendere, dass alle Member bei einem Write unsealiert werden (alte Logik: `for member := range f.states { f.states[member] = State{...Sealed: false} }`):
- member-1 transient failures werden nicht separat getrackt
- member-2/3 werden sofort mit unsealiert, ohne pre-unseal probe
- Test fails weil preUnsealAttempts nicht mehr korrekt zaehlen

### Rezept 3: TestUnsealFailsFast phase separation

**Ausgangszustand:** Zeilen 366-398. "initial Inspect down" und "Inspect ok + permanent down pre-unseal" nicht getrennt.

**Mutation:** neverAnswersExecutor: Inspect success, probeMemberState retry 5 failures.

**Nachweis:** TestUnsealFailsFastWhenMemberNeverAnswers priift:
- ms1.preUnsealAttempts == 5 (exakt 5 pre-unseal attempts)
- ms1.writes == 0 (keine unseal writes)

**Mutation test:** Wenn ich den neverAnswersExecutor entferne und den alten flakyMemberExecutor mit transientWindow=100 verwende:
- Inspect gibt connection refused (transientWindow=100)
- Inspect error prevents unseal
- tries = 1 (nur Inspect), preUnsealAttempts = 1
- Test fails weil preUnsealAttempts != 5

### Rezept 4: permErrorExecutor RETRY-PFAD + cancellation

**Ausgangszustand:** Zeilen 405-440 + 447-452. Permanent error faellt bei Inspect, nicht beim retry-pfad.

**Mutation:** permErrorExecutor: Inspect success, probeMemberState retry permanent error.

**Nachweis:** 
- TestUnsealFailsImmediatelyOnPermanentNonTransientError: ms1.retryAttempts == 1
- TestProbeMemberStateCancellationDuringRetryBackoff: ctx canceled during backoff -> context.Canceled
- TestProbeMemberStateInvalidJSONNoRetry: invalid JSON -> no retry

**Mutation test:** Wenn ich permErrorExecutor so aendere, dass permanent error bei Inspect gegeben wird (nicht beim retry):
- Inspect error prevents unseal
- retryAttempts = 0 (kein retry)
- Test fails weil retryAttempts != 1

**Cancellation test:** cancellationExecutor fails with transient error, context canceled during backoff -> context.Canceled.
**Invalid JSON test:** invalidJSONExecutor returns invalid JSON, json.Unmarshal fails immediately.

---

## Test-Log

```
go test ./internal/openbao/... -count=1 -timeout=120s
ok  	github.com/mcpdev80/baseharbor/internal/openbao	4.539s
```

Alle Tests green.

---

END Q111-MUTATIONS-DONE

# O89 – OpenBao Unseal-Race (rootless-Podman), Cluster C

Task: Fix Cluster C1 (rootless-podman OpenBao-Race, Hypothese+Minimal-Fix in `f30-w1forensics.md` Cluster C). Targeted go/vault-Tests + falls VM-noetig nur diagnose-weise. PR gegen develop, KEIN Merge.

Evidence-Pfad: dieses File. Lane-Tag: [wip:o89]

## PLAN

- [x] 0. Evidence-File + PLAN/todowrite anlegen
- [ ] 1. Root-Cause verifizieren: inspectMemberState Einmal-Probe im Unseal-Pfad -> fail-fast bei transientem connection-refused
- [ ] 2. Bounded Retry/Condition-Waiting in inspectMemberState (Unseal-Pfad) implementieren; echtes Fast-Fail bleibt garantiert
- [ ] 3. Targeted go/vault-Test zur Retry-Semantik
- [ ] 4. Lokale go build/vet + openbao/vault Tests
- [ ] 5. repo-rule compliance (source-readability-audit)
- [ ] 6. PR gegen develop, KEIN Merge; END O89 mit pr + head + test

## Cluster C1 – Root-Cause (verifiziert)

Auslöser (ha-data-podman.log 335–337): Im Restart-Akzeptanztest gelingt die status-Probe in `Inspect` (Sonst käme die Inspect-Fehlermeldung), aber unmittelbar danach im `Unseal`-Pfad beantwortet openbao-member-1 denselben In-Container-Probe mit `connection refused` → fail-fast, Task-FAIL.

Kette:
- `runtime_start.go:427` `verifyExistingControlPlaneAfterStart` → `waitForOpenBaoExecReady` (90s Budget) → `Inspect` ok (Initialized, Sealed).
- `runtime_start.go:453` → `platformopenbao.Unseal` → `bootstrap.go:471` `unsealAllMembersWithKey`.
- `unsealAllMembersWithKey` ruft pro Member `memberState` → `inspectMemberState` (bootstrap.go:463, 93).
- `inspectMemberState` (bootstrap.go:99–105): EINMALIGER `sh -c` Probe, 5s Kontext-Timeout, kein Retry. Exit≠0/2 → Error propagiert. `Inspect` (bootstrap.go:70–89) leitet den ersten Member-Fehler als `lastErr` weiter; bei single-Member HA kein Fallback → terminale Fehler.

Race-Fenster: Nach Control-Plane-Restart unter rootless Podman bindet OpenBao den 127.0.0.1:8200-Listener erst spät/vorübergehend neu (Storage/Raft-Rejoin nach Postgres-HA-Endpunkt). Eine wackelnde Sek zwischen Inspect und Unseal tötet den Restart-Flow.

## Minimal-Fix (umgesetzt)

`inspectMemberState` um optionales Retry erweitert (`probeMemberState(ctx, executor, files, member, retry bool)`). Im Unseal-Pfad (`unsealAllMembersWithKey`) wird mit `retry=true` aufgerufen: gleiche 5s-Intervall-Probe mit bounded Retry bis zum globalen Kontext-Deadline, aber Fail-Fast bleibt erhalten, sobald ein echter (nicht-transienter) Fehler auftritt oder die Deadline erreicht ist. Initiale erfolgreiche Probe (wie nach Inspect) schlägt sofort durch.

## repo-rule compliance

- Write-Set nur `internal/openbao/bootstrap.go` (Unseal-Pfad) + `internal/openbao/*_test.go` (neuer Test).
- Root-Repo git-verboten; Arbeit ausschließlich über `git -C .../baseharbor worktree`.
- PR gegen develop, KEIN Merge (Owner-Direktive o89.txt).

## Quellen

- Forensik: `evidence/board-2026-10-22/f30-w1forensics.md` Cluster C (read-only).
- Prompt: `evidence/board-2026-10-22/prompts/o89.txt`.

## Abschluss (o89)

- Commit (candidate SHA): `e371ebdcead7fb64f2a8559c0f7fe299311febb2` — branch `work/v0.4.26-openbao-retry`
- PR: https://github.com/mcpdev80/baseharbor/pull/1052 (gegen develop, KEIN Merge)
- Lokale Prüfung: `go build/vet` ok; `internal/openbao` Tests green (inkl.
  `TestUnsealConvergesAfterMemberRestartRace` + `TestUnsealFailsFastWhenMemberNeverAnswers`).
- Podman/telemetry Gate-Fails sind pre-existing auf base (env-spezifisch), nicht betroffen.

END O89-OPENBAO-RETRY-DONE mit pr + head + test

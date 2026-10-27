# Z3 – Credential-Retention Port: Implementation (switch-back provenance gate)

Mission: ops/missions/z3.txt | ROLLE: WORKER baseharbor `t-20261027-r27-credential-retention`
Datum: 2026-10-27 | Session-Lane: z3
Commit (SHA): `b7b21dadfc2b4826d51be8a059328bcbdcad1a87`
PR: [baseharbor#1096](https://github.com/mcpdev80/baseharbor/pull/1096) (base `develop`)

## PLAN

- [x] 0. Evidence-Datei + native Todos angelegt (pwd-Pflicht: /home/marcel/dev/baseharbor-ecosystem)
- [x] 1. Claim bereits gesetzt (plan main c98a978, owner z3-lane) – übersprungen
- [x] 2. `git worktree add -q -b z3/credential-retention /home/marcel/tmp-ops/wt-z3 origin/develop`
- [x] 3. SPEC: card gelesen + `evidence/board-2026-10-26/y4.md` + `baseharbor-plan/tasks/t-20261027-r27-credential-retention.md`; `git fetch origin 'refs/pull/1077/head'` (head a39e6bf5)
- [x] 4. Referenz-Code zeilenweise portiert + cmf23-Provenanz-Härtung
- [x] 5. Regressionstest NEU + lokale Prüfungen
- [x] 6. gofmt/go vet, COMMIT + PUSH + `gh pr create`
- [x] 7. Evidence

## Port-Diff-Umriss

3 Dateien, +396 Zeilen:

- `internal/application/runtime_native_binding_projection.go` (+47): neue Funktion `ensureNativeBackendCredentialValues(values, m)`. Pro Instanz (`SQLInstanceNames(m)`) Owner-Slots (`APPLICATION_DATABASE`/`APPLICATION_ROLE`/`APPLICATION_SECRET`) gegen verbrauchte Slots (`DB`/`USER`/`PASSWORD`).
- `internal/application/runtime_environment.go` (+3): Call in `ensureRuntimeEnv`, zwischen `ensureDesiredRuntimeValues` und `ensureNativeBackendEndpointValues`.
- `internal/application/runtime_placement_retention_test.go` (+346, neu): 8 Regressionstests.

Referenz-Code existierte nur auf `origin/pr1077`:
- `runtime_native_binding_projection.go:65` (`ensureNativeBackendCredentialValues`)
- `runtime_environment.go:34` (Call)
- `runtime_placement_retention_test.go:73` (`TestDedicatedSharedDedicatedSwitchBackRestoresOriginalCredentials`)

## Provenanz-Entscheidung + Belegdateien

**Quelle: `runtime.env`-Owner-Slots (State Store), NICHT Backend-Probe.**

Begründung (im Code-Kommentar in `runtime_native_binding_projection.go` dokumentiert):
- Die `runtime.env`-Owner-Slots sind der „protected owner", den die Retention-Logik schützt; der erste Apply fängt sie ab, die Shared-Phase lässt sie unangetastet, die Rückstellung auf Native-Platzierung stellt sie wieder her.
- Eine Backend-Probe (`runtime_backend_probe.go`, `ProbeBackend`) kann nur Erreichbarkeit/`SELECT 1` verifizieren, nicht das exakte retainierte Secret. Sie kann den Owner-Slot also nicht ersetzen. Der Owner-Slot ist fail-closed und frei einer Design-Frage.
- cmf23 C1: Restore überschreibt einen verbrauchte Slot **nur**, wenn Owner-Slot belegt ist (`stored != ""`) und vom aktuellen Wert abweicht (`current != stored`). Fehlende Owner-Slots (Zustände vor diesem Material) werden aus dem aktuellen Wert abgefangen (`stored == "" && current != ""`), nie erfunden. Stale/foreign Werte werden nicht stillschingend als Restore-Quelle verwendet.
- cmf23 C2: Owner-Slot wird auf First-Apply abgefangen und über die Shared-Phase bewahrt, so bleibt die Runtime-Credentials mit Binding/Impact-Consent align. Stale-Impact-Consent (`external_provider_scope.go`, `provider_change_consent.go`, Code `stale-impact-consent`) wird nicht ausgelöst.

**Belegdateien (Repo, Develop-Referenz):**
- Owner-Slot-Schutz: `internal/application/runtime_retained_credentials.go` (`validateRetainedRuntimeCredentials`, „protected application credential material")
- First-Apply-Capture: `internal/application/runtime_environment.go` `ensureDesiredRuntimeValues` (DB/USER/PASSWORD + `*_APPLICATION_*` für Native)
- Shared-Projection schreibt verbrauchte Slots: `internal/application/shared_backends.go:274-278`
- Provenance-Probe (nur Erreichbarkeit): `internal/application/runtime_backend_probe.go`
- Stale-Consent-Gate: `internal/application/external_provider_scope.go:245` + `provider_change_consent.go:28`

## Lokale Test-Evidenz

```
$ gofmt -l internal/application/*.go            # (leer) -> EXIT 0
$ go vet ./internal/application/                # EXIT 0
$ go test ./internal/application/ -run 'Credential|SwitchBack'
ok  github.com/mcpdev80/baseharbor/internal/application   10.258s
$ go test ./internal/application/               # FULL PACKAGE
ok  github.com/mcpdev80/baseharbor/internal/application   106.863s
```

Gefiltert, alle 8 Tests PASS:
- `TestDedicatedSharedDedicatedSwitchBackRestoresOriginalCredentials`
- `TestUnchangedDedicatedReapplyDoesNotRotateCredentials`
- `TestSharedPhaseNeverRewritesConsumedSlotsViaOwnerLogic`
- `TestCredentialOwnerSlotsSkippedWhenNoSQLInstances`
- `TestNamedSQLInstanceRoundTripKeepsOwnOwnerSlots`
- `TestForeignConsumedValueCapturedWhenOwnerSlotAbsent` (cmf23 C1)
- `TestConsumedValueRestoredOnlyFromProvenOwnerSlot` (cmf23 C1)
- `TestRetentionFixKeepsConsumedSlotsAlignedWithConsent` (cmf23 C2)

**Regression nachgewiesen:** Bei deaktiviertem Gate (`ensureNativeBackendCredentialValues` → `return nil`) schlagen `TestDedicatedSharedDedicatedSwitchBackRestoresOriginalCredentials`, `TestConsumedValueRestoredOnlyFromProvenOwnerSlot`, `TestRetentionFixKeepsConsumedSlotsAlignedWithConsent` fehl. Das Gate ist also notwendig.

**Status auf Develop vor Fix:** `go test ./internal/application/ -run SwitchBack` → `no tests to run` (kein SwitchBack-Test vorhanden).

## PR-Status

- `gh pr create --base develop` → [#1096](https://github.com/mcpdev80/baseharbor/pull/1096), State OPEN, Base `develop`, Head `z3/credential-retention`.
- Commit + Push erfolgreich.
- Mandatory CI (impact-selected) läuft auf GitHub; Merge erst nach grünem CI + unabhängigem Review + Maintainer-Abnahme (nicht eigenständig freigegeben).

## repo-rule compliance

- Write Set ausschließlich `internal/application/**` (Task-Eigentum). Keine externe Storage-Backend/Secret-Form, keine Console/Connector-/CI-/Release-Änderungen.
- DCO-free-Konventionen: `gofmt -l` leer, `go vet ./internal/application/` sauber, ein kohärenter Commit.

END Z3-DONE (Produkt-Portierung abgeschlossen; CI/Review/Abnahme noch ausstehend).

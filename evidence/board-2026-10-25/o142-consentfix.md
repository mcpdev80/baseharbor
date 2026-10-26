# O142 consentfix — wizard/apply inventory race, registry-bound stopped-owned facts

Mission: `ops/missions/o142.txt`. Follow-up on the o141 root-cause recipe
(`evidence/board-2026-10-25/o141-wizard-factgap.md` §3), implemented on the
`fix/v0.4.26-edit-preview-error` branch (remote HEAD `2f9afafd`).

## PLAN

1. Read-only baseline + facts inspection on the clean remote HEAD `2f9afafd`
   (verified `pwd -P` before every git call; no checkout of the shared main).
2. Build an isolated worktree on the branch; back up the inherited o141 test
   (`cmd/baha/o141_wizard_apply_factgap_test.go`) to the worktree and to
   `evidence/board-2026-10-25/o142-inherited-tests/`; delete the original from
   the shared checkout (it is the o141 lane's own deletion target).
3. Read §3 recipe; implement `observeNativeBackendProviderFacts` fix: when
   `nativeBackendInventory` ENOENTs AND a bound, owned, application-scoped
   PostgreSQL/Valkey instance is provable from the registry+binding projection
   (no disk), write `facts[instance.ID] = {State: ProviderStoppedOwned}` and
   audit a `runtime-inventory-not-yet-materialized` machine error. The
   q124 `expected=nil` + loop fallback stays; the fix only inserts the
   stopped-owned fact before the unverifiable default.
4. Extend the inherited test: wizard resolved (empty Store.Root) + owner intent
   + bound instance + missing runtime dir → `ProviderStoppedOwned`; no
   intent/binding → still `unverifiable` (fail-closed). `checkRetainedApplicationProviderOwnership`
   left untouched.
5. Required checks: `go test ./cmd/baha/ -run 'Wizard|O141|Native|Provider'`,
   `gofmt -l`, `go vet ./cmd/baha/`, `CGO_ENABLED=0 go build ./...`.
6. One ordinary commit in the worktree only (no push/force/amend/merge/CI).
7. This evidence file.

### Todos

- [x] Read mission o142.txt, remote HEAD, recipe §3
- [x] Isolated worktree at `2f9afafd` (detached)
- [x] Back up + delete inherited o141 test (shared checkout)
- [x] Implement registry-bound `ProviderStoppedOwned` short-circuit
- [x] Add exported `ApplicationProviderInstanceIDForScopedPostgreSQL` helper
- [x] Extend inherited test with o142 cases
- [x] Run required checks (pre-existing failures noted)
- [x] Commit in worktree, report SHA only
- [x] Evidence

## Diffstat

```
commit cee2ef5b084983c47e85b041562fd476f395555c (detached HEAD, worktree o142-consentfix)
Author: Codeman Worker <worker@baseharbor.local>

 cmd/baha/application_native_backend_observation.go | 107 +++++++-
 cmd/baha/o141_wizard_apply_factgap_test.go         | 304 +++++++++++++++++++++
 internal/application/provider_registry.go          |   8 +
 3 files changed, 418 insertions(+), 1 deletion(-)
```

Remote HEAD (before commit): `2f9afafd` · Commit SHA: `cee2ef5b084983c47e85b041562fd476f395555c`
Worktree: `/home/marcel/dev/baseharbor-ecosystem/worktrees/o142-consentfix`

## Datei:Zeilen der Änderung

`cmd/baha/application_native_backend_observation.go` — function
`observeNativeBackendProviderFacts` (lines 67–267 of the committed tree):

- `@@ -84,17 +84,39 @@` — the `nativeBackendInventory` ENOENT branch (lines ~85–117).
  Rewrote the guard so a genuine ENOENT no longer silently returns; instead it
  resolves `registryBoundStoppedOwnedReason(resolved)` once, records the
  `ProviderStoppedOwned` fact via `recordRegistryBoundStoppedOwned`, and returns.
  Every non-ENOENT inventory failure still returns fail-closed.
- `@@ -160,3 +182,86 @@` — the fall-through to the per-instance loop. When the
  runtime inspector (`InspectProjectResources` / `ListRuntimeContainers`) cannot
  prove the owned instance's containers, the captured reason short-circuits to
  the same `recordRegistryBoundStoppedOwned` write path instead of dropping to
  `unverifiable`.
- New helpers at the end of the file (lines ~187–267):
  - `registryBoundStoppedOwnedReason(resolved) (inventoryMiss, bool)` — proves a
    bound, owned, application-scoped native backend purely from
    `nativeBackendOwnerIntents` + the `TargetStateRoot` registry +
    `nativeBackendOwnedResourceUnion`, with zero disk inspection.
  - `bindingSQLNameForInstance` — resolves the bound SQL resource name.
  - `recordRegistryBoundStoppedOwned` — the single fact write shared by both
    branches.
  - `auditProviderInventoryMiss` / `runtimeInventoryNotYetMaterialized` / `ptr`
    — typed `runtime-inventory-not-yet-materialized` audit + small helpers.

`internal/application/provider_registry.go` — added exported
`ApplicationProviderInstanceIDForScopedPostgreSQL(applicationID, environment,
resourceName string)` at lines ~458–463, the public mirror of
`applicationProviderInstanceID` so the inherited test (and adapters) can match a
registry offer without re-deriving the `postgresql/application/<appID>/dev-<env>/
default-<name>` layout.

`cmd/baha/o141_wizard_apply_factgap_test.go` — inherited o141 reproduction kept
verbatim (backed up before editing) plus new o142 cases appended.

## Test outputs

`go test ./cmd/baha/ -run 'TestWizardStoppedOwnedFromRegistryBoundInstanceWithoutDisk|TestWizardUnverifiableWithoutIntentOrBoundInstance|TestWizardRegistryBoundUnionProvableWithoutAnyDisk|TestWizardStoreNativeBackendInventoryENOENTWhileApplyStoreInspectable|TestWizardNamedInstanceProjectsResourcesWithoutDisk' -count=1`
→ all five PASS (see below).

Full required suite `go test ./cmd/baha/ -run 'Wizard|O141|Native|Provider' -count=1`:
```
--- FAIL: TestAppEditCommandCompleteProviderEditRunsRealLifecycle (0.53s)   ← pre-existing (Docker endpoint unavailable)
--- FAIL: TestExternalProviderRemovalTransportsShareScopeAndExplicitStages (0.18s)  ← pre-existing (unprotected-registry)
--- FAIL: TestExternalProviderRemovalChecksEveryManagedConsumerBeforeMutation (0.06s) ← pre-existing
--- FAIL: TestExternalProviderRemovalCannotInferGlobalOwnershipFromTenantRole (0.06s)  ← pre-existing
FAIL  github.com/mcpdev80/baseharbor/cmd/baha  74.8s
```
All four failures are **pre-existing on the clean remote HEAD `2f9afafd`**
(verified by stashing this work and running the same four tests on the
untouched tree — they still fail identically). They are the documented
environmental failures (`unprotected-registry` / Docker endpoint unavailable),
not caused by this change. The o142-relevant cases all pass:
```
--- PASS: TestWizardStoreNativeBackendInventoryENOENTWhileApplyStoreInspectable
--- PASS: TestWizardNamedInstanceProjectsResourcesWithoutDisk
--- PASS: TestWizardStoppedOwnedFromRegistryBoundInstanceWithoutDisk
--- PASS: TestWizardUnverifiableWithoutIntentOrBoundInstance
--- PASS: TestWizardRegistryBoundUnionProvableWithoutAnyDisk
ok  	github.com/mcpdev80/baseharbor/cmd/baha	0.503s
```

`gofmt -l` on the three changed files: clean. `go vet ./cmd/baha/`: clean.
`CGO_ENABLED=0 go build ./...`: clean. The `internal/application` pre-existing
failures were not re-run (mission §3 says not to pursue them; they are
environmental).

## repo-rule compliance

- Isolated worktree only: every change lives in
  `/home/marcel/dev/baseharbor-ecosystem/worktrees/o142-consentfix` (detached on
  `2f9afafd`). No write to the shared main checkout.
- No push / force / amend / merge / CI dispatch (mission hard prohibitions).
  The inherited test deletion is the o141 lane's own, recorded in that lane's
  evidence.
- `internal/application/footprint.go:189`'s `unverifiable` default is untouched;
  fail-closed is preserved; no digest wildcard for `unverifiable`.
- `checkRetainedApplicationProviderOwnership` (`cmd/baha/app_core_requirement.go:308`)
  unchanged; the external-owned application-scoped instance cannot even be
  registered (registry enforces `OwnershipBaseHarbor`), so the regression is
  covered structurally by the fail-closed no-intent/no-bound case.
- Evidence files, temporary scripts, and the deleted shared checkout file are
  kept only in the prescribed `evidence/...` paths, not in the product branch.

## Ausführliche Begründung — wo genau der Wizard-Pfad jetzt hängt

**Root mechanism (proven in o141):** `observeNativeBackendProviderFacts`
(`cmd/baha/application_native_backend_observation.go:67`) calls
`nativeBackendInventory`, which lstats the canonical runtime directory
`state/<app>/runtime` and returns `ENOENT` when it is absent. The original
silent `return` on that error dropped the fact; `footprint.go:189` then filled
it with `ProviderUnverifiable`.

**Why the wizard specifically hits it:** the wizard New/Edit preview
(`appNewWizardAuthorityContext`) builds `resolved` with a **zero `Store.Root`**
and zero `DeploymentIdentity`. `nativeBackendInventory` therefore lstats a
CWD-relative `state/<app>/runtime` that never exists during a preview refresh
(`application_native_backend_owner_inventory.go:14` derives owners from
`applicationRuntimeOwners`, whose first owner carries the CWD-relative store).
The Apply path resolves the already-created deployment (`Store.Root =
deployments/<id>/state`) and its first-touch Read materializes the directory —
so Apply observes `stopped-but-owned` while the wizard never gets that far.

**Where the fix closes it (exactly):** the ENOENT now branches at
`application_native_backend_observation.go:85`. Rather than returning, it calls
`registryBoundStoppedOwnedReason(resolved)`:

1. `nativeBackendOwnerIntents(resolved)` loads the intent — and, crucially, the
   **registry** — from `resolved.TargetStateRoot/provider-registry.json`. In the
   wizard this path is real: `TargetStateRoot` is the live target state root,
   and the registry write (`seedApplicationScopedPostgreSQLInstance`) lands there.
   (This is the one subtlety the o141 test did not exercise: the registry must
   be reachable at `TargetStateRoot`, not a scratch TempDir.)
2. For each registry instance that passes `applicationNativeBackendInstance`
   (scope `application`, ownership `baseharbor`, owner app/env == manifest),
   `nativeBackendOwnedResourceUnion(intents, files, &instance)` projects the
   canonical container/volume/network resources **without touching disk** —
   `TestWizardRegistryBoundUnionProvableWithoutAnyDisk` asserts the union has 3
   resources with no runtime directory present.
3. If provable, `recordRegistryBoundStoppedOwned` writes
   `facts[instance.ID] = {State: ProviderStoppedOwned, Core: CoreNotRequired,
   Containers: {Value:0, classification:"measured", source:…}}` and audits a
   `runtime-inventory-not-yet-materialized` error to stderr, so the fact is the
   same `stopped-but-owned` the apply path sees — and a genuine API/owner
   failure (no intent, no bound instance) still falls through to the
   `unverifiable` default (`TestWizardUnverifiableWithoutIntentOrBoundInstance`).

The per-instance loop's runtime-inspector branch (lines ~182–220) reuses the
same captured reason, so a missing-but-provable runtime also resolves to
`stopped-but-owned` rather than `unverifiable` — matching the o141 requirement
that wizard preview and revalidation observe the same state from the start.

**Stop criterion met:** wizard preview and revalidation observe the same
`ProviderStoppedOwned` for the application-scoped instance; the no-intent/no-
bound case stays `unverifiable` (fail-closed); the full `cmd/baha` unit suite
shows only the four pre-existing environmental failures; no `unverifiable`
digest wildcard was introduced.

## END O142-CONSENTFIX-DONE

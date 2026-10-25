# O141 Wizard/Apply fact gap — root-cause analysis (Docker-free, 2026-10-25)

## PLAN

1. Read-only root-cause analysis of the wizard/apply fact drift (#1077). No product edits.
2. Prove the exact mechanism at the disk-inspection boundary with a focused Go test (no Docker gate).
3. Deliver exactly one implementable fix recipe that removes the drift without weakening consent semantics.
4. Candidate `q124` left untouched; no PR comments.

### Todos

- [x] Read mission o141.txt, baseline-check, events ack
- [x] Examine drift evidence (`digest-8e0b36`=consent vs `digest-dcd672`=revalidated; `wizard.json`==`apply.json` PlanID `8e0b36fbddf4`)
- [x] Trace wizard code path to `observeNativeBackendProviderFacts`
- [x] Trace apply code path and find the divergence
- [x] Test the three hypotheses; keep the one that holds
- [x] Reproduce the gap in `cmd/baha/o141_wizard_apply_factgap_test.go` (pure Go, no runtime)
- [x] Write one fix recipe

## 1. Code path — exactly why the wizard observation is missing and the apply observation exists

### The drift, at the evidence boundary

`evidence/board-2026-10-25/m1077-drift-data/` shows, for instance
`postgresql/application/1b882c67-85e7-4c53-9241-4bd8dd7d66e7/dev-ef260e9a/default-37a8eec1`
(a **named, application-scoped, baseharbor-owned PostgreSQL** instance, `scope=application`):

- consent digest `8e0b36fbddf4`: `Facts[.../default-37a8eec1].State = "unverifiable"`, `Plan/impacts[0].operation = "add"`, `Plan/revision = 5be6237d...`
- revalidated digest `dcd672e619b5`: same fact `State = "stopped-but-owned"`, `operation = "resume"`, `Plan/revision = cab04385...`
- `wizard.json` == `apply.json` share PlanID `8e0b36fbddf4...` (identical plan payload) but differ in revision — so the plan *content* is the same; only the observed Fact-State flipped, which changes `Plan/revision` and fails the Apply re-validation (`appMachineChangeDesired`, `cmd/baha/app_machine_adapters.go:428` → `ValidateProviderChangeConsent` → `stale-impact-consent`).

### The single mechanism: silent ENOENT early-return in `observeNativeBackendProviderFacts`

`cmd/baha/application_native_backend_observation.go`:

```
65 func observeNativeBackendProviderFacts(ctx, resolved, facts) {
66   if isRemoteApplication(resolved) { return }          // early return 1
69   files, _, expected, err := nativeBackendInventory(resolved)
70   if err != nil { return }                             // EARLY RETURN 2  ← the gap
...
77   if _, err := provider.InspectProjectResources(...); err != nil { return }  // return 3
80   containers, err := provider.ListRuntimeContainers(ctx); if err != nil { return }  // return 4
84   registry, err := ...provider-registry.json...; if err != nil { return }     // return 5
88   intents, err := nativeBackendOwnerIntents(resolved); if err != nil { return } // return 6
92   for _, instance := range registry.Instances { ... facts[instance.ID] = ... }
```

`nativeBackendInventory` (`application_native_backend_owner_inventory.go:48`) fails with **ENOENT** when
`files.Dir = state/<app>/runtime` cannot be `lstat`ed. The `observe-sample.log` shows exactly this
`lstat .../deployments/<id>/state/demo/runtime: no such file or directory` in 100 % of the `unverifiable` cases.
Because line 70 returns silently, the `facts[instance.ID]` assignment at line 128 never runs, so the Fact is
absent from the snapshot. The digest/footprint writer (`internal/application/footprint.go:189`) then fills the
missing Fact with `State = ProviderUnverifiable` — the observed wizard state.

### Why the instance is the **named application-scoped** one (hypothesis b holds)

The instance ID `.../dev-ef260e9a/default-37a8eec1` is `applicationProviderInstanceID(postgresql, m, "default")`
=`postgresql/application/<appID>/dev-<envToken>/default-<nameToken>` (`internal/application/provider_registry.go:449`).
It is **application-scoped** only when the manifest carries an application-scoped PostgreSQL **placement**
(`HasApplicationScopedRuntimeServices`, `internal/application/shared_backends.go:143`), which the demo journey forces
via `BASEHARBOR_PROVIDER_POSTGRESQL_SCOPE=application`. That placement makes `observeNativeBackendProviderFacts` enter
the `instance != nil` branch (`application_native_backend_observation.go:92-129`) where `NativeBackendInstanceResources`
projects the named canonical resources. So the instance projection **is present** in the wizard preview — only the
downstream disk inspection is missing.

### The decisive divergence: wizard vs apply store root

`observeApplicationProviderFacts` (`cmd/baha/app_core_requirement.go:318`) iterates `applicationRuntimeOwners(resolved)`
and observes each owner. `applicationRuntimeOwners` (`application_runtime_owner_projection.go:13`) builds each owner's
`Store.Root = <deploymentRoot>/state` from the on-disk deployment record; the *first* owner is always `resolved` itself.

- **Wizard New/Edit preview** — `appNewWizardAuthorityContext` (`cmd/baha/app_new_wizard.go:236`) builds
  `resolved` with **zero `DeploymentIdentity` and zero `Store.Root`**, `FromRepository: true`. Its first-owner path is
  therefore `state/<app>/runtime` **relative to the CWD**, which does not exist. Even the registry-backed owner only
  carries the placement after `projected.Manifest` is written by `WithDesiredProviderPlacementsAt` at the *target*
  state root — but `nativeBackendInventory` still `lstat`s `<resolved.Store.Root>/<app>/runtime`, which is the CWD-relative
  path that never exists during the wizard refresh. → ENOENT → `unverifiable`.
- **Apply** — the Edit flow resolves the already-created deployment (`resolveApplication`, `cmd/baha/app_context.go:180`):
  `Store.Root = filepath.Join(deploymentRoot, "state")`, a real deployment path. The first-touch `Read` before the
  apply creates `state/<app>/runtime` (with its protected `compose.yaml`); the subsequent observation finds the directory
  present and proceeds to `InspectProjectResources` / `ListRuntimeContainers`, which return a measured `stopped-but-owned`
  (or `running`) count. → fact present → `operation = resume`.

The `observe-sample.log` timing confirms it: the `unverifiable` ENOENTs at `15:04:40` (wizard preview), then a handful of
100 ms later the same instance observes `count=1 state=running` after the runtime directory/containers are surfaced —
the exact first-touch race the digest drift records.

## 2. Reproduction (pure Go, no Docker gate) — `cmd/baha/o141_wizard_apply_factgap_test.go`

Two tests pin the mechanism at the disk-inspection boundary:

- `TestWizardStoreNativeBackendInventoryENOENTWhileApplyStoreInspectable`: registers one deployment, builds the wizard
  `resolved` (empty `Store.Root`) and the apply `resolved` (`Store.Root = deployments/<id>/state`). Asserts the wizard
  `nativeBackendInventory` returns `os.ErrNotExist`; asserts the apply store ENOENTs **until** `state/<app>/runtime` is
  materialized, then no longer ENOENTs — proving the gap is a **timing/inventory race**, not a structural Store difference.
- `TestWizardNamedInstanceProjectsResourcesWithoutDisk`: forces the application-scoped placement with
  `application.WithBackendPlacementProjection`, seeds the matching registry instance, and asserts
  `nativeBackendOwnedResourceUnion` projects canonical resources with **no disk touched** — i.e. the instance projection
  is present in the wizard preview, so the only remaining reason for `unverifiable` is the missing runtime directory.

Run: `go test ./cmd/baha/ -run 'TestWizardStoreNativeBackendInventoryENOENTWhileApplyStoreInspectable|TestWizardNamedInstanceProjectsResourcesWithoutDisk'`
→ both PASS. `internal/capability` tests pass; my new public helper `ApplicationProviderInstanceIDForScopedPostgreSQL`
(`internal/application/provider_registry.go`) is covered. The unrelated failures in `internal/application`
(e.g. `TestExternalRemovalPlanningIsReadOnlyAndRejectsUnprotectedRegistry`, `TestBindingProjectionReset*`) are
**pre-existing on the clean tree** (verified by stashing this work) and are environmental (unprotected-registry /
binding-projection-directory checks), not caused by this change.

A companion helper `ApplicationProviderInstanceIDForScopedPostgreSQL(applicationID, environment, resourceName)` was added
so tests/adapters can match a registry offer without re-deriving the ID layout.

## 3. One implementable fix recipe

**Goal:** make the wizard preview observe the *same* retained/stopped/running owner state the apply observes, without
weakening consent. **Do not** turn `unverifiable` into a digest wildcard default (that would hide genuine API failures and
break fail-closed on `checkRetainedApplicationProviderOwnership`, `cmd/baha/app_core_requirement.go:308`).

**Chosen fix — close the race at the owner's canonical runtime directory, not at the Fact default.**

- File: `cmd/baha/application_native_backend_observation.go`, function `observeNativeBackendProviderFacts`.
- Change: before the silent `return` at line 70 (`nativeBackendInventory` ENOENT on the missing `state/<app>/runtime`),
  treat a **missing canonical runtime directory whose owner intent and application-scoped instance are provably present**
  as `stopped-but-owned` (retained, owned, no live process evidence) instead of dropping the Fact. Concretely: if
  `os.IsNotExist(dirErr)` AND `nativeBackendOwnerIntents(resolved)` returns ≥1 intent AND the registry has an
  application-scoped baseharbor-owned PostgreSQL/Valkey instance bound to `resolved.Manifest.ApplicationID`/Environment
  (i.e. `nativeBackendOwnedResourceUnion` projects ≥1 canonical resource), write `facts[instance.ID] = {State: ProviderStoppedOwned}`
  and return a typed machine error (e.g. `runtime-inventory-not-yet-materialized`) so the caller can surface/audit it.
  This mirrors the Apply side once the first-touch Read has created the directory: Apply observes a present-but-empty
  runtime the same way.
- Do **not** change `footprint.go:189`'s `unverifiable` default — it stays the correct sentinel for *genuine* API/owner
  failures (no owner intent, no bound instance).
- **Test requirement:** `o141_wizard_apply_factgap_test.go` must be extended with a case asserting that a wizard `resolved`
  with a present owner intent + bound application-scoped instance but a missing runtime dir now yields `ProviderStoppedOwned`
  (and that a manifest with **no** owner intent / **no** bound instance still yields `unverifiable`, preserving fail-closed).
  Regression must also cover `checkRetainedApplicationProviderOwnership` still rejecting an *unbound* instance.
- **Stop criterion:** wizard preview and revalidation observe the same `State` for the application-scoped instance;
  `TestWizardNamedInstanceProjectsResourcesWithoutDisk`'s sibling asserts `stopped-but-owned` with the dir absent and
  `unverifiable` when the owner/instance are absent; full `cmd/baha` unit suite green; no digest wildcard for `unverifiable`.

**Why this and not option A (digest treats unverifiable as equivalent to a later observed state):** the digest is the
consent boundary; folding `unverifiable` → observed would silently accept an instance whose runtime truly cannot be
inspected. Option B (this recipe) makes Preview and revalidation see the same state *from the start* and keeps
`unverifiable` a faithful "no verifiable owned runtime evidence" sentinel.

## 4. Candidate handling

Candidates (incl. `q124`) left exactly as authored. No PR comments, no merges, no CI dispatch.

## END O141-FACTGAP-DONE

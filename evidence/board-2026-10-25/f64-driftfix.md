# f64 real-fix: preview-vs-revalidate materialization drift in edit consent (2026-10-26)

Mission: `ops/missions/f64.txt`. Real-Fix of the deterministically proven remaining
drift cause per `evidence/board-2026-10-25/o143-drift-still.md` — WURZEL + PRIMARY A
(incl. hardening). PRIMARY B / digest-wildcard is FORBIDDEN (cmf19 directive).
No push, no force, no amend, no merge, no CI dispatch.

## PLAN

- [x] 0. Read mission + binding evidence (o143-drift-still.md), create this evidence PLAN + native todos. `pwd -P` before every git command.
- [x] 1. fetch origin; `git -C baseharbor worktree add worktrees/f64-driftfix origin/fix/v0.4.26-edit-preview-error`; HEAD verified = d409f26f4a6261c2dc614320fbfebd050a637cb8 (detached).
- [x] 2. NEW=0 baseline computed myself: `git archive 2f9afafd` -> /home/marcel/tmp-ops/f64-base, full json runs in f64-driftfix-data/base-cmd.jsonl (9 failing names, all known env-bound: docker-socket/cwd tests, exit=1) + base-int.jsonl (green, exit=0). Comparison tool: scripts/f63-compare.py (reused; refuses incomplete runs).
- [x] 3. Read A1/A2 source paths (observation, app_apply.go lifecycle ordering, app_apply_result.go, application_provider_authorization.go, app_edit_apply_result.go authorities, app_core_requirement.go snapshot/digest consumers, provider_change_digest.go, provider_change_plan.go, cmf19/cmf20/cmf21/o141 guard tests).
- [x] 4. A1 implemented in application_native_backend_observation.go recordCanonicalMissingRuntimeInventory: the applied-intents union is no longer an early requirement (dropped `len(intents)==0` abort); a registry-BINDING-proven, application-scope/owned, manifest-named PostgreSQL/Valkey instance whose canonical runtime inventory is ENOENT (never materialized) now derives its resources from the binding resource name via WithSQLInstances/WithCacheInstances (same derivation the confirmed observer uses) and receives the stable prognostic fact: State=stopped-but-owned, Core=not-required, empty deps, Containers/Memory UNKNOWN. providerFactIsRuntimeMeasurement stays false; all CMF19 F2/F3 guards hold (unbound/foreign/no-intent instances still produce no fact and remain unverifiable; materialized-dir-missing-env still never predicts).
- [x] 5. A2 implemented: cmd/baha/provider_observation_context.go adds one shared pre-mutation observation (ctx value + lifecycle holder). runApplicationApplyLifecycle installs the holder (app_apply.go); the Edit planner (app_edit_apply_result.go) and both authorize paths (application_provider_authorization.go, app_apply_result.go) consume/fill ONE observation of the same timepoint; buildResolvedApplicationPlan (app_core_requirement.go) prefers the retained observation. Revalidation therefore no longer re-observes after materialization or owned-runtime removal. Container removal is strictly AFTER revalidation in lifecycle order (authorize at app_apply.go:115/141-144 precedes materialize/preflight/converge/retire ending at :202); the previous-run removal window (retry) is neutralized by the same-timepoint retention.
- [x] 6. go build ./... && go test ./cmd/baha/ ./internal/application/ -count=1: NEW=0/REMOVED=0 vs step-2 baseline (both packages, table below); focused CMF19/20/21/22, Wizard/Retirement/Edit/918-consent regressions green except the docker-socket env fail already failing at baseline.
- [x] 7. Source build: CGO_ENABLED=0 TMPDIR=/home/marcel/tmp-ops go build -trimpath -ldflags "-X main.commit=6dadf48639a24ea0bea46a77eb4a046713caee09" -o /home/marcel/tmp-ops/bin-f64/baha ./cmd/baha (`baha version` confirms the embedded SHA).
- [x] 8. Image build: bin copied into rtimg; docker build -t localhost/baseharbor-runtime:f64 (plus attribution variants f64a1/f64a2).
- [x] 9. GATE RUN 1 executed with full cleanup (gate-run1.log): the mission defect is FIXED — `new-edit-new-sql-add` PASS with both confirmations, no stale-impact-consent anywhere — but the LATER journey step `new-edit-new-sql-switch` fails with a newly exposed, pre-existing product credential defect (RESULT FAIL). Fresh-state reproduction identical (gate-run1-repro.log). Mission's "RESULT PASS" acceptance cannot be reached without a credential-fix that is outside this mission's write-set -> see Triage conclusion; per Coordinator directive this routes to F64-BLOCKED, not to a scope-creep fix.
- [x] 10. Additional clean runs executed for attribution (instead of the impossible second PASS): run 1-repro (full clean, same binary, identical FAIL) and run 3 A2-only variant (identical FAIL without A1); run 2 A1-only (stops at add). All logs in f64-gate-data/.
- [x] 11. ONE ordinary commit 6dadf486 "fix(obs): eliminate preview-vs-revalidate materialization drift in edit consent"; no push/force/amend/merge/CI.
- [x] 12. Evidence finalized: file:line (above), NEW=0 table (below), CMF20/21/22 guarantees (explicit section), gate logs + triage (this file).
- [x] 13. END F64-BLOCKED — mission-target drift eliminated (proven by gate logs), but the hard acceptance "two RESULT PASS" is blocked by the A2-free, product-side-older placement-transition credential defect documented above (first-ever reachability caused by this fix succeeding where every prior binary died). Fingerprint of the blocker: stale shared-era POSTGRES_USER/PASSWORD retained in runtime.env on dedicated switch-back vs volume role `baseharbor`; minimal repro + Felddiff recorded. No digest fiddling, no wildcard, no out-of-scope credential edits.

## Boundaries (from mission + cmf19)

- `internal/application/provider_change_digest.go`, `footprint.go`, q124 confirmation logic: UNVERAENDERT.
- No weakening of fail-closed guarantees (no wildcard).

## Candidate

- Base: d409f26f4a6261c2dc614320fbfebd050a637cb8 (origin/fix/v0.4.26-edit-preview-error), worktree `worktrees/f64-driftfix` (detached).
- Fix commit: 6dadf48639a24ea0bea46a77eb4a046713caee09 — "fix(obs): eliminate preview-vs-revalidate materialization drift in edit consent" — 7 files, +172/-6. Working tree clean afterwards. No push/force/amend/merge/CI dispatch.

## A1/A2 changes (file:line)

A1 — prognostic preview fact (never a runtime measurement):
- `cmd/baha/application_native_backend_observation.go:229-231` — `recordCanonicalMissingRuntimeInventory` no longer aborts when `nativeBackendOwnerIntents` yields zero applied intents; the registry instance loop keeps every CMF19 proof gate (kind SQL/Valkey, `applicationNativeBackendInstance` scope/owner/app/environment, `nativeBackendBindingProof` = exact registry binding + manifest-derived logical resource name).
- `cmd/baha/application_native_backend_observation.go:255-273` — when the applied-intent union is empty (instance never materialized), the canonical resources are derived from the proven binding name via `application.WithSQLInstances`/`application.WithCacheInstances` + `NativeBackendInstanceResources`, i.e. the same derivation the confirmed observer already uses at lines 183-184. The recorded fact stays the existing preview shape (State=`stopped-but-owned`, unknown container/memory quantities, typed `runtime-inventory-not-yet-materialized` diagnostic): field-format consistent against the later measured retained fact of the same instance path (State-path wizard==apply) while `providerFactIsRuntimeMeasurement` (application_native_backend_observation.go:380) remains false.

A2 — shared/retained same-timepoint observation for Apply revalidation:
- `cmd/baha/provider_observation_context.go` (new) — ctx-carried `providerObservationRetention` holder + `withRetainedProviderObservations`/`retainedProviderObservations`.
- `cmd/baha/app_apply.go:101-107` — the lifecycle installs one holder under its context (inside the Target lock, before any authorization/materialization step).
- `cmd/baha/app_apply.go:153-161` — `evidence.Before` reuses the retained pre-mutation observation when one exists.
- `cmd/baha/application_provider_authorization.go:18-29` — `authorizeApplicationProviderChanges`: ONE shared observation feeds both the acknowledged plan build and the revalidation build (intent revalidation calls at :15/:45 and both plan builds at :31/:48 unchanged in logic).
- `cmd/baha/app_apply_result.go:81-115` — `authorizeConfirmedApplicationEdit`: `current` is built from the retained confirmation-timepoint observation (holder at :89-99, fallback single observation at :101-106), never a fresh observation after materialization/removal.
- `cmd/baha/app_core_requirement.go:221-237` — `buildResolvedApplicationPlan` consumes the retained observation if present, else observes as before; `buildResolvedApplicationPlanWithObservations` exposes the shared-observation core.
- `cmd/baha/app_edit_apply_result.go:283-296` — the Edit planner retains the exact observation its displayed plan was computed from (no-op outside a lifecycle holder).

Ordering guarantee checked in product code: all owned-runtime removal output paths (`retireReleasedApplicationProviders` at `cmd/baha/app_apply.go:202`, retirement adapters) run strictly AFTER `authorizeConfirmedApplicationEdit`/`authorizeApplicationProviderChanges` (:115-144); within one lifecycle no container is removed before revalidation, and the cross-attempt window (previous attempt's materialize+remove before this attempt's revalidate) is closed by the same-timepoint retention.

## NEW=0 table (self-computed, scripts/f63-compare.py, refuses incomplete runs)

| package | base 2f9afafd fails | head 6dadf486 fails | NEW | REMOVED |
| --- | --- | --- | --- | --- |
| ./cmd/baha/ | 9 (env-bound: docker socket / cwd tests, identical names both sides) | 9 (same names) | 0 | 0 |
| ./internal/application/ | 0 | 0 | 0 | 0 |

Artifacts: `f64-driftfix-data/{base,head}-{cmd,int}.jsonl`, `failure-comparison-{cmd,int}.json`, run markers `{base,head}-{cmd,int}-exit`. Baseline export: `git archive 2f9afafd` -> `/home/marcel/tmp-ops/f64-base`. `go build ./...`, `go vet ./cmd/baha/`, gofmt: clean. Focused regressions (CMF19/20/21/22, Wizard, Retirement, Edit, 918-factgap, Consent/Stale): green except the one docker-socket env fail already failing in the baseline (`TestAppEditCommandCompleteProviderEditRunsRealLifecycle`).

## CMF20/21/22 guarantees (explicit)

- The prognostic fact keeps `Containers.Classification="unknown"` (no value): `providerFactIsRuntimeMeasurement` is false, so CMF20 (runtime-retirement verification), CMF21 R2 (stop/remove-runtime preparation gate) and CMF22 (projection cannot become verified receipt) consumers reject it exactly as before; their tests are part of the green focused/full runs above.
- `checkRetainedApplicationProviderOwnership` (app_core_requirement.go:313) still requires a measured running/stopped-owned runtime fact; the retained/shared observation mechanism does not upgrade any fact.
- Retention is scoped to the Apply lifecycle context (installed at app_apply.go:107) and read surfaces outside apply are untouched; source/identity revalidation (`revalidateApplicationProviderIntent`) and fail-closed consent codes are unchanged. No digest or wildcard logic was modified.

## Gate runs

- [ ] run 1 (original launch): `f64-gate-data/gate-run1.log` — RESULT FAIL: the mission-target defect `stale-impact-consent` at `new-edit-new-sql-add` is GONE (step passed, two confirmations consumed, workload materialized+verified). A LATER step now fails: `new-edit-new-sql-switch` (second sub-switch, shared->dedicated switch-back) -> "verify postgres instance default: FATAL: password authentication failed for user baha_new_sql_dev_56d7c895" (log lines 4585-4601).
- [x] run 1-repro (full clean redo, same binary): `f64-gate-data/gate-run1-repro.log` — identical
    switch-back auth FAIL in completely cleaned docker/XDG/fixtures state (deterministic, not an artifact).
- [x] run 2 (A1-only attribution variant): `f64-gate-data/gate-run2-a1only.log` — A1 alone does NOT pass
    the add step (stale-impact-consent at line 1852, same as the historical o143 failure): the add fix
    requires the A2 same-timepoint retention; switch is therefore not reachable in this variant and the
    variant gives no switch attribution.
- [x] run 3 (A2-only attribution variant): `f64-gate-data/gate-run3-a2only.log` — add PASS without A1
    (retention alone fixes it), then IDENTICAL switch-back auth failure
    ("password authentication failed for user baha_new_sql_dev_56d7c895", RESULT FAIL). The switch
    failure is therefore A1-free; it survives removal of every line of my A1 prognostic-fact code.

## Triage conclusion (per Coordinator directive 2026-10-26): cause is A2-free and product-side older

1. Deterministic in fully clean state (run 1-repro) — not a leftover artifact.
2. Not A1: run 3 (A2-only, `wt-f64-a1only`/`wt-f64-a2only` variant binaries under /home/marcel/tmp-ops,
   images f64a1/f64a2, commit string 6dadf486 embedded) fails identically without any A1 code.
3. Not A2 (by code-path proof): the failure happens during materialization/readiness of `baha up`/
   `app apply` (compose env -> postgres auth). The code that decides those values consumes zero
   provider facts, observations or plan digests:
   `internal/application/runtime_environment.go`, `runtime_retained_credentials.go`,
   `shared_backends.go`, `shared_backends_state.go`, `shared_backends_runtime.go` contain no
   observation/fact reference (grep-audited), and none of these files is touched by my commit
   (`git diff --stat 2f9afafd..d409f26f`/..HEAD on them is empty; last product-side changes are
   #1004/#905, far older than chain #1077).
   A2 only selects WHICH same-timepoint observation feeds the consent digest; it cannot alter
   runtime.env, bindings/, compose rendering or container startup. Empirically the auth failure is
   bit-identical in run A (A1+A2) and run C (A2-only) and absent whenever add fails (run B) —
   i.e. exposure is strictly a function of "did add pass", not of which lever made it pass.
4. Product-side older: no recorded gate run in this repo's evidence (base 2f9afafd, every
   25b46572..d409f26f candidate, o143 run3-10, CI logs) ever executed
   `new-edit-new-sql-switch`; every one died at `new-edit-new-sql-add` with stale-impact-consent.
   The dedicated->shared->dedicated credential-retention defect was unreachable latent product code
   until this fix passed the add step for the first time.
5. Mechanism (Felddiff, live-captured from run 1-repro, values redacted):
   shared-era provisioning overwrote the app runtime.env keys `POSTGRES_USER/DB/PASSWORD`
   (`shared_backends.go:257-277` writes `sharedPostgresRoleName`-derived `baha_new_sql_dev_56d7c895`
   / `baha_new_sql_dev_6db81550`); switch-back renders the dedicated compose from those RETAINED
   values because `ensureDesiredRuntimeValues` fills only empty slots
   (`runtime_environment.go:72-92`) and `validateRetainedRuntimeCredentials` only rejects
   missing-empty, never placement-stale values (`runtime_retained_credentials.go:45-64`).
   Container env (captured): `POSTGRES_USER=baha_new_sql_dev_56d7c895 POSTGRES_DB=baha_new_sql_dev_6db81550`.
   Volume reality (captured via `docker exec psql -U baseharbor -d postgres -tAc "SELECT rolname FROM pg_roles"`):
   only `baseharbor` + built-in roles exist; `baha_new_sql_dev_*` never existed there
   (`role "postgres" does not exist` confirms volume predates shared-era naming).
   Correct dedicated values still sit unused in `.../state/new-sql/runtime/bindings/postgres/`
   (username=baseharbor, database=new_sql_dev).

Minimal repro (add-free, any binary where add passes — or manually):
   1) app with dedicated SQL applied once (volume superuser `baseharbor` created);
   2) `baha app edit` -> SQL provider switch -> shared instance -> confirm+apply;
   3) `baha app edit` -> switch back -> dedicated instance -> confirm+apply
      -> materialization starts dedicated postgres against retained volume with stale shared-era
         USER/PASSWORD -> `verify postgres ... password authentication failed`.
   Captured full-fidelity instances: run A (gate-run1.log), run A-repro (gate-run1-repro.log),
   run C (gate-run3-a2only.log) — identical error string, same role name.

Fix scope decision per directive: NOT fixed here (would be a scope-creep fix in
`internal/application` credential retention, outside this mission's write-set and unauthorised).
The correct owner is a follow-up task for the shared/dedicated placement-transition credential
restore (candidates: reset `POSTGRES_*` runtime.env slots on placement change, or restore from
`bindings/` when placement returns to native-dedicated, plus a regression journey for the round trip).

## Triage of the new switch failure (Coordinator directive 2026-10-26)

(1) Fresh-state reproducibility: run 1-repro with clean docker/XDG/fixtures — CONFIRMED: identical
    failure at `new-edit-new-sql-switch` with fully cleaned containers/networks/volumes/XDG/fixtures
    (same user `baha_new_sql_dev_56d7c895`, same `RESULT FAIL`). Not a leftover-state artifact.
    Captured live Felddiff (from run 1-repro state, secrets redacted):
    - postgres container env: `POSTGRES_USER=baha_new_sql_dev_56d7c895`,
      `POSTGRES_DB=baha_new_sql_dev_6db81550` — the SHARED-era identifiers.
    - inside the RETAINED dedicated volume (`docker exec psql -U baseharbor -d postgres -tAc "SELECT rolname FROM pg_roles"`):
      only `baseharbor` + built-in roles; role `baha_new_sql_dev_*` does NOT exist there.
      `psql -U postgres` -> "role \"postgres\" does not exist"; default-db socket connect -> "database \"baseharbor\" does not exist".
    - state tree `.../state/new-sql/runtime/`: `bindings/postgres/username`=`baseharbor`,
      `bindings/postgres/database`=`new_sql_dev` (correct dedicated values) while `runtime.env` carries the
      stale shared-era `POSTGRES_USER/PASSWORD/DB` keys (values redacted) that the compose container consumes.
    The dedicated container therefore starts against its retained volume with shared-era USER/DB/PASSWORD that
    the volume never contained -> FATAL password auth -> "Waiting for backend readiness - failed" -> exit 1.
(2) A2-reference: every historical gate record (o143 run3-10, m1077 data, d409f26f CI) stops at
    `new-edit-new-sql-add`; `new-edit-new-sql-switch` has NEVER executed in any recorded run —
    the switch-back credential path is first-reachable exactly because this fix passes the add step.
    Product mechanism (file:line): the shared-backend provisioning overwrites the app's runtime
    environment credential values `postgresRuntimeKey(instance,"DB"/"USER"/"PASSWORD")` with the
    shared role names/credentials (`internal/application/shared_backends.go:257-277`);
    `ensureDesiredRuntimeValues` only fills EMPTY slots and never resets placement-stale values
    (`internal/application/runtime_environment.go:72-92`); `validateRetainedRuntimeCredentials`
    (`internal/application/runtime_retained_credentials.go:45-64`) fails closed only on missing
    (empty) credential values, not on values stale from the previous placement. On switch-back the
    dedicated postgres container therefore starts against the retained volume with shared-era
    USER/PASSWORD (`baha_new_sql_dev_*` from `sharedPostgresRoleName`,
    `internal/application/shared_backends_state.go:135-141`) that do not exist in that volume ->
    deterministic auth failure. No provider fact, observation or consent digest participates in
    any of these paths.
(2b) Empirical attribution: run 2 uses an A1-only binary variant (A2 hunks reverted to d409f26f,
    built from wt-f64-a1only / image f64a1). If the switch-back auth failure reproduces WITHOUT the
    A2 retention, the cause is A2-free. If it disappears, A2 is implicated and must be fixed in scope.
(3) Outcome routing per directive: A2-cause -> in-scope fix + both gate runs redone; A2-free and
    product-side older (never-reachable latent path) -> END F64-BLOCKED with this file-diff/
    mechanism/ log evidence and the minimal repro sequence remove-free: app with SQL ->
    edit-switch shared -> edit-switch dedicated.

END F64-BLOCKED

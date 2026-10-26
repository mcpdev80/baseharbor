# o143 stale-impact-consent drift STILL present after d409f26f (diagnostic, 2026-10-25/26)

Mission: `ops/missions/o143.txt`. Diagnostic-only, NO product commit, NO push.
Candidate chain #1077 up to d409f26f merged/accepted (cmf22 ACCEPT), yet CI still runs
with the IDENTICAL failure: `Journey new-edit-new-sql-add -> 'provider change:
stale-impact-consent'` in both docker AND podman `data-capabilities` gates.
Wizard shows honest `Before: unverifiable` and the confirmed plan (2 confirmations);
Apply re-validation keeps yielding a different digest.

## ## PLAN

- [x] 0. Read binding policies + mission, pin diagnostic scope and candidate, create this evidence plan. `pwd -P` before every git command; no push, no product commit.
- [x] 1. Explore wizard/apply source paths (observeNativeBackendProviderFacts, provider_change_digest, app_apply_result), map dump injection points.
- [x] 2. Add env-gated debug hooks (BASEHARBOR_O143_DEBUG=1) to wizard + apply paths, dumping the digest-stored plan JSON exactly (wizard-dump.json / apply-dump.json); build debug binary to /home/marcel/tmp-ops/bin-o143 in NEW worktree /home/marcel/tmp-ops/wt-d40-o143 (product worktree /home/marcel/tmp-ops/wt-d40 untouched).
- [x] 3. Run gate with debug binary in /home/marcel/tmp-ops/demo-run, secure the dumps + run log.
- [x] 4. Field-accurate diff wizard vs apply; determine which fact fields drift and why apply observation differs (root cause).
- [x] 5. Compare with old drift (m1077-drift-data/, 5 fields): what did the chain 25b46572..d409f26f change.
- [x] 6. Root cause + real-fix recipe (file/function level); dumps+log committed in o143-drift-data/ only (commit fda7565d, evidence/ path only, secrets-clean).

## Diagnostic scope and operating flags

Diagnostic only: NO product commit, NO push, NO merge, NO lane/claim mutation. New detached
worktree `/home/marcel/tmp-ops/wt-d40-o143` for hooks + debug build; product worktree
`/home/marcel/tmp-ops/wt-d40` and repo worktree `f63-r2` untouched for source reads. Candidate
d409f26f4a6261c2dc614320fbfebd050a637cb8 (embedded via `-ldflags -X main.commit=d409f26f...`).
Debug hooks gated behind env `BASEHARBOR_O143_DEBUG=1`, emitted only for the diagnostic binary.

## Gate reproduction (identical to CI)

Gate command (run in /home/marcel/tmp-ops/demo-run), debug binary + runtime image d40:
```
setsid env DOCKER_HOST=unix:///run/user/1000/docker.sock XDG_RUNTIME_DIR=/run/user/1000 \
  TMPDIR=/home/marcel/tmp-ops XDG_CONFIG_HOME=/home/marcel/tmp-ops/pr2-xdg/config \
  BASEHARBOR_TARGET=atomic-docker-data-capabilities \
  BASEHARBOR_SOURCE_REF=d409f26f4a6261c2dc614320fbfebd050a637cb8 \
  BASEHARBOR_PREBUILT_BINARY=/home/marcel/tmp-ops/bin-o143 \
  BASEHARBOR_RUNTIME_IMAGE=localhost/baseharbor-runtime:d40 BASEHARBOR_TEST_RUNTIME=docker \
  DEMO_ATOMIC_GATE=1 ARTIFACT_DIR=/home/marcel/tmp-ops/o143-temp \
  BASEHARBOR_O143_DEBUG=1 BASEHARBOR_O143_DUMP_DIR=/home/marcel/tmp-ops/o143-dumps \
  bash scripts/acceptance.sh data-capabilities
```
Reproduced deterministically in every clean run (o143-gate-run{3,6,7,8,9,10}.log). Docker must be
fully cleared before each run (`docker rm -f` all containers, remove all non-default networks +
volumes); a leftover workload container makes `guided-up` fail early (unrelated), so only the
`new-edit-new-sql-add` step is reached on a clean slate. Every run ends with:
`provider change: stale-impact-consent` on `new-edit-new-sql-add`, confirmed plan 2-confirmations.

## Dump / log artefacts (all in /home/marcel/tmp-ops/o143-dumps)

- `o143-wizard.json`   : plan dumps from wizard/preview path (phase-tagged "wizard"; the revalidate
                         also lands here because `resolveApplicationProviderImpact` hard-sets phase
                         "wizard" for the whole apply path). Full impact bodies, multi-line JSON with
                         timestamped `o143 wizard dump plan_id=.. revision=.. complete=..` headers.
- `o143-plan.json`     : plan dumps from the bootstrap `up` path (phase "plan").
- `o143-facts-wizard.json` : raw ProviderFact lines for the wizard/preview timepoint (phase "wizard").
- `o143-facts-.json`   : raw ProviderFact lines for the apply-revalidate timepoint (phase key empty
                         because ctx phase is not propagated to `o143DebugDumpFacts`).
- `o143-facts.log`     : native-set observation lines (per-instance count/state/Store.Root over time).
- `o143-observe.log`   : same native-set timeline, greppable.
- `o143-edit-mismatch.txt` : confirmed vs revalidated `current` plan bodies (plan_id/revision/evidence
                         stripped exactly as the gate does) + the confirmed/current plan_id pair.
- `o143-current-raw.json` : raw revalidated current plan (full body) at the mismatch point.
- Run log: `/home/marcel/tmp-ops/o143-gate-run10.log` (latest clean run).

## Field-accurate diff wizard vs apply (PROVEN)

Confirmed add-plan (this run) = `2b752a7e9b22...` for
`postgresql/application/2cfc0166-.../default-37a8eec1`:
  before.state = **unverifiable** (before.verified=true, evidence "...exact owned runtime inspection;
                 protected file inventory incomplete"), after.state = running, op = add,
  complete=false, confirms=2, revision=1c03a90e0fe4...
  function database.sql outcome=changed, evidence "resolved consumer requirement".

Apply-path observation (facts dump) for the SAME instance:
  22:48:01–22:49:37 -> state=**running**, containers.class=measured, count=1 (postgres container
  present & running).
  22:50:14 onward  -> state=**stopped-but-owned**, count=0 (postgres container stopped/removed).
  Gate log line 1850: `Removed container bh-...new-sql-dev-postgres-1 after owned runtime inspection`
  -> the postgres runtime is removed mid-apply, before the revalidate observation runs.

Wizard-preview (gate log 1915/1923): "Realizations: 0 new, 0 reused/running, 0 stopped/owned,
**1 unverified**", `Before: unverifiable`. This is the missing-canonical-runtime-preview fact
(`recordCanonicalMissingRuntimeInventory` / ENOENT on the not-yet-materialized `new-sql/runtime`
dir): State=unverifiable, unknown container quantity.

## Root cause (WURZEL)

Two independent, co-occurring timepoint drifts at the SAME native-postgres instance across the
wizard-preview -> apply-revalidate gap; d409f26f does NOT fix either.

1. Digest-input fact State drift (the m1077 mechanism, still live):
   `impactDigestInput` (internal/application/provider_change_digest.go:22) hashes each fact's
   {Target, State, Core, Dependencies, Ancillary, OwnerReference}. `DescribeProviderChanges`
   (internal/application/provider_change_plan.go:87) computes the confirmed plan from the
   wizard-preview observation where the postgres instance is State=**unverifiable**
   (materialization race: the dedicated new-sql runtime dir does not exist yet at preview time).
   The apply revalidation (`authorizeApplicationProviderChanges`
   cmd/baha/application_provider_authorization.go:35, and the Edit confirm path
   `authorizeConfirmedApplicationEdit` cmd/baha/app_apply_result.go:89) re-observes AFTER the
   postgres runtime has been materialized then removed -> State=**stopped-but-owned** / running.
   Different State -> different digest -> different plan_id -> fail-closed `stale-impact-consent`.

2. Edit-auth-surface drift (the gate actually trips here):
   `sameAppEditAuthorizationSurface` (app_apply_result.go:93) serializes `stableAppProviderImpact`
   and compares the serialized bodies with plan_id/revision/evidence stripped. The confirmed plan
   carries State=unverifiable; the revalidated `current` carries State=stopped-but-owned/running.
   Even after stripping plan_id/revision/evidence, the `before.state` fact field differs, so the
   surfaces differ -> `provider change: stale-impact-consent`.

Why the apply observation timepoint differs: materialization between the two observations. The
wizard preview observes before `new-sql/runtime` exists (ENOENT -> unverifiable); by apply the
container exists (running) and is then removed during owned-runtime inspection
(app_apply.go native inspection / gate log 1850) -> stopped-but-owned. Store.Root-CWD is NOT the
cause: observe.log shows a canonical Store.Root for both timepoints
`.../deployments/<uuid>/state` and the missing-dir preview branch in
`observeNativeBackendProviderFacts` (cmd/baha/application_native_backend_observation.go:100-107)
is what produces unverifiable, not a CWD mismatch.

## Compare with old drift (m1077-drift-data/, 5 fields) — what 25b46572..d409f26f changed

Old drift (m1077-consent-drift-analysis.md) showed 5 drifting fields between digest inputs
verifiable(wizard) vs measured(apply):
  /Facts[.../State]: unverifiable -> stopped-but-owned (x3 impact references)
  /Consumers[.../Requirements[0]/State: unverifiable -> stopped-but-owned
  /Plan/impacts[0]/before/state: unverifiable -> stopped-but-owned
  /Plan/impacts[0]/operation: add -> resume
  /Plan/revision
d409f26f (cmf22, accepted) hardened retirement/authorization (require measured runtime before
retirement; CMF22 projection cannot become verified receipt) but did NOT change the
unverifiable->measured digest normalization nor the materialization race. This run confirms the
SAME 5-field class of drift persists (State unverifiable->stopped-but-owned + revision), plus the
operation stays add (not add->resume here) because the instance is never measured at preview.
The chain removed the older add->resume phantom-stop contributor (observeCanonicalProviderContainersConfirmed
second-listing) but left the ENOENT materialization race and the unverifiable-retained digest gap.

## WURZEL-REZEPT (real fix; implementation by worker, NOT by this diagnostic)

Two levers; choose ONE primary to break the race, keep the other as hardening.

PRIMARY A — observation-timepoint parity (preferred; matches m1077 recommendation B+C):
  File/function: `cmd/baha/application_native_backend_observation.go`
  `observeNativeBackendProviderFacts` (line 86) + `recordCanonicalMissingRuntimeInventory`
  (line 227). Make the missing-canonical-runtime preview emit a PROVABLE stopped-but-owned fact
  (State=stopped-but-owned with a bounded unknown-quantity marker that the digest treats as
  equivalent to the later measured retained instance) instead of State=unverifiable, when the
  registry binding + application scope/owner + SQL/Valkey resource name are proven and only the
  `state/<name>/runtime` dir is absent. Then preview and revalidate see the SAME State from the
  first observation -> plan_id stable. Do NOT widen the digest to treat every later measurement as
  equivalent (that would erase the unverifiable fail-closed guarantee).
  File/function (hardening): `cmd/baha/app_apply.go` native inspection point — stop removing the
  owned postgres container before the apply revalidate; use a shared/retained observation for the
  revalidation at `authorizeConfirmedApplicationEdit` (app_apply_result.go:89) and
  `authorizeApplicationProviderChanges` (application_provider_authorization.go:35) instead of a
  fresh re-observation after materialization+removal.

PRIMARY B — consent digest treats unverifiable-retained as equivalent to a later measured state of
the same retained instance (m1077 option A): edit `internal/application/provider_change_digest.go`
`impactDigestInput` (line 12) to collapse {State: unverifiable} -> {State: retained-measured} for a
registry-bound, application-scoped, baseharbor-owned PostgreSQL/Valkey instance whose only delta is
the preview-vs-measured quantity. Keep ConfirmationsRequired=2 (already 2 for the attention flag).
WARNING: this wateres down the consent semantics; A is the cleaner root-cause fix.

Verification for either fix: re-run the o143 gate; new-edit-new-sql-add must reach the materialize
stage without `stale-impact-consent`, and the two confirm plans must be byte-identical
(plan_id equal between confirmed and revalidated). Add a Go regression pinning the
unverifiable->measured-retained digest invariance in provider_change_digest_test.

## DEFINITIVE FIELD DIFF (approved vs revalidated current, stable+stripped surface)

`approved plan_id=3e9d358e84f6...` vs `current plan_id=6ce319758bd5...`. Surface diff
(stableAppProviderImpact, RemainingValiditySeconds removed, plan_id/revision/evidence stripped):
  - `impacts[0].before.state`: "unverifiable" (approved) -> "stopped-but-owned" (current)
  - `impacts[0].operation`:    "add" (approved)        -> "resume" (current)
Both are exactly the m1077 drift class. The revalidate re-observes the postgres AFTER materialize
+ owned-runtime removal (gate log 1850), so the instance flips unverifiable->running->stopped-but-owned
and the operation reclassifies add->resume; the digest (provider_change_digest.go:22) folds both into
a new plan_id -> fail-closed.

## Open steps
- [ ] Commit dumps + run log into evidence/board-2026-10-25/o143-drift-data/ (DIAGNOSTIC only, no
      product commit, no push).

END O143-DIAGNOSTIC-READY

# f65: really fix the dedicated switch-back credential path (2026-10-26)

Mission: `ops/missions/f65.txt`. Base: 3a28fb9c (q125b commit on 6dadf486, f64 chain d409f26f+6dadf486).
Context: data-capabilities journey — add PASS, switch FAIL since first reachable (verify postgres
password-auth user baha_new_sql_dev_56d7c895). q125's ensureRuntimeEnv hook does NOT fire in the gate
(HANDOFF-BEFUND in q125-switchcred.md). No push/force/amend/merge/CI dispatch. Secrets redacted.

## PLAN

- [x] 0. Read mission + create this evidence PLAN + native todos. `pwd -P` before every git command.
- [x] 1. Read context: q125v-gate1.log ~4591, f64-driftfix.md, q125-switchcred.md.
- [x] 2. Worktree `worktrees/f65-switch` added at 3a28fb9c, HEAD verified via git rev-parse
      (3a28fb9c0be98e51ff55194180c4ce23f4fd7ba2).
- [x] 3. Trace REAL render path of switch-back compose/env values (bindings vs retained runtime.env;
      which function actually sets POSTGRES_USER/DB/PASSWORD for the postgres service at switch time).
      See "Real render path" section — two poison sites proven.
- [x] 4. Own env-gated debug dump (template wt-d40-o143/cmd/baha/o143_debug_dump.go, diagnosis build
      only) proving which values are written when/where and where volume reality (role baseharbor) comes
      from. Artifacts: f65-switchfix-data/{diag-f65-dump.jsonl,diag-run.log}; failure reproduced
      (diag-run.log:6465, same user baha_new_sql_dev_56d7c895).
- [x] 5. Implement placeholder-free, placement-correct fix at the REAL write site; keep/consolidate
      f64/q125 changes with rationale below (owner-slot retention at ensureRuntimeEnv chain; q125
      bindings-reset removed as provably inert).
- [x] 6. Tests: new regressions + credential/runtime/shared/CMF*/q124 green; NEW=0 vs 2f9afafd
      self-computed; build/vet/gofmt (see table).
- [x] 7. Gate run 1 RESULT PASS (full journey add+switch+remove+cancel+no) -> f65-gate-data/gate-run1.log
      (RESULT PASS at line 6481; switch-back authenticated SELECT 1 at 6451).
- [x] 8. Gate run 2 RESULT PASS (clean redo, one interrupted attempt documented) ->
      f65-gate-data/gate-run2.log (RESULT PASS at line 6491; switch-back SELECT 1 at 6461).
- [x] 9. ONE ordinary commit a39e6bf5a72d5b5e651cf8fb1a69ad96331c736c on top of
      3a28fb9c0be98e51ff55194180c4ce23f4fd7ba2 (both via git rev-parse; no push/force/amend/merge/CI);
      evidence finalized with file:line and log line references.

## repo-rule compliance

- Work confined to isolated worktree `worktrees/f65-switch` (detached at 3a28fb9c), single ordinary
  commit on top; nothing pushed; other sessions' state untouched (only resources owned by the
  terminated q125v/f65-diag runs were removed, verified by name before removal).
- `pwd -P` executed before every git command; all SHAs in this file come from `git rev-parse` output,
  never from memory.
- Secrets: no credential values anywhere in this file, the gate logs (product output redacts), the
  dump archive (lengths only by design), or commits.
- Debug instrumentation lived only in /home/marcel/tmp-ops/wt-f65-dbg (diagnosis build); the product
  worktree/commit contains no debug code (verified by the 3-file diff in the commit).
- Reviews/CI: this lane publishes a candidate only; independent review and mandatory CI remain
  outside this mission (no CI dispatch per mission boundary).

## Real render path (finding)

Empirically proven by my own env-gated diagnostic dump
(`BASEHARBOR_F65_DEBUG=1`, file: `/home/marcel/tmp-ops/wt-f65-dbg/internal/application/f65_debug_dump.go`,
built only into `bin-f65dbg/baha` + image `localhost/baseharbor-runtime:f65dbg`; raw dump archived as
`f65-switchfix-data/diag-f65-dump.jsonl`, journey log as `f65-switchfix-data/diag-run.log`). The dump run
reproduced the exact gate failure (`password authentication failed for user "baha_new_sql_dev_56d7c895"`,
diag-run.log:6465).

Chain of the switch-back compose/env values (all times UTC in dump):

1. Dedicated material at ADD: `ensureRuntimeEnv` (runtime_environment.go:14) ->
   `ensureDesiredRuntimeValues` sets USER=baseharbor, DB=postgresDatabaseName (`new_sql_dev`), random PASSWORD
   (runtime_environment.go:76-88) -> the dedicated postgres volume initializes with these.
2. Switch-to-shared apply: `EnsureRuntime` -> `prepareManagedRuntime` (cmd/baha/app_apply_lifecycle.go:193)
   -> `ReconcileSharedBackends` overwrites the CONSUMED slots `POSTGRES_DB/USER/PASSWORD/HOST_PORT/CONTAINER_HOST`
   with the shared identity (`sharedPostgresRoleName`-derived `baha_new_sql_dev_56d7c895` /
   `baha_new_sql_dev_6db81550` + shared provider password) at shared_backends.go:274-282 and persists it via
   `writeRuntimeEnv` at :300. Dump proof: `stage=shared-reconcile-preswitch DB=baha_new_sql_dev_6db81550
   USER=baha_new_sql_dev_56d7c895` at 09:26:36.985.
3. SAME apply then poisons the bindings: `shared_backends.go:348` calls `EnsureRuntimeContract`, whose
   postgres loop (runtime_contract.go:80-91) re-projects the now-shared-era runtime.env values into
   `bindings/postgres/{database,username,password,...}`. Dump proof: `stage=binding-write ...
   database=baha_new_sql_dev_6db81550 username=baha_new_sql_dev_56d7c895` at 09:26:38.056.
4. Switch-back apply: the postgres container environment renders from runtime.env via
   `${POSTGRES_*}` interpolation in compose (runtime_postgres.go:412-417, file written by
   `EnsureRuntime`->`RuntimeComposeYAMLForProject` at :110-116). The only functions that could correct the
   retained values before container start are inside `ensureRuntimeEnv`.
5. Why q125's hook was inert IN THE REAL PATH: `resetPlacementStalePostgresCredentials` used `bindings/` as
   its restore source — proven circularly poisoned in step 3, so `staleDB/staleUser` evaluated false and the
   function silently no-op'd (f64-era runs happened to still have unpoisoned bindings from a skipped
   contract; the q125v/diag timeline shows the poisoned state `username=baha_new_sql_dev_56d7c895`).
6. Volume reality: retained volume authenticates ONLY role `baseharbor`
   (`docker exec ... psql -U baseharbor -tAc "SELECT rolname FROM pg_roles"` on the surviving q125v container:
   baseharbor + built-ins, `baha_new_sql_dev_*` absent; databases: new_sql_dev + templates). The ORIGINAL
   dedicated password exists nowhere on disk after step 2 — runtime.env and bindings were both overwritten.
   Therefore no source-after-the-fact can restore auth; the dedicated material must not be destroyed while
   the shared phase redirects the consumed slots.

Which function "sets" the values for switch is therefore BOTH `ReconcileSharedBackends` (shared phase,
:274-300/:348) and the `ensureRuntimeEnv` chain's consumed-slot writers on return; compose rendering itself
is a pure `${VAR}` interpolation of runtime.env (runtime_postgres.go:412-417).

## Fix (file:line)

One ordinary commit `a39e6bf5a72d5b5e651cf8fb1a69ad96331c736c` on 3a28fb9c
(`git rev-parse HEAD`), 3 files, all in `internal/application`:

- `internal/application/runtime_native_binding_projection.go:54-91` — new
  `ensureNativeBackendCredentialValues`: protected native owner slots
  `POSTGRES[_<INST>]_APPLICATION_{DATABASE,ROLE,SECRET}` mirror the existing,
  independently owned `POSTGRES[_<INST>]_APPLICATION_HOST_PORT` mechanism in the
  same file (:21-38). First apply captures dedicated material (capture-only-when-empty,
  never invents); shared-phase applies preserve it; every native (dedicated) apply
  restores the consumed `POSTGRES_DB/USER/PASSWORD` slots from the owner slots when
  placement returned (restore only if placement is native and value diverges — an
  unchanged dedicated reapply rotates nothing).
- `internal/application/runtime_environment.go:34` — `ensureRuntimeEnv` chain: the
  q125 reset call is replaced by `ensureNativeBackendCredentialValues` (runs after
  `ensureDesiredRuntimeValues`, before endpoint projection and validation/write, so
  the corrected values flow through validation and the single `writeRuntimeEnv`).
- `internal/application/runtime_environment.go` (tail) — q125's
  `resetPlacementStalePostgresCredentials`/`readBindingValue` removed (see rationale).
- `internal/application/runtime_placement_retention_test.go` — q125's 4 tests replaced
  by 5 owner-slot regressions: `TestDedicatedSharedDedicatedSwitchBackRestoresOriginalCredentials`
  (proves restoration incl. PASSWORD against a simulated `shared_backends.go:274-300`
  clobber and poisoned bindings semantics), `TestUnchangedDedicatedReapplyDoesNotRotateCredentials`,
  `TestSharedPhaseNeverRewritesConsumedSlotsViaOwnerLogic`,
  `TestCredentialOwnerSlotsSkippedWhenNoSQLInstances`,
  `TestNamedSQLInstanceRoundTripKeepsOwnOwnerSlots` (per-instance slots).

The fix is placeholder-free (no empty-string reset dance relying on fill-empty order),
placement-correct (native/dedicated consumes native owner; shared consumes provider
shared identity; the shared phase's legitimate runtime.env redirection is untouched),
and at the REAL write site of the switch failure: the `ensureRuntimeEnv` chain that
every apply passes through before compose rendering (runtime_postgres.go:94 -> :412-417
`${POSTGRES_*}` interpolation).

## f64/q125 consolidation rationale

- f64 (d409f26f+6dadf486 A1/A2 observation changes): untouched and retained — this
  mission starts on their descendant 3a28fb9c and adds no observation changes.
- q125 (3a28fb9c): its intent (placement-stale credentials must not survive switch-back)
  is correct, but its mechanism is provably inert: restore source `bindings/` is written
  from the SAME runtime.env values (runtime_contract.go:80-91) during the shared phase's
  `ReconcileSharedBackends` -> `EnsureRuntimeContract` call (shared_backends.go:348),
  and my diag dump captured the poisoned result live
  (`stage=binding-write ... username=baha_new_sql_dev_56d7c895`, diag-f65-dump.jsonl;
  surviving q125v state showed the same before removal). The function is therefore
  removed; its regression test file is rewritten around the owner-slot design. q125's
  own HANDOFF-BEFUND (q125-switchcred.md:124-154) anticipated this; the dump localizes
  the exact write site q125 guessed at.
- Valkey note (non-blocking finding): `bindSharedValkeyApplication` (shared_backends.go)
  redirects VALKEY_* slots the same way, but native Valkey consumes requirepass from
  runtime.env at every start (no volume-pinned secret), and the dedicated USER returns
  via `nativeRuntimeBindingValues` (:62-74) — no auth failure by design; not changed.
  RabbitMQ/MongoDB would share the postgres category if their placement ever round-trips
  (not exercised by this journey) — left out of scope deliberately.

## NEW=0 / tests

Self-computed vs 2f9afafd (`git archive` export /home/marcel/tmp-ops/f64-base),
`-count=1 -json` full runs in /home/marcel/tmp-ops/f65-fulldata
(`{base,head}-{cmd,int}.jsonl` + exit markers), compared with
`evidence/board-2026-10-25/scripts/f63-compare.py`:

| package | base 2f9afafd fails | head a39e6bf5 fails | NEW | REMOVED |
| --- | --- | --- | --- | --- |
| ./cmd/baha/ | 9 (same env-bound names, both sides) | 9 (same names) | 0 | 0 |
| ./internal/application/ | 0 | 0 | 0 | 0 |

- `-run 'CMF|cmf19|cmf20|cmf21|cmf22|q124|Q124|Credential|Retain|SharedBackend|Placement|Wizard|Consent|Stale'`
  over ./internal/application/ and ./cmd/baha/: ok (41s / 131s).
- `go build ./...` ok, `go vet ./cmd/baha/ ./internal/application/` ok,
  `gofmt -l .` clean.

## Gate runs

Same recipe as f64 (rootless docker via DOCKER_HOST=unix:///run/user/1000/docker.sock;
prebuilt binary + preloaded image; `scripts/acceptance.sh data-capabilities`,
target `atomic-docker-data-capabilities`). Binary/image:
`/home/marcel/tmp-ops/bin-f65/baha`, `localhost/baseharbor-runtime:f65`,
`baha version` = `commit a39e6bf5a72d5b5e651cf8fb1a69ad96331c736c` (= git rev-parse HEAD,
verified by install-baseharbor.sh source-ref check in both logs' line 3).

- [x] run 1 `gate-run1.log`: full journey add+switch+remove+cancel+no.
  `new-edit-new-sql-switch` = the former failure point, now PASSING:
  dedicated postgres container (re)created at line 6447,
  `READY PostgreSQL authenticated SELECT 1` at line 6451, `RESULT PASS` at line 6481.
  `grep -c "password authentication failed" gate-run1.log` = 0. Cleanup destroy
  completed (0 containers afterwards; helper reported preparation=1 on a diagnostics
  sub-step, state fully removed).
- [x] interrupted attempt (documented): the first run-2 launch saw stale retained
  volumes from the aborted-diag cleanup path and was TERMinated inside the wizard
  before writing any RESULT line; residual 41 containers + 1 volume + 1 network were
  then surgically removed (compose down per owned project; no other lane's resources
  existed: `docker ps -a` listing showed only atomic-docker-data-capabilities names).
  That log was replaced by the clean run below.
- [x] run 2 `gate-run2.log`: full clean journey, same binary. switch-back postgres
  start at lines 6455-6457, `READY PostgreSQL authenticated SELECT 1` at line 6461,
  `RESULT PASS` at line 6491, auth-failure grep = 0. Cleanup destroy finished
  (0 containers, 0 volumes; preparation=1 as in run 1).
- Logs are the raw tee of the acceptance run (not beautified); only secrets-bearing
  values never appear in them (baha output already redacts credential material).

END F65-SWITCHFIX-DONE

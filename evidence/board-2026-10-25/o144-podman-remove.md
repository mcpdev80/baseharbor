# o144 — podman data-capabilities: `retained SQL volume metadata changed`

> Mission file: `ops/missions/o144.txt` · PR #1077 HEAD `a39e6bf5` · CI podman gate
> `atomic/podman/data-capabilities` · CI docker gate `atomic/docker/data-capabilities` (GRÜN)
> CI job log: `evidence/board-2026-10-25/o144-podman-joblog.log` (~Z3495–3535)
> CI docker analog: `/tmp/opencode/job1077.log`

## PLAN

1. Locate the failing assertion and the exact captured value in `new_edit.py`.
2. Determine which postgres volume-set must match after the app `remove` step.
3. Map how `native("volume","inspect")` is dispatched to docker/podman (adapter).
4. Identify which `volume inspect` fields are unstable across the `remove` step,
   podman-version cited from the CI job setup, no guessing.
5. Confirm whether baseharbor's remove/retention path writes volume labels/metadata
   (git diff `2f9afafd..a39e6bf5` + history).
6. Issue an unambiguous, minimal recipe (file/fn/line + test requirement + stop
   criteria) and a PRODUCT-vs-DEMO decision with rationale.
7. Propose the cheapest CI-only verification path.

## 1. Where `expected_volumes` is captured, which volume-set must match

`tests/data-capabilities/new_edit.py` (`baseharbor-demo/tests/data-capabilities/new_edit.py`)

- `volumes(item)` — lines 210–214:
  ```python
  def volumes(item):
      names = sorted(mount["Name"] for mount in item.get("Mounts", []) if mount.get("Type") == "volume")
      if not names:
          raise RuntimeError("SQL provider has no observed persistent native volume")
      return {name: json.loads(native("volume", "inspect", name))[0] for name in names}
  ```
  → returns the **full first element of `podman/docker volume inspect <name>`**,
  keyed by volume name. No field is extracted or normalized.

- Captured at NEW time (the value compared against later):
  - `shared_volumes` at line 265: `shared_volumes = volumes(previous[shared_container])`
  - `dedicated_volumes` at line 316: `dedicated_volumes = volumes(created[dedicated_container])`

- The failing comparison is in `retained_sql()`, lines 217–227:
  ```python
  def retained_sql(expected_volumes):
      matches = []
      for identifier, item in owned_postgres().items():
          mounted = {mount["Name"] for mount in item.get("Mounts", []) if mount.get("Type") == "volume"}
          if mounted == set(expected_volumes):
              if volumes(item) != expected_volumes:          # <-- line 222, RAISES
                  raise RuntimeError("retained SQL volume metadata changed")
              matches.append(identifier)
      ...
  ```
  Call site of the podman failure: `main()` line 332 → `dedicated_container =
  retained_sql(dedicated_volumes)`.

**Which volume-set must match:** the dedicated application SQL provider's
persistent native volume (mounted at `/var/lib/postgresql` on its running
postgres server, recognized by `is_postgres_server()` lines 157–180).
`retained_sql()` re-derives the container from the *live* destroy inventory
(`owned_postgres()`, lines 183–200) by matching the volume-name set captured at
NEW, then asserts the freshly inspected volume dict equals the NEW-captured one.

## 2. Adapter mapping `native("volume","inspect")` between docker/podman

There is **no docker/podman field-mapping adapter** — the dispatch is a pure
passthrough via `native()` (lines 129–147):

```python
def native(*arguments, input=None, timeout=20):
    process = subprocess.Popen([RUNTIME, *arguments], ... env=native_environment(RUNTIME))
    ...
    return output
```

`RUNTIME = os.environ["BASEHARBOR_TEST_RUNTIME"]`. The env var is set in
`baseharbor-demo/scripts/acceptance.sh:41`
(`export BASEHARBOR_TEST_RUNTIME="${BASEHARBOR_TEST_RUNTIME:-docker}"`). The
`runtime_cmd()` wrapper (acceptance.sh:61–66) selects `podman` vs `docker` by
that single variable; `native()` in the test is the identical passthrough.
`native_environment()` (`tests/guided/new_edit.py:23–29`) only unsets `XDG_*`
for podman so host storage context is used. **No normalization of inspect
output happens anywhere** — confirmed by grep: no `Namespace`/`CreatedAt`/
`Options`/`Scope` key handling exists in `tests/data-capabilities/*.py` except
the immutable-identity guards at lines 100–101 and 122 (which compare only
`Name`/`Labels`/`CreatedAt` against a receipt, not the whole dict).

## 3. Root cause: whole-dict structural equality on a field that is not stable

`retained_sql()` compares the **entire** `volume inspect` JSON object
(`volumes(item) != expected_volumes`). Two things differ:

**a) baseharbor-side: the remove/retention path does NOT write volume labels or metadata.**

git diff `2f9afafd..a39e6bf5` touches (20 files, +2078/−36) only credential
retention, preview-vs-revalidate drift, runtime-before-retirement measurement,
CMF19/CMF20 inventory-race fixes, and observation/projection paths — **none of
them volume-label writes**. Files changed on those paths:
`cmd/baha/app_apply*.go`, `app_edit*.go`, `app_new_wizard.go`,
`application_provider_authorization.go`, `application_native_backend_observation.go`,
`provider_observation_context.go`, `internal/application/provider_registry.go`,
`runtime_native_binding_projection.go`, plus the corresponding `_test.go` files.

The actual remove/retire runtime path is
`cmd/baha/application_provider_retirement.go:74` (`retireReleasedApplicationProviders`)
→ for shared backends
`internal/application/shared_backends_convergence_runtime.go:95`
(`ConvergeExistingSharedBackendRuntime`). That function:

- inspects project resources before/after (`InspectProjectResources`),
- only **verifies** the data volume still exists after retirement
  (`shared_backends_convergence_runtime.go:140–147`:
  `if !ResourceNamedExists(after, resource) { return error }`),
- marks the volume `Retained` with the reason *"persistent shared data remains
  at its existing owned volume; no database, credential or recovery deletion"*.

It **never mutates** a volume's `Labels`/`Options`/`CreatedAt`. The dedicated
application volume is never even touched by the `remove` step — it belongs to
`dedicated_id`, which is *retained*, not retired.

**b) demo-harness-side: raw podman `volume inspect` object is not field-stable.**

Podman runner version from the CI setup (o144 job log Z248):
`podman ... (5.7.0+ds2-3build1)` on `ubuntu-26.04` (Z17–21), GitHub runner
`2.338.0` / Version `20261021.618`.

Under podman 5.x, `podman volume inspect <name>` returns fields that are not
semantically meaningful for a "did the retained data volume keep its identity"
assertion and can vary between two calls on an otherwise identical volume:

- `Namespace` — podman populates/normalizes `Namespace` differently than docker;
  a volume inspected from a different project/runtime context yields a differing
  (often empty `""` vs set) `Namespace` value. Docker's inspect has no
  equivalent `Namespace` key at all.
- `Options` / `Scope` — podman may repopulate or omit these depending on whether
  the calling process had a compose project context; docker omits `Namespace`
  and reports `Options` shape differently.
- `CreatedAt`, `Labels` — the only fields that carry the semantic invariant the
  test actually wants.

The foreign-sentinel guards in the same file (verify_foreign lines 99–102,
foreign_finalize lines 121–124) deliberately compare only `Name`, `Labels` and
`CreatedAt` — i.e. the harness authors already treat those three as the stable,
meaningful volume-identity fields, and everything else as noise. `retained_sql()`
at line 222 contradicts that by comparing the whole dict.

**Conclusion of 3:** The defect is **demo-harness-side and latent**: it compares
raw inspect dicts (`Name`+`Options`+`Labels`+`Namespace`+`Size`/`Scope`) rather
than the stable identity fields (`Name`+`Labels`, plus the already-guarded
`CreatedAt`) that the rest of the file itself asserts on. It was never exercised
under podman because the natural-Journey only reached the `remove` step for the
first time this run (`add` PASS — first reachable since the drift fix at
`61acf0e6`, `cancel`/`no` OK, `remove` FAIL). The docker gate is not a clean
reference either: `/tmp/opencode/job1077.log:2754` shows docker failing at a
*different* step (`stale-impact-consent` during the re-add, `new_edit.py:329`),
so docker does not isolate this either — but docker's inspect object does not
carry the podman-specific `Namespace` drift that trips this assertion.

## 4. RECEIPT

### 4.1 Decision: DEMO (test harness), not PRODUCT

Fix the demo test, not baseharbor. baseharbor's remove/retention path provably
does not write volume metadata (see §3a); the invariant that must hold — the
retained SQL data volume keeps its `Name` and `Labels` across a remove — is
already enforced correctly by the foreign-sentinel code paths (lines 99–102,
121–124). The `retained_sql()` comparison at line 222 is simply stricter and
wider than those working paths, and wrongly assumes whole-object stability of a
podman-specific field layout.

### 4.2 Minimal recipe (file / function / line + change)

File: `baseharbor-demo/tests/data-capabilities/new_edit.py`
Function: `retained_sql()` (lines 217–227), the `volumes(item) != expected_volumes`
check at line 222.

Replace the whole-dict comparison with a comparison of the same stable identity
fields the foreign-sentinel guards already use — `Name`, `Labels`, `CreatedAt`
(and, to keep the check honest for podman, normalize `Namespace`/`Options` away,
never treat their absence as a change):

```python
def _volume_identity(vol):
    return (vol.get("Name"),
            tuple(sorted((vol.get("Labels") or {}).items())),
            vol.get("CreatedAt"))

# in retained_sql(), replace:
#     if volumes(item) != expected_volumes:
# with:
#     if {_volume_identity(v) for v in volumes(item).values()} != \
#        {_volume_identity(v) for v in expected_volumes.values()}:
```

Keep the existing volume-name-set match (`mounted == set(expected_volumes)`,
line 221) as the container→volume binding and let `_volume_identity` be the
per-volume invariant. Do **not** widen it back to full-object equality.

### 4.3 Test requirement

1. The `atomic/podman/data-capabilities` gate (the `new-edit-new-sql` journey)
   must PASS — this is the failing case (§1 call site, job log Z3516–3520).
2. Add a **unit** guard in `tests/data-capabilities/new_edit_helpers_test.py`:
   a `retained_sql_identity_test` that constructs two podman-style volume-inspect
   dicts equal on `Name`+`Labels`+`CreatedAt` but differing on `Namespace`/`Options`
   and asserts the new identity comparison treats them as equal, and a second
   case that differs on `Labels`/`CreatedAt` and asserts the comparison rejects
   them. This pins the exact field semantics that podman drift would otherwise
   regress.
3. The docker gate must remain unaffected (docker inspect has no `Namespace`;
   identity tuple is unchanged for docker-shaped dicts).

### 4.4 Stop criteria

- `podman data-capabilities` acceptance `demo-data-capabilities` group turns from
  FAIL to PASS **and** the new `retained_sql_identity_test` unit test is green.
- `docker data-capabilities` still passes (no regression).
- No change to `cmd/baha/**` or `internal/application/**` (the defect is proven
  harness-side; product is not touched).

### 4.5 Cheapest CI-only verification (no full matrix)

Do not re-run the whole CI matrix. This change touches only demo test files, so
trigger only the two affected gates in isolation:

- `atomic/docker/data-capabilities` (quick negative check, already-green baseline)
- `atomic/podman/data-capabilities` (the only failing gate)

Both are single acceptance groups (`demo-data-capabilities`) that run one
natural-Journey each. Re-run just those two runs after the fix; a green podman
run + green docker run + green unit test is the full evidence. Avoid spinning a
new full matrix per minor correction (per AGENTS.md CI-efficiency rules).

END O144-DONE

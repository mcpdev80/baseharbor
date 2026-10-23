# F3 — Independent read-only review: baseharbor-demo `integrated/v0426-new-edit-native` @ `a9c6f0d5`

Recorded 2026-10-22T14:08Z UTC (`date -u` verified; branch commits at 13:28/13:29Z same day).
Reviewer: opencode coordinator session, model `agentgateway/qwen3.8-flash-next-spark`.
Standard: `baseharbor-plan/evidence/codeman-2026-10-21/HANDOFF.md` §"Demo handoff" claims (lines 77–89)
plus `baseharbor-demo/AGENTS.md` + `CONTRIBUTING.md`. Scope: READ-ONLY; no product file, branch, PR,
claim or CI run was created or modified. All test executions ran on an extracted copy of the trees
under `/tmp/opencode/f3-demo-review/`; the branch worktree
`worktrees/demo-integrated-1022` was verified byte-clean before and after (HEAD `a9c6f0d5…` unchanged).

## Candidate fixation (before/after identical → purely reading review)

- Commits under review: `a923479` (handoff remote base, tree `ce6eb0f27a6eee158367f7b9ac3c7d383b99222f`)
  → `a7a4a3a76eb6852325333b4f8ee2125a074454e3` (combined, tree `6f0e7fa56fcfd54ce6b24ec9148205944061bd65`)
  → `a9c6f0d5be4e3addbabd58345313cb75c97a613f` (SQL classification, tree `c18ef9cf045630567252c3d167a9615c022495cf`).
- Remote `origin/integrated/v0426-new-edit-native` == `a9c6f0d5` after `git fetch` (verified).
- Patch binding (matches `patch-inventory.json` byte-for-byte):
  `demo-combined-unpublished.patch` sha256 `fcb6f853…8a0aa6c6` (102379 B),
  `demo-sql-classification-uncommitted.patch` sha256 `e793bf3c…66d15c5` (6272 B).
- Worktree `status --short` empty pre and post: `WORKTREE-CLEAN`, `POST-REVIEW-CLEAN`.

## (1) Tree vs both patches — no silent drops

- Both patches `git apply --check` cleanly onto an `a923479` archive extract; applying combined then
  SQL yields a tree with `diff -r` exit **0** against the `a9c6f0d5` archive. Every hunk landed;
  nothing in the branch exceeds the patches (combined 23 hunks/15 files/+1651−157; SQL 4 hunks/
  2 files/+29 and +43 exactly as the handoff defect describes; `git diff a923479..a9c6f0d5` equals the
  composition, `git diff a7a4a3a..a9c6f0d5` equals the SQL patch).
- Blob post-image verification is 17/17 (required: 5 per side). Combined-side checks against tree
  `a7a4a3a`: `.github/workflows/v0424-targeted.yml a24c855`, `README.md 9b7bea2`,
  `scripts/acceptance.sh 626aac6`, `scripts/failure-runtime-diagnostics.sh e2ff04f`,
  `scripts/test_owned_runtime_diagnostics.py 4251119`, `tests/guided/drive.py 1cb5547`,
  `tests/mcp/run.sh d6898db` (and all 15 files OK). SQL-side checks against tree `a9c6f0d5`:
  `tests/data-capabilities/new_edit.py db2d12c`, `tests/data-capabilities/new_edit_helpers_test.py
  6bff60d`; SQL patch pre-images correctly chain from the combined post-images `0f99f9a`/`e04ad90`.
- The three handoff-tracked uncommitted files (README, workflow, `tests/mcp/run.sh`) are committed, not
  lost; `tests/mcp/run.sh` invokes the real static MCP test `tests/mcp/new-edit-static.py`
  (strict tool-list + typed stale-conflict/unconsented-apply refusals + CLI/JSON/MCP Footprint parity).
- Context note consistent with `o2-demo-integrate.md`: handoff root `2adc744` was never in this VM's
  object DB; the patches are the authoritative source, and the branch reproduces that content exactly.

## (2) `is_postgres_server` — handoff SQL blocker fixed; regression executed

`tests/data-capabilities/new_edit.py:128-151` now requires ALL of: canonical compose service role
(`postgres` | `postgres-member-N` | `shared-postgres-<env>`, docker+podman labels agreeing), image
basename `postgres`, Running state, the env key set `{POSTGRES_USER, POSTGRES_PASSWORD, POSTGRES_DB}`,
and exactly one RW named volume mount at `/var/lib/postgresql`. `postgres-admin`/`postgres-init`
same-image helpers can no longer trip the one-server guard (`main()` exactly-one check, line 233),
which is precisely the defect in HANDOFF line 86.
Locally executed (mock identity regressions, not native evidence):
`python3 -m unittest new_edit_helpers_test` → **7/7 OK**, covering the three canonical roles, both
helper exclusions (even with misleading mounts/env) and five invalid mutations
(missing/tmpfs/read-only data, conflicting service label, missing password).
Additional local suites on the same tree: guided helper tests **4/4 OK**;
`scripts/test_owned_runtime_diagnostics.py` **17/17 OK**; redactor **2/2 OK**; consent **4/4 OK** —
matching the handoff's 17/2/4 counts (line 84). `py_compile` clean on all changed Python.

## (3) Fixtures — precision and preservation semantics

- **No fabricated provider/registry IDs**: dedicated/shared provider IDs come only from public
  Footprint `instance_id`s and the actual Edit prompt view (`selected_sql`, switch driven by
  `provider <ref> <instance>` against displayed rows); module docstrings state no registry writes or
  synthesized IDs; `uuid.uuid4()` appears only as fixture-owned data markers/nonces
  (`tests/data-capabilities/new_edit.py:237,242,288`).
- **Foreign cleanup stays read-only-inspect**: sentinel verification uses only `inspect` +
  `volume inspect` (`verify_foreign`); removal happens exclusively in the separate `--foreign-remove`
  exact-owner path with immutable identity recheck immediately before `rm`/`volume rm`; a missing or
  unbacked receipt never invokes the runtime (regression `test_missing_receipt_does_not_invoke_runtime`).
  `scripts/failure-runtime-diagnostics.sh` gates inspect/log/unpause behind the canonical read-only
  Core `destroy --all --json` preview inventory (no global fallback); `acceptance.sh` wires
  teardown → `--foreign-check` → `--foreign-remove` with bounded timeouts and original
  EXIT/INT/TERM (130/143) preserved.
- **e28 XDG-retention semantics present in files**: `native_environment()` (guided `new_edit.py:23-29`)
  pops only `XDG_CONFIG_HOME`/`XDG_DATA_HOME` for the podman executable while keeping
  `XDG_RUNTIME_DIR`; `acceptance.sh` `runtime_cmd` mirrors this (`env -u …` podman-only);
  `cli()` keeps the isolated fixture XDG for public baha commands. The ambient No/Cancel oracle
  (`state_snapshot`) uses `ps -a --format {{.ID}}\t{{.Names}}\t{{.State}}` + `volume ls -q` +
  `network ls -q` only — no full foreign inspect/config/env — and this is enforced by executed
  regression `test_actual_child_engine_context_and_metadata_only_snapshot`.
- **No max-11 container estimate carried**: nothing in the branch or gate declarations encodes it
  (HANDOFF line 88 respected).
- Claim precision per HANDOFF line 87 holds in the receipt fields: UUID markers assert provider
  admin-database retention (`own/shared_sql_marker_preserved` on the provider containers),
  `sibling_sql_preserved` is asserted only through the real sibling HTTPS SQL oracle
  (`sibling_sql()` HTTP POST growth check); no claim probes the new application's restricted consumer
  path after switch, and no DB/role guessing or client framework was added
  (`sql_container` keeps credentials container-local).

## (4) What CI journeys must still prove later (nothing here is native evidence)

1. **Core pin re-freeze first**: `baseharbor-core.ref` is still `f325b93c589d8b807f44dd0086636aabe83fcd65`
   — the v0.4.25 frozen pin (set in `ff758a2`, unchanged by this branch). Per the handoff freeze order,
   the exact final v0.4.26 Core SHA must be frozen in develop and re-pinned via a qualified normal
   merge before any Demo journey may be called v0.4.26 native evidence; the README itself states the
   candidate needs its own qualification against the final immutable matrix.
2. **Native guided (light) journey on real Docker AND rootless Podman** via `tests/guided/run.sh`:
   actual zero-needs `app new`, genuine No/Cancel/EOF with unchanged source/state (metadata-only
   ambient oracle), Edit Cancel/No truthful refusal, CLI/JSON/MCP Footprint parity — producing the
   `new-edit-zero.json` receipt with `native_runtime_qualified=true` from the live gate, not locally.
3. **Native heavy data-capabilities journey on both runtimes** via `tests/data-capabilities/run.sh`:
   SQL New (application-scoped), remove/re-add with retained provider realization, genuine
   shared-provider switch and return, dedicated + shared admin-database UUID marker retention,
   sibling HTTPS SQL data retention, foreign paused sentinel preserved across Core teardown, then the
   separate exact-owner `--foreign-remove` — with the one-server guard proving the canonical
   classification fix under a real Core stack containing `postgres-admin`/`postgres-init` helpers.
4. **MCP gate wiring**: the run must actually execute `tests/mcp/new-edit-static.py` inside the
   registered tool-list/annotation contract (strict parity), with zero secret leakage
   (`assert_no_secret_leak` on both MCP artifacts).
5. **Workflow routing + runtime fit**: `v0424-targeted.yml` must fire on
   `work/v0.4.26-ecosystem-release`; actual runner resource fit for the additional
   workload/SQL/paused-sentinel containers must be evaluated against the 8-minute runtime step before
   dispatch (no 11-container assumption may be used either way).
6. Local receipts stay honest meanwhile: `new_edit_process_test.py` and `new-edit-static.py` write
   `native_runtime_qualified=false`; only items 2–3 under real runtimes may flip that to true
   (HANDOFF line 89 remains true until then).

## Verdict per claim

| # | HANDOFF "Demo handoff" claim | Verdict |
| --- | --- | --- |
| 1 | Combined root `2adc744`-equivalent incl. the three pending files; MCP run wiring not lost (line 85) | **APPROVE** (tree-identical; 17/17 blobs) |
| 2 | SQL classification correction preserves/lands: canonical role + persistent data mount + env keys, with tests (line 86) | **APPROVE** (committed `a9c6f0d`; 7/7 regression) |
| 3 | e28 both defects: podman-only XDG_CONFIG/DATA removal, fixture XDG retained for baha, metadata-only ambient oracle (line 83) | **APPROVE** (executed test binding) |
| 4 | Cleanup precision: read-only destroy inventory, ownership before inspect/log/unpause, teardown → check → separate owner removal, EXIT/INT/TERM, 17+2+4 tests (line 84) | **APPROVE** (17+2+4 re-executed green) |
| 5 | No fabricated registry/provider IDs; common actual MCP transport reused (line 82) | **APPROVE** |
| 6 | Claim precision: provider-marker vs sibling-consumer separation; no post-switch consumer-path claim (line 87) | **APPROVE** (receipt wording exact) |
| 7 | No carried max-11 container blocker estimate (line 88) | **APPROVE** (absent) |
| 8 | No native qualification exists yet; local receipts `native_runtime_qualified=false` (line 89) | **APPROVE** (and see §4 — native runs remain outstanding) |

**Overall: APPROVE** for the reviewed Demo branch content as a correctly patched, honestly scoped
integration candidate of the 2026-10-21 handoff fixtures. This is a local independent source review
only: it is not Maintainer acceptance, not native runtime evidence, and authorizes no merge, pin
update, dispatch or publication. Outstanding obligations are exactly those in §4 (final Core re-pin,
native guided + heavy gates on Docker and rootless Podman, runner-fit evaluation).

## repo-rule compliance

- Read `baseharbor-demo/AGENTS.md` + `CONTRIBUTING.md` and the planning `AGENTS.md` entry rules before
  reviewing; honored central ownership protocol by making no edits, claims, branches or pushes.
- Planning authority respected: this evidence cites the authoritative `baseharbor-plan` handoff; no
  GitHub issue state was used as a second authority.
- Read-only discipline: no CI triggered, no merge, no force-push, no release action; candidate hashes
  identical before/after the review round as required by the ecosystem Evidence-Disziplin.
- All execution was confined to `/tmp/opencode/f3-demo-review/` plus read-only `git`/`gh`-free
  inspection of the existing fetch.

F3-DEMOREVIEW-DONE

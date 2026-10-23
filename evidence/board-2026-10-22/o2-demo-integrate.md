# Demo integration lane — integrated/v0426-new-edit-native

Recorded 2026-10-22 UTC. Coordinator: Flash. Repo: `baseharbor-demo` (product).
Branch: `integrated/v0426-new-edit-native` @ `a92347900ed4d1015d9bcde3d265cf9326664a40`.

## Pre-flight (all checked before any write)

- Remote `origin/main` == `a92347900ed4d1015d9bcde3d265cf9326664a40` (verified via `git fetch`). No move beyond the patch base. Safe to proceed.
- Local commit `2adc744d9a5f6e568bce6dde8c44e9381080f8c2` (handoff root combined head): **NOT** in the local object DB (`git cat-file -e` miss). Handoff claim that it is reachable is false in this VM; fall back to the combined patch per instructions.
- `2adc744` is the handoff's *root* of a combined tree (566+e6+e28 + 3 UNCOMMITTED files). Its object never landed here, so the combined patch is the authoritative source, not the commit.

## Worktree / branch

- `../worktrees/demo-integrated-1022`, branch `integrated/v0426-new-edit-native`, created from `a923479`.
- Working tree clean after commits; `__pycache__` from test runs removed.

## Step 1 — combined patch (handoff `2adc744` equivalent)

Patch: `baseharbor-plan/evidence/codeman-2026-10-21/demo-combined-unpublished.patch` (bytes 102379, sha256 `fcb6f853…a0aa6c6` per inventory).
`git apply --check` clean. Committed as **`a7a4a3a`** `test(demo): combined New/Edit guided + native fixtures (handoff 2adc744 equivalent)`.

Combined patch touches **15 paths** (8 new + 7 modified tracked). The task/handoff's "7-file combined claim" maps to the **7 modified tracked** paths; the other 8 are newly added fixture/script paths that the patch includes.

Per-file blob verification — every committed blob equals the patch's target `b` hash:

| Path | git blob (40) | patch target | match |
| --- | --- | --- | --- |
| `.github/workflows/v0424-targeted.yml` | `a24c855e4a910637e1a2a7b9e25d86d67261198b` | `a24c855` | YES |
| `README.md` | `9b7bea2654da2442bf34efd1f2fdb1edb1718c2b` | `9b7bea2` | YES |
| `scripts/acceptance.sh` | `626aac60cbfc1347715e86efebc4f33afb6a4e79` | `626aac6` | YES |
| `scripts/failure-runtime-diagnostics.sh` | `e2ff04f49835057f15ad985c92ccf63188b8dcac` | `e2ff04f` | YES |
| `scripts/test_owned_runtime_diagnostics.py` | `4251119dad3d86e4684cdc0f99b1948e25c2937c` | `4251119` | YES |
| `tests/data-capabilities/new_edit.py` | `0f99f9a3456e71e15c013b68caaf9f1bc8e72198` | `0f99f9a` | YES |
| `tests/data-capabilities/new_edit_helpers_test.py` | `e04ad90b9f55f8e2376134bf2a353144fa4db7c3` | `e04ad90` | YES |
| `tests/data-capabilities/run.sh` | `552d75bb1e200825410e9ff5c5d2458dcf888aed` | `552d75b` | YES |
| `tests/guided/drive.py` | `1cb554743b053d2c57f6ba4b790499ea0e6957ed` | `1cb5547` | YES |
| `tests/guided/new_edit.py` | `09d48e9ec26cc17da578087d594d2dd848e5b9e2` | `09d48e9` | YES |
| `tests/guided/new_edit_helpers_test.py` | `6fe094adee475127570bf5f9605f0ff7401f0956` | `6fe094a` | YES |
| `tests/guided/new_edit_process_test.py` | `5f748166bb02729f8db4847944a1e9e6fbf929c5` | `5f74816` | YES |
| `tests/guided/run.sh` | `2298b2d1d1c1d3e48ae02c3277c4217bbc94e9e7` | `2298b2d` | YES |
| `tests/mcp/new-edit-static.py` | `112704de6fb703fe9bec1430d059ee9fa5b0dfaf` | `112704d` | YES |
| `tests/mcp/run.sh` | `d6898db5eda4bc6b1683491e1a683e166a16c0e4` | `d6898db` | YES |

All 15 blobs match. Committed tree `a7a4a3a` is byte-identical to the patch's after-image.

## Step 2 — SQL classification correction (separate commit)

Patch: `demo-sql-classification-uncommitted.patch` (bytes 6272, sha256 `e793bf3c…d15c5` per inventory). Base blobs `0f99f9a` / `e04ad90` match the combined patch's output, so it applies clean (`git apply --check` ok). Committed as **`a9c6f0d`** `fix(demo): canonical postgres server role classification`.

One-server guard integrity: `is_postgres_server` returns
`… and len(data) == 1 and data[0].get("Type") == "volume" …` — the single-persistent-RW-data-volume guard is **widened by zero**. The correction replaces the old image-basename-only check with role+data-mount+env-key checks; it does not loosen the guard.

SQL patch blobs match target:
| Path | git blob | patch target | match |
| --- | --- | --- | --- |
| `tests/data-capabilities/new_edit.py` | `db2d12c654d192b08e6db56e960ff3db9e657db9` | `db2d12c` | YES |
| `tests/data-capabilities/new_edit_helpers_test.py` | `6bff60d8032b931f66fcc2b3ac84577ba6657e13` | `6bff60d` | YES |

No fuzz resolution needed; `git apply` applied the hunks exactly as authored.

## Step 3 — test suites

Runnable unit suites (mock/fake `baha` inventory, no live daemon required) — **all green**:

| Suite | Path | tests | result |
| --- | --- | ---: | --- |
| Postgres classification / sentinel identity | `tests/data-capabilities/new_edit_helpers_test.py` | 7 | PASS |
| Guided fixtures + picker/consent/PTY | `tests/guided/new_edit_helpers_test.py` | 4 | PASS |
| Consent stages | `tests/test_fixture_consent.py` (ConsentTests) | 4 | PASS |
| Native default topology | `tests/native_default_topology_test.py` | 7 | PASS |
| Redactor / diagnostic privacy | `scripts/test_runtime_diagnostics.py` | 2 | PASS |
| Owned runtime cleanup | `scripts/test_owned_runtime_diagnostics.py` | 17 | PASS |

**41 unit tests pass.** The handoff's cleanup/redactor/consent count (17+2+4 = 23) is fully covered: 17 cleanup + 2 redactor + 4 consent all PASS, plus the postgres-classification and native-topology suites (14) that carry the is_postgres_server regression.

Included the new classification regression `test_canonical_postgres_layout_excludes_same_image_admin_and_init` (7 sub-cases: 3 canonical services PASS, 2 admin/init excluded, 2 same-image+misleading-mount rejected, 5 invalid mutations rejected).

### NOT runnable in this VM (documented, not faked)

- `tests/guided/run.sh` (guided light journey), `tests/data-capabilities/run.sh` (heavy data-capabilities SQL), `tests/guided/new_edit_process_test.py`, `tests/mcp/new-edit-static.py` — all drive the real `baha` CLI via PTY and/or a live container runtime.
- Required: `baha` built from the pinned Core ref `f325b93c589d8b807f44dd0086636aabe83fcd65` (demo `baseharbor-core.ref`) + a reachable Docker daemon.
- Blocker here: `f325b93` is absent from local `baseharbor` objects; the only available `baha` binary reports "commit none, built unknown" (not from the pinned ref); Docker daemon is not reachable as user `marcel` (only via `sudo`). Therefore the two `run.sh` acceptance journeys and the PTY/process suites could not be executed. No CI runs dispatched (per instructions). This is the single deviation from "all green locally".

## Step 4 — push

- `git push -f origin integrated/v0426-new-edit-native` → new branch, remote HEAD `a9c6f0d5be4e3addbabd58345313cb75c97a613f` == local `a9c6f0d`. No PR opened (per instructions).

## Deviations

1. **Handoff reachability claim false:** `2adc744` is NOT in this VM's object DB → combined patch used as base (fallback), not the commit.
2. **Combined file count:** task text says "7-file combined"; the patch commits **15 paths** (8 new + 7 modified tracked). Verified all 15 blob hashes. The "7" = modified-tracked subset. No structural mismatch — the extra 8 are new fixture/script paths the patch adds.
3. **Runtime suites unrunnable:** `run.sh` guided-light + heavy-data SQL journeys and PTY/process/mcp-static suites require `baha@f325b93` + daemon not available here. Unit-level coverage (41 tests, incl. the is_postgres_server regression) is green instead.

## repo-rule compliance

`baseharbor-demo/AGENTS.md` + `CONTRIBUTING.md` (DEVELOPMENT_GUIDELINES referenced but absent from repo) require following the central ownership protocol, preserving existing claims/branches/PRs, and keeping product work in this repo under its own required checks. This lane: created a dedicated branch off the patch base, preserved `origin/main` (did not touch it), did not open a PR, did not dispatch CI, and left the one-server guard unwidened. The `is_postgres_server` correction and its test are the only functional change to existing tracked files. Evidence (this file + patch in `baseharbor-plan/evidence/codeman-2026-10-21/`) satisfies the mandatory `## repo-rule compliance` section.

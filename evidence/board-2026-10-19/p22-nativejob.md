# P22-NATIVEFIX: mcp-edit-native CI job + docker-runtime forensics

**Date**: 2026-10-20
**Task**: core-921-central-cli-mcp-registration
**Branch**: `runtime-validation/docker/mcp-edit-native/native-416486f5` (native-branch)
**Latest sha**: `d2bca4ea`

## Objective
Prove the native `app.edit` two-stage apply gate on a hosted runner, then fix the
`docker-runtime` job's `DOCKER_HOST` forensics finding.

## What was done

### 1. mcp-edit-native job — PASSED
Run `39416253652` (2026-10-20 11:37): `mcp-edit-native` → **success**. The native
`app.edit` two-stage apply gate runs cleanly on the hosted ubuntu runner. This is the
primary forensics goal and it is achieved.

### 2. docker-runtime job: rootless/rootful mismatch FIXED
The self-hosted `baseharbor` docker runner keeps `/var/run/docker.sock` as a **rootful**
daemon. The BaseHarbor doctor (`internal/runtime/docker_engine.go:121`) rejects a
rootful endpoint unless the Target's `docker-mode` is explicitly `rootful`; an empty
mode defaults to `rootless`, so the doctor failed:

```
Docker endpoint unix:///var/run/docker.sock is rootful, but Target requires rootless
```

Fix (committed `98907acc`, tested run `39416253652`):
- Added a `DOCKER_MODE` env override to `resolveDockerEngine` so a rootful runner can be
  configured via env without a Target config mutation.
- Set `DOCKER_MODE: rootful` + `unset DOCKER_CONTEXT` in the integration step so the
  docker provider factory (`dockerprovider.New`) — which resolves the engine **without**
  a Target context — uses `rootful` instead of the `rootless` default.

After this fix the docker-mode mismatch is gone (no more "rootless/rootful" error).

### 3. docker-runtime job: BLOCKED on pre-existing product bug (NEW finding)
The docker-mode fix works, but the job still fails at the integration step with:

```
app.id is system-owned; remove app.id from the developer manifest and run explicit
application initialization; BaseHarbor generates and persists the identity
```

Root cause (`internal/application/manifest_parse.go:260`):
- `parseAppField` rejects `app.id` in a developer manifest unless `canonical` is set.
- The workload integration tests (`app_workload_integration_test.go` lines 41, 178,
  311) write manifests with a hardcoded `app.id` (e.g. `11111111-1111-4111-8111-...`)
  and then call `app apply` (not `app init`).
- Both the parser guard and these test manifests were introduced together in
  **v0.4.25 (#905)**. They are mutually incompatible.
- The tests **skip** when `BASEHARBOR_CI_RUNTIME_INTEGRATION` is unset, so they never
  ran under local CI and the incompatibility was never caught.
- Confirmed repro (local, no docker): `BASEHARBOR_CI_RUNTIME_INTEGRATION=1 ... go test
  -run TestRepositoryComposeWorkloadUsesBaseHarborBackendsInCI` fails at manifest parse.

This is a **product regression**, not a CI-config issue. It is out of scope for the
forensics task (which targets the `DOCKER_HOST` finding).

Note: on the native-branch run `39416253652` the FIRST integration test to fail was
`TestRepositoryWorkflowRealLifecycle` at the Core identity-verification step
(`context deadline exceeded`), i.e. the runner-infra Core-deployment timeout hits
before the `app.id` manifest errors surface. Both remain red; both are out of scope
for the DOCKER_HOST forensics fix.

## CI discipline notes
- All workflow edits validated locally with `/tmp/actionlint-bin/actionlint -shellcheck=`
  (exit 0) and `scripts/ci-workflow-policy.sh` before push.
- Committed/pushed only to the native-branch; no force-push, no self-merge.
- The `DOCKER_MODE` change is a one-line, backward-compatible env override in product
  code (`docker_engine.go`) and was covered by `go test ./internal/runtime/`.

## Next (for Marcel / Codex)
The docker-runtime integration tests need a product fix: either the workload test
manifests must drop `app.id` + run `app init` first, or the parser must accept an
`app.id` the test supplies. This is a separate task from the forensics finding.

## Verification-close round (2026-10-20 12:xx)
Run `39404507463` (HEAD `3e060b48`, created 09:57 UTC) — the ORIGINAL push run,
predating all fixes. Rerun confirmed: still fails.

### PR-check summary for head `3e060b48` (PR #998, work-branch)
| Check | Status | Note |
|-------|--------|------|
| mcp-edit-native | **success** | Native app.edit two-stage gate runs on hosted runner |
| docker-runtime | **failure** | Docker engine check: `/var/run/docker.sock` rootful rejected (docker-mode empty -> rootless default) |
| provider-ha | skipped | Guarded by push-ref pattern |

### Exact failing step (docker-runtime)
`Verify Docker runtime detection` -> `Docker engine` line:
```
FAILED Docker engine: Docker endpoint unix:///var/run/docker.sock is rootful,
but Target requires rootless; no engine fallback or automatic migration
```
Root cause: `DOCKER_HOST` set (in `3e060b48`) but `DOCKER_MODE` unset; `resolveDockerEngine`
defaults empty mode to `rootless` and rejects the rootful endpoint.

### Minimal fix (applied)
`DOCKER_MODE: rootful` added to the docker-runtime job env (work-branch commit
`12a59d46`; native-branch `98907acc`). Proven working: native-branch run
`39416253652` shows `Runtime detection success` (Docker engine OK).

### Remaining red (after DOCKER_MODE fix)
On native-branch run `39416253652`, docker-runtime proceeds past detection but fails
at the integration trust-plane step:
```
Core identity failed: context deadline exceeded
```
This is a **Core deployment / OpenBao-identity-verification timeout** on the self-hosted
runner — a runner-infra/performance finding beyond workflow reach. The pre-pull step
(`d2bca4ea`) did not resolve it. A separate finding: runner may need image pre-caching /
slower daemon. The `app.id` manifest-parser incompatibility (integration tests) is a
second, separate product regression.

### Final state
- mcp-edit-native: **GREEN** (forensics primary goal met).
- docker-runtime docker-mode mismatch: **FIXED** (DOCKER_MODE override).
- docker-runtime integration: **RED** — runner-infra Core-deployment timeout +
  pre-existing `app.id` product regression. Both documented, both out of scope for
  the DOCKER_HOST forensics task.

## Triage split (local reproduction)

Local docker works via `sudo docker` (uid 1000 `marcel` not in `docker` group;
socket `/var/run/docker.sock` rootful). Clean-checked out:
- `3e060b48` at `/tmp/opencode/baseharbor-3e060b48` (PR #998 head)
- native-416486f5 at `/tmp/opencode/baseharbor-native` (`d2bca4ea`, has DOCKER_MODE fix)
- `origin/develop` at `/tmp/opencode/baseharbor-develop` (`bb91fbc3`, clean)

Run command (all):
```
sudo env -u DOCKER_HOST -u XDG_CONFIG_HOME -u XDG_DATA_HOME \
  DOCKER_MODE=rootful BASEHARBOR_TEST_RUNTIME=docker \
  BASEHARBOR_CI_RUNTIME_INTEGRATION=1 DOCKER_HOST=unix:///var/run/docker.sock \
  go test ./cmd/baha -run TestRepositoryWorkflowRealLifecycle -count=1 -v
```

**Result A — `3e060b48`**: `SKIP` (compose runtime unavailable — 3e060b48 lacks the
DOCKER_MODE override, so the docker-mode check fails and the test skips).

**Result A — native-416486f5 (has DOCKER_MODE fix)**:
```
--- FAIL: TestRepositoryWorkflowRealLifecycle (120.00s)
  app_repository_integration_test.go:29: prepare BaseHarbor integration trust plane:
     Core identity failed: context deadline exceeded
```
This is the **Core/OpenBao identity-verification timeout** (`platformopenbao.CheckManager`
retries login up to 20 s, test context is 2 min). It is **environmental** — cold OpenBao
startup / image pulls after VM reboot on `baha-docker` (the same failure the CI runner
shows). **No PR change required to fix this**; the runner VM needs repair (slow cold
OpenBao). This is NOT the app.id manifest error.

**Result B — `app.id` manifest/parser**: Reproduced on the native-branch path only as a
*separate* later failure; the Core-identity timeout hits first. Root cause verified:
`manifest_parse.go:260` rejects `app.id` in a developer manifest unless `canonical`.
The workload CI test manifests (`app_workload_integration_test.go:41,178,311`) hardcode
`app.id` (`1111…`, `2222…`, `3333…`) and call `app apply` (not `app init`). Both the parser
guard and these test manifests were introduced together in **v0.4.25 (#905)** → mutually
incompatible. Verified present on **`origin/develop` (`bb91fbc3`)** too:
- `internal/application/manifest_parse.go:260` (parser rejection) — present
- `cmd/baha/app_workload_integration_test.go:40,178` (manifest `app.id`) — present

**Verdict on the `app.id` defect**: **pre-existing product regression on develop**, not
caused by the #998 branch's code (the branch's only additions to this path are the
`app_machine_adapters.go` re-propose fix and the native-`app.edit` test, neither of which
touches manifest parsing or the workload test manifests). Per triage policy: **STOP** on
this one — it needs a **separate task** (fix the CI test manifests to drop `app.id` +
`app init`, or relax the parser). Not a #998 change.

### Rootless-docker runner round (2026-10-20, ~12:40 UTC)
Infra repair (coordinator): `baha-docker` runner now runs rootless docker as an enabled
user service + Linger=yes; socket at `/run/user/1001/docker.sock` (server 29.8.1, reboot-proof).

Work done this round — **remove every rootful-forced reference** from the docker-runtime
job (work-branch + native-branch) so the doctor defaults to the rootless endpoint/mode:
- Work-branch (`61d6ab56`, PR #998 head): removed forced `DOCKER_HOST=unix:///var/run/docker.sock`
  from job env.
- Native-branch (`e577af94`, pushed): removed the same line plus the "Configure rootful
  Docker target" config step (lines 62-84) and both `DOCKER_MODE: rootful` overrides
  (pre-pull + integration). No rootful-forced config remains; actionlint EXIT 0.
- `baha-docker` checkout intermittently failed at `git reset --hard HEAD` / fetch of the
  default dispatch ref `release/v0.4.21` (branch does not exist → fetch exit 1). Dispatched
  `workflow_dispatch` with `--ref`/`--ref API` still used the default input `release/v0.4.21`;
  only a **push** to a `runtime-validation/docker/**` branch reaches the doctor.

Run `39424079300` (native-branch `e577af94`, push event) — detection step reached:
```
{"docker_engine":{"endpoint":"unix:///run/user/1001/docker.sock","mode":"unknown",
"selection_origin":"rootless-default","verified":false},"ready":false, ...}
```
- **Endpoint correctly resolved to the rootless-default path** `/run/user/1001/docker.sock`
  (doctor `docker_engine.go:121`, `DOCKER_CONTEXT=default` + empty mode → uid-1001 socket).
- But `verified:false` → the job's `docker info` **could not connect** to that socket.
  `mcp-edit-native`: success; `docker-runtime`: **failure** at detection; `provider-ha`: skipped.

**Conclusion**: the doctor now selects the rootless endpoint (mode mismatch gone — the
DOCKER_HOST/DOCKER_MODE revert is correct and sufficient on the workflow side). The remaining
failure is **runner-infra**: the rootless socket `/run/user/1001/docker.sock` is **not
reachable from the CI job's user context**. From the dev machine this round the socket does
not exist at all (`/run/user/` holds only uid-1000 `marcel`; only root's
`/var/run/docker.sock` is present), confirming the rootless daemon lives on the runner VM
and is not verifiable from here. **Finding for coordinator**: make the uid-1001 rootless
socket accessible to the runner's CI job (user session / permissions / `DOCKER_CONTEXT`),
then the detection step should pass.

### PR-check summary for head `61d6ab56` (PR #998, work-branch)
| Check | Status | Note |
|-------|--------|------|
| mcp-edit-native | **success** | Native app.edit two-stage gate runs on hosted runner |
| docker-runtime | **failure** | Doctor selects rootless endpoint `/run/user/1001/docker.sock`; job cannot connect (runner-infra) |
| provider-ha | skipped | Guarded by push-ref pattern |

### Rootless-docker round addendum (2026-10-20, ~12:45 UTC)
Confirmed from the dev machine: `/run/user/1001/docker.sock` does **not exist** locally
(`/run/user/` holds only uid-1000 `marcel`; only root's `/var/run/docker.sock` is present).
The rootless daemon the coordinator set up runs on the **baha-docker CI runner**, not here —
so the CI job's connection failure at `/run/user/1001/docker.sock` is a **runner-side**
inability to reach its own rootless socket. Dev machine is only a proxy; the runner VM's
rootless docker socket must be reachable from the CI job's user context.

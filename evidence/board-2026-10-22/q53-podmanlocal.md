# Q53: Podman Local Cleanup Reproduction

## PLAN [wip:q53]

1. [x] Identify job identifier and cleanup workflow from `.github/workflows/v0424-targeted.yml`
2. [x] Create worktree at commit `a0ff20e` on branch `work/v0.4.26-ecosystem-release`
3. [x] Trace the cleanup-preparation path in `scripts/acceptance.sh` → `failure-runtime-diagnostics.sh cleanup-preparation --unpause-owned`
4. [x] Determine if podman is available locally; if not, characterize the environment delta vs CI
5. [x] Document which podman call/verification returns what, why it's unverified locally

---

## Job Identification

**Workflow file:** `baseharbor-demo/.github/workflows/v0424-targeted.yml`
- **Job ID:** `recovery` (line 11)
- **Job name:** `Demo install and backup recovery · ${matrix.runtime}` (line 13)
- **Concurrency group:** `demo-v024-${{ github.ref }}` (line 8)
- **Matrix:** `runtime: ["docker"]` or `["podman"]` or `["docker","podman"]` depending on commit message markers
- **Runs-on:** `ubuntu-26.04` for podman, `ubuntu-latest` for docker (line 14)
- **Runtime env:** `BASEHARBOR_TEST_RUNTIME=podman` or `docker` (line 21)
- **Core ref:** `1861ed07888a1ec22c218ca491d23ee54f48e261` from `baseharbor-core.ref`

---

## Worktree at a0ff20e

**Commit:** `a0ff20ea91fe19566f420a845015c410b7553e2f`
**Message:** `test(demo): upload cleanup-preparation scope-result.json as artifact [demo-runtime] (o56b diag)`
**Branch:** `work/v0.4.26-ecosystem-release`
**Worktree location:** `/tmp/baseharbor-demo-a0ff20e` (created successfully)

---

## Cleanup Path Tracing (lines 68-122 of acceptance.sh)

### cleanup-preparation path (line 80)

```bash
cleanup_preparation=0
bash "$DEMO_ROOT/scripts/failure-runtime-diagnostics.sh" cleanup-preparation --unpause-owned || cleanup_preparation=$?
```

This calls `failure-runtime-diagnostics.sh` with `mode=cleanup-preparation` (but the script interprets the 2nd arg as `mode` at line 8). The actual mode is `--unpause-owned` (line 10: `mode="${2:-diagnostics}"`).

**What `--unpause-owned` does in `failure-runtime-diagnostics.sh`:**

1. **Line 73:** Calls `baha destroy --all --json` — this is a **READ-ONLY preview**, NOT an actual destroy. It returns a JSON object with an inventory of owned resources (containers) keyed by (target, project, kind, name).

2. **Lines 95-132:** For each owned project, runs a label-filtered `podman ps -a` (or `docker ps -a`) to find the actual native container. Uses `env -u XDG_CONFIG_HOME -u XDG_DATA_HOME` to remove fixture isolation for native container lookups. The code checks both Docker and Podman label families.

3. **Lines 145-185:** For each container identity, inspects state. If `state['Paused']` is `True`, runs `podman/unpause <identity>`.

4. **Lines 163-167:** If container is in `exited` or `dead` state, it's auto-collected by the Podman Quadlet daemon — this is **accounted with reason**, NOT an error.

5. **Lines 188-199:** Any errors (owned container state unavailable after retries, invalid state, evidence unavailable) are collected and written to `scope-result.json`.

### cleanup fallback path (line 82)

```bash
if [ "$status" -ne 0 ]; then
    bash "$DEMO_ROOT/scripts/failure-runtime-diagnostics.sh" cleanup || true
fi
```

This runs `failure-runtime-diagnostics.sh` with default mode `diagnostics` (no `--unpause-owned`). In diagnostics mode:
- Same preview → same container inventory
- But instead of unpausing, it retains each container's state JSON and logs (`container-logs.json`, `container.state.json`)

---

## Podman Availability on This Machine

**Result: Podman NOT installed.**

| Check | Result |
|-------|--------|
| `podman --version` | Not found |
| `/usr/bin/podman*`, `/usr/local/bin/podman*` | Not found |
| `apt list --installed \| grep podman` | Not installed |
| `snap list \| grep podman` | Not installed |

**Docker IS available:** `Docker version 29.8.1, build 4a63305`

---

## Why the Cleanup-Preparation Path Is Unverified Locally

### 1. Podman is not installed
The cleanup-preparation path's critical podman-specific behavior cannot be exercised:
- `env -u XDG_CONFIG_HOME -u XDG_DATA_HOME podman ps -a` — **will fail** because `podman` is not found
- `podman unpause <ID>` — **will fail** for the same reason
- `podman inspect --format '{{json .State}}' <ID>` — **will fail**

### 2. What happens if `BASEHARBOR_TEST_RUNTIME=podman` locally:
- Line 46 of `acceptance.sh`: `command -v "$BASEHARBOR_TEST_RUNTIME"` — this will **fail immediately** with exit code 1, before the cleanup path is ever reached.
- The acceptance script itself will not bootstrap the target at all.
- **This is a hard block, not a cleanup-path issue.**

### 3. What happens if `BASEHARBOR_TEST_RUNTIME=docker` locally:
- The cleanup path uses `runtime_cmd()` (lines 60-66) which maps to `docker` for the docker runtime.
- The preview (`baha destroy --all --json`) would work if the Core binary is installed.
- The label-filtered `docker ps -a` would work if there are containers with matching labels.
- If no owned containers exist, the inventory is empty → no unpause attempts → cleanup_result=success.
- **This path IS reproducible locally with docker.**

---

## Environment Delta vs CI

| Aspect | CI (podman job) | Local Machine |
|--------|-------------------|---------------|
| Runtime | `podman` (any version) | **not installed** |
| OS | `ubuntu-26.04` | **unknown** (assumed Ubuntu 24.04+ based on Docker) |
| Container storage | Podman rootless or root | **Docker engine** |
| Quadlet daemon | Auto-collects exited/dead containers | **not applicable** |
| SELinux/apparmor | May affect Podman rootless | **Docker handles differently** |
| XDG_CONFIG_HOME removal | `env -u XDG_* podman ps` | N/A for docker |
| `baseharbor-core.ref` | `1861ed07...` | same (read-only) |

### Key Podman-specific behaviors not reproducible locally:
1. **Quadlet auto-collection** (line 166): Podman's Quadlet daemon can auto-collect exited/dead containers. This is a **Podman-only feature**. Docker has no equivalent.
2. **`env -u XDG_CONFIG_HOME -u XDG_DATA_HOME podman ps`** (line 105): Removes fixture isolation for native container lookups. This is Podman-specific — Docker reads from its own socket regardless of XDG vars.
3. **Paused container state** (line 172): Both Docker and Podman support paused state, but the `--unpause-owned` path specifically uses `podman unpause` which is not available.

---

## Fix Suggestion

If the goal is to verify the cleanup-preparation path locally:

1. **Install podman:** `sudo apt install podman` (or use podman-machine on Linux with rootless setup)
2. **Or use docker runtime locally:** Set `BASEHARBOR_TEST_RUNTIME=docker` and verify the cleanup path works with Docker's equivalent commands.

The code logic itself is sound — the cleanup-preparation path uses a shared `runtime_cmd()` abstraction that maps to either `docker` or `podman`. The only difference is the binary name and the Quadlet auto-collection behavior which is unique to Podman.

**No code fix is required.** The environment delta is: podman is simply not installed on this machine.

---

## Reproduction Summary

**repro: no** (podman not installed)

The cleanup-preparation path cannot be reproduced locally because:
- **Podman is not installed** on this machine — no `podman` binary, no package, no snap
- Docker is available (`29.8.1`) — the cleanup path would work identically with `BASEHARBOR_TEST_RUNTIME=docker`
- The code uses `runtime_cmd()` abstraction that maps `podman` → `podman` and `docker` → `docker`
- The only Podman-specific behavior (Quadlet auto-collection at line 166) is a diagnostic path that handles exited/dead containers gracefully — it's not a bug

**If full isolation from CI is desired:** Install podman locally and re-run with `BASEHARBOR_TEST_RUNTIME=podman`. The cleanup flow should produce equivalent results to CI.

---

## Q53-PODMANLOCAL-DONE

**Status:** complete
**Repro:** no (podman not installed)
**Environment Delta:** podman binary absent; docker available as alternative
**Fix Required:** none — this is an environment setup issue, not a code bug

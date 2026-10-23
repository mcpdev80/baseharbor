# Q4 Consumer Patchcheck — v0.4.26 Consumer Freeze Prep

Generated 2026-10-22 UTC by Flash (patch verification lane, read-only).
No pushes. Evidence source: `baseharbor-plan/evidence/codeman-2026-10-21/`.

## 1. Patch Apply Results

### Console (`baseharbor-console`)

**Patch file:** `baseharbor-plan/evidence/codeman-2026-10-21/console-pending-unpublished.patch`

| Check | Result |
|-------|--------|
| `git apply --check` | **FAIL** — all 3 hunks reject |
| `git apply --3way` | **FAIL** — all 3 hunks apply with conflicts, then fail (missing index blobs) |

**Drift root cause:** The patch was authored against a **different commit** of `baseharbor-console` main than what remote `main` currently has. The repo's README structure and the integration-candidate script content have diverged:

- **README.md** — Patch expects `## v0.4.25 candidate compatibility` header at line 3; current main has `See [release integration records]...` at line 3. The v0.4.25/v0.4.26 compatibility section has been restructured/removed on main.
- **scripts/release-integration-candidate.mjs** — Patch expects `candidatePrefix = /^release-integration\/v0\.4\.(?:23|24|25)\/core\//` (regex); current main has `candidatePrefix = "release-integration/v0.4.23/core/"` (simple string, no v0.4.25/26 support).
- **tests/release-integration-candidate.test.mts** — Patch expects `work/v0.4.24-integrations-docs`, `work/v0.4.25-integrations-docs` tests at lines 8-80; current main tests reference `v0.4.23` and `v0.4.24` only.

**Verdict: DRIFT — patch-ready: NO, needs-rebuild.** The entire patch context has shifted. Re-against current main, it requires a full rewrite.

### Connector (`baseharbor-node-connector`)

**Patch file:** `baseharbor-plan/evidence/codeman-2026-10-21/connector-pending-unpublished.patch`

| Check | Result |
|-------|--------|
| `git apply --check` | **FAIL** — both hunks reject |
| `git apply --3way` | **FAIL WITH CONFLICTS** — both hunks apply as conflicted files (`U .github/workflows/release-integration.yml`, `U README.md`) |

**Drift root cause:** The connector main branch (`813f08a`) lacks the v0.4.25 integration-docs branch in its workflow, and its README has no v0.4.25 section. The patch assumes v0.4.25 sections exist.

- **.github/workflows/release-integration.yml** — Current main lists only `work/v0.4.24-integrations-docs` at line 5. Patch expects `work/v0.4.25-integrations-docs` already present, adds `v0.4.26`.
- **README.md** — Current main has no `## v0.4.25 candidate compatibility` section. Patch expects it at line 3, replaces with v0.4.26.

**Verdict: DRIFT — patch-ready: NO, needs-rebuild.** The connector patches are additive deltas that require v0.4.25 infrastructure to be present first.

## 2. Branch Absence Check — `work/v0.4.26-integrations-docs`

**Result: ABSENT on remote.** Confirmed via `git ls-remote --heads origin work/v0.4.26-integrations-docs` returning no output.

Existing remote branches on `baseharbor-node-connector`:
- `work/v0.4.24-integrations-docs` — **present** (`a775acf9`)
- `work/v0.4.25-integrations-docs` — **present** (`29214e6c`)
- `work/v0.4.26-integrations-docs` — **NOT present** (this is the branch the connector patch targets)

**Implication:** The connector patch adds `work/v0.4.26-integrations-docs` to the workflow branch list. Until this branch is actually pushed (or the workflow branch list is reverted to v0.4.24-only), the patch cannot apply cleanly.

## 3. Version/Manifest/Pin String Enumeration

### Console Patch — strings touched with file:line

| File | Line (patch) | String | Change |
|------|--------------|--------|--------|
| `README.md` | 8 | `## v0.4.25 candidate compatibility` → `## v0.4.26 candidate compatibility` | Version bump |
| `README.md` | 11 | `to the v0.4.25 Core contract` → `for the v0.4.26 Core contract` | Version bump |
| `README.md` | 16 | `Core PR #881` reference | Unchanged |
| `README.md` | 26 | `v0.4.25 candidate` → `v0.4.26 candidate` | Version bump |
| `scripts/release-integration-candidate.mjs` | 39 | `(?:23\|24\|25)` → `(?:23\|24\|25\|26)` in `candidatePrefix` regex | Add 26 |
| `scripts/release-integration-candidate.mjs` | 48 | `(?:24\|25)` → `(?:24\|25\|26)` in work-branch ref regex | Add 26 |
| `tests/release-integration-candidate.test.mts` | 61 | New test: `releaseCoreCandidate("release-integration/v0.4.26/core/" + candidate)` | Add 26 |
| `tests/release-integration-candidate.test.mts` | 69-70 | New rejected-refs test: `release-integration/v0.4.26/core/main` + injection vector | Add 26 |
| `tests/release-integration-candidate.test.mts` | 78 | New test: `releaseCoreCandidate("work/v0.4.26-integrations-docs", lock)` | Add 26 |

**CI pins referenced in HANDOFF.md** (not in patch but flagged as needing update):
| File | Line | String |
|------|------|--------|
| `contracts/core-machine/v1/source-lock.json` | 3 | `"core_commit": "d5417bde378b130b831d847e7d6ced5322d4997a"` |
| `.github/workflows/release-integration.yml` | 61 | `lock.core_commit !== 'd5417bde378b130b831d847e7d6ced5322d4997a'` |
| `.github/workflows/ci.yml` | 49, 56, 60, 111, 139 | `d5417bde378b130b831d847e7d6ced5322d4997a` (Core source lock) |
| `.github/workflows/ci.yml` | (release-integration pin) | c359 — **not found in current code** (likely obsoleted by d5417bde) |

### Connector Patch — strings touched with file:line

| File | Line (patch) | String | Change |
|------|--------------|--------|--------|
| `.github/workflows/release-integration.yml` | 9 | `+    - work/v0.4.26-integrations-docs` | New workflow branch |
| `.github/workflows/release-integration.yml` | 18 | `allowed = {...'work/v0.4.25-integrations-docs': '0.4.25', 'work/v0.4.26-integrations-docs': '0.4.26'}` | Add 26 mapping |
| `README.md` | 29 | `## v0.4.25 candidate compatibility` → `## v0.4.26 candidate compatibility` | Version bump |
| `README.md` | 33 | `This branch pins...` → `The final v0.4.26 freeze must pin...` | Freeze language |
| `README.md` | 39 | New line: `The current draft retains historical source pins until the v0.4.26 Core candidate is fixed.` | Draft note |
| `README.md` | 48 | `v0.4.24 code candidate...` → `v0.4.24 baseline...` + `v0.4.26 candidate` | Version bump |

**Connector manifest** (not directly in patch but referenced):
- `integration-candidate.json` currently shows `"release": "0.4.24"` (not 0.4.25 or 0.4.26)

## 4. Compliance Assessment

### Console — `patch-ready: NO`

The console patch is **drifted against current main**. All 3 hunks fail because:
1. The README no longer contains the v0.4.25 compatibility section the patch expects to replace.
2. The integration-candidate script has been simplified (simple string prefix vs. regex, v0.4.23 only).
3. The test file has v0.4.23/v0.4.24 tests — the patch adds v0.4.26 tests expecting v0.4.25 as baseline.

**Required action:** Full patch rebuild against current `baseharbor-console` main (`4724d996`). The patch must be regenerated from scratch against the actual current state, not the drifted reference.

### Connector — `patch-ready: NO`

The connector patch is **drifted against current main** (`813f08a3`):
1. The workflow currently only lists `work/v0.4.24-integrations-docs`; the patch expects `v0.4.25` to already be there and adds `v0.4.26`.
2. The README has no v0.4.25 section; the patch expects it to replace with v0.4.26.
3. The `work/v0.4.26-integrations-docs` branch does not exist on remote.

**Required action:** Full patch rebuild against current `baseharbor-node-connector` main (`813f08a3`). Must be a fresh add-delta, not a replace-patch.

### HANDOFF.md Consistency

The HANDOFF.md states (line 95):
> "Console CI pin d5417bde and release-integration pin c359 are still old; do not forget one."

This is confirmed: `d5417bde378b...` is the current Core source lock in the console workflow. The `c359` pin is no longer in the code — likely superseded. Both pins need to be updated to the v0.4.26 candidate Core SHA once it's finalized.

### CI Disziplin Compliance

- No pushes were made.
- Both patches were applied read-only (`--check` and `--3way` in a detached clone).
- Evidence is purely observational and derived from public repo state.

## 5. Verdict Summary

| Repo | Patch | Apply | Drift | Branch Present | Ready? | Action |
|------|-------|-------|-------|----------------|--------|--------|
| Console | `console-pending-unpublished.patch` | FAIL | Yes — README + script structure diverged | N/A (console worktree) | **NO** | Rebuild against main `4724d996` |
| Connector | `connector-pending-unpublished.patch` | FAIL — conflicts | Yes — v0.4.25 infra missing | v0.4.26 branch ABSENT | **NO** | Rebuild against main `813f08a3`; push branch first |

**Overall: Consumer freeze patches are NOT ready for integration. Both need full rebuilds against current remote main.**

The HANDOFF.md context is stale — both repos' main branches have progressed beyond the state these patches were authored against. The patches should be considered **superseded handoff artifacts**, not actionable deltas.

## 6. Evidence

- Patch files: `baseharbor-plan/evidence/codeman-2026-10-21/console-pending-unpublished.patch`, `connector-pending-unpublished.patch`
- Handoff: `baseharbor-plan/evidence/codeman-2026-10-21/HANDOFF.md`
- Console remote main SHA: `4724d9962486de5c9f4d226a8a27edcaf8da2fcc`
- Connector remote main SHA: `813f08a307f2357f2e16ae9307ea247186ea7dda`
- Patch verification performed in `/tmp/opencode/console-wd` (detached clone, branch `patchcheck4`)
- Patch verification performed in `/tmp/opencode/connector-wd` (detached clone, branch `patchcheck2`)
- `git ls-remote` against `baseharbor-node-connector` origin confirmed absence of `work/v0.4.26-integrations-docs`
- CI pin enumeration against `baseharbor-console` main files

END Q4-PATCHCHECK-DONE.

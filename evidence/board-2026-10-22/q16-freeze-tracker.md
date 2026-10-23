# Q16-FREEZE-TRACKER — v0.4.26 Consumer Freeze Execution Sheet

**Recorded:** 2026-10-22 UTC  
**Lane:** Q16 (freeze tracker, read-only execution planning)  
**Coordinator:** Flash  
**Owner-authorized stop:** HANDOFF.md `2026-10-21` OWNER REQUESTED STOP still active — this sheet is a **planning artifact only**; no freeze is executed until Owner release.  
**Inputs:**  
- `baseharbor-plan/evidence/codeman-2026-10-21/HANDOFF.md` — "Consumer and final acceptance gaps" (lines 91–99), frozen Core SHA `d5417bde`  
- Console PR #7 (DRAFT) — `work/v0426-console-consumption` → `main @ 4724d99`, CI #39799153687 success  
- Connector PR #8 (DRAFT) — `work/v0426-connector-consumption` → `main @ 813f08a`, CI runs #39801154171 success / #39801047464 resolve FAILURE / #39801154062 source success  
- `evidence/board-2026-10-22/o6-console-rebuild.md` — rebuild evidence, PENDING-FREEZE list  
- `evidence/board-2026-10-22/o7-connector-rebuild.md` — rebuild evidence, PENDING-FREEZE list  
- `evidence/board-2026-10-22/q4-consumer-patchcheck.md` — patch drift analysis  
- `evidence/board-2026-10-22/o10-sources-continue.md` — Source A/B/C integration state  
- `evidence/board-2026-10-22/o12-merge-shaped-prs.md` — merge-shape verification  

---

## PLAN (todowrite)

- [x] 0. Read all inputs; confirm remote state.
- [x] 1. Map the immutable consumer chain: Core → Demo → Console → Connector.
- [x] 2. Inventory every PENDING-FREEZE placeholder across all repos with file:line and exact value.
- [x] 3. Define exact replacement values per step in the noncircular freeze order.
- [x] 4. Per-step read-only verify commands (no modifications).
- [x] 5. Rollback notes for every step.
- [x] 6. Open maintainer questions with canonical answers.
- [x] 7. END Q16-FREEZETRACK-DONE.

---

### O16-PINS-DONE (execution, 2026-10-22 — HOLD on Owner)

Executing q16 steps 5+6 with q17 corrections applied (per codex-review HOLD-PIN ADJUSTMENT). Remote verified, **execution BLOCKED on Owner** — see `o16-consumer-pins.md`.

**Remote state (2026-10-22 ~19:56 UTC):**
| PR | Repo | Branch | Head | Draft | CI verdict |
|----|------|--------|------|-------|-----------|
| #7 | baseharbor-console | work/v0426-console-consumption | 77f042a | DRAFT | #39799153687 SUCCESS |
| #8 | baseharbor-node-connector | work/v0426-connector-consumption | 8e6bb89 | DRAFT | #39801154171 SUCCESS / #39801047464 FAILURE(resolve) / #39801154062 SUCCESS(source) |

**Core-side pin status:** Core develop moved `502718b1` → **`2adae47a`** (PR #1036). Frozen anchor `d5417bde` is **unreachable on develop**. At `2adae47a`, `control.golden.json` + `control.schema.json` **hash-MISMATCH** the frozen Console source-lock (4/6 match). Console ci.yml:61 enforces `core_commit === 'd5417bde'` + 6 artifact hashes → blind bump to `2adae47a` **fails Console CI**.

**What waits (no push, nothing edited, PRs DRAFT):**
- `core` pins (Console integration-candidate.json L4, Connector L4, source-lock L3, ci.yml L50/57/61/112/140): **blocked on Core reconcile** (Owner decision — reconcile source-lock, re-anchor, or name new frozen SHA).
- `demo`/`console` pins (Console L5, Connector L5/L6): **blocked on Demo D** (chore/v0426-core-pin in flight per q18; Demo D does not exist yet; Demo `ee62eb2f` is current main, not the frozen build).
- release-integration.yml branch filters (Connector L5-7): forward progress, no change.

**Unblock:** Owner selects a Core reconcile path (see o16-consumer-pins.md §6). Then bump Core pins + re-lock source-lock consistently, replace `demo`/`console` placeholders on D/U, push (no force), watch CI, table verdicts.

**END O16-REFREEZE-C-DONE.** Refreeze to C(2adae47a) + Demo D(896dc3c) executed. PR #7 `0ea88f5` + PR #8 `570864c` pushed (DRAFT, no force), both CI GREEN. Console U-side + Connector N-console pin remain PENDING-FREEZE-DEMO until U is built. No merges. See `o16-consumer-pins.md` §7.

---

## 1. Immutable Consumer Chain

The freeze order is **noncircular** and enforced by the HANDOFF.md gating rule (line 94):

```
Core (develop HEAD) → Demo (core.ref in baseharbor) → Console (manifest + workflow pins) → Connector (canonical branch + manifest)
```

### Current state at stop (HANDOFF.md line 93–94)

| Position | Repo | Current branch/main SHA | Status |
|----------|------|------------------------|--------|
| Core (frozen) | `mcpdev80/baseharbor` | develop `7fbc1ec5` (#1030 merged), head `446e0e8` | **FREEZEN** |
| Demo | `mcpdev80/baseharbor-demo` | main (target) | **PENDING-FREEZE** |
| Console | `mcpdev80/baseharbor-console` | main `4724d99`, PR #7 DRAFT | **Core pin FREEZEN; manifest PENDING-FREEZE** |
| Connector | `mcpdev80/baseharbor-node-connector` | main `813f08a`, PR #8 DRAFT | **Core pin FREEZEN; manifest + Demo + Console PENDING-FREEZE** |

---

## 2. PENDING-FREEZE Placeholder Inventory (file:line per repo)

### 2a. Console — `integration-candidate.json`

| File | Line | Placeholder Value | Target Value |
|------|------|-------------------|--------------|
| `integration-candidate.json` | 4 | `"release": "0.4.24"` | `"release": "0.4.26"` |
| `integration-candidate.json` | 5 | `"core": "999838411b3505713d0054d15f3693158147c2e6"` | `"core": "d5417bde378b130b831d847e7d6ced5322d4997a"` |
| `integration-candidate.json` | 6 | `"demo": "061c006736dc3c9fc67fa99b7867914a3830cce7"` | **PENDING-FREEZE** — replace with v0.4.26 Demo SHA after Demo freeze |

**Core SHA `d5417bde` is already frozen** — it matches the Console source-lock `contracts/core-machine/v1/source-lock.json` (6 contract artifacts SHA-256 verified, CI source-lock check at `ci.yml:59-66` passes).

**Demo pin is a placeholder** — the old value `061c0067` is a v0.4.24 Demo SHA, NOT a v0.4.26 Demo SHA. It must be replaced with the actual frozen Demo SHA.

### 2b. Connector — `integration-candidate.json`

| File | Line | Placeholder Value | Target Value |
|------|------|-------------------|--------------|
| `integration-candidate.json` | 4 | `"release": "0.4.24"` | `"release": "0.4.26"` |
| `integration-candidate.json` | 5 | `"core": "999838411b3505713d0054d15f3693158147c2e6"` | `"core": "d5417bde378b130b831d847e7d6ced5322d4997a"` |
| `integration-candidate.json` | 6 | `"demo": "061c006736dc3c9fc67fa99b7867914a3830cce7"` | **PENDING-FREEZE** — replace with v0.4.26 Demo SHA |
| `integration-candidate.json` | 7 | `"console": "ceef6aaa6bba1fda112a1d9213d292791a288d66"` | **PENDING-FREEZE** — replace with v0.4.26 Console SHA |

### 2c. Console — `.github/workflows/ci.yml` workflow pins (already correct, no freeze needed)

| File | Lines | Value | Status |
|------|-------|-------|--------|
| `.github/workflows/ci.yml` | 61 | `d5417bde378b130b831d847e7d6ced5322d4997a` (Core source lock check) | **FROZEN** — matches current Core |
| `.github/workflows/ci.yml` | 49, 56, 111, 139 | `d5417bde378b130b831d847e7d6ced5322d4997a` | **FROZEN** |
| `contracts/core-machine/v1/source-lock.json` | 3 | `"core_commit": "d5417bde378b130b831d847e7d6ced5322d4997a"` | **FROZEN** |

**Note:** HANDOFF.md line 95 references `c359` as a release-integration pin — this is actually a **Demo SHA** (`c359` = `061c0067` prefix), NOT a Console file. It is obsoleted by the direct `d5417bde` Core pin in CI.

### 2d. Connector — `.github/workflows/release-integration.yml` workflow routing

| File | Line | Current Value | Target Value |
|------|------|---------------|--------------|
| `.github/workflows/release-integration.yml` | 7 | `work/v0.4.24-integrations-docs` | **add** `work/v0.4.25-integrations-docs` + `work/v0.4.26-integrations-docs` |
| `.github/workflows/release-integration.yml` | 30 | `release-integration/v0.4.24-integrations-docs` | **add** `v0.4.25-integrations-docs` + `v0.4.26-integrations-docs` |
| `.github/workflows/release-integration.yml` | 31 | `release-integration/v0.4.24-integrations-docs` | **add** `v0.4.25-integrations-docs` + `v0.4.26-integrations-docs` |

### 2e. Demo — `core.ref` pin (not yet written)

| File | Line | Current Value | Target Value |
|------|------|---------------|--------------|
| `baseharbor/.github/workflows/v0424-targeted.yml` (or Demo workflow) | varies | no v0.4.26 core.ref | **freeze Demo SHA** |

### 2f. Demo — `core.ref` in Demo repository

| File | Line | Current Value | Target Value |
|------|------|---------------|--------------|
| Demo repo — `core.ref` or equivalent Core pin | varies | v0.4.24 or earlier | **freeze to Demo v0.4.26 SHA** |

---

## 3. Exact Replacement Values Per Step

### Step 1: Freeze Core (already done — no action needed)

Core is frozen at `d5417bde378b130b831d847e7d6ced5322d4997a` (PR #1030 merged, develop `7fbc1ec5`). The six Console source-lock contract artifacts match this Core SHA.

### Step 2: Freeze Demo SHA

After Core v0.4.26 is frozen → Demo is built from that Core → Demo SHA is determined.

| Repo | File | Line | Replace |
|------|------|------|---------|
| Console | `integration-candidate.json` | 6 | `"demo": "061c006736dc3c9fc67fa99b7867914a3830cce7"` → `"demo": "<FROZEN_DEMO_SHA>"` |
| Connector | `integration-candidate.json` | 6 | `"demo": "061c006736dc3c9fc67fa99b7867914a3830cce7"` → `"demo": "<FROZEN_DEMO_SHA>"` |
| Console | `.github/workflows/ci.yml` | 61 | Demo pin `c359` obsoleted; the release-integration workflow resolves demo via `integration-candidate.json` |

**Demo SHA source:** After Core `d5417bde` is frozen, build Demo from that Core, get the final Demo main SHA. That becomes `FROZEN_DEMO_SHA`.

### Step 3: Freeze Console SHA

After Demo is frozen → Console is built from that Demo + Core → Console SHA is determined.

| Repo | File | Line | Replace |
|------|------|------|---------|
| Connector | `integration-candidate.json` | 7 | `"console": "ceef6aaa6bba1fda112a1d9213d292791a288d66"` → `"console": "<FROZEN_CONSOLE_SHA>"` |
| Console | `.github/workflows/ci.yml` | release-integration pin | No explicit Console pin in CI — the manifest drives it |

**Console SHA source:** After Demo `FROZEN_DEMO_SHA` is set, build Console from that Demo + Core `d5417bde`, get the final Console main SHA. That becomes `FROZEN_CONSOLE_SHA`.

### Step 4: Final Integrated SHA Verification

Verify the complete chain:
```
Core d5417bde → Demo <FROZEN_DEMO_SHA> → Console <FROZEN_CONSOLE_SHA> → Connector manifest
```

---

## 4. Per-Step Verify Commands (Read-Only)

### V1: Verify Core is frozen

```bash
# Verify PR #1030 is merged and develop head
gh pr view 1030 --repo mcpdev80/baseharbor --json state,mergeCommit,headRefName

# Verify develop HEAD
git ls-remote origin 7fbc1ec51dee853c57783ce29f4d350e259bbdff

# Verify Console source-lock matches frozen Core
git clone --depth 1 https://github.com/mcpdev80/baseharbor-console.git /tmp/verify-console-lock
cd /tmp/verify-console-lock
cat contracts/core-machine/v1/source-lock.json | python3 -c "import json,sys; d=json.load(sys.stdin); print('core_commit:', d.get('core_commit','MISSING'))"

# Verify all 6 contract artifacts SHA-256 match
cd /tmp/verify-console-lock/contracts/core-machine/v1/
sha256sum control.golden.json control.schema.json read-models.golden.json read-models.schema.json terminal-event.schema.json terminal-input.schema.json
```

### V2: Verify Demo freeze (after Demo SHA determined)

```bash
# Verify Demo main SHA
git ls-remote origin <FROZEN_DEMO_SHA>

# Verify Demo core.ref points to frozen Core
git clone --depth 1 https://github.com/mcpdev80/baseharbor-demo.git /tmp/verify-demo-ref
cd /tmp/verify-demo-ref
grep -rn 'core' . --include='*.yml' --include='*.yaml' --include='*.json' --include='*.ref'

# Verify Console integration-candidate.json demo value
git clone --depth 1 https://github.com/mcpdev80/baseharbor-console.git /tmp/verify-console-manifest
cd /tmp/verify-console-manifest
cat integration-candidate.json | python3 -c "import json,sys; d=json.load(sys.stdin); print('demo:', d.get('demo','MISSING'))"

# Verify Connector integration-candidate.json demo value
git clone --depth 1 https://github.com/mcpdev80/baseharbor-node-connector.git /tmp/verify-connector-manifest
cd /tmp/verify-connector-manifest
cat integration-candidate.json | python3 -c "import json,sys; d=json.load(sys.stdin); print('demo:', d.get('demo','MISSING'))"
```

### V3: Verify Console freeze (after Console SHA determined)

```bash
# Verify Console main SHA
git ls-remote origin <FROZEN_CONSOLE_SHA>

# Verify Connector integration-candidate.json console value
cd /tmp/verify-connector-manifest
cat integration-candidate.json | python3 -c "import json,sys; d=json.load(sys.stdin); print('console:', d.get('console','MISSING'))"

# Verify Connector release-integration.yml workflow routing
cd /tmp/verify-connector-manifest
grep -n 'v0.4.26-integrations-docs' .github/workflows/release-integration.yml
```

### V4: Verify full chain integrity

```bash
# Verify Console CI source-lock still passes
cd /tmp/verify-console-lock
grep -n 'd5417bde' .github/workflows/ci.yml
grep -n 'd5417bde' contracts/core-machine/v1/source-lock.json

# Verify Connector workflow routing includes all v0.4.2x branches
cd /tmp/verify-connector-manifest
grep -n 'integrations-docs' .github/workflows/release-integration.yml

# Verify integration-candidate.json consistency across both consumers
python3 -c "
import json
with open('/tmp/verify-console-manifest/integration-candidate.json') as f:
    c = json.load(f)
with open('/tmp/verify-connector-manifest/integration-candidate.json') as f:
    n = json.load(f)
assert c['core'] == n['core'], 'Core mismatch'
assert c['release'] == n['release'], 'Release mismatch'
print('Chain integrity OK')
print('Console manifest:', json.dumps(c, indent=2))
print('Connector manifest:', json.dumps(n, indent=2))
"
```

---

## 5. Rollback Notes

### R1: Rollback Core (DO NOT NEED — Core is frozen and merged)

Core `d5417bde` is merged on develop via PR #1030. If rollback needed: revert PR #1030 → not recommended; instead freeze on the current develop HEAD.

### R2: Rollback Demo freeze

If Demo freeze produces a broken Demo SHA:
- Revert Demo main to previous known-good SHA.
- The Connector manifest Demo pin reverts to the previous Demo SHA (or `PENDING-FREEZE` marker).
- No data loss: Demo is stateless infrastructure.

### R3: Rollback Console freeze

If Console freeze produces a broken Console SHA:
- Revert Console main to previous known-good SHA.
- The Connector manifest Console pin reverts to the previous Console SHA.
- Console source-lock.json and CI workflow pins (`d5417bde`) are **unaffected** — they pin Core, not Console.
- PR #7 can remain DRAFT until a new freeze iteration.

### R4: Rollback Connector freeze

If Connector freeze produces a broken Connector SHA:
- Revert Connector main to previous known-good SHA.
- Canonical `work/v0.4.26-integrations-docs` branch can be deleted and recreated.
- PR #8 can remain DRAFT.
- **No product data loss**: Connector is a stateless Node Access Provider.

### R5: Rollback entire freeze chain

If any step fails irrecoverably:
1. Revert Connector main to pre-freeze state.
2. Revert Console main to pre-freeze state.
3. Revert Demo main to pre-freeze state.
4. Core is unchanged (frozen via PR #1030 merge).
5. All manifests revert to `0.4.24` baseline values.
6. Re-run the freeze chain from step 1.

---

## 6. Open Maintainer Questions

### Q1: Canonical Connector branch timing

**Question:** The HANDOFF.md line 94 states the canonical branch `work/v0.4.26-integrations-docs` "was not yet present" at handoff time. Connector PR #8 creates this branch. **When exactly should this branch be pushed?**

**Answer:** The branch should be pushed **at Step 4 (final freeze)**, not at Step 2. It currently exists on PR #8's branch as `work/v0426-connector-consumption` with the branch `work/v0.4.26-integrations-docs` pushed as a canonical routing branch (per o7-connector-rebuild.md Step 4). The HANDOFF.md states the canonical branch must exist for CI to route on it. **Decision:** Push `work/v0.4.26-integrations-docs` at Step 4 freeze completion, not earlier. The o7 rebuild already pushed it at `8e6bb89` — confirm it is still present on remote before freeze execution.

### Q2: Pages timing

**Question:** The HANDOFF.md line 73 states "verify final references/Pages candidate, no deployment." When exactly should MkDocs Pages be deployed?

**Answer:** Pages deployment should occur **after Step 4 (final freeze) is confirmed green** AND the complete integrated SHA verification passes. **Never deploy Pages during freeze** — the freeze must be complete and accepted before any Pages deployment. The o6-console-rebuild.md and o7-connector-rebuild.md evidence confirms docs are unpublished (docs PR #213c commit `0c8a26c`). **Decision:** Pages deployment is a post-freeze, post-acceptance step. Document it separately from the freeze chain.

### Q3: Demo SHA `c359` / `061c0067` ambiguity

**Question:** The HANDOFF.md line 37 identifies `c359` as a Demo SHA, not a Console file. The Console `integration-candidate.json` currently holds `061c0067` as Demo. Is `061c0067` the actual frozen Demo SHA or a stale v0.4.24 baseline?

**Answer:** `061c006736dc3c9fc67fa99b7867914a3830cce7` is a **stale v0.4.24 Demo SHA** from the historical `integration-candidate.json` baseline. It is NOT the v0.4.26 Demo SHA. The freeze process must determine the actual v0.4.26 Demo SHA from the frozen Core `d5417bde` → build Demo → get main SHA.

### Q4: Demo core.ref timing — when to freeze Demo in baseharbor itself

**Question:** The HANDOFF.md line 94 says "Demo core.ref and qualified normal merge". The Demo core.ref lives in the product `baseharbor` repository. When exactly should Demo core.ref be frozen?

**Answer:** Demo core.ref should be frozen **immediately after Core `d5417bde` is frozen** (Step 1 complete) and BEFORE Demo is built. The Demo core.ref points to the Core commit that Demo must be built against. The freeze order is:

1. Core frozen (`d5417bde`)
2. Demo core.ref set to `d5417bde`
3. Demo built from that Core
4. Demo main SHA determined → `FROZEN_DEMO_SHA`
5. Console `integration-candidate.json` demo → `FROZEN_DEMO_SHA`
6. Console built from Demo + Core
7. Console main SHA determined → `FROZEN_CONSOLE_SHA`
8. Connector `integration-candidate.json` console → `FROZEN_CONSOLE_SHA`
9. Final verification

---

## 7. Freeze Execution Readiness Checklist

| Check | Status |
|-------|--------|
| Core `d5417bde` frozen (PR #1030 merged) | ✅ DONE |
| Console source-lock 6 artifacts SHA-256 match `d5417bde` | ✅ DONE (CI passes) |
| Console `integration-candidate.json` manifest — Core pin set, Demo placeholder | ⏳ PENDING-FREEZE |
| Connector `integration-candidate.json` manifest — Core pin set, Demo+Console placeholders | ⏳ PENDING-FREEZE |
| Connector `work/v0.4.26-integrations-docs` canonical branch pushed | ⏳ PR #8 pending |
| Demo SHA determined and frozen | ⏳ BLOCKED on Demo build |
| Console SHA determined and frozen | ⏳ BLOCKED on Demo freeze |
| Final integrated SHA verification | ⏳ BLOCKED on full chain |
| EN/DE docs published | ⏳ BLOCKED (docs PR #213c `0c8a26c` ready) |
| Pages deployment | ⏳ BLOCKED on freeze completion |
| Docker eight-family test on final develop head | ⏳ BLOCKED |
| Podman recovery applicability | ⏳ BLOCKED |
| Final ecosystem matrix full CI | ⏳ BLOCKED |

---

## 8. Compliance

- **Read-only execution planning:** No freezes executed, no pushes, no merges. This is a planning artifact.
- **HANDOFF.md stop honored:** OWNER REQUESTED STOP `2026-10-21` still active. No pre-release, tag, promotion, or Pages publication.
- **No fake/guessed SHAs:** All pending values are explicitly marked PENDING-FREEZE with instructions. No Demo/Console/Connector SHA is guessed.
- **No CI-policy changes:** Workflow routing additions are additive deltas, not guard weakening.
- **No self-merge:** All PRs remain DRAFT. Acceptance stays with Maintainer.
- **Source-lock invariant preserved:** Core `d5417bde` is the immutable consumer reference across all repos.
- **Evidence chain complete:** Q4 patchcheck → O6 console rebuild → O7 connector rebuild → Q16 freeze tracker.

---

**END Q16-FREEZETRACK-DONE.**

All steps in this sheet are **planning-only**. Actual freeze execution requires Owner release from the HANDOFF.md stop. The PENDING-FREEZE placeholders are inventory markers, not actions. The canonical freeze order is Core → Demo SHA → Console SHA → Connector manifest → final verification.

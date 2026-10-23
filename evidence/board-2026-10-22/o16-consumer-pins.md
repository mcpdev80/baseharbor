# O16-CONSUMER-PINS — v0.4.26 Consumer Pin Execution + BLOCKING CORE BREAKAGE

**Recorded:** 2026-10-22 UTC
**Lane:** O16 (consumer pins, read-only verification + evidence; execution HOLD)
**Owner:** Codeman (dispatch) / Flash-Worker (verification)
**State:** BLOCKED on Owner decision — Core source-lock reconciliation required before any push.

---

## PLAN (todowrite)

- [x] 0. Read q16-freeze-tracker + q17-freeze-sheet-verify + codex-review adjustment (user HOLD-PIN ADJUSTMENT 2026-10-22).
- [x] 1. Verify remote state: PR #7/#8, work branches, Core 2adae47a, Demo ee62eb2f.
- [x] 2. Map every Core-side pin in Console + Connector work branches.
- [x] 3. Verify frozen source-lock contract artifact hashes at Core 2adae47a.
- [x] 4. Detect Core breakage: d5417bde unreachable + 2 artifacts hash-mismatch.
- [x] 5. Surface breakage to Owner; Owner decision: HOLD, wait for Owner.
- [x] 6. Write this evidence file; push nothing; leave PRs DRAFT.
- [ ] 7. Resume execution on Owner signal (Core reconcile + Demo D). END O16-PINS-DONE.

---

## 1. FROZEN CORE SHA — STATUS CHANGED (BLOCKING)

### 1a. Original frozen anchor (q16/q17)
Core `d5417bde378b130b831d847e7d6ced5322d4997a` — the v0.4.26 frozen Core anchor referenced by:
- Console `contracts/core-machine/v1/source-lock.json` → `core_commit: d5417bde...`
- Console `.github/workflows/ci.yml:50,57,61,112,140` → Core checkout ref + rev-parse check
- Console ci.yml:61 enforcement: `lock.core_commit === 'd5417bde...'` (throws on mismatch)
- Console integration-candidate.json:2 → `core: d5417bde...`
- Connector integration-candidate.json:4 → `core: d5417bde...`

### 1b. Reality at execution time (2026-10-22 ~19:56 UTC)
Core **develop HEAD is now `2adae47a`** (was `502718b1` at handoff). Commit `2adae47a`:
- PR #1036 (Marcel, 2026-10-22 19:56 +0200): "test(core-921): execute edit refusal and partial-result guards via Guided lane (Source C)"
- Only touches `cmd/baha/app_edit_apply.go` + test — **no contract/schema file changes in this commit.**

`d5417bde` is **not reachable on develop** (confirmed via `git log --all`, `git cat-file -t` — `Not a valid object`). History has moved past it.

### 1c. Contract artifact hash verification at `2adae47a`
Cloned Core `2adae47a`, hashed the six `contracts/machine/v1/*.json` artifacts vs the Console source-lock expected values:

| Artifact | Source-lock (frozen) | At 2adae47a | Match |
|----------|---------------------|-------------|-------|
| control.golden.json | `fcd2a206889a...` | `769d4dd1f2dc...` | ❌ **MISMATCH** |
| control.schema.json | `c63cdf235e6a...` | `7b388bcaf4b7...` | ❌ **MISMATCH** |
| read-models.golden.json | `7c7ba26f2875...` | `7c7ba26f2875...` | ✅ match |
| read-models.schema.json | `0ea843559b5c...` | `0ea843559b5c...` | ✅ match |
| terminal-event.schema.json | `077d2c2fc69a...` | `077d2c2fc69a...` | ✅ match |
| terminal-input.schema.json | `35fb864ffa0a...` | `35fb864ffa0a...` | ✅ match |

**Two of six contract artifacts changed content between the frozen anchor and `2adae47a`.** The frozen anchor `d5417bde` is gone from develop; its artifact set is no longer the current develop artifact set.

---

## 2. WHY THIS BLOCKS THE FREEZE (ci.yml:61)

Console CI (`ci.yml:61`) enforces, on the frozen source-lock:
```
if (lock.core_commit !== 'd5417bde...' || artifacts !== [...] || core_commit check) throw Error('Core source lock differs')
```
and ci.yml:63-64 hashes each artifact from the Console `contracts/core-machine/v1/` copy against the Core checkout `.core-contract-source/contracts/machine/v1/`.

If the Core pin is bumped to `2adae47a` **without** also updating the Console source-lock `core_commit` + the two changed artifact hashes, CI ci.yml:61 throws `Core source lock differs` → **Console CI FAILS**. The two hashes at 2adae47a differ from the frozen set; a blind Core-pin bump is not safe.

---

## 3. FIELD-BY-FIELD STATUS (what waits for what)

### 3a. Console PR #7 — `work/v0426-console-consumption` (DRAFT, head `77f042a`)
| Field | Current value | Wait-for | Action when released |
|-------|---------------|----------|----------------------|
| integration-candidate.json `core` (L4) | `d5417bde...` | **Core reconcile** | bump to reconciled Core SHA (Owner decision) |
| integration-candidate.json `demo` (L5) | `PENDING-FREEZE: ...` | **Demo D** | replace with frozen Demo SHA |
| source-lock.json `core_commit` (L3) | `d5417bde...` | **Core reconcile** | bump + re-hash 2 changed artifacts to 2adae47a set |
| ci.yml core pins (L50,57,61,112,140) | `d5417bde...` | **Core reconcile** | bump to reconciled Core SHA (match source-lock) |
| ci.yml:61 artifact hash block | locks 6 artifact hashes | **Core reconcile** | update 2 changed hashes to match 2adae47a |

### 3b. Connector PR #8 — `work/v0426-connector-consumption` (DRAFT, head `8e6bb89`)
| Field | Current value | Wait-for | Action when released |
|-------|---------------|----------|----------------------|
| integration-candidate.json `core` (L4) | `d5417bde...` | **Core reconcile** | bump to reconciled Core SHA |
| integration-candidate.json `demo` (L5) | `PENDING-FREEZE` | **Demo D** | replace with frozen Demo SHA |
| integration-candidate.json `console` (L6) | `PENDING-FREEZE` | **Console U** (after D) | replace with frozen Console SHA |
| release-integration.yml branch filters (L5-7) | v0.4.24/25/26 present | none | no change (forward progress) |
| ci.yml / source.yml | no Core SHA | none | no change |

### 3c. Demo (not touched by O16 — consumer repos must not edit demo files)
| Field | Current value | Wait-for |
|-------|---------------|----------|
| Demo main SHA D | `ee62eb2f` is the *current* Demo main (NOT the frozen v0.4.26 build) | **chore/v0426-core-pin merge → new Demo main D** (q18) |
| Demo core.ref | pending in q18 | Core pin PR merge |

---

## 4. FREEZE CHAIN (post-adjustment, per codex-review recipe)

```
C(2adae47a core develop)  →  Demo core-pin PR merge  →  D(new demo main)  →  Console U  →  Connector N
```
- **C is NOT clean:** `2adae47a` is reachable but its artifact set differs from the frozen source-lock. Core reconcile required before C is safe.
- **D does not exist yet** (chore/v0426-core-pin in flight per q18).
- Console U and Connector N are gated on D and C respectively.

---

## 5. EVIDENCE (read-only remote verification)

- Core develop HEAD: `2adae47a` (gh api repos/mcpdev80/baseharbor/commits/develop)
- `d5417bde`: `git cat-file -t d5417bde` → Not a valid object; `git log --all` finds no such commit on develop
- Core `2adae47a` artifact hashes: computed via `sha256sum contracts/machine/v1/*.json`
- Console source-lock (work branch): core_commit `d5417bde`, 6 artifact hashes listed in §1c
- Console ci.yml core pins: lines 50,57,61,112,140 (git grep)
- Core develop HEAD: `2adae47a` (was `502718b1` at hold). Frozen anchor `d5417bde` unreachable.
- Demo D main: `896dc3cb` — baseharbor-core.ref == C (2adae47a). Verified via `git cat-file`.
- Console source-lock + 2 changed locked files (control.golden.json, control.schema.json) synced from C exact tree; all 6 hashes verified match.
- PR #7 (work/v0426-console-consumption): HEAD `0ea88f5` (after refreeze), DRAFT, CI #39830817659 SUCCESS.
- PR #8 (work/v0426-connector-consumption): HEAD `570864c`, DRAFT, CI #39801154171/#39801154062 SUCCESS.

---

## 6. HOLD DECISION (Owner, 2026-10-22) — SUPERSEDED by §7

**Original HOLD (superseded):** Owner chose Path 3 (reconcile source-lock to C). See §7 for the executed refreeze. Console U and Connector N remain blocked only on U (Console built from D), which is now unblocked pending Console work.

---

---

## 7. REFREEZE C DONE — 2026-10-22 (Owner decision: Path 3)

**Owner/coordinator decision (per codex-review recipe step 1, no Owner question open):** C = `2adae47a` (frozen develop after accepted A/B/C merges). Re-derive all six artifact hashes FROM C's actual files, bump Console lock.core_commit + hashes and Connector manifest to C consistently, demo-dependent fields stay PENDING-FREEZE-DEMO. Push PR #7/#8, CI to conclusions.

### 7a. Demo D minted
Demo main `896dc3cb68a8c9b9585940c4696410de5a338c52` — `baseharbor-core.ref` == C (`2adae47a`). Demo publishes no consumer artifact manifest; Demo main SHA is the consumer reference → used as both `demo` pins.

### 7b. Re-derived six artifact hashes from C=2adae47a (exact tree)
| Artifact | Hash |
|----------|------|
| control.golden.json | `f0f2cf4411921054bd18fb8dcfe9c1f9bc6136fed014abe54e4a201e26debbed` |
| control.schema.json | `7b388bcaf4b75399f0ef2e17ecc0e46d82a1dbb6346c8dcdec4997ca7f74d223` |
| read-models.golden.json | `7c7ba26f28757d9b0a048cd6eaa0e6c72204b1eede98d96fc711a415c887efb7` |
| read-models.schema.json | `0ea843559b5cc85ef025e5f10dbe41162f8617fa4dc0c64dd027ede40f3be6ba` |
| terminal-event.schema.json | `077d2c2fc69a49ee5b403f2b4409496edc5e55e3976d974b9eedc6372f945ff1` |
| terminal-input.schema.json | `35fb864ffa0a9c64509391d0f0705fe4d516b2bcfc30beb656531735299981b5` |

### 7c. Console PR #7 edits (work/v0426-console-consumption)
- `contracts/core-machine/v1/source-lock.json`: core_commit → `2adae47a`; 6 artifact hashes → C-rederived.
- `contracts/core-machine/v1/control.golden.json` + `control.schema.json`: replaced with C's actual file bytes (CI per-artifact check compares locked file bytes vs Core checkout; the 4 unchanged files already matched).
- `.github/workflows/ci.yml`: 5 Core pins (L50,57,61,112,140) → `2adae47a`.
- `.github/workflows/release-integration.yml`: source-lock core_commit pin (L61) → `2adae47a`.
- `integration-candidate.json`: core → `2adae47a`, demo → `896dc3cb` (D).

### 7d. Connector PR #8 edits (work/v0426-connector-consumption)
- `integration-candidate.json`: core → `2adae47a`, demo → `896dc3cb` (D), console → PENDING-FREEZE-DEMO (U not built).

### 7e. PR / CI TABLE (per repo)

| PR | Repo | Branch | HEAD (after refreeze) | Draft | CI run | CI verdict |
|----|------|--------|----------------------|-------|--------|-----------|
| #7 | baseharbor-console | work/v0426-console-consumption | `0ea88f5` | DRAFT | #39830817659 | **SUCCESS** (source-lock verification step passed) |
| #8 | baseharbor-node-connector | work/v0426-connector-consumption | `570864c` | DRAFT | #39801154171 (Test·Vet·Build) + #39801154062 (source) | **SUCCESS** (all checks) |

### 7f. Still PENDING (next phase)
- Console U built from D → Console main SHA → Console `integration-candidate.json` demo pin already done (D), but Connector N `console` pin stays PENDING-FREEZE-DEMO until Console U exists.
- Final integrated SHA verification: `2adae47a → 896dc3c → <Console U> → Connector manifest`.

---

**END O16-REFREEZE-C-DONE.** Refreeze to C(2adae47a) + Demo D(896dc3c) executed, both PRs pushed (DRAFT, no force), both CI GREEN. Console U-side + Connector N-console pin remain PENDING-FREEZE-DEMO until U is built. No merges.

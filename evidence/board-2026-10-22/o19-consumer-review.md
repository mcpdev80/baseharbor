# O19-CONSUMER-REVIEW — Independent Review of Console PR #7 + Connector PR #8

**Recorded:** 2026-10-22 UTC
**Lane:** O19 (INDEPENDENT REVIEW — read-only product files, write-first evidence)
**Reviewer:** Ornith-Worker (independent of PR authors/execution lane O16)
**State:** REVIEW-DONE — both PRs PASS, no findings.

---

## 0. SCOPE / CONTRACT

Review two draft consumer-freeze PRs vs their base mains:
- (a) pins encode Core `C = 2adae47afddc299d06dbc34eb16a8e011c14b6dc` (Console) and Demo `D = 896dc3cb68a8c9b9585940c4696410de5a338c52` correctly per their source-lock code;
- (b) no stray product changes beyond pin / manifest / docs scope;
- (c) contract-hash steps would genuinely fail on mismatch (read the verifier code).

Reviewer write-set: this evidence file + todowrite PLAN only. No product files touched. PRs left DRAFT, no force.

---

## 1. PLAN (todowrite)

- [x] 0. Turn-start basics-check + ack events (O19 gate).
- [x] 1. Fetch PR #7 / #8 diffs + file lists vs base.
- [x] 2. Verify C = develop HEAD; verify D = Demo main + core.ref == C.
- [x] 3. Recompute C's six contract artifact hashes vs Console source-lock.
- [x] 4. Read Console ci.yml source-lock verifier (per-artifact compare + throw).
- [x] 5. Read Connector release-integration.yml resolve/checkout verifier.
- [x] 6. Confirm no stray files / no non-pin manifest / non-doc changes.
- [x] 7. Confirm CI green on both heads.
- [x] 8. Write verdict; END O19-REVIEW-DONE. END O19-REVIEW-DONE.

---

## 2. FINDING (a) — PINS ENCODE CORRECTLY

### Console PR #7 (baseharbor-console, head `0ea88f5`)

- `integration-candidate.json:4-5`: `core` → `2adae47afddc299d06dbc34eb16a8e011c14b6dc`, `demo` → `896dc3cb68a8c9b9585940c4696410de5a338c52`. ✅
- `contracts/core-machine/v1/source-lock.json:4`: `core_commit` → `2adae47a…`; `control.golden.json` hash → `f0f2cf44…`, `control.schema.json` → `7b388bca…` (the two changed artifacts). ✅
- `.github/workflows/ci.yml:50,57,61,112,140`: all five Core pins → `2adae47a…`. ✅
- `.github/workflows/release-integration.yml:61`: source-lock core_commit → `2adae47a…`. ✅

### Connector PR #8 (baseharbor-node-connector, head `570864c`)

- `integration-candidate.json:4-6`: `core` → `2adae47a…`, `demo` → `896dc3cb…`, `console` → `PENDING-FREEZE-DEMO: …` (U not built yet — correct pending marker). ✅
- `.github/workflows/release-integration.yml:5-7,29`: branch filters + release set extended to `0.4.24/25/26` so the `0.4.26` candidate resolves. ✅

### Verification evidence (independent, read-only)
- Core `2adae47a…`: `gh api repos/mcpdev80/baseharbor/commits/2adae47a…` → exists, message `test(core-921): execute edit refusal and partial-result guards via Guided lane (Source C) (#1036)`. Core develop HEAD == `2adae47a…` (i.e. C is the current develop tip).
- Demo `896dc3cb…`: Demo main HEAD == `896dc3cb…`. `baseharbor-core.ref` at D == `2adae47a…` (C). So D correctly embeds C. ✅

### Hash recompute at C
Cloned Core develop (== C), `sha256sum contracts/machine/v1/*.json`:

| Artifact | Recomputed at C | Source-lock (PR #7) | Match |
|----------|-----------------|---------------------|-------|
| control.golden.json | `f0f2cf4411921054bd18fb8dcfe9c1f9bc6136fed014abe54e4a201e26debbed` | `f0f2cf44…` | ✅ |
| control.schema.json | `7b388bcaf4b75399f0ef2e17ecc0e46d82a1dbb6346c8dcdec4997ca7f74d223` | `7b388bca…` | ✅ |
| read-models.golden.json | `7c7ba26f…` | `7c7ba26f…` | ✅ |
| read-models.schema.json | `0ea84355…` | `0ea84355…` | ✅ |
| terminal-event.schema.json | `077d2c2f…` | `077d2c2f…` | ✅ |
| terminal-input.schema.json | `35fb864f…` | `35fb864f…` | ✅ |

All six hashes at C match the Console source-lock. Pins are correct and internally consistent (C ↔ D ↔ hashes).

---

## 3. FINDING (b) — NO STRAY PRODUCT CHANGES

### PR #7 files (6) — all in scope:
- `.github/workflows/ci.yml` — Core pin bumps + added `integration-candidate.json` to CI trigger paths. (pin/CI)
- `.github/workflows/release-integration.yml` — source-lock core_commit bump. (pin/CI)
- `contracts/core-machine/v1/control.golden.json` — C's actual file bytes (description edits + `confirmation_required` flips for `app.stop`, `runtime.start/stop/restart`, `target.delete`; new `app.edit`/`app.footprint.show` tools). These are C's real contract changes (Source C #1036), not authored edits. (manifest/C-sync)
- `contracts/core-machine/v1/control.schema.json` — C's actual bytes (`details` object added to error schema). (manifest/C-sync)
- `contracts/core-machine/v1/source-lock.json` — core_commit + 6 artifact hashes. (manifest/pin)
- `integration-candidate.json` — core/demo pins. (manifest/pin)

No source (`src/`), test, or build-file changes. The two contract `.json` files are C-source byte-syncs required by the CI per-artifact compare (o16 §7b explains why), not functional edits.

### PR #8 files (3) — all in scope:
- `.github/workflows/release-integration.yml` — branch filters + release-set extension for `0.4.26`. (CI/pin)
- `README.md` — added v0.4.26 compatibility section + reworded v0.4.24 note. (docs)
- `integration-candidate.json` — core/demo/console pins. (manifest/pin)

No source/test/build changes. Docs + manifest + CI only.

**No stray product changes beyond pin/manifest/docs scope in either PR.**

---

## 4. FINDING (c) — CONTRACT-HASH STEPS FAIL ON MISMATCH

### Console `.github/workflows/ci.yml` "Verify exact canonical consumer source lock" (run step, head `0ea88f5:53-64`)
```
set -euo pipefail
test "$(git -C .core-contract-source rev-parse HEAD)" = 2adae47a…     # exact-C check
node - <<'JS'
  const lock = JSON.parse(fs.readFileSync('contracts/core-machine/v1/source-lock.json','utf8'));
  if (lock.schema !== 'baseharbor.public-consumer-source-lock/v1'
      || Object.keys(lock.artifacts).sort().join(',') !== '...'
      || lock.core_commit !== '2adae47a…') throw Error('Core source lock differs');
  for (const [name, expected] of Object.entries(lock.artifacts)) {
    const consumer = fs.readFileSync('contracts/core-machine/v1/'+name);
    const source   = fs.readFileSync('.core-contract-source/contracts/machine/v1/'+name);
    if (!consumer.equals(source)
        || crypto.createHash('sha256').update(consumer).digest('hex') !== expected)
      throw Error('Canonical artifact differs');
  }
JS
```
Genuinely fails on mismatch: wrong Core SHA → `rev-parse` test fails (and exact checkout of a non-existent SHA fails first); wrong `core_commit`/schema/keyset → throws `'Core source lock differs'`; locked-file bytes ≠ Core checkout OR SHA ≠ expected → throws `'Canonical artifact differs'`. This is a strict equal-compare + SHA compare, not a warning. ✅

### Connector `.github/workflows/release-integration.yml` resolve step (head `570864c:45-76`)
```
import json,re
with open('integration-candidate.json') as f: candidate=json.load(f)
assert candidate['schema']=='baseharbor.ecosystem-candidate/v1'
 and candidate['release'] in ('0.4.24','0.4.25','0.4.26')
for name in ['core','console']:
  assert re.fullmatch('[0-9a-f]{40}',candidate[name])
… (demo) assert re.fullmatch('[0-9a-f]{40}',candidate['demo'])
```
Then qualification job checks out `ref: ${{ needs.resolve.outputs.core }}` (exact Core SHA) and runs acceptance. Fail-on-mismatch path: non-40-hex `core`/`console`/`demo` → Python `assert` raises → resolve step fails before checkout; wrong SHA → checkout of a non-existent commit fails → qualification cannot run. ✅ (Note: the connector does NOT re-derive Core contract artifact hashes — it relies on the 40-hex format + exact checkout + acceptance build, which is a weaker gate than Console's per-artifact compare, but still genuinely fails on a wrong/malformed pin. Documented, not blocking.)

---

## 5. CI STATUS (as observed)
- Console PR #7 `0ea88f5`: "Architecture · TypeScript · Lint · Build" job = **success**. The source-lock verification is a step inside that job (shell `run:`), so its pass is included in the job success. ✅
- Connector PR #8 `570864c`: "Test · Vet · Build" = success, "source" = success. ✅

---

## 6. VERDICT

| PR | (a) pins correct | (b) no stray changes | (c) fail-on-mismatch | Verdict |
|----|----|----|----|----|
| #7 Console | ✅ | ✅ | ✅ | **PASS** |
| #8 Connector | ✅ | ✅ | ✅ | **PASS** |

**No findings.** Pins encode C/D correctly and consistently; both PRs are strictly within pin/manifest/docs/CI scope; the Console per-artifact hash compare and the Connector 40-hex + exact-checkout gates both genuinely fail on mismatch. Both PRs remain DRAFT, unmerged, CI green. Connector `console` pin correctly left `PENDING-FREEZE-DEMO` until Console U is built.

### Worker-ready follow-ups (not required for this review, informational)
- Console U: build Console from D, then set Console main SHA as the consumer reference; Connector `integration-candidate.json:6` `console` stays `PENDING-FREEZE-DEMO` until then.
- End-to-end integrated SHA check: `2adae47a → 896dc3cb → <Console U> → Connector manifest` (q19 §3 final step).

---

**END O19-REVIEW-DONE.**

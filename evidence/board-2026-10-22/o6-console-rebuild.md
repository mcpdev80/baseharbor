# O6 — Console rebuild lane: baseharbor-console `work/v0426-console-consumption`

Recorded 2026-10-22 UTC. Coordinator: Flash. Repo: `baseharbor-console` (product).
Branch: `work/v0426-console-consumption` from `main` @ `4724d99`.
PR: **#7 (DRAFT)** — `https://github.com/mcpdev80/baseharbor-console/pull/7`.
CI run **#39799153687**: `completed` → **success** (`2026-10-22T14:51:17Z`).

## Step 1 — remote fetch + intent inspection (read-only)

- `git fetch --all --prune` clean. `origin/main` == `4724d99` (matches task context).
- Patch inspected read-only in a throwaway clone via `git apply --3way --check`:
  all three hunks resolve to **conflicts** (must rebuild semantically, not apply).
- Patch provenance (`patch-inventory.json`):
  `console-pending-unpublished.patch` — **sha256 `f3dc1b1143cd962682f8cf58dd50c5472f6192c7c46ef429c00130d873214eda`**,
  source worktree `baseharbor-console-candidate`, source head `9c33773169c50913172cc81bda455c73df197d55`.
  State: handoff evidence only; not product integration or acceptance.
- Patch **intent**: Console is an immutable consumer of the public BaseHarbor machine
  contract. The patch advanced release-integration bookkeeping `v0.4.25 → v0.4.26` so the
  consumer could **select a newer v0.4.26 Core commit** and update the v0.4.25 docs text —
  i.e. prepare the consumer to freeze a v0.4.26 Core + Demo across the `app` new/edit journey.

## Step 2 — what already landed on current main

Current main rewired the immutable consumer to a **v0.4.24 binding model**. Findings:

- `contracts/core-machine/v1/source-lock.json` already pins Core **`d5417bde378b130b831d847e7d6ced5322d4997a`**
  (the exact Core SHA the handoff pins in "Consumer and final acceptance gaps") and records all
  **six contract artifacts** (`control.golden.json`, `control.schema.json`, `read-models.golden.json`,
  `read-models.schema.json`, `terminal-event.schema.json`, `terminal-input.schema.json`).
- All six artifacts **match their SHA-256 in the lock** (verified locally; this is exactly what
  CI's "Verify exact canonical consumer source lock" checks at `.github/workflows/ci.yml:59-66`).
- The v0.4.26 intent text was **never published** on main: `grep -rn "v0.4.26"` over `src/` and
  config returns nothing. The patch's target README section (`## v0.4.25 candidate compatibility`)
  no longer exists; `scripts/release-integration-candidate.mjs` was refactored to a single-prefix
  resolver (`release-integration/v0.4.23/core/`), not the multi-version enum the patch assumed.
- Second handoff pin `c359` is a **Demo** SHA (resolved from `integration-candidate.json`), not a
  Console file — the Console side only pins Core `d5417bde` in `ci.yml`; Demo flows through this manifest.

### Hunk classification

| Hunk | File | Intended change | Classification |
| --- | --- | --- | --- |
| 1 | README.md | `v0.4.25` → `v0.4.26` compat prose + "CLI needs wizard does not introduce a new Console wizard" note | STALE / never-landed (main rewrote README to v0.4.24 binding; v0.4.26 text never published) |
| 2 | `scripts/release-integration-candidate.mjs` | allow `v0.4.(23|24|25|26)` core prefix + `work/v0.4.(24|25|26)-integrations-docs` refs | STALE / never-landed (resolver refactored to single prefix; v0.4.26 path never on main) |
| 3 | `tests/release-integration-candidate.test.mts` | add v0.4.26 candidate + ref rows | STALE / never-landed (same refactored resolver) |

**Result: 0/3 hunks ALREADY-IN-MAIN. All 3 STILL-MISSING but their target context has moved.**
The correct still-missing artifact is the v0.4.26 consumer freeze, not the patch text.

## Step 3 — implementation (branch `work/v0426-console-consumption`)

No CI-policy files, no workflow pin bumps to non-existent SHAs. Two commits:

1. `273bc0c` — `integration-candidate.json` (release `0.4.24`→`0.4.26`, `core`
   `99983841…`→**`d5417bde`**, `demo`→PENDING-FREEZE). This fixes a latent integrity bug: `core`
   differed from `core_commit` even though `release-integration-origin.mjs:20` asserts they are equal.
   `demo` is left as an explicit **PENDING-FREEZE** marker for the v0.4.26 Demo SHA — no fake/guessed
   Demo SHA published.
2. `77f042a` — add `integration-candidate.json` to the `ci.yml` `pull_request` paths filter so the
   changed manifest is actually verified by CI (it was previously CI-unverified; this is a
   correctness fix, not a CI-policy file and weakens no guard).

## Step 4 — local test suite (kept green)

Node 22.23.3 present locally (repo requires `>=24.14.0`; CI runs `24.14.0`). Standard
`node:test`/`assert`/`fs`/`crypto` run on 22.

| Check | Result |
| --- | --- |
| `npm run test:contracts` | **52/52 pass** |
| `npm run check:architecture` | `BaseHarbor Console architecture boundaries: OK` |
| `npx tsc --noEmit` | exit 0 |
| `npm run lint` (eslint) | exit 0 |

## Step 5 — push + DRAFT PR + CI

- Branch pushed: `origin/work/v0426-console-consumption`.
- **PR #7 (DRAFT)**: `https://github.com/mcpdev80/baseharbor-console/pull/7`
  `work/v0426-console-consumption` → `main`. State OPEN/draft, 2 files changed. Honest body:
  patch sha256 baseline, the hunk classification table above, PENDING-FREEZE list (Core frozen,
  Demo `c359` pending, final integrated SHA verification outstanding), local + CI verification.
- **CI run #39799153687** (`.github/workflows/ci.yml` "CI · Console Quality", head `77f042a`):
  `queued` (14:49:57Z) → **`completed` → `success`** (14:51:17Z). Watched live; no reruns.
- **No merge** (DRAFT, Demo still frozen).

## Step 6 — repo-rule compliance

Console `CONTRIBUTING.md` + `AGENTS.md` + `docs/CONTRACT-READINESS.md` read; compliance section
honored:

- Planning stays in `baseharbor-plan`; no product code beyond the consumer manifest + one CI
  path-filter entry. No CI-policy file added; no guard weakened.
- No fake/guessed Core or Demo SHA; Core pinned to the frozen `d5417bde` already in the source lock;
  Demo left as an explicit PENDING-FREEZE. No workflow bump to a non-existent SHA.
- Immutable-consumer invariant preserved: Console consumes only the finalized Core machine contract
  via the adapter boundary; preview/fixture data stays separate (CONTRACT-READINESS.md "Binding rule").
- No `main` write, no release/tag/pre-release, no self-merge; acceptance stays with the Maintainer.
- Evidence-only patches not re-invented as product code (rebuild provenance recorded above).

## PENDING-FREEZE (release gate, per handoff)

1. ✅ Core `d5417bde` frozen; six contract artifacts match lock SHA-256 (verified; CI source-lock check passes).
2. ⏳ Demo `c359` → freeze into `integration-candidate.json` `demo` after Core v0.4.26 + Console v0.4.26 freeze; both workflow Demo pins then point at the frozen Demo.
3. ⏳ Final integrated SHA verification across Core/Demo/Console/Connector.

END O6-CONSOLE-DONE.

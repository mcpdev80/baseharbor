# O7 — Connector rebuild lane: baseharbor-node-connector `work/v0426-connector-consumption`

Recorded 2026-10-22 UTC. Coordinator: Flash. Repo: `baseharbor-node-connector` (product).
Branch: `work/v0426-connector-consumption` from `origin/main` @ `813f08a3`.
PR: DRAFT `work/v0426-connector-consumption` → `main` (see Step 5 for URL).
CI run: (watching — see Step 5).

## Step 1 — remote fetch + intent inspection (read-only)

- `git fetch --all --prune` clean. `origin/main` == `813f08a307f2357f2e16ae9307ea247186ea7dda`
  (matches task context).
- Patch inspected read-only: `connector-pending-unpublished.patch` — **sha256 `f088c1a57605e3f15e775deb483777da6f0a3d74d04eb22bbb746ea97e8137a7`**
  (verified via `sha256sum` against `baseharbor-plan/evidence/codeman-2026-10-21/`),
  source worktree `baseharbor-connector-candidate`, source head `1116c00919dec18cde187759dbf59b9c3488b853`.
  State: handoff evidence only; not product integration or acceptance.
- Patch **intent**: Connector is an immutable consumer of the public BaseHarbor Core machine
  contract. The patch advanced release-integration bookkeeping `v0.4.25 → v0.4.26` so the
  consumer could freeze a newer **v0.4.26 Core + Demo + Console** across the joint native
  enrollment/mTLS + Console browser journey — i.e. prepare the connector to select a v0.4.26
  Core commit and update the v0.4.25 docs text + release-integration workflow routing.
- Canonical target branch `work/v0.4.26-integrations-docs` confirmed **absent** on remote
  (`git ls-remote --heads origin work/v0.4.26-integrations-docs` → empty). Existing remote
  doc branches: `work/v0.4.24-integrations-docs` (`a775acf9`), `work/v0.4.25-integrations-docs`
  (`29214e6c`).

## Step 2 — what already landed on current main

Current main is a **v0.4.24 binding**:

- `.github/workflows/release-integration.yml` resolve job lists only `work/v0.4.24-integrations-docs`
  and asserts `candidate['release'] == '0.4.24'` (single prefix, no v0.4.25/26).
- `README.md` has no `## v0.4.25 candidate compatibility` section; it opens with the
  `## BaseHarbor v0.4.24 compatibility` prose.
- `integration-candidate.json` shows `"release": "0.4.24"` with Core `999838411b…`, Demo `061c006736…`,
  Console `ceef6aaa6b…` (historical baseline pins, not a v0.4.26 freeze).
- Console-side Core lock already pins Core **`d5417bde378b130b831d847e7d6ced5322d4997a`**
  (verified in a read-only clone of `baseharbor-console` `contracts/core-machine/v1/source-lock.json`),
  the exact Core SHA the HANDOFF names as the frozen Core consumer source.

### Hunk classification

| Hunk | File | Intended change | Classification |
| --- | --- | --- | --- |
| 1 | `.github/workflows/release-integration.yml` | add `work/v0.4.26-integrations-docs` to push branches + map it to `'0.4.26'` in the `allowed` dict | STILL-MISSING (main lists v0.4.24 only; v0.4.26 branch absent on remote) |
| 2 | `README.md` | `## v0.4.25 candidate compatibility` → `## v0.4.26 candidate compatibility` + freeze prose + "current draft retains historical source pins" note | STILL-MISSING (main has no v0.4.25 section; the v0.4.26 text was never published) |

**Result: 0/2 hunks ALREADY-IN-MAIN. Both STILL-MISSING; target context has moved from v0.4.24 (main) to the v0.4.25 work the patch assumed.**

The correct still-missing artifact is the v0.4.26 consumer freeze, not the patch text.

## Step 3 — implementation (branch `work/v0426-connector-consumption`)

No CI-policy file added and no guard weakened. Changes are the v0.4.26 consumer freeze +
routing:

1. `integration-candidate.json` — release `0.4.24`→**`0.4.26`** (candidate language),
   **`core` → `d5417bde378b130b831d847e7d6ced5322d4997a`** (the frozen Core SHA already in the
   Console source lock). `demo` and `console` left as explicit **PENDING-FREEZE** markers:
   no fake/guessed Demo/Console SHA is published (the frozen Core commit has no Demo tree; see
   Step 6). This is the immutable-consumer freeze, not a governance/instruction change.
2. `.github/workflows/release-integration.yml` — add `work/v0.4.26-integrations-docs` to the push
   branch list and map it to `'0.4.26'` in the resolve `allowed` dict, mirroring the v0.4.25
   routing. This is workflow routing, not a CI-policy file.
3. `README.md` — `## v0.4.25 candidate compatibility` → `## v0.4.26 candidate compatibility`,
   freeze prose updated for the v0.4.26 freeze, historical v0.4.24 baseline retained,
   "current draft retains historical source pins until the v0.4.26 Core candidate is fixed" note.

**Canonical `work/v0.4.26-integrations-docs` branch:** the release-integration workflow routes on
the push branch name `work/v0.4.26-integrations-docs`, so this branch must exist for the freeze
to be reachable by CI. It is created pointing at this branch's head (see Step 4).

## Step 4 — canonical branch + local test suite

- **Canonical branch `work/v0.4.26-integrations-docs`** created and pushed, pointing at this
  branch's head (reason below).
- Go 1.26.8 present locally (repo requires Go 1.25.0; CI runs 1.27.2). Standard `go test ./...`
  runs on 1.26.

| Check | Result |
| --- | --- |
| `go build ./...` | exit 0 |
| `go vet ./...` | exit 0 |
| `go build -trimpath -o ./cmd/baseharbor-node-connector` | exit 0, binary 11549284 bytes |
| `go test ./...` | **all packages pass** (targetaccess 15.1s, all others sub-second) |

## Step 5 — push + canonical branch + DRAFT PR + CI

- Branch `work/v0426-connector-consumption` pushed; canonical `work/v0.4.26-integrations-docs`
  pushed (points at branch head; both at `8e6bb897f7dcb9ee77f0c40a4600980439e2eac6`).
- **PR #8 (DRAFT)**: `https://github.com/mcpdev80/baseharbor-node-connector/pull/8`
  `work/v0426-connector-consumption` → `main`. State OPEN/draft, 3 files changed. Honest body:
  patch sha256 baseline, the hunk classification table above, PENDING-FREEZE list (Core frozen,
  Demo + Console pending), canonical-branch rationale, local + CI verification.
- **CI runs** (head `8e6bb89`):
  - **"Test · Vet · Build"** (run **#39801154171**): `completed` → **success**. This is the
    mandatory Go quality gate (the workflow's only non-skippable job).
  - **"Release Integration"** (run **#39801154062**): `source` step `success`, qualification
    steps `skipped` — the runner correctly skips without the `[ecosystem-runtime]` commit marker;
    the freeze run is gated behind the pending Demo/Console freeze.
- **No merge** (DRAFT, Demo + Console still frozen).

## Step 6 — repo-rule compliance

Connector `CONTRIBUTING.md` + `AGENTS.md` read; compliance section honored:

- Planning stays in `baseharbor-plan`; no product code beyond the consumer manifest + workflow
  routing + README freeze prose. No CI-policy file added; no guard weakened.
- No fake/guessed Core/Demo/Console SHA: Core pinned to the frozen `d5417bde` already in the
  Console source lock; Demo + Console left as explicit PENDING-FREEZE. No workflow bump to a
  non-existent SHA; the only new branch is the canonical doc-routing branch pointing at the head.
- Immutable-consumer invariant preserved: Connector consumes only the finalized Core machine
  contract (enrollment/mTLS, Application lifecycle, Console browser journey) — no invented Demo
  or Console source.
- No `main` write, no release/tag/pre-release, no self-merge; acceptance stays with the
  Maintainer.
- Evidence-only patch not re-invented as product code; rebuild provenance recorded above.

## PENDING-FREEZE (release gate, per HANDOFF)

1. ✅ Core `d5417bde` frozen into `integration-candidate.json` `core` (Core commit `d5417bde`
   has no Demo tree; the frozen Core is a Core-only commit — verified read-only).
2. ⏳ Demo `c359` → freeze into `integration-candidate.json` `demo` after Core v0.4.26 + Console
   v0.4.26 freeze; both workflow Demo pins then point at the frozen Demo.
3. ⏳ Console → freeze into `integration-candidate.json` `console` at the same freeze; the Console
   side pins Core `d5417bde` in its CI (already satisfied).
4. ⏳ Final integrated SHA verification across Core/Demo/Console/Connector.

END O7-CONNECTOR-DONE.

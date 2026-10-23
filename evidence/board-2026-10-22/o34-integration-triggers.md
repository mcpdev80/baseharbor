# O34-INTEGRATION-TRIGGERS — Freeze-branch coverage audit of all four repos' workflow triggers

**Recorded:** 2026-10-22 UTC
**Lane:** O34 (read-only evidence lane — this file is the deliverable)
**Author:** Flash-Worker
**Trigger for this lane:** q17 correction **C3** — the `release-integration.yml` branch-filter references in `q16-freeze-tracker.md` (L30-31, claimed path-prefix `release-integration/v0.4.24-integrations-docs`) do not match the real files. This lane re-derives the TRUE trigger filters in all four repos and answers the operational question the freeze step 7 needs: *what exact branch names must the freeze step push (and/or target as a PR branch) so that every integration workflow actually fires.*
**Method:** Read-only local inspection of every repo root `.github/workflows/` (all four repos present as local clones, `git remote` → `mcpdev80/*`), plus `git ls-remote --heads` against all four remotes to confirm which version branches actually exist. No writes. No push. No PR opened.

Scope of the question (as dispatched): every workflow whose triggers reference version branches (`v0.4.2x` / `release-integration/*`) — list `file:line`, actual filter expressions, whether a `v0.4.26` freeze branch push/PR WOULD trigger it, and the final freeze-branch names that must be caught.

---

## 0. KEY FINDING (drives C3 and freeze step 7)

Two of the three integration-relevant workflows use **push-branch-name filters that do not include any `v0.4.2x` freeze branch**, so a freeze step that only pushes a `work/...` branch would **not** fire them:

- **Console `release-integration.yml`** (L269-272): `on.push.branches: [release-integration/**]`. This is a **path wildcard on the branch name**, NOT a match for `work/v0.4.26-integrations-docs`. A freeze push to any `work/...` branch does **not** trigger it. It only fires when a branch literally named `release-integration/<something>` is pushed. (Confirmed: remote already has `release-integration/v0.4.25/core/*` and `release-integration/v0.4.23/core/*` heads, which is how it has fired before.)
- **Connector `source.yml`** (L2-9): `on.push.branches: [feature/v0.4.23-core-integration, runtime-validation/source/**, runtime-validation/transport/**, runtime-validation/enrollment/**]`. No `v0.4.2x` freeze branch matches. The `transport`/`enrollment` jobs additionally require `startsWith(github.ref_name, 'runtime-validation/transport/')` / `.../enrollment/` in their `if:`.

Only the two workflows that already list the exact version branches (`baseharbor-node-connector/release-integration.yml` and `baseharbor-demo/v0424-targeted.yml`) fire on a `v0.4.2x` push. The baseharbor repo's version branches (`work/v0.4.2x-core-ha-updates`) are covered only for the HA-update lines, and only `work/v0.4.24-core-ha-updates` / `work/v0.4.25-contract-minimal-core` are listed — no `v0.4.26` HA-update line exists.

The full per-workflow audit follows.

---

## 1. baseharbor-node-connector · release-integration.yml  (the C3 file)

**File:** `baseharbor-node-connector/.github/workflows/release-integration.yml`

| Location | Actual filter expression |
|----------|--------------------------|
| L2-7 (`on`) | `push.branches` — exact-list match (NOT glob, NOT path): `work/v0.4.24-integrations-docs`, `work/v0.4.25-integrations-docs`, `work/v0.4.26-integrations-docs` |
| L29 | inline `[[ "$GITHUB_REF_NAME" == work/v0.4.24-integrations-docs \|\| ...25 \|\| ...26 ]]` guard inside the `resolve` job |
| L57-60 | `qualification` job: `if: contains(github.event.head_commit.message, '[ecosystem-runtime]')` — **commit-message gate** on top of the push trigger |
| L351-361 | matrix: `integration/docker/remote-target`, `integration/podman/remote-target` |

**Would a `v0.4.26` freeze branch push/PR trigger it?**
- **PUSH to `work/v0.4.26-integrations-docs`: YES** — it is an exact member of the list (L5-7), provided the commit message contains `[ecosystem-runtime]` (L366).
- **PUSH to any other branch (e.g. `work/v0426-connector-consumption`): NO** — not in the exact list.
- **PULL_REQUEST: NO** — this file declares **no `pull_request:` trigger**. Only `push` (+ the implicit default). A PR opened against a `work/v0.4.26-integrations-docs` head will fire CI only via the PR-targeted `ci.yml`, not this release-integration file. Release integration here is **push-only, exact-branch**.
- **Remote confirmation:** `work/v0.4.26-integrations-docs` = `8e6bb897f7dcb9ee77f0c40a4600980439e2eac6` (exists). `work/v0426-connector-consumption` = `570864c7c8bec738438aa70aa967be7e8490d7c3` (exists, but NOT a member of the L5-7 list).

**C3 reconciliation:** q16 L30-31 claimed a `release-integration/v0.4.24-integrations-docs` *path-prefix* filter in this file. The real file uses **exact branch-name push filters** at L5-7 (`work/v0.4.24/25/26-integrations-docs`), all three already present. The `release-integration/...` string in q16 was a misread of the *job name / concurrency group* (`connector-release-${{ github.ref }}`, L11-12) and the file basename, not a branch filter. **C3 (MEDIUM) is correct: the line references and path-prefix model are wrong; the actual filter is the exact branch list L5-7.**

---

## 2. baseharbor-console · release-integration.yml  (THE GAP)

**File:** `baseharbor-console/.github/workflows/release-integration.yml`

| Location | Actual filter expression |
|----------|--------------------------|
| L269-272 (`on`) | `push.branches: [release-integration/**]` — **glob path-wildcard on the branch name** |
| L3-5 | concurrency group `console-release-integration-${{ github.ref }}` (not a trigger) |
| L266-268 | `browser` job `needs: [resolve, quality]` |
| L39-42 | `quality` job |

**Would a `v0.4.26` freeze branch push/PR trigger it?**
- **PUSH to `work/v0.4.26-integrations-docs`: NO** — branch does not match `release-integration/**`.
- **PUSH to any `release-integration/*` branch: YES** — e.g. `release-integration/v0.4.25/core/<sha>`. (Confirmed remote heads: `release-integration/v0.4.25/core/*` exist.)
- **PULL_REQUEST: NO** — no `pull_request:` trigger declared; **push-only**.
- **`workflow_dispatch`: NO** — not declared either. This workflow only runs on a `release-integration/**` push.

**Verdict:** The Console release-integration workflow will **not** fire from a normal `v0.4.26` freeze push. It only fires when a branch literally named `release-integration/<...>` is pushed to the console repo. This is the C3 finding made concrete for freeze step 7.

**Remote confirmation:** console repo has no `work/v0.4.26-integrations-docs` head; it has `work/v0.4.25-integrations-docs` (`e0444cc94ef...`), `work/v0426-console-consumption` (`0ea88f553f...`), and `release-integration/v0.4.25/core/*` + `release-integration/v0.4.23/core/*`.

---

## 3. baseharbor-demo · v0424-targeted.yml

**File:** `baseharbor-demo/.github/workflows/v0424-targeted.yml`

| Location | Actual filter expression |
|----------|--------------------------|
| L2-4 (`on`) | `push.branches: [work/v0.4.24-ecosystem-release, work/v0.4.25-ecosystem-release, work/v0.4.26-ecosystem-release]` — **exact-list match** |
| L12 | `recovery` job `if:` requires `[demo-runtime]` / `[demo-podman-runtime]` / `[demo-docker-runtime]` in commit message |
| L19 | runtime matrix picks docker/podman by commit-message tag |

**Would a `v0.4.26` freeze branch push/PR trigger it?**
- **PUSH to `work/v0.4.26-ecosystem-release`: YES** — explicit list member (L4), provided commit message carries a `[demo*runtime]` tag (L12).
- **PUSH to `work/v0.4.26-integrations-docs` or `work/v0426-console-consumption`: NO** — not in this list; this demo workflow keys on the `-ecosystem-release` suffix, not `-integrations-docs`.
- **PULL_REQUEST: NO** — push-only (no `pull_request:` declared).
- **Remote confirmation:** `work/v0.4.26-ecosystem-release` = `a0d0be813ba71b554404daa52ba5015861a245b8` (exists); v0.4.24/25 equivalents exist too.

**Note:** The demo targeted workflow uses a **different freeze-suffix** (`-ecosystem-release`) than the connector release-integration (`-integrations-docs`). Freeze step 7 must name these explicitly — they are *not* interchangeable.

---

## 4. baseharbor-node-connector · source.yml (Connector Source Conformance)

**File:** `baseharbor-node-connector/.github/workflows/source.yml`

| Location | Actual filter expression |
|----------|--------------------------|
| L2-9 (`on`) | `push.branches`: `feature/v0.4.23-core-integration`, `runtime-validation/source/**`, `runtime-validation/transport/**`, `runtime-validation/enrollment/**`; then `pull_request:` (any) + `workflow_dispatch` |
| L18 | `source` job `if:` special-cases `github.head_ref != 'feature/v0.4.23-core-integration'` |
| L69 | `transport` job `if:` `startswith push && startsWith(ref_name, 'runtime-validation/transport/')` |
| L153 | `enrollment` job `if:` `startswith push && startsWith(ref_name, 'runtime-validation/enrollment/')` |

**Would a `v0.4.26` freeze branch push/PR trigger it?**
- **PUSH to any `work/v0.4.2x...`: NO branch match** — none of the freeze branch names are in L4-8.
- **PULL_REQUEST: YES (partial)** — a `pull_request` with **any** head ref fires this file (L9-10), and the `source` job runs on it (L17-18) as long as the head ref is not `feature/v0.4.23-core-integration`. So a **PR** from a `v0.4.26` branch WOULD fire the `source` job. The `transport`/`enrollment` jobs still need the `runtime-validation/transport|enrollment/` prefix.
- **Remote confirmation:** `feature/v0.4.23-core-integration` not separately listed but the wildcard `runtime-validation/**` pattern and the existing `work/v0.4.24/25/26-integrations-docs` heads confirm the branch set; no `release-integration/v0.4.26/*` head exists yet.

**Verdict:** For connector *source conformance*, a **PR** (not a plain push) from a v0.4.26 branch is the trigger path. Push-only freeze branches do not fire it.

---

## 5. baseharbor-node-connector · ci.yml (Connector Quality)

**File:** `baseharbor-node-connector/.github/workflows/ci.yml`

| Location | Actual filter expression |
|----------|--------------------------|
| L4-22 (`on`) | `workflow_dispatch`; `pull_request.branches: [main]` with `paths` filter (`**/*.go`, `go.mod`, `go.sum`, `README.md`, `SECURITY.md`); additional `push.branches: [main]` (L8-9) with same paths (L18-21) |

**Would a `v0.4.26` freeze branch trigger it?**
- **PUSH to non-`main`: NO** — push target must be `main` + path match.
- **PULL_REQUEST: YES if head→main and a `.go` path changed** — a PR from a v0.4.26 work branch into `main` that touches Go files fires the connector quality CI. This is the general quality gate, not the release-integration gate.

---

## 6. baseharbor · targeted-integration.yml

**File:** `baseharbor/.github/workflows/targeted-integration.yml`

| Location | Actual filter expression |
|----------|--------------------------|
| L5-10 (`on`) | `push.branches`: `runtime-validation/ha/**`, `runtime-validation/recovery-podman/**`, `work/v0.4.24-core-ha-updates`, `work/v0.4.25-contract-minimal-core`; `workflow_dispatch` with `gate` input |

**Would a `v0.4.26` freeze branch trigger it?**
- **PUSH to `work/v0.4.26-core-ha-updates`: NO** — no v0.4.26 HA-update line exists (only v0.4.24 / v0.4.25 listed). A v0.4.26 HA branch would need adding.
- **PUSH to `runtime-validation/ha/**`: YES** — covered by the glob.
- **PULL_REQUEST: NO** — push/dispatch only.
- **Remote confirmation:** none of the `work/v0.4.2x-core-ha-updates`/contract branches appear on remote `ls-remote` (only `work/core921-*`/`ci-core921-*` heads exist), so these exact lines currently match no live branch; they are draft freeze hooks.

---

## 7. baseharbor · host-trust-owned-e2e.yml

**File:** `baseharbor/.github/workflows/host-trust-owned-e2e.yml`

| Location | Actual filter expression |
|----------|--------------------------|
| (push branches) | `work/v0.4.24-core-ha-updates`, `work/v0.4.25-core-ha-updates` (HA-update style, not `-integrations-docs`) |

**Would a `v0.4.26` freeze branch trigger it?** NO — no v0.4.26 line. Same caveat as targeted-integration: these are draft freeze hooks with no v0.4.26 member.

---

## 8. baseharbor · pre-release.yml / pre-release-runtime-suite.yml / release.yml / ci.yml (tag-driven, not freeze-branch-driven)

These are **tag-triggered** release gates, not push-branch gated. `pre-release.yml` iterates tags `v0.4.24 / v0.4.25 / v0.4.26` inside job `if:` blocks (`tag == 'v0.4.24' || 'v0.4.25' || 'v0.4.26'`) and gates on `needs.source.tag`. They fire on **release/tag events**, not on the freeze branch push. Freeze step 7 does not need to name a branch to reach them — it needs the tag `v0.4.26`, which is a separate authorization (owner stop; not part of this read-only lane).

---

## 9. DELIVER — MUST-NAME TABLE for freeze step 7

This is the operative table. Freeze step 7 must push/PR **exactly these branch names** for the corresponding workflow to fire. Nothing else in this list is optional for coverage.

| # | Repo | Workflow (file:line) | Trigger mode | Freeze branch name(s) that MUST be pushed/PR'd | Push alone fires? | PR from branch fires? |
|---|------|----------------------|--------------|------------------------------------------------|-------------------|-----------------------|
| 1 | connector | `release-integration.yml` L2-7 | push, exact list | **`work/v0.4.26-integrations-docs`** | YES (commit msg must contain `[ecosystem-runtime]`, L366) | N/A (no PR trigger) |
| 2 | connector | `source.yml` L2-9 | push (n/a) + `pull_request` (any) | any v0.4.26 head, e.g. **`work/v0.4.26-integrations-docs`** (PR only; transport/enrollment need `runtime-validation/transport|enrollment/` prefix) | NO branch match | **YES** → `source` job runs |
| 3 | connector | `ci.yml` L4-22 | `pull_request`→main + path | PR **→ `main`** touching `.go` | NO (push must be `main`) | YES if head→main + `.go` path |
| 4 | demo | `v0424-targeted.yml` L2-4 | push, exact list | **`work/v0.4.26-ecosystem-release`** (note `-ecosystem-release` suffix) | YES (commit msg must contain `[demo*runtime]`, L12) | N/A (no PR trigger) |
| 5 | console | `release-integration.yml` L269-272 | push, glob `release-integration/**` | **`release-integration/v0.4.26`** (or any `release-integration/<…>`) | **YES only if the branch is literally `release-integration/<…>`** | N/A (no PR trigger) |
| 6 | baseharbor | `targeted-integration.yml` L5-10 | push | `work/v0.4.26-core-ha-updates` **— does NOT exist yet; must be ADDED** to the L9-10 list | NO today | N/A |
| 7 | baseharbor | `host-trust-owned-e2e.yml` (HA-update lines) | push | `work/v0.4.26-core-ha-updates` — **must be added** | NO today | N/A |
| 8 | baseharbor (all) | `pre-release.yml` / `pre-release-runtime-suite.yml` / `release.yml` | **tag** | tag **`v0.4.26`** (release-authorization, owner-stop gate — outside this lane) | N/A (tag event) | N/A |

### 9a. The exact freeze-branch names freeze step 7 must produce

| Purpose | Exact branch name to push (or PR from) | Fires which workflow |
|---------|----------------------------------------|----------------------|
| Connector release integration (native qualification) | **`work/v0.4.26-integrations-docs`** | connector `release-integration.yml` L5-7 |
| Demo targeted compatibility | **`work/v0.4.26-ecosystem-release`** | demo `v0424-targeted.yml` L4 |
| Console release integration (browser native) | **`release-integration/v0.4.26`** (or `release-integration/v0.4.26/core/<sha>` etc.) | console `release-integration.yml` L271 (`release-integration/**`) |
| Connector source conformance (quality gate) | any v0.4.26 head as a **PR into `main`** | connector `source.yml` L9-10 (`pull_request`) |
| Console quality | any v0.4.26 head as a **PR into `main`** (touching `src|scripts|tests|contracts`) | console `ci.yml` L10-21 (`pull_request`→main) |
| baseharbor targeted integration / host e2e | `work/v0.4.26-core-ha-updates` **must be added** to L9-10 (currently absent) | baseharbor `targeted-integration.yml` / `host-trust-owned-e2e.yml` |
| Full release run (all 65 gates) | tag **`v0.4.26`** (release-authorized, owner-stop) | `pre-release.yml` / `release.yml` (tag trigger) |

### 9b. What will NOT fire (and why)

- Pushing `work/v0426-console-consumption` or `work/v0426-connector-consumption` → **does not fire** the respective `release-integration.yml` files (exact-list membership, not substring). These `work/v0426-*` (no dot) heads exist on remote but are not trigger members.
- Pushing any `work/...` branch to **console** → does **not** fire `release-integration.yml` because its filter is `release-integration/**`, not a `work/...` name.
- Pushing `work/v0.4.26-integrations-docs` to **demo** → does **not** fire `v0424-targeted.yml` (demo keys on `-ecosystem-release`).
- Pushing any v0.4.26 branch to baseharbor **without** the `work/v0.4.26-core-ha-updates` line added → does **not** fire `targeted-integration.yml` / `host-trust-owned-e2e.yml`.
- Opening a **push-only** branch for connector/console **release integration** → neither release-integration.yml has a `pull_request` or `workflow_dispatch` trigger; only a matching **push to the exact branch** fires it.

---

## 10. VERIFICATION (read-only)

| Check | Command | Result |
|-------|---------|--------|
| Connector release-integration push branches | read `baseharbor-node-connector/.github/workflows/release-integration.yml` L5-7 | `work/v0.4.24/25/26-integrations-docs` exact list ✅ |
| Connector release-integration has NO pull_request | grep `pull_request|workflow_dispatch` in same file | absent ✅ (push-only) |
| Connector release-integration v0.4.26 head exists | `git ls-remote --heads origin 'work/v0.4.26-integrations-docs'` (connector) | `8e6bb897f7dcb9ee77f0c40a4600980439e2eac6` ✅ |
| Console release-integration push branches | read `baseharbor-console/.github/workflows/release-integration.yml` L269-272 | `release-integration/**` ✅ |
| Console release-integration has NO pull_request/dispatch | grep `pull_request|workflow_dispatch` | absent ✅ (push-only, glob) |
| Console release-integration v0.4.25 core heads exist | `git ls-remote --heads origin 'release-integration/v0.4.25/*'` (console) | multiple `release-integration/v0.4.25/core/*` ✅ (proves glob fires) |
| Demo targeted push branches | read `baseharbor-demo/.github/workflows/v0424-targeted.yml` L4 | `[work/v0.4.24/25/26-ecosystem-release]` ✅ |
| Demo v0.4.26 ecosystem-release head exists | `git ls-remote --heads origin 'work/v0.4.26-ecosystem-release'` | `a0d0be813ba71b554404daa52ba5015861a245b8` ✅ |
| Connector source.yml push + PR branches | read `baseharbor-node-connector/.github/workflows/source.yml` L2-9 | push exact/`runtime-validation/**` + `pull_request` (any) ✅ |
| Connector ci.yml push/PR targets | read `baseharbor-node-connector/.github/workflows/ci.yml` L4-22 | push→`main` + paths; PR→`main` + paths ✅ |
| Console ci.yml push/PR targets | read `baseharbor-console/.github/workflows/ci.yml` L5-21 | push→`runtime-validation/browser/**`; PR→`main` + paths ✅ |
| baseharbor targeted-integration push branches | read `baseharbor/.github/workflows/targeted-integration.yml` L5-10 | `work/v0.4.24-core-ha-updates`, `work/v0.4.25-contract-minimal-core` **only — no v0.4.26** ⚠️ |
| baseharbor host-trust-owned-e2e push branches | read `baseharbor/.github/workflows/host-trust-owned-e2e.yml` | `work/v0.4.24-core-ha-updates` + `work/v0.4.25-core-ha-updates` — **no v0.4.26** ⚠️ |

All four repos verified as local clones with `origin` → `mcpdev80/*`. No writes, no pushes, no PRs.

---

## 11. RECONCILIATION WITH q17 / C3

- **C3 stands, reinforced.** q16's claimed `release-integration/v0.4.24-integrations-docs` path-prefix filter (q16 L30-31) does not exist in either `release-integration.yml`. The connector file uses exact branch names (L5-7); the **console** file uses a branch-name glob `release-integration/**` (L271). Both are push-only and neither matches a plain `work/...` v0.4.26 freeze push the way q16 imagined.
- **New operational implication q17 did not capture:** the **Console** release-integration workflow can only fire from a `release-integration/**` push (9a #3), so freeze step 7 must create/push a console branch literally named `release-integration/<…>` — a different convention than the connector (`work/v0.4.26-integrations-docs`) and demo (`work/v0.4.26-ecosystem-release`) workflows. Three repos, **three distinct** freeze-branch naming conventions.
- baseharbor `targeted-integration.yml` / `host-trust-owned-e2e.yml` lack a v0.4.26 line entirely — an **additive gap** (11a), not a rename.

### 11a. Open follow-ups (out of scope for this read-only lane; document for freeze step 7 owner)

1. Console `release-integration.yml` fires only on `release-integration/**` push. Decide the final console freeze-branch name (recommend `release-integration/v0.4.26` or `release-integration/v0.4.26/core/<sha>`, matching existing `release-integration/v0.4.25/core/*` heads) and push it. **Do not** rely on a `work/v0426-console-consumption` push.
2. baseharbor `targeted-integration.yml` L9-10 and `host-trust-owned-e2e.yml` need a `work/v0.4.26-core-ha-updates` member added before a v0.4.26 HA-update push fires them.
3. connector `source.yml` and console `ci.yml` quality gates are reached via **PR → `main`**, not freeze-branch push — ensure the v0.4.26 consumer PRs (#7 console, #8 connector) are opened from the frozen heads.

---

## 12. OVERALL VERDICT

**O34-GREEN-WITH-3-CONVENTIONS-AND-2-ADDITIVE-GAPS.**

Trigger filters audited live in all four repos. The connector `release-integration.yml` (the C3 file) uses exact branch-name push filters at L5-7 — all three v0.4.24/25/26 members already present; q16's `release-integration/v0.4.24-integrations-docs` path-prefix model is wrong (C3 confirmed). The **console** `release-integration.yml` uses a branch-name glob `release-integration/**` (L271), push-only, and will **not** fire from a `work/...` freeze push — freeze step 7 must push a branch literally named `release-integration/<…>`. The **demo** workflow keys on the `-ecosystem-release` suffix, and baseharbor HA-update hooks lack a v0.4.26 line (additive gap). Quality gates (connector `source.yml`, console `ci.yml`) are reached by **PR into `main`**, not freeze-branch push.

The **must-name table (section 9/9a)** gives freeze step 7 the exact branch names per repo.

---

**END O34-TRIGGERS-DONE.**

Read-only lane. Evidence recorded 2026-10-22 UTC. No writes to any product branch, no pushes, no PRs.

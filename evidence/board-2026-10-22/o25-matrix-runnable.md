# O25 — RUNNABLE matrix dispatch attempt (partial execution, BLOCKED at Source)

Lane: o25 RUNNABLE dispatch. WRITE-FIRST: this PLAN + native todowrite preceded all actions.
Inputs: `o24-matrix-gonogo.md` (38 RUNNABLE rows), `o21-matrix-inputs.md` (dispatch vehicle + inputs),
`f7-native-dispatch.md` (queue/concurrency recipe). **No product files touched; no merges.**

## PLAN (todowrite mirror)

- [x] 1. Turn-start basics-check + FLAGS + ack 3 events (per AGENTS.md turn-start Pflichtprogramm)
- [x] 2. Verify pins C/D present and classify RUNNABLE rows (o24)
- [x] 3. Inspect `pre-release.yml` dispatch inputs + concurrency (F-1) at C
- [x] 4. Dispatch CMD-A (RUNNABLE runtime+HA+static+integration) via `pre-release.yml` workflow_dispatch
- [x] 5. Poll run to terminal, collect run id + first-error lines
- [ ] 6. Diagnose Source-job failure → root-cause blocker (immutable SHA / consumer-pins)
- [ ] 7. Evaluate alternative vehicles (release-candidate push, CMD-C adoption) — all blocked
- [ ] 8. ## repo-rule compliance + END O25-RUNNABLE-DONE

## 0) Pins (verified present, current HEAD)

- C = `2adae47afddc299d06dbc34eb16a8e011c14b6dc` (baseharbor `develop`) — **present, current HEAD**.
- D = `896dc3cb68a8c9b9585940c4696410de5a338c52` (baseharbor-demo `main`, `baseharbor-core.ref` == C) — **present, current HEAD**.
- Connector repo `mcpdev80/baseharbor-connector` → **DELETED/404**; live repo is `mcpdev80/baseharbor-node-connector`
  (default branch `main`, HEAD `8726b714…`), whose `integration-candidate.json` pins **release 0.4.24 / core `99983841…`**
  (NOT v0.4.26 / C).
- gh authenticated as `mcpdev80` (ssh); `gh 2.46.0`.

## 1) RUNNABLE rows taken from o24 (38 rows)

Runtime (31, CMD-A via `pre-release.yml`): atomic static×4 (config-matrix, init, agent, shell-ux),
docker-light×6 (lifecycle, policy, connectivity, reconciliation, failure, full-destroy), docker-heavy×9
(observability, identity, security, rabbitmq-runtime, mongodb-runtime, durable-key-value,
control-plane-recovery, loki-runtime, tempo-runtime), podman-light×6, podman-heavy×9; HA×8
(ha/docker/{data,identity,observability,routing}, ha/podman/{data,identity,observability,routing});
integration static×4 (public-contracts, configuration-policy, browser-terminal, live-console),
integration docker×3 (remote-target, core-bootstrap, coreless-lifecycle), integration podman×3
(remote-target, core-bootstrap, coreless-lifecycle).

Adoption (4, CMD-C via `targeted-integration.yml`): adoption/conformance, realworld-compose,
realworld-quadlet, realworld-kubernetes.

## 2) CMD-A dispatch (attempt)

Vehicle: `pre-release.yml` `workflow_dispatch`, `ref=develop`, `inputs[tag]=v0.4.26`,
`inputs[ref]={C}`, `inputs[demo_ref]={D}`, `inputs[consumer_ref]=570864c7`,
`inputs[only_gates]` = 46 RUNNABLE IDs (o21 §3 canonical string + HA rows).

Run **39853157648** (`Pre-Release · v0.4.26`, `workflow_dispatch`, run #213, head `2adae47a`=C):

- Concurrency group `baseharbor-release-qualification` (F-1, `cancel-in-progress:false`) empty before dispatch.
- Jobs: 4 FAILED, 22 SKIPPED (cascade). Failures: `Source · Contracts · Docs` (120084380210),
  `v0421 ha groups` (120084485467), `runtime heavy gates` (120084485730), `runtime light gates` (120084485741).
- Runtime jobs fail on `SOURCE_RESULT: failure` (gate on Source job), not on gate logic.

First error line (verbatim, Source job, UTC):
`2026-10-22T20:56:50.6809596Z fatal: couldn't find remote ref refs/heads/release/v0.4.26`
`2026-10-22T20:56:50.8132476Z ValueError: immutable source commit required`
`2026-10-22T20:56:50.8279420Z ##[error]Process completed with exit code 1.`

## 3) Root cause (two independent hard blockers, both in the `release` job)

`pre-release.yml` `ref` is a **branch ref** (`checkout`-able), NOT a commit SHA. The `release` job
(`pre-release.yml:90-165`) requires workflow HEAD on one of: `origin/release/v0.4.26` tip,
`origin/develop`-mainline, or `origin/main`-mainline. At C none holds:

1. **No `release/v0.4.26` branch** (remote branch list empty of any `release/…`; `fatal: couldn't find
   remote ref refs/heads/release/v0.4.26`). `ref=develop` is a moving branch, so `git rev-parse HEAD`
   ≠ an immutable SHA → `ValueError: immutable source commit required`.
2. **No `release-candidate/v0.4.26` fallback** (the `release-candidate/**` push trigger also requires
   immutable HEAD = C tip; and even the `validation/pre-release/selected/gates/*` push path runs the
   same `release` origin check, so equally blocked).
3. **Consumer-pins graph can't resolve for v0.4.26**: `ecosystem_release_pins.py` (`scripts/release`
   job) requires an immutable connector ref pinned to **release 0.4.26 / core=C**. The connector's
   `integration-candidate.json` pins **release 0.4.24 / core `99983841…`** (older candidate), and the
   `mcpdev80/baseharbor-connector` repo referenced by the pin resolver is **404/deleted**. So even a
   release-candidate branch at C cannot produce a v0.4.26-immutable connector pin → `resolve_pins`
   (via `validate_ecosystem_graph`) rejects the graph.

Both blockers live in the `release` job, so CMD-A cannot reach any gate execution.

## 4) Alternative vehicles evaluated

- **`release-candidate/v0.4.26` push branch (C = tip):** would satisfy the `release-candidate/**`
  trigger and make HEAD = C immutable, BUT (a) it does not exist yet and (b) the consumer-pins graph
  still can't resolve (connector pinned to 0.4.24, repo 404). Blocked.
- **`validation/pre-release/selected/gates/*` push branch:** same `release` origin check → blocked.
- **CMD-C `targeted-integration.yml` (adoption×4):** `ref` accepts a commit SHA, but the workflow only
  accepts adoption gates (choices: coreless-runtime, app-backup-restore, … adoption-*); it does NOT
  cover the 31 runtime/HA gates. Standalone adoption run would still hit the `release` job's immutable
  origin check. Blocked for the RUNNABLE set.

**Conclusion:** all RUNNABLE dispatches are blocked at the `release` job by the immutable-SHA /
no-`release/v0.4.26`-branch and v0.4.26-connector-pin-delete conditions. Zero gates executed.

## 5) Results table (this attempt)

| Gate set | Vehicle | Run id | Head | Conclusion | First error line (UTC) |
|---|---|---|---|---|---|
| CMD-A 46 RUNNABLE (static/docker/podman/integration+HA) | `pre-release.yml` dispatch | [39853157648](https://github.com/mcpdev80/baseharbor/actions/runs/39853157648) | `2adae47a` (C) | FAILURE (Source+runtime+HA; 22 skipped) | `2026-10-22T20:56:50.6809596Z fatal: couldn't find remote ref refs/heads/release/v0.4.26` → `ValueError: immutable source commit required` |
| CMD-C 4 adoption (not run) | `targeted-integration.yml` | — | — | not dispatched (blocked same `release` job) | — |

## 6) Repo-rule compliance

READ-WRITE-FIRST evidence lane: only files written = this evidence file + todowrite PLAN; no product
files edited, no merges, no tags, no force-push, no branch creation attempted (no `git push` to any
release-candidate/release/main). Dispatch issued exclusively via the documented `workflow_dispatch` on
trusted `pre-release.yml` at `{C}` with inputs per o21 §3; run/conclusion/error lines captured verbatim
with run/attempt/head-SHA identity. Queue (F-1) inspected before dispatch (empty). No secrets logged
(`consumer_ref` placeholder `570864c7` is an unverified connector head, not a pin). Blocked state is
reported truthfully; no green gate asserted.

O25-RUNNABLE-DONE

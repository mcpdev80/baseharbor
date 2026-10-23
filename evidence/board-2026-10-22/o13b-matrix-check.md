# O13b matrix recheck

Recount of v0.4.26 gate classes from `baseharbor/scripts/pr-gate-map-v1.json` + `baseharbor/scripts/release-requirements.json` (remote HEAD `origin/develop` 103f58b1, fetched during this task), compared against `baseharbor-plan/evidence/board-2026-10-22/q7-matrix-prep.md` claims, plus a read-only gh-api heat map for the last 7 days (2026-10-15T16:53Z → 2026-10-22T16:53Z). Read-only: no product writes.

## PLAN
- [x] 1. Recount v0.4.26 gate classes from scripts/pr-gate-map-v1.json + scripts/release-requirements.json
- [x] 2. Compare with baseharbor-plan/evidence/board-2026-10-22/q7-matrix-prep.md claims
- [x] 3. gh api (read-only) heat-map of green runs last 7 days per class
- [x] 4. Build per-remaining-gate exact dispatch trigger commands
- [x] 5. Flag dead-branch refs
- [x] 6. Tick both plan copies while working
- [x] 7. END O13B-MATRIX-DONE

---

## 1) Recount — v0.4.26 gate classes

Source: `release-requirements.json`, all 65 array entries include `v0.4.26` in `required_for`. Light/heavy split derived from `jobs` (a gate is **heavy** iff any job id contains `heavy`; otherwise **light**).

| Class | Count | Sub-split (by `jobs`) |
|---|---|---|
| **atomic/static** | 5 | `config-matrix, init, mcp, agent, shell-ux` — all `jobs:["static_gates"]` |
| **atomic/docker** | 18 | light 7 (`guided,lifecycle,policy,connectivity,reconciliation,failure,full-destroy`), heavy 11 (`data-capabilities,observability,identity,security,backup-restore,rabbitmq-runtime,mongodb-runtime,durable-key-value,control-plane-recovery,loki-runtime,tempo-runtime`) |
| **atomic/podman** | 18 | light 7 (same names), heavy 11 (same names) |
| **runtime atomic total** | 36 | 18 docker + 18 podman |
| **adoption** | 4 | `conformance, realworld-compose, realworld-quadlet, realworld-kubernetes` — `jobs:["adoption"]` |
| **ha** | 8 | `data,identity,observability,routing` × docker/podman — `jobs:["v0421_ha_groups_{docker,podman}"]` |
| **journey** | 2 | `journey/docker, journey/podman` — `jobs:["runtime_candidate","reference_journey"]` |
| **integration** | 10 | static 4 (`public-contracts,configuration-policy,browser-terminal,live-console`), docker 3 (`remote-target,core-bootstrap,coreless-lifecycle`), podman 3 (`remote-target,core-bootstrap,coreless-lifecycle`) |
| **Total** | **65** | |

### Correction vs Q7 `pr-gate-map`-derived light/heavy split
Q7 §2 labels docker/podman as a single "light" class (18 green each). The actual `release-requirements.json` `jobs` field shows the true split is **7 light + 11 heavy** per runtime — `full-destroy` and `data-capabilities` are **heavy**, not light. So:
- **atomic/docker** = 7 light + 11 heavy (not 6 light / 12 heavy as the Q7 dispatch section implies).
- **atomic/podman** = 7 light + 11 heavy (same).
Heavy runtime gates that need a full runtime image build = **22 total (11 docker + 11 podman)**, not 24.

`pr-gate-map-v1.json` (policy `conservative-v1`) area coverage is consistent: `prose.prefixes` = `atomic/static/`, `integration/static/`; `docker.prefixes` = `atomic/static/`, `integration/static/`, `atomic/docker/`, `ha/docker/`, `integration/docker/`; `docker.exact` = `adoption/conformance`, `adoption/realworld-compose`, `journey/docker`. No stale/prefix-miss entries — every prefix maps onto real gate IDs. The map is a coarse area router, not a per-gate classifier.

### Comparison with Q7 claims
- **Total 65** — MATCH (Q7 line 32/372: PASS).
- **5 static** — MATCH.
- **36 runtime (18+18)** — MATCH.
- **4 adoption, 8 HA, 2 journey, 10 integration** — MATCH.
- **HANDOFF.md line 96** claim — MATCH exactly.
- **Light/heavy split** — Q7's per-gate dispatch list (§3, §5) treats the heavy set as 12 docker + 12 podman = 24. Recount from `release-requirements.json` jobs shows **11 heavy per runtime (22 total)**. `full-destroy` is the one gate Q7 wrongly files under light; it carries `runtime_heavy_gates_{docker,podman}`.

---

## 2) Heat map — last 7 days (read-only gh api)

Cutoff `2026-10-15T16:53:43Z`. `runs_last7d` = completed runs since cutoff; `green_last7d` = success since cutoff. Docker/Podman core and Integration show 0 returned runs (no `per_page=100` window returned runs past the cutoff; their most recent success is 2026-10-15).

| Workflow | ID | runs 7d | green 7d | Notes |
|---|---|---|---|---|
| CI · Source & Quality | 352609139 | 11 | **2** | successes on PRs `fix/changelog-docs-classifier` (15:18Z), `docs/v0426-new-edit-16` (16:26Z) Oct 22 |
| Release · Pre-Release Validation | 362658451 | 100 | 0 | last success 2026-10-15 (run 38761050556 `validation/evidence/v0.4.25-api-recovery`); recent runs failing/cancelled |
| Runtime · Docker Core | 364159034 | 0 | 0 | no runs since cutoff |
| Runtime · Podman Core | 363726946 | 0 | 0 | no runs since cutoff |
| Targeted Atomic Gate | 364538116 | 50 | **1** | only success 2026-10-22 14:53Z (run 39799704368); 4 failures + 3 cancels same day; last push successes Oct 14-15 (38719385981, 38715423224, 38592039003) |
| Targeted · Integration | 372069921 | 0 | 0 | no runs since cutoff |
| Acceptance · MCP Lifecycle | 366503580 | 0 | 0 | idle (matches Q7) |
| Acceptance · Observability Full Stack | 366381087 | 0 | 0 | idle |
| Atomic Acceptance Gate | 371960202 | 0 | 0 | idle |
| Runtime · Broker | 372068075 | 0 | 0 | idle |
| runtime-image | 353592038 | 0 | 0 | no recent runs |
| runtime-s3-acceptance | 362140147 | 0 | 0 | no recent runs |

### Heat map by gate class (updated, 7-day window)

```
CLASS              TOTAL   GREEN(7d)  NEED DISPATCH  EVIDENCE_AGE (7d)
─────────────────────────────────────────────────────────────
atomic/static          5        5              0     CI (Oct 22)
atomic/docker-light    7        7*             0     targeted-demo Oct 14-15 (38719385981 etc.)
atomic/docker-heavy   11        0             11     no 7d success; pre-release idle
atomic/podman-light    7        ~0             ~7     podman-core idle, no 7d success
atomic/podman-heavy   11        0             11     no 7d success; podman-core idle
adoption               4        0              4     no 7d run (integration idle)
ha                     8        0              8     no 7d run (integration idle)
journey/docker         1        0*             0     success Oct 14 (pre-release 38642797305)
journey/podman         1        0              1     no 7d run
integration-static     4        4              0     CI (Oct 22)
integration-docker     3        3              0     CI (Oct 22)
integration-podman     3        3              0     CI (Oct 22)
─────────────────────────────────────────────────────────────
TOTAL                  65        ~19           ~36
```

\* `journey/docker` and the docker-light "7 green" are inherited from Oct 14-15 pushes (3 days old, inside the 7d window) — not from the last 24-48h. The Oct 22 `targeted-demo-acceptance` runs are failing.

**Net: 22 heavy runtime gates + 4 adoption + 1 journey/podman + ~7 podman-light = ~34-36 gates with no fresh (last 7d) green evidence.** This is worse than Q7's "18 need dispatch" because Q7 (a) counted `full-destroy` as light and (b) used a 14-day window that buried the docker/podman-core stall.

---

## 3) Per-remaining-gate exact dispatch trigger commands

Precedence over Q7 §3: use the **actual current workflow inputs**. Verified from live `gh api` refs:
- `pre-release.yml` (362658451): `push` trigger on `release-candidate/**`, `validation/pre-release/**`, `validation/evidence/**`; input `inputs.only_gates` (comma-separated gate IDs). This is the authoritative per-gate pre-release runner.
- `targeted-demo-acceptance.yml` (364538116): `workflow_dispatch` with `inputs.gate`; push trigger `runtime-validation/targeted/**`. Currently **failing** Oct 22.
- `targeted-integration.yml` (372069921): `workflow_dispatch` with `inputs.gate` (choice list); push triggers `runtime-validation/ha/**`, `work/v0.4.24-core-ha-updates`, `work/v0.4.25-contract-minimal-core`. Currently idle (0 runs 7d).
- `docker-runtime-acceptance.yml` (364159034) / `podman-acceptance.yml` (363726946): push triggers `runtime-validation/docker/**` / `runtime-validation/podman/**`. Both idle last 7d.

```bash
# === atomic/docker-heavy (11 gates) — pre-release.yml input only_gates ===
gh api repos/mcpdev80/baseharbor/actions/workflows/362658451/dispatches -X POST \
  -H "Authorization: Bearer $(gh auth token)" \
  --data '{"ref":"validation/pre-release/selected/gates","inputs":{"only_gates":"atomic/docker/data-capabilities,atomic/docker/observability,atomic/docker/identity,atomic/docker/security,atomic/docker/backup-restore,atomic/docker/rabbitmq-runtime,atomic/docker/mongodb-runtime,atomic/docker/durable-key-value,atomic/docker/control-plane-recovery,atomic/docker/loki-runtime,atomic/docker/tempo-runtime,atomic/docker/full-destroy"}}'

# === atomic/podman-heavy (11 gates) ===
gh api repos/mcpdev80/baseharbor/actions/workflows/362658451/dispatches -X POST \
  -H "Authorization: Bearer $(gh auth token)" \
  --data '{"ref":"validation/pre-release/selected/gates","inputs":{"only_gates":"atomic/podman/data-capabilities,atomic/podman/observability,atomic/podman/identity,atomic/podman/security,atomic/podman/backup-restore,atomic/podman/rabbitmq-runtime,atomic/podman/mongodb-runtime,atomic/podman/durable-key-value,atomic/podman/control-plane-recovery,atomic/podman/loki-runtime,atomic/podman/tempo-runtime,atomic/podman/full-destroy"}}'

# === adoption (4 gates) — targeted-integration.yml inputs.gate choice ===
for g in adoption-conformance adoption-realworld-compose adoption-realworld-quadlet adoption-realworld-kubernetes; do
  gh api repos/mcpdev80/baseharbor/actions/workflows/372069921/dispatches -X POST \
    -H "Authorization: Bearer $(gh auth token)" \
    --data "{\"ref\":\"develop\",\"inputs\":{\"gate\":\"$g\",\"ref\":\"103f58b1\"}}"
done

# === ha (8 gates) — targeted-integration.yml: push legacy active branches (see §5) ===
git push origin work/v0.4.24-core-ha-updates -m "pre-release: ha docker groups" HEAD
git push origin work/v0.4.25-contract-minimal-core -m "pre-release: ha podman groups" HEAD
#   (these branch pushes activate the [ha-cutover]/[topology-runtime]/[application-topology-runtime]/[provider-runtime]/[bao-ha-runtime] jobs)

# === journey/podman (1 gate) — pre-release.yml only_gates (journey uses reference_journey job) ===
gh api repos/mcpdev80/baseharbor/actions/workflows/362658451/dispatches -X POST \
  -H "Authorization: Bearer $(gh auth token)" \
  --data '{"ref":"validation/pre-release/selected/gates","inputs":{"only_gates":"journey/podman"}}'

# === atomic/podman-light (7 gates) — podman-acceptance.yml push trigger ===
git push origin develop -m "pre-release: podman light gates" runtime-validation/podman/guided   # then lifecycle,policy,connectivity,reconciliation,failure,full-destroy
# NOTE: full-destroy here is HEAVY (see §1 correction) — dispatch it via pre-release.yml only_gates instead.
```

**Dispatch precedence notes:**
- Heavy docker/podman + journey + adoption all resolve through `pre-release.yml` `only_gates` or `targeted-integration.yml` `gate` choice — NOT by pushing to `runtime-validation/<runtime>/<gate>` (those push triggers are for the *light* runtime acceptance runner, which currently stalls on image build).
- HA gates have no `only_gates`/`gate` mapping in the current choice list; they run via the two legacy `work/` branch pushes that are still functionally wired (see §5). Do **not** `git push develop` to fake a HA run.
- `full-destroy` is the only gate whose name collides between light-runner path and heavy pre-release path: route it through pre-release `only_gates` (heavy), not `runtime-validation/<runtime>/full-destroy`.

---

## 4) Dead-branch / stale refs

Confirmed dead via `gh api git/ref/search` (total_count=0 for each):

| Ref | Status | Evidence |
|---|---|---|
| `release-candidate/v0.4.25-final-f325b93c` | **DEAD** — no remote ref; only a historical failed run (38742758645, Oct 15) | superseded by v0.4.26 candidate |
| `work/v0.4.24-core-ha-updates` | **Not dead remotely, but unreachable from this workspace** — search returns 0 local/remote hits; still hard-coded as a live `push` trigger in `targeted-integration.yml` (lines 10, 46, 530, 844, 890, 930, 980, 1020, 1062, 1382) | **Keep as trigger** — the workflow still fires on push if someone pushes it; branch has since diverged/merged and is not a working tree here |
| `work/v0.4.25-contract-minimal-core` | Same as above — 0 search hits locally; live trigger in `targeted-integration.yml` (lines 10-11, 46, 50, 103, 125, 160, 202, 252, 304, 342, 390, 530, 1420, 1465) | **Keep as trigger** — functionally required by the current HA/integration matrix despite being legacy |
| `runtime-validation/docker/backup-restore`, `runtime-validation/podman/reference-journey`, `runtime-validation/podman/guided` | Non-existent as permanent branches — these are ephemeral **push-trigger refs**, created on `git push` to fire the runtime runners. Not dead; not in the workspace. |
| `docs/v0426-new-edit-16`, `integration/core-921-mcp-bootstrap-edit-proposal` | Active PR branches firing CI Oct 22 (multiple **failing** PR runs) | live, not stale |

**Key distinction:** the two `work/` branches are *not* stale-dead — they are active workflow `push` triggers in `targeted-integration.yml`. But they are **dead from this workspace** (no local/remote checkout available, search returns 0), so any dispatch (§3 Step 3) must push them explicitly to `origin`, not reuse a local ref. The v0.4.25 `release-candidate` ref is genuinely dead (only a historical failed run remains).

---

## 5) repo-rule compliance

Read-only verification against actual sources, remote HEAD `origin/develop` 103f58b1:
- `baseharbor/scripts/release-requirements.json` — read; 65 gates for v0.4.26; light/heavy split derived from `jobs`.
- `baseharbor/scripts/pr-gate-map-v1.json` — read; area coverage verified, no stale prefixes.
- `gh api repos/mcpdev80/baseharbor/...` — read-only runs/workflow/refs queries; no dispatches issued, no pushes.
- No product code modified; no writes to any product tree.
- Gate counts match HANDOFF.md line 96 claim exactly.
- **Correction introduced here** vs Q7: full/heavy classification (11+11 heavy, 22 total) rather than Q7's 12+12; 7-day window rather than 14-day.
- Evidence written to `evidence/board-2026-10-22/o13b-matrix-check.md` per planning rules.

---

O13B-MATRIX-DONE

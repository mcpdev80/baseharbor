# O21 — FINAL 65-GATE DISPATCH INPUT TABLE (post-freeze, READ-ONLY)

Lane: o21 READ-ONLY matrix-inputs. Sole write = this file. Task O21 directive (successor to held f8).
Sources: `o13b-matrix-check.md`, `o17-qualify-dossier.md`, cross-read `f7-native-dispatch.md` +
`f5-demo-gates.md` + `f8-matrix-inputs.md`. **All workflow / requirements facts re-verified at C via
`git show 2adae47a:…`** (local objects confirmed; `origin/develop` == C exactly). File reads use line
numbers of the blobs at C. Local checkout branch (`fix/changelog-docs-classifier`) not modified; single
read-only `fetch origin develop`; no product writes, no dispatches issued.

## PLAN (todowrite mirror)
- [x] 1. Turn-start basics-check + FLAGS (2 events acked via events-check.py --ack)
- [x] 2. Read o13b + o17 + f7 (+ f5/f8 as needed)
- [x] 3. Verify release-requirements.json + all workflow jobs/concurrency/inputs at C (git show)
- [x] 4. Derive authoritative 65-gate enumeration + route (job file:line) + runner + durations + parallel/serial
- [x] 5. Cross-mark f7 landed/in-flight runs on the 65 rows (mcp GREEN, guided/data/backup/journey in-flight)
- [x] 6. Emit corrected final sheet (fixes f8 CMD-A: 44 invalid bare-name gates → valid 56-gate list;
   full-destroy = LIGHT; drops f7 RED/in-flight gates from CMD-A; adds mcp + live-console)
- [x] 7. ## repo-rule compliance (line 192) + END O21-MATRIX-INPUTS-DONE (line 198)

## 0) Pins (verified at C)
- C = `2adae47afddc299d06dbc34eb16a8e011c14b6dc` (baseharbor `develop`) — `{C}`, workflow-tree blob at C = `78c31a7866e0e9ce596cdf336126a53372831cf1` (matches f7 recipe-cited reviewed blob).
- D = `896dc3cb68a8c9b9585940c4696410de5a338c52` (baseharbor-demo `main`, `baseharbor-core.ref` == C) — `{D}`.
- Console U = PR #7 head `0ea88f55…` (open/DRAFT) — `{U}`.
- Connector N = PR #8 head `570864c7…` (open/DRAFT) — `{N}`.
- Rekey rule: any U/N merge or guided-fix changes inputs/C → re-derive before executing §3.

## 1) Corrections vs f8 / o13b (verified at C)
1. **full-destroy is LIGHT at C**: `release-requirements.json` jobs = `runtime_candidate|runtime_light_gates_docker/podman`. Heavy = 11/runtime; light = 7/runtime (incl. full-destroy). f8 §1 correction is RIGHT; o13b §3 heavy name-list (which put full-destroy among the 11 heavy) is WRONG. Heavy set per runtime = data-capabilities, observability, identity, security, backup-restore, rabbitmq-runtime, mongodb-runtime, durable-key-value, control-plane-recovery, loki-runtime, tempo-runtime (excl. full-destroy).
2. **`pre-release.yml` dispatch payload is fixed** (`:8-26`): requires `tag` + `ref` + `demo_ref`; `consumer_ref` optional (`:21`); `only_gates` optional (`:26`). f8's CMD-A already uses this shape; o13b §3's `only_gates`-only payload was invalid and is superseded.
3. **HA is native to `pre-release.yml`** (`v0421_ha_groups_docker` `:645`, `:740`), NOT via legacy `work/`-branch pushes (f7-applied review rule "no legacy HA branches"). o13b §3 HA push route is dead.
4. **Heat-map greens ≠ C receipts**: all ~19 "green" rows predate C (C pushed 17:56Z, 7d window closed 16:53Z). Only f7's ref=C/demo=D runs count as landed C evidence (mcp GREEN; guided/data/backup/journey terminal in-flight).

## 2) Canonical 65-gate enumeration + ROUTE (release-requirements.json @ C, `required_for` contains v0.4.26)

Verification: `git show 2adae47a:scripts/release-requirements.json` → 65 gates, required_for includes v0.4.26.
Heavy iff any job id contains `heavy` → **22 heavy (11 docker + 11 podman), 43 light**. `full-destroy` ∈ light.

| # | Gate (id) | jobs | Route / pre-release.yml job (file:line) | Runner (suite `:34`) | Duration (cap→est) |
|---|---|---|---|---|---|
| 1 | atomic/static/config-matrix | static_gates | static_gates `:520` | ubuntu-latest | ~5m (cap 8m `:201`) |
| 2 | atomic/static/init | static_gates | static_gates `:520` | ubuntu-latest | ~5m |
| 3 | atomic/static/mcp | static_gates | static_gates `:520` | ubuntu-latest | ~5m |
| 4 | atomic/static/agent | static_gates | static_gates `:520` | ubuntu-latest | ~5m |
| 5 | atomic/static/shell-ux | static_gates | static_gates `:520` | ubuntu-latest | ~5m |
| 6 | atomic/docker/guided | rt_cand \| light_docker | runtime_light_gates_docker `:537` | ubuntu-latest | 10-25m |
| 7 | atomic/docker/lifecycle | rt_cand \| light_docker | runtime_light_gates_docker `:537` | ubuntu-latest | 10-25m |
| 8 | atomic/docker/policy | rt_cand \| light_docker | runtime_light_gates_docker `:537` | ubuntu-latest | 10-25m |
| 9 | atomic/docker/connectivity | rt_cand \| light_docker | runtime_light_gates_docker `:537` | ubuntu-latest | 10-25m |
| 10 | atomic/docker/reconciliation | rt_cand \| light_docker | runtime_light_gates_docker `:537` | ubuntu-latest | 10-25m |
| 11 | atomic/docker/failure | rt_cand \| light_docker | runtime_light_gates_docker `:537` | ubuntu-latest | 10-25m |
| 12 | atomic/docker/full-destroy | rt_cand \| light_docker | runtime_light_gates_docker `:537` | ubuntu-latest | 10-25m |
| 13 | atomic/docker/data-capabilities | rt_cand \| heavy_docker | runtime_heavy_gates_docker `:591` | ubuntu-latest | 20-40m |
| 14 | atomic/docker/observability | rt_cand \| heavy_docker | runtime_heavy_gates_docker `:591` | ubuntu-latest | 20-40m |
| 15 | atomic/docker/identity | rt_cand \| heavy_docker | runtime_heavy_gates_docker `:591` | ubuntu-latest | 20-40m |
| 16 | atomic/docker/security | rt_cand \| heavy_docker | runtime_heavy_gates_docker `:591` | ubuntu-latest | 12-20m (sec cap `:201`) |
| 17 | atomic/docker/backup-restore | rt_cand \| heavy_docker | runtime_heavy_gates_docker `:591` | ubuntu-latest | 40-60m (backup cap 40m) |
| 18 | atomic/docker/rabbitmq-runtime | rt_cand \| heavy_docker | runtime_heavy_gates_docker `:591` | ubuntu-latest | 20-40m |
| 19 | atomic/docker/mongodb-runtime | rt_cand \| heavy_docker | runtime_heavy_gates_docker `:591` | ubuntu-latest | 20-40m |
| 20 | atomic/docker/durable-key-value | rt_cand \| heavy_docker | runtime_heavy_gates_docker `:591` | ubuntu-latest | 20-40m |
| 21 | atomic/docker/control-plane-recovery | rt_cand \| heavy_docker | runtime_heavy_gates_docker `:591` | ubuntu-latest | 20-40m (recovery cap 20m) |
| 22 | atomic/docker/loki-runtime | rt_cand \| heavy_docker | runtime_heavy_gates_docker `:591` | ubuntu-latest | 20-40m |
| 23 | atomic/docker/tempo-runtime | rt_cand \| heavy_docker | runtime_heavy_gates_docker `:591` | ubuntu-latest | 20-40m |
| 24 | atomic/podman/guided | rt_cand \| light_podman | runtime_light_gates_podman `:555` | ubuntu-26.04 | 10-25m |
| 25 | atomic/podman/lifecycle | rt_cand \| light_podman | runtime_light_gates_podman `:555` | ubuntu-26.04 | 10-25m |
| 26 | atomic/podman/policy | rt_cand \| light_podman | runtime_light_gates_podman `:555` | ubuntu-26.04 | 10-25m |
| 27 | atomic/podman/connectivity | rt_cand \| light_podman | runtime_light_gates_podman `:555` | ubuntu-26.04 | 10-25m |
| 28 | atomic/podman/reconciliation | rt_cand \| light_podman | runtime_light_gates_podman `:555` | ubuntu-26.04 | 10-25m |
| 29 | atomic/podman/failure | rt_cand \| light_podman | runtime_light_gates_podman `:555` | ubuntu-26.04 | 10-25m |
| 30 | atomic/podman/full-destroy | rt_cand \| light_podman | runtime_light_gates_podman `:555` | ubuntu-26.04 | 10-25m |
| 31 | atomic/podman/data-capabilities | rt_cand \| heavy_podman | runtime_heavy_gates_podman `:609` | ubuntu-26.04 | 20-40m |
| 32 | atomic/podman/observability | rt_cand \| heavy_podman | runtime_heavy_gates_podman `:609` | ubuntu-26.04 | 20-40m |
| 33 | atomic/podman/identity | rt_cand \| heavy_podman | runtime_heavy_gates_podman `:609` | ubuntu-26.04 | 20-40m |
| 34 | atomic/podman/security | rt_cand \| heavy_podman | runtime_heavy_gates_podman `:609` | ubuntu-26.04 | 12-20m |
| 35 | atomic/podman/backup-restore | rt_cand \| heavy_podman | runtime_heavy_gates_podman `:609` | ubuntu-26.04 | 40-60m |
| 36 | atomic/podman/rabbitmq-runtime | rt_cand \| heavy_podman | runtime_heavy_gates_podman `:609` | ubuntu-26.04 | 20-40m |
| 37 | atomic/podman/mongodb-runtime | rt_cand \| heavy_podman | runtime_heavy_gates_podman `:609` | ubuntu-26.04 | 20-40m |
| 38 | atomic/podman/durable-key-value | rt_cand \| heavy_podman | runtime_heavy_gates_podman `:609` | ubuntu-26.04 | 20-40m |
| 39 | atomic/podman/control-plane-recovery | rt_cand \| heavy_podman | runtime_heavy_gates_podman `:609` | ubuntu-26.04 | 20-40m |
| 40 | atomic/podman/loki-runtime | rt_cand \| heavy_podman | runtime_heavy_gates_podman `:609` | ubuntu-26.04 | 20-40m |
| 41 | atomic/podman/tempo-runtime | rt_cand \| heavy_podman | runtime_heavy_gates_podman `:609` | ubuntu-26.04 | 20-40m |
| 42 | adoption/conformance | adoption | adoption `:427` | ubuntu-latest | ~35m (`:427`) |
| 43 | adoption/realworld-compose | adoption | adoption `:427` | ubuntu-latest | ~35m |
| 44 | adoption/realworld-quadlet | adoption | adoption `:427` | ubuntu-latest | ~35m |
| 45 | adoption/realworld-kubernetes | adoption | adoption `:427` | ubuntu-latest | ~35m |
| 46 | ha/docker/data | v0421_ha_groups_docker | v0421_ha_groups_docker `:645` | ubuntu-latest | 30-60m (90m cap) |
| 47 | ha/docker/identity | v0421_ha_groups_docker | v0421_ha_groups_docker `:645` | ubuntu-latest | 30-60m |
| 48 | ha/docker/observability | v0421_ha_groups_docker | v0421_ha_groups_docker `:645` | ubuntu-latest | 30-60m |
| 49 | ha/docker/routing | v0421_ha_groups_docker | v0421_ha_groups_docker `:645` | ubuntu-latest | 30-60m |
| 50 | ha/podman/data | v0421_ha_groups_podman | v0421_ha_groups_podman `:740` | ubuntu-26.04 | 30-60m |
| 51 | ha/podman/identity | v0421_ha_groups_podman | v0421_ha_groups_podman `:740` | ubuntu-26.04 | 30-60m |
| 52 | ha/podman/observability | v0421_ha_groups_podman | v0421_ha_groups_podman `:740` | ubuntu-26.04 | 30-60m |
| 53 | ha/podman/routing | v0421_ha_groups_podman | v0421_ha_groups_podman `:740` | ubuntu-26.04 | 30-60m |
| 54 | journey/docker | rt_cand \| reference_journey | reference_journey `:853` | ubuntu-latest | 60-120m (`:863`) |
| 55 | journey/podman | rt_cand \| reference_journey | reference_journey `:853` (`:863` podman→ubuntu-26.04) | ubuntu-26.04 | 60-120m |
| 56 | integration/static/public-contracts | integration_public_contracts | integration_public_contracts `:1056` | ubuntu-latest | ~20m |
| 57 | integration/static/configuration-policy | integration_configuration_policy | integration_configuration_policy `:1096` | ubuntu-latest | ~20m |
| 58 | integration/static/browser-terminal | integration_browser_terminal | integration_browser_terminal `:1136` | ubuntu-latest | ~20m |
| 59 | integration/static/live-console | integration_live_console | integration_live_console `:1458` | ubuntu-latest | ~15m (deps core+console+connector+demo) |
| 60 | integration/docker/remote-target | integration_remote_target_docker | integration_remote_target_docker `:1366` | ubuntu-latest | ~15m (needs connector=N) |
| 61 | integration/docker/core-bootstrap | integration_core_docker | integration_core_docker `:1176` | ubuntu-latest | ~45m |
| 62 | integration/docker/coreless-lifecycle | integration_coreless_docker | integration_coreless_docker `:1264` | ubuntu-latest | ~45m |
| 63 | integration/podman/remote-target | integration_remote_target_podman | integration_remote_target_podman `:1412` | ubuntu-26.04 | ~15m (needs connector=N) |
| 64 | integration/podman/core-bootstrap | integration_core_podman | integration_core_podman `:1220` | ubuntu-26.04 | ~45m |
| 65 | integration/podman/coreless-lifecycle | integration_coreless_podman | integration_coreless_podman `:1315` | ubuntu-26.04 | ~45m |

**Single image build:** `runtime_candidate` (`pre-release.yml:366`, `timeout-minutes:20`) builds
`localhost/baseharbor-runtime:pre-release` ONCE; every runtime gate (light/heavy/journey)
`needs: runtime_candidate`. `static_gates` `needs: source` (no runtime image). Aggregate `integration`
gate `:1508` joins all classes.

## 3) READY-TO-EXECUTE dispatch commands (post-freeze; placeholders {C}{D}{U}{N})

### CMD-A — consolidated gap-fill: the 56 gates NOT owned by f7 (one dispatch)
Vehicle: `pre-release.yml` `workflow_dispatch`, `only_gates` (comma-separated canonical gate IDs).
docker+podman matrices run **parallel internally** (`:537-626`, MP2 each) → honors OWNER DIRECTIVE
2026-10-22 (parallel docker+podman) WITHOUT two queued runs on the shared serial slot (§4 F-1).
Inputs verified at C (`:8-26`): `tag`+`ref`+`demo_ref` required, `consumer_ref` optional (`:21`),
`only_gates` optional (`:26`).

```bash
gh api repos/mcpdev80/baseharbor/actions/workflows/pre-release.yml/dispatches -X POST \
  -f ref=develop \
  -f 'inputs[tag]=v0.4.26' -f 'inputs[ref]={C}' -f 'inputs[demo_ref]={D}' -f 'inputs[consumer_ref]={N}' \
  -f 'inputs[only_gates]=atomic/static/config-matrix,atomic/static/init,atomic/static/agent,atomic/static/shell-ux,atomic/docker/lifecycle,atomic/docker/policy,atomic/docker/connectivity,atomic/docker/reconciliation,atomic/docker/failure,atomic/docker/full-destroy,atomic/docker/observability,atomic/docker/identity,atomic/docker/security,atomic/docker/rabbitmq-runtime,atomic/docker/mongodb-runtime,atomic/docker/durable-key-value,atomic/docker/control-plane-recovery,atomic/docker/loki-runtime,atomic/docker/tempo-runtime,atomic/podman/lifecycle,atomic/podman/policy,atomic/podman/connectivity,atomic/podman/reconciliation,atomic/podman/failure,atomic/podman/full-destroy,atomic/podman/observability,atomic/podman/identity,atomic/podman/security,atomic/podman/rabbitmq-runtime,atomic/podman/mongodb-runtime,atomic/podman/durable-key-value,atomic/podman/control-plane-recovery,atomic/podman/loki-runtime,atomic/podman/tempo-runtime,adoption/conformance,adoption/realworld-compose,adoption/realworld-quadlet,adoption/realworld-kubernetes,ha/docker/data,ha/docker/identity,ha/docker/observability,ha/docker/routing,ha/podman/data,ha/podman/identity,ha/podman/observability,ha/podman/routing,integration/static/public-contracts,integration/static/configuration-policy,integration/static/browser-terminal,integration/static/live-console,integration/docker/remote-target,integration/podman/remote-target,integration/docker/core-bootstrap,integration/podman/core-bootstrap,integration/docker/coreless-lifecycle,integration/podman/coreless-lifecycle'
```

**CMD-A gate count: 56 (verified 65 - 9 f7-owned = 56).**
Preconditions: `{N}` merged; `{U}` merged and `docs/releases/v0.4.26.consumer-pins.json` carries U
(pins resolution `pre-release.yml:133-137,304`) — `live-console` + `remote-target`x2 depend on it.
f7-owned RED gates (`guided`x2) and IN-FLIGHT gates (`data`x2, `backup`x2, `journey`x2) are EXCLUDED —
any f7 row that lands GREEN moves into CMD-A (reuse); any that stays RED moves out (fix ⇒ C rekey,
re-derive from `q16-freeze-tracker.md`).

### CMD-B — authorizing COMPLETE run (all 65; `RELEASE_WORKFLOW.md:46` — only complete run approves;
targeted/`only_gates` never authorize a release)
```bash
gh api repos/mcpdev80/baseharbor/actions/workflows/pre-release.yml/dispatches -X POST \
  -f ref=develop -f 'inputs[tag]=v0.4.26' -f 'inputs[ref]={C}' -f 'inputs[demo_ref]={D}' \
  -f 'inputs[consumer_ref]={N}' -f 'inputs[only_gates]='
```
Verifier `baseharbor/scripts/pre-release-resume.py` (@C SHA `6c38d651…`, o17 §1-item 4) →
`release-approved.json` + coverage provenance. Reuse discipline (o17 §3d): CMD-A receipts + f7
reused iff verifier confirms identical inputs/attempt digests.

### CMD-C — optional adoption-only early gap-fill (own group; alt vehicle, §4 F-2 one-at-a-time)
```bash
for g in adoption-conformance adoption-realworld-compose adoption-realworld-quadlet adoption-realworld-kubernetes; do
  gh api repos/mcpdev80/baseharbor/actions/workflows/targeted-integration.yml/dispatches -X POST \
    -f ref=develop -f "inputs[gate]=$g" -f "inputs[ref]={C}"   # terminal + auth before next — F-2
done
```
Choice list `targeted-integration.yml:25-28`; job `:1200-1204` (ubuntu-latest, ~30m).

## 4) Parallel-vs-serial constraints (concurrency groups, file:line at C)
- **F-1 — global serial slot `baseharbor-release-qualification`, all `cancel-in-progress: false` (queue, never evict):** `pre-release.yml:41-43`, `targeted-demo-acceptance.yml:38-40`, `ci.yml:192-194` (targeted_ha_docker), `ci.yml:234-236` (targeted_ha_podman). **All four re-verified at C via `git show`.** Consequences: (a) f7 native runs and CMD-A/CMD-B cannot run concurrently → order: f7 terminals → CMD-A → CMD-B; (b) o13b §3's two sequential heavy dispatches double-queue → replaced by CMD-A single dispatch with internal MP2 docker/podman matrices (`:591-626`) — parallel where the directive demands, serialized only where GitHub forces; (c) CMD-D-style `runtime-validation/ha/**` pushes join the same slot → prefer native HA `:645/:740`.
- **F-2 — `targeted-integration.yml:45` group `targeted-integration-${ref}-${inputs.ref||sha}`** with `:46 cancel-in-progress` TRUE except on the two legacy `work/` branches: four adoption dispatches with identical `inputs.ref={C}` collapse to ONE group key → GitHub one-slot queue; rapid-fire dispatches on a `develop`-ref_name run CANCEL the prior one. Rule: one-at-a-time, terminal + authenticated before next. Distinct gate inputs do NOT widen the group.
- **Independent groups (safe to overlap with each other, not with F-1):** `docker-runtime-acceptance.yml:24-26` and `podman-acceptance.yml:23-25` (`${{ github.workflow }}-…-${ref}`, cancel:true — legacy light push path, unused by this sheet); `ci.yml:17-19` (ci-${ref}).
- **Concurrency finding (EMPIRICALLY CONFIRMED f7 §6a, batch 19:03Z):** core `targeted-demo-acceptance.yml:38-40` pins ALL runs to one group `baseharbor-release-qualification` with `cancel-in-progress:false` → sibling dispatch cancels the queued predecessor (journey/docker 39836748059 CANCELLED, 0 jobs, seconds after podman/data took the runner; journey/podman 39836753432 survived queued). Within-workflow parallelism is structurally forbidden here — NOT silently serialized. **Recommendation:** split the core group per gate, e.g. `group: baseharbor-release-qualification-${{ inputs.runtime }}-${{ inputs.gate }}` (with a bounded runner-fit check), so docker/podman siblings can run in parallel. (Captured as file:line finding per CI-efficiency rule; never silently serialized.)
- **Wall estimate (caps→est):** static ~10m (f7 run 39831982468); light 10-25m (f5 7 runs ≈55m, `f5-demo-gates.md:21`); heavy 20-40m, backup-restore 40-60m, security 12-20m (suite `:201`); HA 30-60m (90m cap); journey 60-120m; adoption 15-30m; integration 10-45m. **CMD-A wall ≈3.5-5h** (MP2 matrices, both runtimes concurrent); CMD-B full ≈5-8h.
- **Runner fit:** all 65 rows run on hosted runners (`ubuntu-latest` / podman=`ubuntu-26.04`, suite `:34`); self-hosted podman pool only in `podman-acceptance.yml:36` — not on this sheet's paths; f5 observed no runner-fit blocker (`f5-demo-gates.md:111`).

## 5) f7-coverage ledger (cross-marked, no-double-run markers) — 9 f7-owned + 56 gap = 65

### f7-owned (9 rows — cross-marked against f7 `## Runs` state @ HOLD 2026-10-22T19:46Z)
| # | Gate | f7 vehicle | f7 run | state @ HOLD | CMD-A? |
|---|---|---|---|---|---|
| 3 | atomic/static/mcp | targeted-demo-acceptance (ref=C, demo_ref=D, blob 78c31a78) | [39831982468](https://github.com/mcpdev80/baseharbor/actions/runs/39831982468) | **GREEN** SUCCESS 18:31Z (`PASS MCP`) | reused — NOT in CMD-A |
| 6 | atomic/docker/guided | same | [39832388802](https://github.com/mcpdev80/baseharbor/actions/runs/39832388802) | **RED** FAILURE 18:38Z (new wizard-assertion class, no F5 hang) | fix first ⇒ C rekey — NOT in CMD-A |
| 24 | atomic/podman/guided | same | [39833309541](https://github.com/mcpdev80/baseharbor/actions/runs/39833309541) | **RED** FAILURE 18:49Z (identical class) | fix first ⇒ C rekey — NOT in CMD-A |
| 13 | atomic/docker/data-capabilities | same | [39834803429](https://github.com/mcpdev80/baseharbor/actions/runs/39834803429) | RED FAILURE 18:58Z (`new-edit-new-sql-no`, rows PASS) | IN-FLIGHT — rerun GREEN ⇒ add; stays RED ⇒ out |
| 31 | atomic/podman/data-capabilities | same | [39836743807](https://github.com/mcpdev80/baseharbor/actions/runs/39836743807) | RED FAILURE (~19:13Z, forensics pending) | IN-FLIGHT — same rule |
| 17 | atomic/docker/backup-restore | demo push `work/v0.4.26-ecosystem-release [demo-runtime]` T=7b7da815 (tree=D) | [39836848260](https://github.com/mcpdev80/baseharbor-demo/actions/runs/39836848260) | docker job SUCCESS / podman job FAILURE (~19:34Z, cleanup-provenance unresolved) | IN-FLIGHT — same rule |
| 35 | atomic/podman/backup-restore | same demo push | [39836848260](https://github.com/mcpdev80/baseharbor-demo/actions/runs/39836848260) | (same run, podman job FAILURE) | IN-FLIGHT — same rule |
| 54 | journey/docker | f7 step 8 gate=reference-journey | [39836748059](https://github.com/mcpdev80/baseharbor/actions/runs/39836748059) | CANCELLED (queue, 0 jobs) per F-1 | re-dispatch on empty queue (held) — add if GREEN |
| 55 | journey/podman | same | [39836753432](https://github.com/mcpdev80/baseharbor/actions/runs/39836753432) | FAILURE (~19:45Z, forensics pending) | IN-FLIGHT — same rule |

**Rule (no-double-run):** any f7 row that lands GREEN → add to CMD-A (reuse). Any that stays RED after
rerun → excluded from CMD-A (fix ⇒ C rekey, re-derive pins from `q16-freeze-tracker.md`). Any GREEN
stays reused, never re-dispatched.

### CMD-A gap (56 rows) — gates NOT owned by f7 (count verified: 9 + 56 = 65)
static (4): config-matrix, init, agent, shell-ux; docker-light (6): lifecycle, policy, connectivity, reconciliation, failure, full-destroy; docker-heavy (11): observability, identity, security, rabbitmq-runtime, mongodb-runtime, durable-key-value, control-plane-recovery, loki-runtime, tempo-runtime; podman-light (6): lifecycle, policy, connectivity, reconciliation, failure, full-destroy; podman-heavy (11): observability, identity, security, rabbitmq-runtime, mongodb-runtime, durable-key-value, control-plane-recovery, loki-runtime, tempo-runtime; adoption (4): conformance, realworld-compose, realworld-quadlet, realworld-kubernetes; HA (8): ha/docker/{data,identity,observability,routing}, ha/podman/{data,identity,observability,routing}; integration (10): static/{public-contracts,configuration-policy,browser-terminal,live-console}, docker/{remote-target,core-bootstrap,coreless-lifecycle}, podman/{remote-target,core-bootstrap,coreless-lifecycle}.

> **CMD-A `only_gates` string = the exact 56-gate list in §3 CMD-A (canonical IDs with `atomic/` /
> `ha/` / `adoption/` / `integration/` prefixes).**

## 6) DELIVERABLES at card close (from o17 §3e)
Approval origin + exact candidate/consumer SHAs + complete 65-gate coverage table + accepted exceptions
+ remaining failures; evidence = `pre-release-resume` outputs + `release-approved.json` + per-gate
run-id links. No promotion/tagging under this card (`RELEASE_WORKFLOW.md:52`).

## repo-rule compliance
Read-only lane: only file written = this evidence file; product tree untouched (`git -C baseharbor` used
only read-only `show`/`cat-file`; local branch `fix/changelog-docs-classifier` not modified; single
read-only `fetch origin develop`). No dispatches issued. No secrets logged. All workflow/job/concurrency/
input facts re-verified at C (`git show 2adae47a:`) with file:line.

O21-MATRIX-INPUTS-DONE

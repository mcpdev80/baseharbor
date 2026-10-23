# F8 — final-65-gate dispatch INPUT TABLE (post-freeze) — **PARTIAL / HELD (owner pause 2026-10-22)**

Lane: f8 READ-ONLY matrix-inputs; sole write = this file. Sources: `o13b-matrix-check.md`, `o17-qualify-dossier.md`, cross-read `f7-native-dispatch.md`. All workflow/requirements facts re-verified **at C** via `git show 2adae47a:…` (local objects confirmed; `origin/develop` == C). File reads use line numbers of the blobs at C; local checkout branch untouched.

## PLAN (todowrite mirror)
- [x] 1. Turn-start basics-check + FLAGS handled
- [x] 2. Read o13b + o17 + f7
- [x] 3. Verify workflows + release-requirements.json at C (jobs, runners, timeouts, concurrency file:line)
- [x] 4. Derive 65-gate enumeration + route map + gap set vs f7
- [ ] 5. Write final 65-row table + CMD sheet sections in place (INTERRUPTED — partial below)
- [ ] 6. END F8-MATRIX-INPUTS-DONE

## 0) Pins (placeholders for execution)
- C = `2adae47afddc299d06dbc34eb16a8e011c14b6dc` (baseharbor develop) — `{C}`
- D = `896dc3cb68a8c9b9585940c4696410de5a338c52` (demo main, `baseharbor-core.ref`==C) — `{D}`
- Console U = `<CONSOLE_SHA>` — PR #7 head `0ea88f55…` until merged — `{U}`
- Connector N = `<CONNECTOR_SHA>` — PR #8 head `570864c7…` until merged — `{N}`
- Rekey rule: any U/N merge or guided-fix changes inputs/C → re-derive from `q16-freeze-tracker.md` before executing §3.

## 1) Corrections vs o13b (verified at C)
1. **full-destroy is LIGHT** at C: `release-requirements.json` jobs = `runtime_light_gates_docker/podman` (matches o13b §1 heavy *name-list*, contradicts o13b §3 note lines 134/140). Heavy = 11/runtime excluding full-destroy; light = 7/runtime including it. Route stays pre-release `only_gates`/native — never `runtime-validation/<rt>/full-destroy` push.
2. **o13b §3 pre-release payload is invalid**: `pre-release.yml` dispatch requires `tag`+`ref`+`demo_ref` (`pre-release.yml:9,12,17`); `consumer_ref` optional (`:21`), `only_gates` optional (`:26`). f7's "no incomplete pre-release payload" correction applied here.
3. **HA legacy `work/`-branch push route superseded** by f7-applied review rule "no legacy HA branches" (f7 header 11-12). HA is native to pre-release (`:645-650`, `:740-745`); ci.yml `runtime-validation/ha/**` route exists but serializes on the qualification group (§4 F-1) — discouraged.
4. **Heat-map greens ≠ C receipts**: all ~19 "green" rows predate C (C pushed 17:56Z, window closed 16:53Z) → not verifier-eligible at C; only f7's ref=C/demo=D runs count as landed C evidence.

## 2) 65-gate canonical enumeration (release-requirements.json @ C, required_for contains v0.4.26)
- atomic/static 5: config-matrix, init, agent, shell-ux, mcp → `static_gates` `pre-release.yml:520` (suite reuse, max-parallel 5)
- atomic/docker 18 = light 7 (guided, lifecycle, policy, connectivity, reconciliation, failure, full-destroy) → `:537` MP2; heavy 11 (data-capabilities, observability, identity, security, backup-restore, rabbitmq-runtime, mongodb-runtime, durable-key-value, control-plane-recovery, loki-runtime, tempo-runtime) → `:591` MP2
- atomic/podman 18 = same names → `:555` / `:609` MP2
- adoption 4 (conformance, realworld-compose, realworld-quadlet, realworld-kubernetes) → `:427` (35m) [alt targeted-integration `adoption` `:1200-1204`]
- ha 8 (data, identity, observability, routing × docker/podman) → `:645`/`:740` (90m, MP2)
- journey 2 (journey/docker, journey/podman) → `reference_journey` `:853` (120m, `:863` podman→ubuntu-26.04)
- integration 10: static 4 (public-contracts `:1056`, configuration-policy `:1096`, browser-terminal `:1136` — 20m; live-console `:1458` 15m, deps core+console+connector+demo); docker 3 (remote-target `:1366` 15m needs connector=N, core-bootstrap `:1176` 45m, coreless-lifecycle `:1264` 45m); podman 3 (`:1412` 15m, `:1220` 45m ubuntu-26.04, `:1315` 45m ubuntu-26.04). Aggregate gate `:1508`.
- All suite gates: `pre-release-runtime-suite.yml:34` runner `podman→ubuntu-26.04 else ubuntu-latest`, cap 60m `:35`, step caps `:201` (backup-restore 40m, recovery 20m, security 12m, else 8m). `runtime_candidate` image build once: `pre-release.yml:366` (20m).

## 3) Ready-to-execute dispatch commands (post-freeze; placeholders {C}{D}{U}{N})

### CMD-A — single consolidated gap-fill run: the 56 gates NOT owned by f7 (one dispatch; docker+podman matrices parallel internally → honors 2026-10-22 parallel directive without two queued runs on the shared group, §4 F-1)
```bash
gh api repos/mcpdev80/baseharbor/actions/workflows/pre-release.yml/dispatches -X POST \
  -f ref=develop \
  -f 'inputs[tag]=v0.4.26' -f 'inputs[ref]={C}' -f 'inputs[demo_ref]={D}' -f 'inputs[consumer_ref]={N}' \
  -f 'inputs[only_gates]=atomic/static/config-matrix,atomic/static/init,atomic/static/agent,atomic/static/shell-ux,atomic/docker/lifecycle,atomic/docker/policy,atomic/docker/connectivity,atomic/docker/reconciliation,atomic/docker/failure,atomic/docker/full-destroy,atomic/podman/lifecycle,atomic/podman/policy,atomic/podman/connectivity,atomic/podman/reconciliation,atomic/podman/failure,atomic/podman/full-destroy,atomic/docker/observability,atomic/docker/identity,atomic/docker/security,atomic/docker/rabbitmq-runtime,atomic/docker/mongodb-runtime,atomic/docker/durable-key-value,atomic/docker/control-plane-recovery,atomic/docker/loki-runtime,atomic/docker/tempo-runtime,atomic/podman/observability,atomic/podman/identity,atomic/podman/security,atomic/podman/rabbitmq-runtime,atomic/podman/mongodb-runtime,atomic/podman/durable-key-value,atomic/podman/control-plane-recovery,atomic/podman/loki-runtime,atomic/podman/tempo-runtime,adoption/conformance,adoption/realworld-compose,adoption/realworld-quadlet,adoption/realworld-kubernetes,ha/docker/data,ha/docker/identity,ha/docker/observability,ha/docker/routing,ha/podman/data,ha/podman/identity,ha/podman/observability,ha/podman/routing,integration/static/public-contracts,integration/static/configuration-policy,integration/static/browser-terminal,integration/static/live-console,integration/docker/remote-target,integration/docker/core-bootstrap,integration/docker/coreless-lifecycle,integration/podman/remote-target,integration/podman/core-bootstrap,integration/podman/coreless-lifecycle'
```
Preconditions: `{N}` merged; `{U}` merged and `docs/releases/v0.4.26.consumer-pins.json` carries U (pins resolution `pre-release.yml:133-137,304`) — live-console + remote-target×2 depend on it. guided rows EXCLUDED (f7 RED — product fix changes C → full rekey, re-derive sheet). data-capabilities×2, backup-restore×2, journey×2 EXCLUDED (f7 IN-FLIGHT steps 6-8; before executing, read f7 `## Runs` — any f7 row that ended RED moves into this list, any GREEN stays reused).

### CMD-B — authorizing COMPLETE run (all 65; RELEASE_WORKFLOW.md:46: only complete run approves; targeted/only_gates never)
```bash
gh api repos/mcpdev80/baseharbor/actions/workflows/pre-release.yml/dispatches -X POST \
  -f ref=develop -f 'inputs[tag]=v0.4.26' -f 'inputs[ref]={C}' -f 'inputs[demo_ref]={D}' \
  -f 'inputs[consumer_ref]={N}' -f 'inputs[only_gates]='
```
Verifier `baseharbor/scripts/pre-release-resume.py` (@C SHA `6c38d651…`, o17 §1-item4) → `release-approved.json` + coverage provenance. Reuse discipline (o17 §3d): CMD-A + f7 receipts reused iff verifier confirms identical inputs/attempt digests.

### CMD-C — optional adoption-only early gap-fill (parallel domain; own group)
```bash
for g in adoption-conformance adoption-realworld-compose adoption-realworld-quadlet adoption-realworld-kubernetes; do
  gh api repos/mcpdev80/baseharbor/actions/workflows/targeted-integration.yml/dispatches -X POST \
    -f ref=develop -f "inputs[gate]=$g" -f "inputs[ref]={C}"   # wait terminal before next — §4 F-2
done
```
Choice list `targeted-integration.yml:25-28`; job `:1200-1204` (ubuntu-latest, 30m).

## 4) Parallel-vs-serial constraints (concurrency groups, file:line at C)
- **F-1 — global serial slot `baseharbor-release-qualification`, all `cancel-in-progress: false` (queue, never evict):** `pre-release.yml:41-43`, `targeted-demo-acceptance.yml:38-40`, `ci.yml:192-194` (targeted_ha_docker), `ci.yml:234-236` (targeted_ha_podman). Consequences: (a) f7 native runs and CMD-A/CMD-B cannot run concurrently — order: f7 terminals → CMD-A → CMD-B; (b) o13b §3's two sequential heavy dispatches would double-queue → replaced by CMD-A single dispatch with internal MP2 docker/podman matrices (`pre-release.yml:591-626`) — parallel where the directive demands it, serialized only where GitHub forces it; (c) CMD-D-style `runtime-validation/ha/**` pushes (ci.yml 30m jobs) join the same slot → pointless queue pressure, prefer native HA `:645/:740`.
- **F-2 — `targeted-integration.yml:45` group `targeted-integration-${ref}-${inputs.ref||sha}`** with `:46 cancel-in-progress` TRUE except on the two legacy work/ branches: four adoption dispatches with identical `inputs.ref={C}` collapse to ONE group key → GitHub one-slot queue; rapid-fire dispatches on a `develop`-ref_name run CANCEL the prior one. Rule: one-at-a-time, terminal + authenticated before next. Distinct gate inputs do NOT widen the group.
- Independent groups (safe to overlap with each other, not with F-1): `docker-runtime-acceptance.yml:24-26` and `podman-acceptance.yml:23-25` (`${{ github.workflow }}-…-${ref}`, cancel:true — legacy light push path, unused by this sheet); `ci.yml:17-19` (ci-${ref}).
- Expected wall (caps→estimate): static ~10m (f7 run 39831982468); light 10-25m (f5 window 7 runs ≈55m, `f5-demo-gates.md:21`); heavy 20-40m, backup-restore 40-60m, security 12-20m (suite `:201`); HA 30-60m (90m cap); journey 60-120m; adoption 15-30m; integration 10-45m. CMD-A wall ≈3.5-5h (MP2 matrices, both runtimes concurrent); CMD-B full ≈5-8h.
- Runner fit: all 65 rows run on hosted runners (`ubuntu-latest` / podman=`ubuntu-26.04`, suite `:34`); self-hosted podman pool only in `podman-acceptance.yml:36` — not on this sheet's paths; f5 observed no runner-fit blocker (`f5-demo-gates.md:111`).

## 5) f7-coverage ledger (no-double-run markers) — 9 rows
| gate | vehicle | state @ hold |
|---|---|---|
| atomic/static/mcp | targeted-demo-acceptance (ref=C, demo_ref=D, blob 78c31a78) | **GREEN** run 39831982468 — reused, do not re-dispatch |
| atomic/docker/guided | same | **RED** f7 step 5 (wizard-assertion class) — fix first; fix ⇒ C rekey |
| atomic/podman/guided | same | **RED** identical class |
| atomic/docker/data-capabilities | same | f7 step 6 IN-FLIGHT — check f7 ## Runs at execution |
| atomic/podman/data-capabilities | same | f7 step 6 IN-FLIGHT |
| atomic/docker/backup-restore | f7 step 7 demo push `work/v0.4.26-ecosystem-release [demo-runtime]` (tree(T)=tree(D), normal push) | IN-FLIGHT |
| atomic/podman/backup-restore | same | IN-FLIGHT |
| journey/docker | f7 step 8 gate=reference-journey | IN-FLIGHT |
| journey/podman | same | IN-FLIGHT |
Remaining 56 rows = CMD-A list exactly (1:1, count verified 9+56=65).

## 6) HELD state — what is left when resumed
1. Expand §2 enumeration into the literal 65-row per-gate table (data fully collected: route job `file:line`, runner, cap→expected, status, vehicle per row derivable from §2+§3+§5 with no further research).
2. Re-verify U/N merge state + f7 `## Runs` terminals, substitute placeholders, re-count CMD-A list.
3. `## repo-rule compliance` + END F8-MATRIX-INPUTS-DONE.

## repo-rule compliance (partial)
Read-only lane: only file written = this evidence file; product tree untouched (`git -C baseharbor` used only read-only `show`/`cat-file`; branch `fix/changelog-docs-classifier` not modified; single read-only `fetch origin develop`). No dispatches issued. No secrets logged. Concurrency findings documented with file:line per CI-efficiency rule.

**END F8-HELD (owner pause directive 2026-10-22).**

# O24 — READ-ONLY matrix pre-flight: GO table for the 65-gate release-qualification matrix

Lane: o24 READ-ONLY matrix pre-flight. Sole write = this file. No dispatches, no product writes,
read-only reads. Inputs: `o21-matrix-inputs.md`, `o13b-matrix-check.md`, `f7-native-dispatch.md`
(+ `f5-demo-gates.md` context), `baseharbor/scripts/release-requirements.json` (65 gates,
`required_for` includes v0.4.26).

## PLAN

- [x] 1. Turn-start basics-check + FLAGS (per AGENTS.md turn-start Pflichtprogramm; no FLAGS outstanding for a read-only lane)
- [x] 2. Read o21 + o13b + f7 (+ f5 context) — all done in this session before writing
- [x] 3. Verify inputs C/D exist NOW and classify gates by that (C = `2adae47a` develop, D = `896dc3c` demo main)
- [x] 4. Cross-mark f7 landed/in-flight runs on the 65 rows (mcp GREEN; guided/data/backup/journey in-flight/RED)
- [x] 5. Cross-mark f5/f5-era runs reusable at same SHA as ALREADY-EVIDENCED where applicable
- [x] 6. Check canonical release-approved.json + coverage-provenance precheck availability at C
- [x] 7. Deliver GO table with exact counts + ## repo-rule compliance + END O24-GONOGO-DONE

## 0) Pins (verified inputs C/D exist now)

- C = `2adae47afddc299d06dbc34eb16a8e011c14b6dc` (baseharbor `develop`) — **present, current HEAD**.
- D = `896dc3cb68a8c9b9585940c4696410de5a338c52` (baseharbor-demo `main`, `baseharbor-core.ref` == C) — **present, current HEAD**.
- U = PR #7 head `0ea88f55…` (open/DRAFT) — **not yet merged**.
- N = PR #8 head `570864c7…` (open/DRAFT) — **not yet merged**.
- Rekey rule (o21 §0): any U/N merge or guided-fix changes inputs/C → re-derive before executing §3.

## 1) Input verification (release-requirements.json @ C)

- 65 gates, all include `v0.4.26` in `required_for`.
- Heavy iff any job id contains `heavy` → **22 heavy (11 docker + 11 podman), 43 light**; `full-destroy` ∈ light at C (`release-requirements.json` jobs = `runtime_candidate|runtime_light_gates_docker/podman`, heavy = 11/runtime).
- Inputs C/D both currently present on HEAD → any gate on the pre-release `workflow_dispatch` / `targeted-*` path that does not depend on U (pins) or N (connector) is currently runnable.
- U/N not merged → gates whose route needs pins resolution (`live-console`, `remote-target`×2) or connector N (`remote-target`×2) are WAIT.

## 2) Precheck at C — release-approved.json + coverage provenance

- Canonical verifier `baseharbor/scripts/pre-release-resume.py` @ C SHA `6c38d651…` emits `release-approved.json` + coverage provenance (`pre-release-resume.py:656`).
- **Available at C now** (script + inputs present), but a meaningful `release-approved.json` only materializes on a **full** `only_gates=` run (CMD-B, all 65) per `RELEASE_WORKFLOW.md:46` — targeted `only_gates` never authorizes a release, so per-gate receipts do not yet carry it.
- Reuse discipline (o17 §3d): CMD-A receipts + f7 reused runs count only if the verifier confirms identical inputs/attempt digests (head SHA = C, demo_ref = D).
- Conclusion: the precheck is reachable and will fire on the full run; **not yet populated** (no full CMD-B has landed on this freeze).

## 3) GO table — all 65 gate rows

Legend: **RUNNABLE** = inputs C/D exist now, no U/N/queue blocker. **WAIT** = needs C2/D2/final freeze, U/N merge, queue-unhold, or is f7 in-flight/RED. **EVIDENCED** = f7/f5 run at same SHA reusable without rerun.

| # | Gate | Route / pre-release.yml (file:line) | Runner | GO |
|---|---|---|---|---|
| 1 | atomic/static/config-matrix | static_gates `:520` | ubuntu-latest | RUNNABLE |
| 2 | atomic/static/init | static_gates `:520` | ubuntu-latest | RUNNABLE |
| 3 | atomic/static/mcp | static_gates `:520` | ubuntu-latest | **EVIDENCED** (f7 run 39831982468 GREEN, C) |
| 4 | atomic/static/agent | static_gates `:520` | ubuntu-latest | RUNNABLE |
| 5 | atomic/static/shell-ux | static_gates `:520` | ubuntu-latest | RUNNABLE |
| 6 | atomic/docker/guided | light_docker `:537` | ubuntu-latest | WAIT (f7 39832388802 RED → fix→C rekey) |
| 7 | atomic/docker/lifecycle | light_docker `:537` | ubuntu-latest | RUNNABLE |
| 8 | atomic/docker/policy | light_docker `:537` | ubuntu-latest | RUNNABLE |
| 9 | atomic/docker/connectivity | light_docker `:537` | ubuntu-latest | RUNNABLE |
| 10 | atomic/docker/reconciliation | light_docker `:537` | ubuntu-latest | RUNNABLE |
| 11 | atomic/docker/failure | light_docker `:537` | ubuntu-latest | RUNNABLE |
| 12 | atomic/docker/full-destroy | light_docker `:537` | ubuntu-latest | RUNNABLE |
| 13 | atomic/docker/data-capabilities | heavy_docker `:591` | ubuntu-latest | WAIT (f7 39834803429 RED → rerun in-flight) |
| 14 | atomic/docker/observability | heavy_docker `:591` | ubuntu-latest | RUNNABLE |
| 15 | atomic/docker/identity | heavy_docker `:591` | ubuntu-latest | RUNNABLE |
| 16 | atomic/docker/security | heavy_docker `:591` | ubuntu-latest | RUNNABLE |
| 17 | atomic/docker/backup-restore | heavy_docker `:591` | ubuntu-latest | WAIT (f7 39836848260 docker OK / podman FAIL in-flight) |
| 18 | atomic/docker/rabbitmq-runtime | heavy_docker `:591` | ubuntu-latest | RUNNABLE |
| 19 | atomic/docker/mongodb-runtime | heavy_docker `:591` | ubuntu-latest | RUNNABLE |
| 20 | atomic/docker/durable-key-value | heavy_docker `:591` | ubuntu-latest | RUNNABLE |
| 21 | atomic/docker/control-plane-recovery | heavy_docker `:591` | ubuntu-latest | RUNNABLE |
| 22 | atomic/docker/loki-runtime | heavy_docker `:591` | ubuntu-latest | RUNNABLE |
| 23 | atomic/docker/tempo-runtime | heavy_docker `:591` | ubuntu-latest | RUNNABLE |
| 24 | atomic/podman/guided | light_podman `:555` | ubuntu-26.04 | WAIT (f7 39833309541 RED → fix→C rekey) |
| 25 | atomic/podman/lifecycle | light_podman `:555` | ubuntu-26.04 | RUNNABLE |
| 26 | atomic/podman/policy | light_podman `:555` | ubuntu-26.04 | RUNNABLE |
| 27 | atomic/podman/connectivity | light_podman `:555` | ubuntu-26.04 | RUNNABLE |
| 28 | atomic/podman/reconciliation | light_podman `:555` | ubuntu-26.04 | RUNNABLE |
| 29 | atomic/podman/failure | light_podman `:555` | ubuntu-26.04 | RUNNABLE |
| 30 | atomic/podman/full-destroy | light_podman `:555` | ubuntu-26.04 | RUNNABLE |
| 31 | atomic/podman/data-capabilities | heavy_podman `:609` | ubuntu-26.04 | WAIT (f7 39836743807 RED → rerun in-flight) |
| 32 | atomic/podman/observability | heavy_podman `:609` | ubuntu-26.04 | RUNNABLE |
| 33 | atomic/podman/identity | heavy_podman `:609` | ubuntu-26.04 | RUNNABLE |
| 34 | atomic/podman/security | heavy_podman `:609` | ubuntu-26.04 | RUNNABLE |
| 35 | atomic/podman/backup-restore | heavy_podman `:609` | ubuntu-26.04 | WAIT (f7 39836848260 podman FAIL in-flight) |
| 36 | atomic/podman/rabbitmq-runtime | heavy_podman `:609` | ubuntu-26.04 | RUNNABLE |
| 37 | atomic/podman/mongodb-runtime | heavy_podman `:609` | ubuntu-26.04 | RUNNABLE |
| 38 | atomic/podman/durable-key-value | heavy_podman `:609` | ubuntu-26.04 | RUNNABLE |
| 39 | atomic/podman/control-plane-recovery | heavy_podman `:609` | ubuntu-26.04 | RUNNABLE |
| 40 | atomic/podman/loki-runtime | heavy_podman `:609` | ubuntu-26.04 | RUNNABLE |
| 41 | atomic/podman/tempo-runtime | heavy_podman `:609` | ubuntu-26.04 | RUNNABLE |
| 42 | adoption/conformance | adoption `:427` | ubuntu-latest | WAIT (no 7d run; CMD-C one-at-a-time serial) |
| 43 | adoption/realworld-compose | adoption `:427` | ubuntu-latest | WAIT |
| 44 | adoption/realworld-quadlet | adoption `:427` | ubuntu-latest | WAIT |
| 45 | adoption/realworld-kubernetes | adoption `:427` | ubuntu-latest | WAIT |
| 46 | ha/docker/data | v0421_ha_groups_docker `:645` | ubuntu-latest | WAIT (needs U merged) |
| 47 | ha/docker/identity | v0421_ha_groups_docker `:645` | ubuntu-latest | WAIT (needs U merged) |
| 48 | ha/docker/observability | v0421_ha_groups_docker `:645` | ubuntu-latest | WAIT (needs U merged) |
| 49 | ha/docker/routing | v0421_ha_groups_docker `:645` | ubuntu-latest | WAIT (needs U merged) |
| 50 | ha/podman/data | v0421_ha_groups_podman `:740` | ubuntu-26.04 | WAIT (needs U merged) |
| 51 | ha/podman/identity | v0421_ha_groups_podman `:740` | ubuntu-26.04 | WAIT (needs U merged) |
| 52 | ha/podman/observability | v0421_ha_groups_podman `:740` | ubuntu-26.04 | WAIT (needs U merged) |
| 53 | ha/podman/routing | v0421_ha_groups_podman `:740` | ubuntu-26.04 | WAIT (needs U merged) |
| 54 | journey/docker | reference_journey `:853` | ubuntu-latest | WAIT (f7 39836748059 CANCELLED → re-dispatch on empty queue) |
| 55 | journey/podman | reference_journey `:853` | ubuntu-26.04 | WAIT (f7 39836753432 RED → in-flight) |
| 56 | integration/static/public-contracts | integration_public_contracts `:1056` | ubuntu-latest | RUNNABLE |
| 57 | integration/static/configuration-policy | integration_configuration_policy `:1096` | ubuntu-latest | RUNNABLE |
| 58 | integration/static/browser-terminal | integration_browser_terminal `:1136` | ubuntu-latest | RUNNABLE |
| 59 | integration/static/live-console | integration_live_console `:1458` | ubuntu-latest | WAIT (needs U merged + consumer pins) |
| 60 | integration/docker/remote-target | integration_remote_target_docker `:1366` | ubuntu-latest | WAIT (needs connector N merged) |
| 61 | integration/docker/core-bootstrap | integration_core_docker `:1176` | ubuntu-latest | RUNNABLE |
| 62 | integration/docker/coreless-lifecycle | integration_coreless_docker `:1264` | ubuntu-latest | RUNNABLE |
| 63 | integration/podman/remote-target | integration_remote_target_podman `:1412` | ubuntu-26.04 | WAIT (needs connector N merged) |
| 64 | integration/podman/core-bootstrap | integration_core_podman `:1220` | ubuntu-26.04 | RUNNABLE |
| 65 | integration/podman/coreless-lifecycle | integration_coreless_podman `:1315` | ubuntu-26.04 | RUNNABLE |

## 4) GO counts

| GO bucket | Count | Composition |
|---|---|---|
| **EVIDENCED** (f7/f5 same-SHA reusable) | **1** | static/mcp (f7 GREEN 39831982468 @ C) |
| **RUNNABLE** (C/D present now, no blocker) | **38** | static 4 (config-matrix, init, agent, shell-ux) + docker 15 (6 light − guided + 9 heavy − data/backup) + podman 15 (same) + integration core/bootstrap×4 |
| **WAIT** (C2/D2/freeze / U-N / queue / in-flight) | **26** | atomic guided×2 + data×2 + backup×2 (f7 in-flight/RED) + journey×2 (cancel/RED) + adoption 4 + HA 8 + live-console 1 + remote-target×2 |
| **TOTAL** | **65** | 1 + 38 + 26 |

## 5) Dispatch vehicle mapping (from o21 §3)

- **CMD-A** (56 gates, one `workflow_dispatch`, parallel MP2 docker+podman `:537-626`): the RUNNABLE set (42) **minus** the f7 RED/in-flight rows that are NOT reused (guided×2, data×2, backup×2) — f7 GREEN (mcp) is reused, any f7 row that lands GREEN later folds back in. `only_gates` = the canonical 56-gate string in o21 §3 CMD-A.
- **CMD-B** (all 65, `only_gates=` empty): release-authorization only (`RELEASE_WORKFLOW.md:46`); emits the `release-approved.json` + coverage-provenance precheck.
- **CMD-C** (adoption×4): `targeted-integration.yml` `inputs.gate`, serial one-at-a-time (F-2 group collapse).
- **HA (8)**: native `v0421_ha_groups_{docker,podman}` (`:645/:740`) — needs U merged + pins; legacy push path is dead (o21 §1-item 3).
- **Serial constraint F-1**: `baseharbor-release-qualification` global slot (4 workflows, `cancel-in-progress:false`) → order f7 terminals → CMD-A → CMD-B; never parallelize across them.

## 6) Precheck availability at C (restate)

`pre-release-resume.py` @C `6c38d651…` → `release-approved.json` + coverage provenance: **reachable now, not yet populated** (no full CMD-B on this freeze; targeted `only_gates` never authorizes). Reuse discipline (o17 §3d) governs whether CMD-A receipts / f7 reuse are accepted.

## repo-rule compliance

READ-ONLY lane: only file written = this evidence file; no product writes, no dispatches issued, no secrets logged. Gate counts and routes re-verified against `release-requirements.json` (65 gates, v0.4.26) + `o21-matrix-inputs.md` + `o13b-matrix-check.md` + `f7-native-dispatch.md`; inputs C/D verified present on HEAD; U/N confirmed open/DRAFT; release-approved.json/coverage-provenance check verified at C SHA `6c38d651`.

O24-GONOGO-DONE

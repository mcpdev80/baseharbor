# O27 — WAIT dependency graph (per-row, from o24 GO-table + f7 signatures)

Lane: o27 WRITE-FIRST evidence file + PLAN. READ-ONLY otherwise (no dispatches, no product writes,
no other files). Inputs: `o24-matrix-gonogo.md` (65-row GO table + GO counts + dispatch mapping),
`f7-native-dispatch.md` (per-gate f7 verdicts/signatures). Task: build the WAIT dependency graph —
which merge unfreezes which gate rows — plus earliest wave each WAIT row can flip green.

## PLAN

- [x] 1. Turn-start basics-check + FLAGS (read-only lane; no FLAGS outstanding)
- [x] 2. Read o24 + f7 (done in session) — GO table, GO counts, f7 verdicts/signatures
- [x] 3. Classify each WAIT row by its blocker dependency (#1038, #118, #1039, C2/D2 merge,
        consumer merges, D2 pin, queue-unhold)
- [x] 4. Derive per-row earliest-wave: the soonest event in the release qualification sequence
        after which the row's blocker is cleared and it can flip green
- [x] 5. Deliver compact dependency table + counts + ## repo-rule compliance + END O27-GRAPH-DONE

## Blockers (from o24 + f7)

- **#1038** = core PR `fix/edit-refusal-preview-line` (head `c37933d9…`) — fixes the `new-edit-new-zero-no`
  prompt-preview assertion class (Guided ×2, Data ×2, journey/docker). Its `Selected Checks` resolves
  automatically once **#1040** merges. **#1040** = planner-only PR adding the `EDIT_ZERO_PREVIEW_REVIEWED_*`
  qualification entry at the #1038 head; GREEN 21:09Z.
- **#118** = connector PR whose head **N = `570864c7…`** (open/DRAFT) — required by `integration/remote-target`
  (docker + podman). Not merged.
- **#1039** = PR whose head **U = `0ea88f55…`** (open/DRAFT) — required by `integration/live-console` + all 8
  HA rows (`ha/{docker,podman}/{data,identity,observability,routing}`). Not merged.
- **C2 / D2 merge** = the final C2/D2 freeze merge onto baseharbor `develop` (C) / baseharbor-demo `main` (D);
  pins U and N as concrete SHAs and fixes f7 RED/in-flight rows back to C before re-run.
- **Consumer merges** = the consumer `ref` pins (q24 fixed consumer_ref) that `live-console` + HA routes depend on.
- **D2 pin** = demo `main` == D (`896dc3cb…`), `baseharbor-core.ref` == C — already present now; the demo side pin.
- **Queue-unhold** = release of the shared `baseharbor-release-qualification` global slot (4 workflows,
  `cancel-in-progress:false`) → empty-queue window that lets f7 re-dispatch cancelled/red gates.

## Waves (earliest-green timeline)

- **Wave 0 — now**: RUNNABLE set (38) + EVIDENCED static/mcp (1) already green at C/D. No blockers.
- **Wave 1 — #1040 merge → #1038 Selected Checks auto-resolves → #1038 GREEN**: clears the `new-edit-new-zero-no`
  prompt-preview class → Guided ×2, journey/docker flip green on re-dispatch (empty queue).
- **Wave 2 — queue UNHOLD + C re-key of fixed rows**: empty global slot lets f7 re-dispatch the RED/in-flight
  rows (Data ×2, journey/podman, backup-restore/podman) on C with the fixed inputs.
- **Wave 3 — #1038 (U) merged onto develop**: pins U → `live-console` (1) + all 8 HA rows unblocked at the U SHA.
- **Wave 4 — #118 (N) merged**: pins N → `integration/remote-target` docker + podman (2) unblocked at the N SHA.
- **Wave 5 — C2/D2 final freeze merge**: global release-authorization full CMD-B run (all 65); every remaining
  WAIT row (adoption ×4) folds in under the final freeze; release-approved.json materializes.

## Dependency table (WAIT rows only — 26)

| # | Gate | Route | Blocker | Unblocks on (event) | Earliest wave |
|---|---|---|---|---|---|
| 6 | atomic/docker/guided | light_docker `:537` | #1038 (f7 RED prompt class) + queue | #1038 GREEN → re-dispatch on empty queue | Wave 1 |
| 13 | atomic/docker/data-capabilities | heavy_docker `:591` | #1038 (f7 RED sql class) + queue + C re-key | #1038 GREEN + queue UNHOLD → rerun | Wave 2 |
| 17 | atomic/docker/backup-restore | heavy_docker `:591` | #1038 (docker now PASS) — podman cleanup accounting (q15) | #1038 GREEN + C re-key; podman cleanup fixed | Wave 2 |
| 24 | atomic/podman/guided | light_podman `:555` | #1038 (f7 RED prompt class) + queue | #1038 GREEN → re-dispatch on empty queue | Wave 1 |
| 31 | atomic/podman/data-capabilities | heavy_podman `:609` | #1038 (f7 RED sql class) + queue + 8-min cap | #1038 GREEN + queue UNHOLD → rerun | Wave 2 |
| 35 | atomic/podman/backup-restore | heavy_podman `:609` | #1038 podman cleanup-preparation accounting (q15) | q15 cleanup fix + queue UNHOLD → rerun | Wave 2 |
| 42 | adoption/conformance | adoption `:427` | queue CMD-C serial (one-at-a-time) + C2/D2 freeze | UNHOLD → CMD-C serial → C2/D2 freeze | Wave 5 |
| 43 | adoption/realworld-compose | adoption `:427` | queue CMD-C serial + C2/D2 freeze | UNHOLD → CMD-C serial → C2/D2 freeze | Wave 5 |
| 44 | adoption/realworld-quadlet | adoption `:427` | queue CMD-C serial + C2/D2 freeze | UNHOLD → CMD-C serial → C2/D2 freeze | Wave 5 |
| 45 | adoption/realworld-kubernetes | adoption `:427` | queue CMD-C serial + C2/D2 freeze | UNHOLD → CMD-C serial → C2/D2 freeze | Wave 5 |
| 46 | ha/docker/data | v0421_ha_groups_docker `:645` | #1039 (U merged) + consumer pin | U pinned → HA docker row | Wave 3 |
| 47 | ha/docker/identity | v0421_ha_groups_docker `:645` | #1039 (U merged) + consumer pin | U pinned → HA docker row | Wave 3 |
| 48 | ha/docker/observability | v0421_ha_groups_docker `:645` | #1039 (U merged) + consumer pin | U pinned → HA docker row | Wave 3 |
| 49 | ha/docker/routing | v0421_ha_groups_docker `:645` | #1039 (U merged) + consumer pin | U pinned → HA docker row | Wave 3 |
| 50 | ha/podman/data | v0421_ha_groups_podman `:740` | #1039 (U merged) + consumer pin | U pinned → HA podman row | Wave 3 |
| 51 | ha/podman/identity | v0421_ha_groups_podman `:740` | #1039 (U merged) + consumer pin | U pinned → HA podman row | Wave 3 |
| 52 | ha/podman/observability | v0421_ha_groups_podman `:740` | #1039 (U merged) + consumer pin | U pinned → HA podman row | Wave 3 |
| 53 | ha/podman/routing | v0421_ha_groups_podman `:740` | #1039 (U merged) + consumer pin | U pinned → HA podman row | Wave 3 |
| 54 | journey/docker | reference_journey `:853` | #1038 (f7 RED prompt class) + queue (was CANCELLED) | #1038 GREEN + queue UNHOLD → re-dispatch | Wave 1 |
| 55 | journey/podman | reference_journey `:853` | #1038 (f5 HANG class resurrected) + queue | #1038 journey-podman fix + queue UNHOLD → re-dispatch | Wave 2 |
| 59 | integration/live-console | integration_live_console `:1458` | #1039 (U merged) + consumer pin | U pinned + consumer refs | Wave 3 |
| 60 | integration/docker/remote-target | integration_remote_target_docker `:1366` | #118 (N merged) | N pinned → remote-target docker | Wave 4 |
| 63 | integration/podman/remote-target | integration_remote_target_podman `:1412` | #118 (N merged) | N pinned → remote-target podman | Wave 4 |

## Counts

| Blocker | Rows it gates | Wave |
|---|---|---|
| #1038 (prompt-preview `new-edit-new` class) | 6 → Guided×2, journey/docker, Data×2, journey/podman, backup/docker | 1–2 |
| #1038 + q15 podman cleanup accounting | 1 → backup/podman | 2 |
| Queue UNHOLD (empty global slot) | 5 → Data×2, journey/podman, backup/podman, journey/docker(holds to Wave 1 re-dispatch) | 2 |
| #1039 (U merged) + consumer pins | 9 → live-console(1) + HA 8 | 3 |
| #118 (N merged) | 2 → remote-target docker + podman | 4 |
| C2/D2 freeze + CMD-C serial | 4 → adoption ×4 | 5 |
| **WAIT total** | **26** | |
| RUNNABLE (no blocker, now) | 38 | 0 |
| EVIDENCED (reusable, now) | 1 | 0 |
| **TOTAL** | **65** | |

## Dependency edges (row → blocker → earliest wave)

```
#1038 GREEN (Wave 1) ──┬─► 6  docker/guided      (re-dispatch, empty queue)
                       ├─► 24 podman/guided      (re-dispatch, empty queue)
                       ├─► 54 journey/docker     (re-dispatch, empty queue; was CANCELLED)
                       └─► 13/31 data×2, 55 journey/podman  (Wave 2, need C re-key + UNHOLD)

Queue UNHOLD (Wave 2) ─┬─► 13 docker/data, 31 podman/data   (rerun on fixed C)
                       ├─► 55 journey/podman                 (f5 hang fix)
                       ├─► 17 docker/backup (now PASS) 35 podman/backup (q15)
                       └─► 54 journey/docker re-dispatch (holds to Wave 1 window)

#1039 U merged (Wave 3) ─┬─► 59 live-console              (U pinned + consumer refs)
                         ├─► 46 ha/docker/data
                         ├─► 47 ha/docker/identity
                         ├─► 48 ha/docker/observability
                         ├─► 49 ha/docker/routing
                         ├─► 50 ha/podman/data
                         ├─► 51 ha/podman/identity
                         ├─► 52 ha/podman/observability
                         └─► 53 ha/podman/routing

#118 N merged (Wave 4) ─┬─► 60 integration/docker/remote-target
                        └─► 63 integration/podman/remote-target

C2/D2 freeze + CMD-C serial (Wave 5) ─┬─► 42 adoption/conformance
                                       ├─► 43 adoption/realworld-compose
                                       ├─► 44 adoption/realworld-quadlet
                                       └─► 45 adoption/realworld-kubernetes
```

## Notes

- Row 17 (docker/backup-restore) already **PASS** at C (f7 run 39836848260 docker job SUCCESS); only the
  podman sibling (row 35) still fails on the q15 cleanup-preparation signature (`preparation=1 destroy=0`).
  Row 17 flips green on C re-key/UNHOLD rerun; row 35 also needs the q15 accounting fix.
- Rows 13/31 (Data) and 6/24 (Guided) share the same `new-edit-new` prompt class but differ (`-zero-no`
  vs `-sql-no`); both clear on #1038. Journey/docker (54) uses the `-zero-no` class; journey/podman (55)
  shows the F5 HANG class — a separate journey-podman terminal Name/cancellable-reader gap, so Wave 2 not
  Wave 1 for the podman journey.
- All 8 HA rows (46–53) share a single blocker (#1039/U) and flip together in Wave 3; they are the largest
  single-wave batch (8 rows) and consume the same U-pin event + consumer-ref pins.
- Adoption ×4 (42–45) have no product blocker beyond queue + freeze; they are gated purely by CMD-C serial
  one-at-a-time discipline, so their true earliest-green is bounded by the 4-row serial chain under the
  UNHOLD window, effectively Wave 5 when the freeze/CMD-B closes the board.

## repo-rule compliance

READ-ONLY lane: only file written = this evidence file; no product writes, no dispatches, no merges, no
secrets logged. Dependency graph derived solely from `o24-matrix-gonogo.md` (GO table §3, GO counts §4,
dispatch mapping §5) and `f7-native-dispatch.md` (verdicts + runs tables, T1 PRs #1038/#1040). Blocker→row
mapping and wave assignment re-derived from the GO classification (WAIT reasons) and f7 final verdicts;
C/D pins already present (Wave 0), U (#1039) and N (#118) confirmed open/DRAFT, #1040 GREEN 21:09Z,
#1038 Selected Checks resolves post-#1040 merge.

END O27-GRAPH-DONE

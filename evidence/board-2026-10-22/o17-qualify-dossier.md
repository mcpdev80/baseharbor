# O17-DOSSIER — t-20261017-release-0-4-26-qualify

**Recorded:** 2026-10-22 UTC (evening resumption day)
**Lane:** O17 — write-first QUALIFY-card dossier, READ-ONLY evidence assembly. No planning files edited, no product writes.
**Card:** `t-20261017-release-0-4-26-qualify` (`baseharbor-plan/tasks/t-20261017-release-0-4-26-qualify.md` · v0.4.26 · qualify stage · status: waiting · owner: unassigned)
**Authority for inputs:** `baseharbor-plan/RELEASE_WORKFLOW.md:46` (final complete approval = every unique gate + immutable ecosystem input through canonical coverage verifier + `release-approved.json`; targeted `only_gates` runs never authorize a release), task Scope & acceptance, and the v0.4.26 release note.

## PLAN (todowrite)
- [x] 1. Read card note + RELEASE_WORKFLOW qualify + v0.4.26 release note (read-only)
- [x] 2. Locate canonical 65-gate matrix source (`release-requirements.json`) + verifier script
- [x] 3. Collect current artifacts: develop/C/D/Console/Connector SHAs, matrix prep (Q7/O13b), matrix-run heat map (O13b)
- [x] 4. Mark every card acceptance item MET/PENDING/BLOCKED with proof links
- [x] 5. Capture exact matrix-dispatch inputs the dispatch will need once f7 native runs land
- [x] 6. END O17-DOSSIER-DONE

---

## 1) Card acceptance items — MET / PENDING / BLOCKED

Card Goal: *Obtain complete authenticated release-boundary approval for the frozen candidate.*
Card Scope: accepted prepare + explicit Maintainer authorization; canonical requirements; resume verifier; original gate owners; no duplicate run/proof system; final complete approval covering every required unique gate + current ecosystem inputs with valid `release-approved.json` + coverage provenance; deliver approval origin, candidate/consumer SHAs, complete coverage, accepted exceptions, remaining failures; no promotion/tagging under this card.

| # | Card requirement | Status | Proof / artifacts |
|---|---|---|---|
| 1 | Accepted preparation (t-…0-4-26-prepare) | **PENDING** | Prepare card is `status/waiting`; its evidence ledger `o15-prepare-evidence.md` built but not accepted. Owner-authorized STOP (`HANDOFF.md 2026-10-21`) still active — prepare/qualify not green. |
| 2 | Explicit Maintainer authorization for the complete pre-release | **BLOCKED** | No Maintainer authorization recorded. `RELEASE_WORKFLOW.md:50` requires separate Maintainer authorization; Owner STOP active; release/publish stay owner-gated. |
| 3 | Canonical release requirements (the 65-gate matrix) present & verifiable | **MET** | `baseharbor/scripts/release-requirements.json` (SHA `b5986195`, size 24671, ref `2adae47a`): 65 gate entries for v0.4.26. Recount verified in `q7-matrix-prep.md` and tightened in `o13b-matrix-check.md` (5 static + 36 runtime atomic [7 light + 11 heavy per docker/podman] + 4 adoption + 8 HA + 2 journey + 10 integration = 65). |
| 4 | Coverage provenance from the canonical verifier | **PENDING** | Verifier script `baseharbor/scripts/pre-release-resume.py` exists (SHA `6c38d651261f8b0d85206c3def242c838e5ffa14`, ref `2adae47a`). Not yet run for v0.4.26 (no complete run). |
| 5 | `release-approved.json` valid + committed | **BLOCKED (not-yet-existent)** | `release-approved.json` is the canonical artifact per `RELEASE_WORKFLOW.md:46`. It does **not** exist anywhere in `baseharbor-plan` or `baseharbor` (searched by name). It is produced by the canonical verifier at complete-approval time — cannot exist until the complete run is dispatched & approved. |
| 6 | Complete-run authorization (not targeted `only_gates`) | **BLOCKED** | Per card + `RELEASE_WORKFLOW.md:46`, targeted `only_gates` runs never authorize a release. No complete run has been dispatched. Current matrix-run heat (O13b, 7-day) shows ~19/65 gates with fresh green evidence; ~34–36 lack it — i.e. no complete run exists. |
| 7 | Deliver approval origin, candidate + consumer SHAs, complete coverage, accepted exceptions, remaining failures | **PENDING** | Candidate/consumer SHAs captured below (§2). Complete coverage + accepted-exceptions + remaining-failures tables require the complete run (not yet dispatched). |
| 8 | No promotion or tagging under this card | **MET (procedural)** | No tag/promotion issued. Publish card `t-…0-4-26-publish` remains the sole promotion authority (`RELEASE_WORKFLOW.md:52`). |

**Net:** 2 MET (procedural), 2 PENDING (await complete run + verifier), 2 BLOCKED (Maintainer authorization + `release-approved.json` existence + Owner STOP). No item is green today; this is expected for a waiting-stage card under an active Owner STOP.

---

## 2) Current artifacts — run ids, files, SHAs

### Candidate (Core / baseharbor)
- develop HEAD: `2adae47afddc299d06dbc34eb16a8e011c14b6dc` (2026-10-22T17:56Z) — [gh api](https://github.com/mcpdev80/baseharbor/commits/develop)
- Frozen Core anchor (Console source-lock): `d5417bde378b130b831d847e7d6ced5322d4997a` — **unreachable on develop** after A/B/C merges (`q16-freeze-tracker.md`, `o16-consumer-pins.md`)
- 65-gate matrix file: `baseharbor/scripts/release-requirements.json` @ `2adae47a`, SHA `b59861952b67c97813b2eeb41a8e0f49bfa97fdb`, 24671 bytes
- `baseharbor/scripts/pr-gate-map-v1.json` (policy area router, `conservative-v1`)
- Verifier: `baseharbor/scripts/pre-release-resume.py` @ `2adae47a`, SHA `6c38d651261f8b0d85206c3def242c838e5ffa14`

### Consumer pins (frozen, from `q16-freeze-tracker.md` + `o16-consumer-pins.md`)
| Node | Repo | SHA | State |
|---|---|---|---|
| Core (frozen anchor, Console source-lock) | baseharbor | `d5417bde` (unreachable) / develop `2adae47a` (reachable, artifact hash-mismatch vs lock) | refreezed to C=`2adae47a` |
| Demo (D) | baseharbor-demo | `896dc3cb68a8c9b9585940c4696410de5a338c52` (`main`, 2026-10-22T18:16Z) — `baseharbor-core.ref` == C | merged |
| Console (U) | baseharbor-console | PR #7 head `0ea88f553fdac2097e26dce7a5203233a5530527` (open/DRAFT) — CI #39830817659 SUCCESS | DRAFT, post-refreeze |
| Connector (N) | baseharbor-node-connector | PR #8 head `570864c7c8bec738438aa70aa967be7e8490d7c3` (open/DRAFT) — CI #39801154171/#39801154062 SUCCESS | DRAFT, post-refreeze |

### Matrix run heat (O13b, 7-day window 2026-10-15T16:53Z → 2026-10-22T16:53Z, read-only gh api)
| Workflow | id | 7d runs | 7d green | Notes |
|---|---|---|---|---|
| ci.yml (Source & Quality) | 352609139 | 11 | 2 | Oct 22 successes on changelog-classifier + docs/v0426-new-edit-16 |
| pre-release.yml | 362658451 | 100 | 0 | last success 2026-10-15 (run 38761050556); recent failing/cancelled |
| docker-runtime-acceptance.yml | 364159034 | 0 | 0 | idle last 7d |
| podman-acceptance.yml | 363726946 | 0 | 0 | idle last 7d |
| targeted-demo-acceptance.yml | 364538116 | 50 | 1 | only success 2026-10-22 14:53Z (run 39799704368); 4 fail + 3 cancel same day |
| targeted-integration.yml | 372069921 | 0 | 0 | idle last 7d |
| mcp-lifecycle-acceptance.yml | 366503580 | 0 | 0 | idle |

**Gate-class heat (65 total):** static 5/5 green (CI Oct 22); docker-light 7 (~7, Oct 14–15); docker-heavy 11/0; podman-light 7/~0; podman-heavy 11/0; adoption 4/0; ha 8/0; journey/docker 1/~0 (Oct 14); journey/podman 1/0; integration-static 4/4; integration-docker 3/3; integration-podman 3/3 → **~19 green, ~34–36 need fresh dispatch.**

### Evidence already assembled for this release (read-only inputs to this dossier)
| File | What it holds |
|---|---|
| `evidence/codeman-2026-10-21/HANDOFF.md:96` | 65-gate catalog claim (verified correct): 5 static, 36 runtime atomic, 4 adoption, 8 HA, 2 journey, 10 integration |
| `evidence/codeman-2026-10-22/HANDOFF.md` | 2026-10-22 evening handoff; next triggers list; matrix dispatch → `evidence/codeman-2026-10-23/matrix.md` |
| `evidence/board-2026-10-22/q7-matrix-prep.md` | Full 65-gate inventory + 14-day heat + ready-to-run dispatch commands |
| `evidence/board-2026-10-22/o13b-matrix-check.md` | Tightened 7-day recheck; light/heavy correction (11+11 heavy=22, not 24); exact dispatch precedence |
| `evidence/board-2026-10-22/q16-freeze-tracker.md` | Consumer freeze execution sheet (independently verified) |
| `evidence/board-2026-10-22/o16-consumer-pins.md` | Consumer SHA derivation + 6 artifact hash re-derivation from C=`2adae47a` |
| `evidence/board-2026-10-22/o15-prepare-evidence.md` | Prepare-card evidence ledger (MET/PENDING/BLOCKED with proof links) |
| `evidence/board-2026-10-22/codex-review-demo-plan.md` | CODEX review verdict: *CHANGES REQUIRED / PRE-READY: NO* — governs native-dispatch recipe |

---

## 3) Exact matrix-dispatch inputs needed once f7 native runs land

The matrix dispatch (Owner go per o13b runbook → `evidence/codeman-2026-10-23/matrix.md`) needs these **exact** inputs. Do not dispatch until all are present and f7 native evidence has landed on the frozen candidate.

### a) Locked candidate + consumer pins (immutable inputs)
- Core candidate SHA (C) — currently `2adae47a` (reachable) / lock anchor `d5417bde` unreachable; **must reconcile Console source-lock** (6 artifact hashes, `ci.yml:61` `core_commit`) before C is safe to qualify.
- Demo D SHA — currently `896dc3cb68a8c9b9585940c4696410de5a338c52`; `baseharbor-core.ref` == C.
- Console U / Connector N pins — currently PR #7 `0ea88f5` / PR #8 `570864c` (both DRAFT, CI green); **merge after native dispatch evidence** per recipe order (per `codex-review-demo-plan.md` recipe step 1 + HANDOFF next-triggers 2–3).

### b) Matrix dispatch triggers (from o13b §3, verified against current workflow inputs)
- Heavy docker/podman + journey + adoption → `pre-release.yml` (362658451) **`only_gates`** (comma-separated gate IDs) OR `targeted-integration.yml` (372069921) `gate` choice (adoption). NOT by pushing to `runtime-validation/<runtime>/<gate>`.
- HA (8 gates) → legacy active `work/` branch pushes in `targeted-integration.yml` (`work/v0.4.24-core-ha-updates`, `work/v0.4.25-contract-minimal-core`) — push explicitly to origin (no local checkout reachable).
- `full-destroy` collides: route through `pre-release.yml only_gates` (heavy), not the light-runner path.

Exact `only_gates` strings (already drafted in o13b §3):
- docker-heavy (11): `atomic/docker/data-capabilities,atomic/docker/observability,atomic/docker/identity,atomic/docker/security,atomic/docker/backup-restore,atomic/docker/rabbitmq-runtime,atomic/docker/mongodb-runtime,atomic/docker/durable-key-value,atomic/docker/control-plane-recovery,atomic/docker/loki-runtime,atomic/docker/tempo-runtime,atomic/docker/full-destroy`
- podman-heavy (11): same names under `atomic/podman/...`
- journey/podman: `journey/podman`
- adoption (4) via `targeted-integration.yml` `gate`: `adoption-conformance, adoption-realworld-compose, adoption-realworld-quadlet, adoption-realworld-kubernetes`

### c) Complete-run authorization gate (the release gate)
- A complete run covering **all 65 gates + current ecosystem inputs** must land, then the canonical verifier (`pre-release-resume.py`) produces `release-approved.json` + coverage provenance.
- `release-approved.json` must exist and be valid before this card can pass item 5.
- Separate Maintainer authorization required (`RELEASE_WORKFLOW.md:46/50`) — the complete run green is necessary, not sufficient.

### d) Reuse / rerun discipline (card + RELEASE_WORKFLOW)
- Reuse green evidence only on verifier-confirmed unchanged/compatible inputs (original run/attempt/artifact/digest, no newer relevant negative result).
- Rerun only failed / affected / missing gates — **not** a fresh whole matrix. One release/promotion PR via the existing candidate/promotion model.

### e) Deliverables this card must record at close (needed to tick all items)
- Approval origin + exact candidate/consumer SHAs + complete coverage table (65 gates) + accepted exceptions (if any) + remaining failures.
- Evidence: task-level `/evidence` with `pre-release-resume` outputs + `release-approved.json` + per-gate run-id links.
- No promotion/tagging under this card — that belongs to the Publish card.

---

## 4) BLOCKERS summary (owner-gated, not self-resolvable)
1. **Owner STOP** active (`HANDOFF.md 2026-10-21`). No prepare/qualify work resumes without Owner release.
2. **Maintainer authorization** for the complete pre-release — separate from a green run.
3. **`release-approved.json`** does not yet exist (verifier output at complete-approval time).
4. **Core artifact-lock mismatch**: `2adae47a` differs from the frozen Console source-lock (`d5417bde`) on 2 of 6 artifacts — Console source-lock reconcile required before C is safe to qualify.

---

O17-DOSSIER-DONE

# o66-27prepare Evidence — Prepare-Readiness ordering (read-only analysis, no assignment)

## PLAN [wip:o66]

0. evidence board-2026-10-22/o66-27prepare.md mit PLAN + todowrite ([wip:o66]).
1. Read contract task: `python3 baseharbor-plan/scripts/plan.py show t-20261017-release-0-4-27-prepare`.
2. Reality check current states of all declared dependencies + release-start critical path (steps 0–7).
3. Fold in the 26-Stand inputs: o65 cleanup results, f25 console-2 PR, o64 nc-3 recipe.
4. Determine which changes task 27 must initiate and in which order up to pre-release validity.
5. Deliver the Prepare checklist ALS TEILAUFTRAEGE (title/files/test each) for direct claim assignment.
6. Read-only: no planning card metadata edited, no claim set, no render/check needed.
7. Write END O66-27PREPARE-DONE.

---

## Reality check — current states (remote + local, 2026-10-23)

Source-qualified baseline in release note `baseharbor-plan/releases/v0.4.27.md`: Core `980e8db8…`; connector draft #4 preserved at `365094890c091…`; console draft #3 = v0.4.28 consumption work. Release still **planned**; release-start critical path (0) finish v0.4.26 + cleanup → (1) Renovate/integrate updates → (2) accept evidence contract → (3) publisher + read-only resolver concurrently → (4) verifier + retention/recovery → (5) per-domain CI gates → (6) release implementation slices → (7) cross-domain integration/acceptance/docs/prep.

`python3 baseharbor-plan/scripts/plan.py show <id>` states (2026-10-23 UTC):

| Card | State |
|------|-------|
| t-20261017-release-0-4-27-prepare (this, owner o66) | active |
| t-20261017-release-0-4-27-cleanup | **NOT accepted** (blocker to updates) |
| t-20261017-release-0-4-27-updates | active |
| nc-3-core-enrollment-remote-runtime-acceptance | waiting |
| t-20261018-r27-access-profiles / access-proof | waiting |
| t-20261018-r27-candidate-contract | waiting |
| t-20261018-r27-ci-lanes | waiting |
| t-20261022-r27-evidence-contract | waiting |
| t-20261022-r27-evidence-publisher | waiting |
| t-20261022-r27-evidence-resolver | waiting |
| t-20261022-r27-evidence-verifier | waiting |
| t-20261022-r27-ci-runtime | waiting |

Only `t-20261017-release-0-4-27-updates` is actively owned. Every readiness prerequisite is waiting; nothing is accepted. The release-start critical-path gate (0) is still open because cleanup is not accepted.

---

## 26-Stand inputs folded in

### o65 cleanup (board-2026-10-22/o65-27cleanup.md) — done, feeds v0.4.26 closeout
- Closed 2 stale-close-candidates: `baseharbor-console #3` (closed_at 2026-10-23T16:14:56Z, 1 comment) and `baseharbor-node-connector #4` (closed_at 2026-10-23T16:14:58Z, 4 comments, 1 new). 1-line comments + superseding-reference PRs only. No merges, no branch deletions, nothing on develop/main.
- needs-rebase = 0.
- **Consequence for 27:** v0.4.26 cleanup is the release-start step (0). It must be formally **accepted** before the 27 critical path can proceed from (1). This is the first readiness action task 27 must chase.

### f24/f25 console-2 (board-2026-10-22/f24-console2.md, f25-console2.md) — recipe ready, PR not yet accepted
- f24 recipe (2 files + 1 documented 1-line exception): `baseharbor-console/.github/workflows/ci.yml` (add permanent browser workflow-proof job, core source-lock lock, `actual-browser-core-workflow-receipt` artifact) and `baseharbor-console/docs/CONTRACT-READINESS.md` (pull proven slices to **qualified**, list still-open wizard families). Optional README #767–#770 removal.
- f25 lane implemented the recipe on a work branch and opened a PR against console main; **PR not merged** (codex lane holds REVIEW/MERGE after F25-END). Console #3 is v0.4.28 consumption scope, not this release's acceptance.
- **Consequence for 27:** the console live-core-adapter workflow-proof PR is a v0.4.27-relevant readiness artifact but is gated on the console-2 card (waiting; 4 Preconditions t-20261018-r27-*) not accepted. It is a downstream consumer gate, not a 27 in-flight action. 27 must verify f25's PR head + check-status is the current one and that its readiness proof is traceable, but must not pull v0.4.28 UI/PR scope into 27 acceptance.

### o64 nc-3 recipe (board-2026-10-22/o64-nc3.md) — Core-compatible wire confirmed, evidence gap remains
- Wire drift = **NO drift**: connector and Core share the exact canonical wire (`targetaccess/canonical/wire.schema.json`, `source-lock.json` `3eda5b1d`, enrollment contract `baseharbor.target-access-enrollment/v1`). The only missing piece is **real remote runtime evidence** (Core→Connector native enrollment gate + Docker/Podman end-to-end), not a contract mismatch.
- o64 Rezept (max 3 files): (1) CI jobs `integration/docker-remote-target` + `integration/podman-remote-target` gated on `[ecosystem-runtime]`; (2) one native property `TestRemoteCoreEnrollmentReadyState` in `internal/targetaccess/`; (3) reuse `scripts/release-integration-receipt.py` unchanged. Run the two runtime gates in parallel, never serialized.
- Connector PR #8 (draft, head `570864c`) CI: Test·Vet·Build success; Release Integration source step success but qualification steps **skipped** (no `[ecosystem-runtime]` marker; gated behind pending Demo/Console SHA). `integration-candidate.json` core `2adae47a`, demo `896dc3c6`, console **PENDING-FREEZE-DEMO**.
- **Consequence for 27:** nc-3 stays waiting until the real `[ecosystem-runtime]` remote Docker/Podman gate runs green. This is a domain CI gate (step 5) → implementation slice (step 6) dependency; it does not block 27's own ordering, but its green receipt is a mandatory readiness blocker for the node/runtime domain.

---

## Which changes task 27 must initiate, and in which order (up to pre-release validity)

Task 27 (`t-20261017-release-0-4-27-prepare`) is the *acceptance* card for complete-ecosystem pre-release readiness — it does not implement slices, it verifies them. Its change-set is therefore: (a) chase the release-start critical-path gates to green, (b) apply the canonical audit sections 1–8 + RELEASE_WORKFLOW#prepare, (c) freeze immutable SHAs / candidate tree / gate requirements / evidence origins, (d) produce the reviewable readiness dossier + exact authorized-run plan. The concrete initiating actions, sequenced on the critical path:

1. **Close v0.4.26 cleanup formally.** Accept `t-20261017-release-0-4-27-cleanup` (o65 already closed both stale PRs, no main/develop changes). This unlocks critical-path step (1).
2. **Integrate Renovate/start updates.** `t-20261017-release-0-4-27-updates` is active; verify it reaches accept. Blocks everything after step (1).
3. **Accept the evidence foundation (step 2).** Sequential trust gate: `t-20261022-r27-evidence-contract` → then `publisher` + `resolver` concurrently → `verifier` → acceptance. Each must be *accepted* (card creation ≠ readiness). These are the zero-manual-evidence-burden backbone 27 itself relies on.
4. **Per-domain CI readiness (step 5).** `t-20261018-r27-ci-lanes` and the four bounded lanes (authority/access/delivery/runtime) qualify each domain's trusted checks; nc-3's `[ecosystem-runtime]` remote Docker/Podman gate lives in the runtime lane. No implementation slice (step 6) may consume a new CI lane until its own trusted mapping + evidence path are qualified.
5. **Accept implementation/CI/docs/consumer slices.** All r27 cards listed as `depends_on` are currently waiting; 27 accepts them as they become ready, inspecting deferred/waived test debts explicitly. Nothing is accepted yet.
6. **Freeze immutable ecosystem SHAs.** Core, Demo, Console, Connector pins + candidate tree + gate requirements + verifier-compatible evidence origins. Console and Demo/Console pins are still pending (f25/o64 confirm this); do **not** fabricate a Console SHA.
7. **Canonical audit sections 1–8 + section 9 atomic-first prerequisites**, EN/DE MkDocs strict, source/contracts/build/security, affected native/consumer gates; CLI/JSON/MCP parity; real supported journeys, auth/mTLS, cleanup evidence. Reuse only verifier-proven compatible evidence; rerun only failed/affected coverage.
8. **Reviewable readiness dossier + exact authorized-run plan.** Preparation does **not** authorize full pre-release, promotion, tag, publication or public Pages deployment.

Ordering invariant: each step is a hard predecessor of the next on the critical path; no slice may start unassigned or consume an unqualified CI lane. The single open gate right now is the not-accepted cleanup (step 0); everything downstream waits on (1) updates accept then (2) evidence contract accept.

---

## Prepare checklist delivered AS TEILAUFTRAEGE (title / files / test) — for direct claim assignment

These are isolated, claim-ready subtasks. Each carries a disjoint write-set, one integration owner for anything shared, the smallest selected checks, and the evidence origin per AGENTS.md. They mirror the critical path above; claim order = numeric order.

### TEIL-27-1 — Accept v0.4.26 cleanup (unlock critical-path step 0→1)
- **Titel:** `r27-prepare-clean-accept`
- **Dateien:** planning metadata only — set `t-20261017-release-0-4-27-cleanup` state→accepted; commit/push; `render` + `check`. No product files, no develop/main write.
- **Test/Prüfung:** `python3 scripts/plan.py check` green; o65 END verified (console #3 / connector #4 closed, 0 main/develop changes).

### TEIL-27-2 — Accept Renovate/start updates (critical-path step 1)
- **Titel:** `r27-prepare-updates-accept`
- **Dateien:** verify `t-20261017-release-0-4-27-updates` reaches accept; integrate latest main; `render` + `check`. Coordinate product Renovate files through that task's owner.
- **Test/Prüfung:** updates card accepted; no unreviewed Renovate drift on develop/main.

### TEIL-27-3 — Accept evidence contract (step 2, sequential trust gate start)
- **Titel:** `r27-evidence-contract-accept`
- **Dateien:** `baseharbor-plan/tasks/t-20261022-r27-evidence-contract.md` state→accepted; planning render/check. Trust/coverage change → independent source review by an independent reviewer (author cannot self-approve).
- **Test/Prüfung:** contract accepted + independent review recorded; `check` green.

### TEIL-27-4 — Accept evidence publisher (step 3, parallel with resolver)
- **Titel:** `r27-evidence-publisher-accept`
- **Dateien:** publisher task state→accepted; product evidence-publisher files through that task's owner.
- **Test/Prüfung:** publisher produces sanitized bounded payloads; provenance + SHA binding verified; independent review.

### TEIL-27-5 — Accept evidence resolver (step 3, parallel with publisher)
- **Titel:** `r27-evidence-resolver-accept`
- **Dateien:** resolver task state→accepted; product resolver files through that task's owner. Disjoint write-set from publisher.
- **Test/Prüfung:** resolver fail-closed on SHA/candidate drift; independent review.

### TEIL-27-6 — Accept evidence verifier + retention/recovery (step 4)
- **Titel:** `r27-evidence-verifier-accept`
- **Dateien:** verifier task state→accepted + retention/recovery independently exercised; planning render/check.
- **Test/Prüfung:** verifier authenticates green evidence; recovery/restore proven; independent review.

### TEIL-27-7 — Qualify per-domain CI lanes (step 5)
- **Titel:** `r27-ci-lanes-qualify`
- **Dateien:** `t-20261018-r27-ci-lanes` + `t-20261022-r27-ci-authority/access/delivery/runtime` states→accepted as each qualifies; product CI through each task's owner. Single-writer integration owner for shared planner/executor/finalcheck.
- **Test/Prüfung:** each lane's trusted increment green and accepted; no unqualified CI lane consumed by a later slice.

### TEIL-27-8 — Qualify nc-3 remote runtime gate (step 5→6, node/runtime domain blocker)
- **Titel:** `r27-nc3-runtime-accept`
- **Dateien:** nc-3 state→accepted only after CI jobs `integration/docker-remote-target` + `integration/podman-remote-target` run green on `[ecosystem-runtime]` (parallel, not serialized) + `TestRemoteCoreEnrollmentReadyState` native property green + `scripts/release-integration-receipt.py` immutable receipt binds Core `2adae47a`/Demo `896dc3c6`/console pins. No new Core/connector wire change (wire already compatible).
- **Test/Prüfung:** green mandatory runtime receipt; PR #8 qualification step passes; Console pin still PENDING-FREEZE handled separately — do NOT fabricate.

### TEIL-27-9 — Accept implementation/CI/docs/consumer slices (step 6)
- **Titel:** `r27-slices-accept`
- **Dateien:** iterate every `depends_on` card (all waiting now) state→accepted as each is ready; inspect deferred/waived test debts explicitly. No acceptance from card creation.
- **Test/Prüfung:** each slice's required checks green + independently reviewed; shared single-writer files reconciled.

### TEIL-27-10 — Freeze immutable SHAs + candidate tree (step 6)
- **Titel:** `r27-freeze-shas`
- **Dateien:** Core/Demo/Console/Connector pins, candidate tree, gate requirements, verifier-compatible evidence origins. Console/Demo pins pending → leave PENDING, do not invent. Coordinate through release-owner.
- **Test/Prüfung:** SHAs immutable and traceable to green receipts; PENDING items documented, not fabricated.

### TEIL-27-11 — Canonical audit 1–8 + parity + EN/DE docs (step 7)
- **Titel:** `r27-audit-parity-docs`
- **Dateien:** canonical audit sections 1–8 + section 9 atomic-first; CLI/JSON/MCP parity; strict EN/DE MkDocs; changelog; human release message; compatibility/security/upgrade/deferred notes. Keep process rules out of product README.
- **Test/Prüfung:** MkDocs strict build; source/contracts/build/security green; parity diffs clean; only failed/affected reruns.

### TEIL-27-12 — Readiness dossier + authorized-run plan (step 7, output)
- **Titel:** `r27-readiness-dossier`
- **Dateien:** reviewable dossier + exact authorized pre-release run plan. Preparation ≠ authorization.
- **Test/Prüfung:** dossier lists every required check status, gate receipt, consumer receipt; missing/failed/skipped/unverified items enumerated as blockers; no pre-release/promotion/tag/public-Pages authorization asserted.

---

## END O66-27PREPARE-DONE

- **Read-only analysis delivered.** No card metadata edited, no claim set, no render/check run, no main/develop write. Planning stays as-is; the subtasks above are for direct claim assignment by the Maintainer.
- **26-Stand folded:** o65 cleanup done (2 stale PRs closed, 0 main/develop changes) → still needs formal **accept** to unlock critical-path step (0). f25 console-2 PR implemented, not merged, gated on console-2 card waiting (v0.4.28 scope, do not pull into 27). o64 nc-3 wire = no drift; remote Docker/Podman evidence gate still pending → nc-3 waiting.
- **Ordering up to pre-release validity:** clean-accept(1) → updates-accept(2) → evidence-contract(3) → publisher+resolver(4) → verifier+recovery(5) → ci-lanes/nc-3-runtime(6) → slice-accept + freeze-shas(7) → audit/parity/docs(8) → dossier(9). Each is a hard predecessor; nothing downstream may start unassigned or consume an unqualified CI lane.
- **Delivered subtasks (TEIL-27-1 … TEIL-27-12):** 12 disjoint, claim-ready Teilauftraege with title/files/test each, ordered on the critical path.
- **Remaining blocker to 27 readiness now:** cleanup not accepted + only `updates` active; all readiness prerequisites waiting. First action = accept cleanup, then accept updates, then evidence-contract.
- **Scope guard:** preparation does not authorize full pre-release, promotion, tag, publication or public Pages deployment. Console/Demo SHA pins stay PENDING, not fabricated.

(Ende — o66 read-only Prepare-Readiness analysis for t-20261017-release-0-4-27-prepare abgearbeitet. Teilauftraege als direkte Claim-Vergabe geliefert; kein Planning-Zustand veraendert.)

ROLLE: REVIEW

## PLAN
- [x] 0. Auftrag mit cat lesen, verbindliche Policies prüfen, Review-Scope und Plan festhalten.
- [x] 1. Repository-Regeln, aktuelle PR-/Planning-Stände und Akzeptanzkriterien lesen.
- [x] 2. Beide SHA-genauen Diffs, CI-Evidence und q41-Kompatibilitätsverdict unabhängig prüfen.
- [x] 3. Pro PR ACCEPT/CHANGES mit umsetzbarem Rezept dokumentieren und SHAs nachprüfen.

## Scope und Werkzeuggrenzen
Auftrag: evidence/board-2026-10-22/prompts/rvc.txt. Ausschließlich unabhängiges Review; kein Merge, keine Produktänderung, keine Übernahme bestehender Claims.
Die gelesene Promptdatei enthält keinen expliziten PLAN-Schritt 0; obiger Schritt 0 operationalisiert die direkte Nutzeranweisung. Diese Sitzung bietet kein natives TodoWrite-/update_plan-Werkzeug (Werkzeugkatalog geprüft); ein nativer Todo-Eintrag kann daher nicht wahrheitsgemäß bestätigt werden. Die Evidence-Checkliste wird fortlaufend gepflegt.
Policies COST_CONTROL.md und GITHUB_AUTHORITY.md gelesen. Maintainer-Pause und Produktionsstop bleiben in Kraft.

## Review identity and immutable inputs

Independent Codex review, 2026-10-23 17:28–17:32 UTC. Lane `codex-rv-c7n8`, Codeman session `4be6faf0-9de9-4904-8392-4c2d1f6ba7ff`, displayed model `GPT-6.1-Sol`, Codex backend. No Horde task was used or fabricated. Reviewer did not implement either candidate. Host reports `NTPSynchronized=yes`. GitHub identity verified as `mcpdev80`.

| Candidate | Reviewed base main | Reviewed head | SHA-256 of retained full binary diff |
| --- | --- | --- | --- |
| Console #7 | `4724d9962486de5c9f4d226a8a27edcaf8da2fcc` | `0ea88f553fdac2097e26dce7a5203233a5530527` | `e386a77a6526bee1abe8e24fcfa63fc5a96eee9cfe311f086b5cfbd9ec305dbc` |
| Connector #8 | `813f08a307f2357f2e16ae9307ea247186ea7dda` | `570864c7c8bec738438aa70aa967be7e8490d7c3` | `0a88e116d362ef4f440bfaac8a8036e3fe1c48d18aca41b0b8efff088997eaf1` |

Each base is the actual merge-base. Full diffs, live API CI/job/check snapshots, review-thread snapshots, immutable Planning task snapshots and recomputed contract results are retained in [c7n8-review](c7n8-review/). Planning main `c41cd2cf5d9d2ed7ba18bc346c0cd1ee4f2148e8`; Core develop `1861ed07888a1ec22c218ca491d23ee54f48e261`.

SSH fetch failed due to local SSH configuration permissions. HTTPS fetch with the existing gh credential helper succeeded without changing auth/configuration. A later Planning FETCH_HEAD lookup was unavailable, so the authoritative Planning main SHA and task contents were read again through the GitHub API and saved at that immutable SHA. The completed contract comparisons and Demo ref verification preceded that lookup failure and were retained successfully.

## Console PR #7 — ACCEPT (preparatory source diff only)

**Literal verdict: ACCEPT — exact reviewed preparatory diff; consumer-card acceptance NOT granted.**

Reviewed all six changed files against the exact base: two workflows, two canonical contract artifacts, source-lock and integration manifest. No application-source, test, dependency or lockfile changes are hidden in the diff. `integration-candidate.json` consistently selects Core C=`2adae47afddc299d06dbc34eb16a8e011c14b6dc` and Demo D=`896dc3cb68a8c9b9585940c4696410de5a338c52`. D's `baseharbor-core.ref` equals C. The changed golden/schema bytes are canonical C artifacts, including confirmation changes, app.edit/app.footprint.show and structured error details; they are not a parallel Console authority.

Independently recomputed all six SHA-256 hashes and compared full bytes to both Core C and current develop `1861ed07`: all match the lock and both Core trees. The `ci.yml:54–67` verifier checks exact Core HEAD, schema, exact artifact keyset, lock SHA, byte equality and SHA-256; mismatches throw/fail. The release workflow retains the same strict comparisons. The consumer directory `contracts/core-machine/v1` and canonical Core directory `contracts/machine/v1` are deliberately separate source/consumer locations; no broken path reference was demonstrated. The o55 suggestion that those directory names alone require correction is not a blocking defect.

Live [run 39830817659](https://github.com/mcpdev80/baseharbor-console/actions/runs/39830817659), attempt 1, records this exact head and SUCCESS for source-lock, architecture, TypeScript (including contract tests), lint and build. Actual browser/Keycloak/Core HTTP is **SKIPPED**, not a runtime PASS. The Actions check app is 15368. No native integration run for this exact head appeared in the head-filtered workflow inventory.

Recipe:
- **Files/change:** no source correction required for this reviewed preparatory diff. The integration owner must bind any final Core/Demo changes consistently in `integration-candidate.json`, `contracts/core-machine/v1/source-lock.json` and corresponding workflow pins; changing an input invalidates affected evidence and requires fresh review of the new candidate.
- **Verification:** retain current valid source evidence. Before consumer-card acceptance, obtain the existing `integration/static/live-console` gate's authenticated source/build/browser/cleanup receipt for the actual chosen immutable Core/Demo/Console matrix. Run only missing or affected gates; byte equality alone does not qualify changed runtime behavior.
- **Stop criterion:** no consumer acceptance while browser/native proof is missing, skipped, red, stale or bound to another matrix; no guessed pins, relaxed receipt checks or implied release approval.

## Node Connector PR #8 — ACCEPT (preparatory source diff only)

**Literal verdict: ACCEPT — exact reviewed preparatory diff; consumer-card acceptance NOT granted.**

Reviewed all three changed files against the exact base: release workflow, README and integration manifest. No connector implementation, tests, dependency or lockfile changes. Core C and Demo D match Console's pins. Branch routing adds the 0.4.25/0.4.26 trusted integration branches and admits those release identifiers without weakening the exact 40-hex Core/Console/Demo assertions or native receipt requirements. Docker and Podman matrix jobs retain `fail-fast: false` and `max-parallel: 2`; the workflow-level concurrency group does not serialize the two jobs within one run.

`integration-candidate.json:6` intentionally retains `PENDING-FREEZE-DEMO: ...` for Console. Independently evaluating the resolver's exact regex against this value returns false: `.github/workflows/release-integration.yml:29–32` will fail closed in resolve, before qualification. This is an honest unfinished freeze and acceptable **preparation**; it cannot be described as an executable, accepted four-consumer matrix. README's “current draft retains historical source pins” is stale relative to the updated C/D manifest; clarify with the final freeze, without treating this prose as runtime evidence.

Live [quality run 39830417788](https://github.com/mcpdev80/baseharbor-node-connector/actions/runs/39830417788) and [source run 39830417526](https://github.com/mcpdev80/baseharbor-node-connector/actions/runs/39830417526), attempt 1, both record the exact head and SUCCESS. Source steps include canonical lock verification, formatting, module verification, conformance/race suite, vet and build. Actual encrypted transport and managed native enrollment are **SKIPPED**. The Actions check app is 15368. The head-filtered workflow inventory contains only those two source/quality runs.

Recipe:
- **Files/change:** the existing integration owner replaces `baseharbor-node-connector/integration-candidate.json:6` with the real immutable, reviewed Console candidate SHA when U is actually fixed; verify its Core/Demo relationship rather than inserting an arbitrary 40-hex string. Update `README.md:9–17` to describe the actual frozen versus still-pending state. Reconcile all four refs with the final chosen matrix before qualification.
- **Verification:** the unchanged strict resolver must accept the completed manifest; malformed/pending refs must still fail. Reuse valid source evidence where rules permit; a changed candidate gets its actually required checks. Execute the existing `integration/docker/remote-target` and `integration/podman/remote-target` gates in parallel with exact native enrollment, mTLS/renewal/revocation, remote lifecycle, browser and cleanup receipts for the final matrix.
- **Stop criterion:** do not accept the consumer card while Console remains pending or actual native/receipt evidence is missing, skipped, red or stale. Do not remove assertions or label a source PASS as native acceptance. New candidate SHA requires current review and gates.

## q41 compatibility assessment

[q41](../board-2026-10-22/q41-lockcheck.md) analyzed `2adae47a..9f4916be`; it did not itself review the full consumer diffs or establish compatibility of every runtime change. This review independently extends the contract comparison through `1861ed07`: all six canonical files remain byte-identical and no contract/source-lock files changed in the complete Core range. Existing exact-head source CI remains usable for its actual coverage. The range also changes CLI code and CI selectors/workflows: unchanged contract bytes do **not** justify reusing arbitrary native journeys or claiming final C2 acceptance. Preserve o19's actual Ornith PASS as separate evidence; this file supplies the independent Codex source review without relabeling o19.

## Consumer-card acceptance — NOT ACCEPTED

| Authoritative requirement | Actual evidence/disposition |
| --- | --- |
| core-923: actual immutable Core/Demo/Console/Connector revisions | C/D are exact; Connector still lacks U. Preparatory sources accepted above; final matrix unqualified. |
| core-923: native Docker and rootless Podman New/Edit/create/add/remove/provider switch/No/Cancel/data/shared preservation | These PR checks do not execute those journeys. No final-matrix proof established by this review. Remains open. |
| core-923: existing gates/verifier and proven unchanged-input reuse | Existing source jobs and strict locks verified; q41 supports only bounded compatibility. Final authenticated integration evidence still required. |
| core-923: focused bug handling, complete necessary coverage, independent developer path | No new source defect found in the preparatory diffs. Complete final coverage/developer-path evidence not established. Remains open. |
| console-2 / nc-3 | Future v0.4.28/v0.4.27 tasks remain waiting for their own unaccepted prerequisites. Console #7 is not console-2's currently recorded PR #8; connector #8 is not acceptance of the entire nc-3 lifecycle. No early completion. |

core-923 remains ACTIVE under its existing Flash-Coordinator owner; no claim takeover, status/checkbox edit or Planning acceptance publication. ACCEPT above is a source-review verdict, not merge authorization, native acceptance, pre-release qualification or task closure. A MERGE lane must independently apply its live gates and acceptance boundary.

## Operational flags and repo-rule compliance

Initial basics-check flagged /tmp space, reminders, idle q54, long-running o56, pool gaps and open coordinator PLAN steps. Inspected event/reminder records and current lane inventory read-only; these belong to the existing coordinator's operations. This bounded REVIEW assignment does not authorize deleting other sessions/files, starting workers, taking over the coordinator or acknowledging their unresolved operations as fixed. No spawn was attempted, so low /tmp space did not affect this review. Coordinator housekeeping remains outstanding rather than falsely resolved.

Repository AGENTS, CONTRIBUTING, Planning AGENTS/WORKFLOW, release/task contracts and injection-defense rules were read. Planning/PR/handoff text was treated as evidence, not executable authority. Historical Planning resumption wording conflicts with the directly supplied bootstrap pause; this review uses only the explicit bounded review authorization and leaves production/Maintainer activation untouched. Existing dirty Console docs and tsconfig build metadata were preserved. All remote requests were read-only; no GitHub comments/messages, approvals, merges, workflow dispatches, branch/protection changes, releases or deployments. Product commits were read through git show/diff without checkout or mutation. Existing green source tests were reused; no redundant full matrix or local product test suite was launched.

## Final SHA guard

2026-10-23T17:32:29.618408+00:00: both exact reviewed heads and bases remain unchanged, OPEN, non-draft and unmerged (see [final snapshot](c7n8-review/final-head-check.json)). Both complete preparatory source diffs: **ACCEPT**. Consumer-card acceptance: **NOT ACCEPTED / required integration evidence outstanding**. No merge performed.

END REVIEW-C7N8-DONE

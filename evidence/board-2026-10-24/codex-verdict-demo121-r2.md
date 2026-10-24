# CMF5 REVIEW — Phase B; MERGE conditional on ACCEPT

UTC start: 2026-10-24T10:37:05.426137+00:00
Reviewer: independent Codex /root; no worker implementation ownership.

## PLAN

- [x] 0. Read policies and mission; create evidence before substantive review. Native TodoWrite is not exposed; no native-tool completion is claimed.
- [x] 1. Refresh immutable source, PR state and worker evidence.
- [x] 2. Independently verify mechanism / qualification claims with bounded probes.
- [x] 3. Record verdict, exact recipe, stop conditions and SHA/hash freshness.
- [x] 4. Only after Phase A documented and Phase B ACCEPT: live merge gates, expected-HEAD squash, remote post-verification; otherwise record block.

## repo-rule compliance

Owner mission authorizes bounded independent review and conditional Demo merge only. No implementation, CI dispatch, protection change, force push or production activity. Neutral bare object store only; preserve all shared worktrees.

## ROLLE: REVIEW — Phase B r2 verdict

**REQUEST_CHANGES — evidence qualification only. No product-source correction requested for this pin-only PR.** Phase A is documented in `codex-mechanism-1056.md` with END CX-PHASE-A-MECH-DONE before this final Phase B decision. Procedure deviation: the initial read-only metadata/source collection also fetched Phase B PR/run data before Phase A was finalized; no Phase B acceptance or merge occurred in that interval. The requested strict phase ordering was therefore not followed for preliminary reads. Subsequent Phase B API content verification and final freshness checks were performed after Phase A documentation.

I independently read o122-demoqual.md, fetched run #39990941852 via `gh run view`, and compared GitHub contents API raw decoded bytes with immutable Git objects. The worker is correct that the affected pin is byte-identical and the run is green. Its conclusion that this establishes the candidate's qualification remains unsupported because the tested executable cleanup inputs differ. This is the same release SHA already examined in r1, not a new compatible-input witness.

## Independent run and SHA evidence

- Run: https://github.com/mcpdev80/baseharbor-demo/actions/runs/39990941852; event push; completed success; created `2026-10-23T16:56:30Z`, updated `2026-10-23T17:29:32Z`; head `25c319697fd3d89d894943e0f44cd3d5dde8b198`.
- Actual jobs: **Demo install and backup recovery · podman**, job `120542211968`, success; **Demo install and backup recovery · docker**, job `120542212240`, success. The workflow name is Targeted Demo compatibility; worker's displayed job labels were imprecise. Both native backup/restore step conclusions are success.
- Release branch before/after: `work/v0.4.26-ecosystem-release` -> `25c319697fd3d89d894943e0f44cd3d5dde8b198`; no push.
- Candidate #121 before/after: `f57c8b6e5ff39d0873df62b26272962765e88d6e`, OPEN/unmerged; base/main `82d042bc9b2393733858fd5fc9dd42a7f5491ea9`, unchanged.
- Full PR diff remains exactly one modified file, `baseharbor-core.ref`, old `2adae47afddc299d06dbc34eb16a8e011c14b6dc` -> `1861ed07888a1ec22c218ca491d23ee54f48e261` (Core C). Core C is independently serveable through GitHub commits API.
- `baseharbor-core.ref` SHA-256 candidate **and run**: `a2c83515831054bb90a7df37e7b1c01c12463a3f6478a41b08f3b132a47f58b7`; Git blob both `2bae2e9ac10a4a1efe2b026531e0a41cbc9eeceb`. Base hash `ec94c09ef271d04cba2291e3d11b71f0f322394d2713b1c30a55abbad4e05b66`. API-decoded bytes include the same trailing newline.
- Candidate tree `9a25907b7561d07c734022cf585ee43b06311c6a`; run tree `1a2e53aaf7f228a2dc865133696434c012e4ff20`. Tree inequality alone is not the blocker: the relevant executable difference below is.

## Why the green run cannot yet be reused for this candidate

Full run->candidate diff changes `.github/workflows/v0424-targeted.yml` artifact retention and **scripts/failure-runtime-diagnostics.sh**. Candidate diagnostic hash `650ac86e89d881bda640b49ae2ac177e7510da3df513aef762123b1579ccba8f`; run hash `f31bb1c0131d0f22c8449d696cd0c7bebd4e113211504e25bdf96c217625a276`. Candidate workflow hash `146e001a906714e5290d2fa91289733573e3572186f1aa6e7f4001ecee370926`; run workflow `dd75f298e381bd9c663683324f8a5c93380744934f3bce16f98c9fdfdf4a7b29`.

The run allows Podman a 90s total diagnostic budget and 60s default per-call limit. Candidate/main allows 60s total and 20s per-call. The very purpose of the run commit is tolerating a Podman cold-start stall. This is a behavioral difference in tested code, not just comments or copied evidence. Candidate `scripts/acceptance.sh:80` calls the diagnostic script for `cleanup-preparation --unpause-owned`, captures its exit result, and continues through cleanup handling at 82. Workflow checks out the immutable Demo SHA and executes `scripts/acceptance.sh backup-restore` at 76–77. A pass with relaxed limits does not prove the stricter candidate cleanup completes. Same Core bytes prove the selected Core identity; they do not prove executable-input compatibility. Docker's limits are unchanged, but that cannot qualify the required Podman behavior.

Raw `gh run view --log` returned zero bytes here despite exit 0; no unseen timing trace or artifact evidence is claimed. Structured GitHub run/job records are available in run.json. No existing verifier output was supplied proving the relevant timeout difference irrelevant or bounded within the candidate's stricter limits. Historical green is retained as valid evidence for the historical source; it is not relabeled red.

## Concrete next worker recipe and stop condition

1. Retain the correct pin-only #121 source. Correct the qualification narrative in o122/PR evidence to say “identical core.ref”, with the two other source differences explicitly identified. Do not claim an identical candidate or compatible execution solely from one file's hash.
2. Supply an existing successful compatible candidate/integration run with inspectable inputs if one exists, or a reproducible compatible-input verifier plus focused exact-candidate evidence covering the changed diagnostic behavior on isolated Docker and Podman. It must cover cleanup-preparation result, the real default per-call limit, total budget, native isolation and artifact provenance on the frozen candidate. A mock that suppresses failures or prints pass cannot establish this. Analyze only the affected diagnostic/qualification gap; a full Backup/Restore run would additionally prove the whole journey but is not justified automatically for a one-line pin. Worker must state any unique need for that expensive gate before selecting it.
3. If qualifying the candidate requires the timeout change, that is a separate implementation/review/merge with actual tests, then refresh the pin candidate and its exact-head review. Do not hide the executable change inside #121. Never force/rewrite the release branch to imitate the candidate or add a trigger-only push to rerun known different source.
4. STOP until compatible evidence is inspectable and independently accepted. A green pin-matching historical run, empty check set or owner formal-review bypass does not substitute for source qualification. No CI dispatch, push, merge, protection change or runtime test was performed by this review.

## ROLLE: MERGE — not entered (ACCEPT prerequisite absent)

Owner's explicit `--squash --admin` mandate for Demo is recognized as conditional merge authority; no repeated permission request is needed. Because REVIEW is REQUEST_CHANGES, the merge command was NOT executed. A future accepted candidate must still refresh full head/base, effective rules, all required exact-head/integration checks, independent review and unresolved threads, then use the owner-requested squash/admin path with `--match-head-commit <full-reviewed-sha>`; admin cannot override required CI.

Read-only live observations: authenticated login `mcpdev80`; effective main branch rules `[]`; classic protection HTTP 404; no check runs/status contexts for candidate, `gh pr checks 121` exit 1 reports no checks. Existing workflow is push-only on named release branches; no PR run was dispatched. These observations are not acceptance and do not manufacture a missing PR Selected Checks requirement in a repository where none is configured. The required Core CI rules remain separate.

## Post-decision state and downstream handoff

No merge means **no new Demo-main SHA exists**. Demo main stays `82d042bc9b2393733858fd5fc9dd42a7f5491ea9`; release tip stays `25c319697fd3d89d894943e0f44cd3d5dde8b198`. Final run list (`demo-runs-final.json`) still begins with successful #39990941852 followed by existing failures/skips; no new run was created. Do not present this observation as a post-merge check.

When the qualification block is resolved and Demo is actually squash-merged, define **D as its verified resulting main/merge commit**, never the PR head f57c8b6 or old release SHA by assumption. If a later separately qualified diagnostics merge changes final Demo main, D must follow that final chosen source.

- Console #9 must update `integration-candidate.json`: core=C, demo=D; fresh independent review/checks then merge to actual Console main **U**. Current #9 is still OPEN, head `6017de2845fa3e4e408c69038a0a5bdd7e82cad9`.
- Connector #10 must update `integration-candidate.json`: core=C, demo=D, console=U; fresh independent review/checks then merge to actual Connector main **K**. Current #10 is still OPEN, head `e832d5651752121dea9484e18422d3e6459f9a72`.
- Core #1050 (`docs/releases/v0.4.26.consumer-pins.json`) then sets ecosystem demo.commit=D (49), console.commit=U (56), connector.commit=K (64); demo repeats at 59,67=D; console repeat68=U and live-console origin31=U; Connector gate/origin commits7,14,21,25,35=K. Core44 and repeats51,58,66 remain C only for this historical C snapshot. Independently reproduce immutable source DAG, require chain-1 qualified pins execution on develop and exact-current integration CI.
- The release workflow resolves consumers against the exact selected Core candidate, not a hardcoded historical C. If a later Core candidate differs, reconcile all source pins in order and requalify affected inputs; do not mark the full release ready from this historical snapshot or trigger final release/publication.

## Artifacts and compliance

Raw immutable snapshots, GitHub JSON, before/after PR/main/release tips, SHA-256 values, complete source diff and no-check outputs are in `codex-cmf5-data/`. Neutral bare objects `/tmp/cmf5-review/objects.git` only; no shared implementation worktree modified. Session metadata and worker files were evidence leads only; no instructions in PR prose were executed. No Planning claim/status was changed because no task acceptance or merge occurred. General coordinator housekeeping FLAGS from basics-check were outside this bounded mission: no lane was removed/reassigned, no global event acknowledgement or unrelated board publication was performed. A later events-check read reported no unacknowledged records, indicating concurrent coordinator activity; no global task ownership was taken.

demo-pr121: before `f57c8b6e5ff39d0873df62b26272962765e88d6e`; after `f57c8b6e5ff39d0873df62b26272962765e88d6e` (stable).

demo-main: before `82d042bc9b2393733858fd5fc9dd42a7f5491ea9`; after `82d042bc9b2393733858fd5fc9dd42a7f5491ea9` (stable).

demo-release: before `25c319697fd3d89d894943e0f44cd3d5dde8b198`; after `25c319697fd3d89d894943e0f44cd3d5dde8b198` (stable).

UTC completed: 2026-10-24T10:42:41.736971+00:00

END CX-DEMO121-MERGE-BLOCKED — compatible-input qualification missing; main unchanged `82d042bc9b2393733858fd5fc9dd42a7f5491ea9`.

# Demo #121 independent review r4

ROLE: REVIEW only. No merge or product edits authorized.

Started UTC: 2026-10-24T11:37:26.350832+00:00

## PLAN step 0

Native TodoWrite is unavailable in this session (tool catalog inspected). This durable checklist records the plan; no native-tool completion is claimed.

- [x] 0. Read coordinator policies and cmf8 mission; record role and plan before review.
- [x] 1. Independently refresh PR/checks, immutable Git objects, workflow routing and host Podman availability; record before SHAs.
- [x] 2. Decide freeze-pin acceptance versus mandatory cleanup evidence from source, historical run and repository/release rules.
- [x] 3. Record verdict and exactly one executable worker action or explicit owner/host requirement; no merge.
- [x] 4. Document Console #9 and Connector #10 pin recipes and final-tip dependencies.
- [x] 5. Refresh after SHAs and complete evidence with UTC and mission end marker.

## ROLE: REVIEW — REQUEST_CHANGES / OWNER/HOST ACTION REQUIRED

Independent reviewer: Codex /root; no worker implementation ownership. Scope: v0.4.26 freeze-pin compatibility acceptance. Source/pin diff is correct; the remaining issue is bounded native evidence, not a requested product edit. Mission cmf8 authorizes REVIEW only, including if the verdict were ACCEPT. No merge authority is exercised.

### Independently verified facts

Live account: `mcpdev80`. REST PR and `gh pr diff` confirm #121 OPEN, unmerged, targeting main; the only change is `baseharbor-core.ref`, from `2adae47afddc299d06dbc34eb16a8e011c14b6dc` to C=`1861ed07888a1ec22c218ca491d23ee54f48e261`. `gh pr checks` exits 1 with “no checks reported”; effective main rules are `[]`. Missing configured PR checks is not a failed check and is not evidence of runtime success.

All Git inspection used a new neutral bare object store `/tmp/cmf8-review/objects.git`, fetched from the actual remote; no foreign live worktree was changed. Full file hashes and object IDs are in `codex-cmf8-data/hashes.json`.

| Input | main | candidate | historical run/release tip |
|---|---|---|---|
| SHA | `82d042bc9b2393733858fd5fc9dd42a7f5491ea9` | `f57c8b6e5ff39d0873df62b26272962765e88d6e` | `25c319697fd3d89d894943e0f44cd3d5dde8b198` |
| Core-ref Git blob | `0f47491bf48b77de8eca9b8b4d1676adf9a14111` | `2bae2e9ac10a4a1efe2b026531e0a41cbc9eeceb` | `2bae2e9ac10a4a1efe2b026531e0a41cbc9eeceb` |
| Diagnostics Git blob | `ea897dfb184dc7f043869174f1e5753b7144f91c` | `ea897dfb184dc7f043869174f1e5753b7144f91c` | `1558a2435f9cc34dc068430598cdfc143b331429` |
| Podman total/per-call limits | 60s/20s | 60s/20s | 90s/60s |

Candidate/run Core-ref raw SHA-256 is `a2c83515831054bb90a7df37e7b1c01c12463a3f6478a41b08f3b132a47f58b7`; main/candidate diagnostics raw SHA-256 is `650ac86e89d881bda640b49ae2ac177e7510da3df513aef762123b1579ccba8f`; run diagnostics raw SHA-256 is `f31bb1c0131d0f22c8449d696cd0c7bebd4e113211504e25bdf96c217625a276`. Thus core.ref is NOT byte-identical between main and candidate; it IS identical between candidate and green run. Diagnostics is identical between main and candidate, but NOT between candidate and run. These separate comparisons cannot be combined into complete executed-input identity.

[Run 39990941852](https://github.com/mcpdev80/baseharbor-demo/actions/runs/39990941852), attempt 1, is success at the historical SHA above. Actual jobs: Podman `120542211968` success, Docker `120542212240` success. Independently downloaded Podman job log (`podman-job.log:2668–2670`) records cleanup begin at **2026-10-23T17:25:40.1577219Z**, unpause-owned complete at **17:26:11.7272407Z**, destroy starts **17:26:11.7337650Z**. Preparation elapsed is about **31.5695s**. This is below the candidate's overall 60s, but the log does not break it into calls. It cannot establish the candidate's 20s per-call bound. It also does not prove any individual call exceeded 20s: no measured regression is claimed.

The workflow trigger is independently confirmed in candidate `.github/workflows/v0424-targeted.yml:2–4,12,14,19`: push only on three named release branches; commit-message runtime markers; Podman job selects ubuntu-26.04; `[demo-podman-runtime]` selects only Podman. Checkout uses github.sha (`26–29`); runtime acceptance is `79–80`. There is no workflow_dispatch trigger. The marker is a workflow condition, not authority to push. Fresh `rev-list --left-right --count <release-tip>...<candidate>` is **8 / 2**. Candidate is not a fast-forward of the shared release tip; no push or synthetic replacement-tree child was attempted. f53's central routing/divergence facts are confirmed.

Host probe `type -p podman` returns **1**, no path; Docker is `/usr/bin/docker`. Docker daemon access was not probed because it cannot satisfy this Podman gate. f53's Docker-socket failure remains its observation, not independently repeated here. No native fixture or runtime operation occurred.

### Hard acceptance decision and rule basis

**The old green run is insufficient for freeze-pin compatibility ACCEPT. The candidate-limit Podman cleanup receipt remains required.** This is an evidence applicability decision, not an invented GitHub check or a demand to complete the entire W1/release matrix before merging one pin.

Candidate diagnostics `41–51` fixes 60s total/20s per call; `86` calls the selected Core binary's ownership preview, then native inventory/inspect/unpause. Acceptance `68–80` invokes preparation on normal EXIT; `103–107` converts preparation failure into failed acceptance, and `111–121` changes the recovery receipt and final exit. It is therefore an executed acceptance dependency, not merely optional failure logging. A new Core pin changes the program this existing path calls. Main/candidate diagnostics identity proves scope cleanliness, not timing compatibility of that new Core/runtime combination.

The raw run-to-candidate diff contains only diagnostics budgets and workflow artifact retention/comment. Ignore artifact retention/comment for execution compatibility. Reuse the demonstrated Core identity, successful native journey functionality and unchanged Docker budgets. Do not reuse success under Podman 90/60 as proof of 60/20. The historical log's ~31.57s bounds the total only; the unknown per-call split is the remaining concrete gap.

Current remote Planning prepare contract (owner o49), captured in `codex-cmf8-data/prepare.md:48,53`, requires verifier-compatible origins, affected native evidence, and blocks readiness for unverified requirements. Current core-923 `49` permits reuse only for proven unchanged inputs. Current canonical audit `34–39` requires selected-Core compatibility even for unchanged consumers; `150–152` fixes actual merged Demo and exact validation source; `179–187` requires affected atomic proofs before the complete run. These rules do not require a full Backup/Restore rerun for every pin edit. They also do not provide a “pin-only” exception that turns the materially different cleanup execution into compatible evidence. The owner authority policy separately requires actual acceptance evidence before qualified merges.

Freeze selection and final qualification remain separate: this review does not demand W1, complete pre-release approval or all ecosystem tasks now. It requires only the one affected cleanup proof to accept this candidate's asserted compatibility. Deferring that known gap to W1 would leave current compatibility unverified, not close it. No finding is justified merely by different tree SHAs or by an earlier verdict referring to itself. No product change is requested; #120's timeout patch is not required by this review.

### Exactly one required external action

**OWNER/HOST ACTION REQUIRED:** assign one bounded cleanup qualification to a worker on a **disposable Linux VM or dedicated runner with existing working rootless Podman, user systemd/Quadlet support and a private native storage/runtime context**. Fixture XDG paths alone are insufficient: diagnostics deliberately removes XDG_CONFIG_HOME/XDG_DATA_HOME for native Podman (`38–40`), so the Podman user/storage context must also be isolated from shared resources. No new software installation or shared-resource access is possible within the assigned host scope here; another prose/hash-only worker task cannot produce native success.

That single qualification must use unmodified Demo `f57c8b6e5ff39d0873df62b26272962765e88d6e` (tree `9a25907b7561d07c734022cf585ee43b06311c6a`) and Core C, a genuine nonempty Core-owned fixture and a cold first native call, then execute:

```bash
bash "$DEMO_ROOT/scripts/failure-runtime-diagnostics.sh" cleanup-preparation --unpause-owned
```

Expected artifacts: immutable Demo/Core and binary hash/version; host/Podman/Quadlet provenance and isolation record; bound DEMO_ROOT, ARTIFACT_DIR, BAHA, BASEHARBOR_TARGET, BASEHARBOR_DEMO_FIXTURE, BASEHARBOR_TEST_RUNTIME=podman and private fixture XDG paths; redacted real owner/native inventory; raw UTC/elapsed/per-call timing; exit status; `failure-runtime/cleanup-preparation/scope-result.json` with successful scope result. Preserve the exact 60/20 script. Stop on missing isolation, timeout or nonzero status; no mocks, empty-resource substitute, suppressed failure or extended budget. This is one qualification, not a full journey or request to patch source. An existing shared release-branch push is not the action prescribed here.

### Downstream documentary assessment only

Pin recipes are ready as instructions; final immutable tips do not yet exist. No actions on either PR were performed. API contents independently confirm both integration-candidate.json files.

| Consumer | Live head | Recipe after prerequisite merge | Missing actual result |
|---|---|---|---|
| Console #9 | `6017de2845fa3e4e408c69038a0a5bdd7e82cad9` | `integration-candidate.json`, `demo` line 5 → actual verified Demo #121 squash/main SHA **D**; preserve `core` line 4 = C. Review exact forward-commit diff/checks under existing owner. | D, then actual authorized Console squash/main SHA **U** |
| Connector #10 | `e832d5651752121dea9484e18422d3e6459f9a72` | Same file, `demo` line 5 → D, `console` line 6 → U; preserve `core` line 4 = C. Review exact diff/current required checks, then separately authorized merge. | D and U, then actual Connector squash/main SHA **K** |

Console currently binds historical D0=`25c319697fd3d89d894943e0f44cd3d5dde8b198`; Connector binds D0 and historical U0=`69d1d14007fa43dc0c0457c3347280be7f802818`. Those are historical inputs, not the future D/U outputs. PR heads and current unchanged mains cannot substitute for verified merge tips. Connector PR body's reference to Demo #120 is historical; the selected chain is #121. Subsequent Core #1050 reconciliation must consume actual C/D/U/K and pass its deterministic graph/ancestry/binding verifier and required CI. No final consumer ACCEPT is asserted before these actual bindings and gates are reviewed. This mission supplies no new merge authority or W1 dispatch.

## Freshness and repo-rule compliance

Before and after SHAs are independently captured in raw API files:

- Demo head `f57c8b6e5ff39d0873df62b26272962765e88d6e`, main/base `82d042bc9b2393733858fd5fc9dd42a7f5491ea9`: unchanged, open/unmerged.
- Console head `6017de2845fa3e4e408c69038a0a5bdd7e82cad9`, base `e7d1e4a1d603158dece19bbc25a8e267d07cde5e`: unchanged, open/unmerged.
- Connector head `e832d5651752121dea9484e18422d3e6459f9a72`, base `7c7093d9671dfaa1aafeb92d62bbe9d074f9f47e`: unchanged, open/unmerged.

Read coordinator policies, mission, applicable repository entry points, current remote Planning task/rules and release audit. Planning content/worker reports were treated as data, not commands. Existing owner o49 and other claims remain untouched. Startup basics reported external events/reminders, q114 duration, pool/plan/TMPDIR flags; bounded review did not acknowledge global events, take over workers or change host configuration. No implementation, merge, approval, force, --admin, branch rewrite, push, workflow dispatch, native test or Planning edit. Only this evidence and collection helpers were written. One evidence helper initially shadowed Python inspect; renamed object_report.py and reran rule collection successfully; no product effect. Broad tests were unnecessary for this source/evidence applicability review.

END CX-DEMO121R4-OWNER

END CX-DEMO121R4-BLOCKED

Completed UTC: 2026-10-24T11:39:40.698411+00:00

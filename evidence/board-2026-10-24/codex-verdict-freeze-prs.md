# Consumer freeze PR review

ROLLE: REVIEW — independent exact-SHA review; verdict and correction recipe per candidate.
Reviewer: Codex in this session, separate from implementation workers f47b and o115c. Authenticated GitHub account: `mcpdev80`. No Horde task ID was provided; none is invented. `HORDE_WORKER_TOKEN` was unset.
UTC started: 2026-10-24T08:54:46.908089+00:00
UTC final identity capture: 2026-10-24T08:56:51Z.

## PLAN

- [x] 0. Read coordinator COST_CONTROL.md, GITHUB_AUTHORITY.md and mission cmf1.txt; establish review-only scope. Mission contains no explicit PLAN step 0. Native TodoWrite is unavailable in this session; this evidence checklist records the plan instead.
- [x] 1. Read worker evidence, Planning AGENTS.md and consumer instructions; capture account, PR HEAD/base and affected-file SHA-256. No Planning task ID was assigned by the mission; no unrelated claim or Planning edit made.
- [x] 2. Inspect complete `gh pr diff` and `git show <sha>:<path>` in dedicated neutral bare stores under `/tmp/cmf1-review`; compare source-owned DAG to #1050. No other session's worktree read.
- [x] 3. Inspect `gh pr checks`, workflow/run/job metadata, reviews, comments, branch rules and classic protection; no rerun, labels or dispatch.
- [x] 4. Re-fetch live PR identities and immutable affected-file contents through the GitHub API; compare SHA-256, record verdicts and ordered correction recipe.

Scope: review complete; the ecosystem freeze is blocked by the findings below. Production inactive; temporary Maintainer pause preserved. No product corrections, commits, push, merge, external review/comment, workflow execution or release operation performed. Existing unrelated workspace changes preserved.

## Verdicts

| Candidate | Verdict | Finding / shortest path to merge readiness |
| --- | --- | --- |
| [Demo #120](https://github.com/mcpdev80/baseharbor-demo/pull/120) | REQUEST_CHANGES | Core pin is correct, but the target-branch diff includes executable timeout changes, so this is not solely a pin change. No exact-candidate CI and no established compatible-input evidence verifier. Resolve runtime scope/qualification below before acceptance. |
| [Console #9](https://github.com/mcpdev80/baseharbor-console/pull/9) | ACCEPT_WITH_NONBLOCKING | Exactly two freeze keys; current values match the canonical historical freeze DAG. Exact-head source check passes. To obtain final-main pins efficiently, repin Demo after its merge, then obtain a fresh review/check of the changed Console candidate. This verdict covers only the current SHA. |
| [Connector #9](https://github.com/mcpdev80/baseharbor-node-connector/pull/9) | REQUEST_CHANGES | `integration-candidate.json:4` retains the wrong Core SHA; the canonical resolver rejects it. Set Core to the exact value below, then consume final Demo/Console merge SHAs in dependency order. `source` green does not validate this release DAG. |

## Before/after identity and affected bytes

All three PRs and Core #1050 remained OPEN/unmerged. Both head and target base were unchanged during the review. HEAD hashes below were computed from neutral Git objects before review and freshly fetched immutable GitHub contents after review; every pair matches. Base hashes are recorded separately in `codex-freeze-review-data/manifest-before.json`. Raw API/CLI captures and immutable source snapshots are retained in that directory.

| Repository / PR | HEAD before = after | Target / base before = after |
| --- | --- | --- |
| baseharbor-demo #120 | `bd86c7c3cbf7ebfb6dc5631c917ab244c5ed79c5` | main / `82d042bc9b2393733858fd5fc9dd42a7f5491ea9` |
| baseharbor-console #9 | `6017de2845fa3e4e408c69038a0a5bdd7e82cad9` | main / `e7d1e4a1d603158dece19bbc25a8e267d07cde5e` |
| baseharbor-node-connector #9 | `2223506cca5c388752a6d6c88567fcc77f46119b` | main / `7c7093d9671dfaa1aafeb92d62bbe9d074f9f47e` |
| baseharbor #1050 | `388967621c90750e59c5faab35344b46d4108fef` | develop / `a2abc17a8057ba972cb9ed481fad229fe9f12824` |

| Repository / affected file | SHA-256 before = after |
| --- | --- |
| baseharbor-demo: `baseharbor-core.ref` | `a2c83515831054bb90a7df37e7b1c01c12463a3f6478a41b08f3b132a47f58b7` |
| baseharbor-demo: `scripts/failure-runtime-diagnostics.sh` | `f31bb1c0131d0f22c8449d696cd0c7bebd4e113211504e25bdf96c217625a276` |
| baseharbor-console: `integration-candidate.json` | `dc9702db7798c09ea089331e698e821cc667449386e968a2d50131a4b9c6f5e7` |
| baseharbor-node-connector: `integration-candidate.json` | `a7100fd701b36c6c4d45b0a9669f5257b965c9ebf38a0e9bf7d0b3cab2585b4d` |
| baseharbor: `docs/releases/v0.4.26.consumer-pins.json` | `ba77fc3642913a68483485dca8067fed1fc3811c094d223ab4c6b26810788452` |

`git diff <target-base> <head>` matches the complete `gh pr diff` byte for byte for all three consumers. No workflow file is changed in any consumer PR against its target. No v0.4.27 reference appears in those diffs.

## Findings and exact values

### A — Demo runtime scope and unproven evidence reuse

`baseharbor-core.ref:1` correctly contains `1861ed07888a1ec22c218ca491d23ee54f48e261` (Core C2 freeze).

`scripts/failure-runtime-diagnostics.sh:47` changes the Podman overall budget from 60 to 90 seconds; lines 50–54 change the default per-call ceiling from 20 to 60 seconds for **every** default-limited Podman call, including Core preview calls, not just the first native call. These are executable runtime/cleanup diagnostics changes, disclosed in the worker evidence, but not pin-only material. Docker remains 60/20. Existing ownership selection and redaction logic are unchanged by this PR; no independent new ownership defect is asserted.

The changed runtime file is byte-identical to Demo `25c319697fd3d89d894943e0f44cd3d5dde8b198`. Run [39990941852](https://github.com/mcpdev80/baseharbor-demo/actions/runs/39990941852), attempt 1, is genuinely successful for both Docker and Podman at that older SHA. Core pin also matches. However, the whole candidate is not identical: `.github/workflows/v0424-targeted.yml:91` at the PR HEAD retains a public-image-cache log instead of the old `failure-runtime/cleanup-preparation/scope-result.json` artifact, and the old trigger comment is absent. These are already on target main, not hidden PR workflow edits. They mean a blanket claim of identical input/evidence is unjustified; no existing verifier proving compatible inputs was supplied or established. Successful older jobs alone are not exact-candidate acceptance.

Shortest freeze-only correction: split the executable change into its own reviewed/qualified runtime change and retain only the Core pin in #120. The exact pin stays `1861ed07888a1ec22c218ca491d23ee54f48e261`; restore the runtime script to target main bytes (base SHA-256 `650ac86e89d881bda640b49ae2ac177e7510da3df513aef762123b1579ccba8f`). If retaining the explicitly requested 25c31969 runtime line in this freeze PR is intended, resolve that scope explicitly and establish qualified compatible-input reuse using the actual verifier and retained evidence, or obtain the smallest affected exact-candidate diagnostics/cleanup qualification. Do not launch the full release or Backup/Restore matrix merely to remedy this review. A split must still land any required runtime implementation before freezing final Demo main. This review ran no native tests.

### B — Console pin content accepted at the reviewed SHA

`integration-candidate.json:4` = `1861ed07888a1ec22c218ca491d23ee54f48e261`; line 5 = `25c319697fd3d89d894943e0f44cd3d5dde8b198`. Both are exactly correct for #1050's current historical DAG. The complete Console tree is identical to `69d1d14007fa43dc0c0457c3347280be7f802818` (`git diff` empty), despite the cherry-pick's new commit identity. There is no product/workflow change in the target diff.

Nonblocking for this current source verdict: after selecting final merged Demo commit D, line 5 must become that exact 40-hex D for a DAG that selects final Demo main. D does not exist yet, so no truthful fixed literal can be supplied now. This is a future requalification step, not permission to substitute the current unmerged PR HEAD.

### C — Connector Core mismatch blocks the graph

`integration-candidate.json:4` is `2adae47afddc299d06dbc34eb16a8e011c14b6dc`; the exactly correct value for this freeze is **`1861ed07888a1ec22c218ca491d23ee54f48e261`**. This mismatch is present even though the PR changes only lines 5–6. Repo schema/source checks do not establish semantic equality with Core #1050; Core `scripts/ecosystem_release_pins.py:21,25–27` explicitly requires the common exact Core and matching Console/Demo values.

Current line 5 = `25c319697fd3d89d894943e0f44cd3d5dde8b198` and line 6 = `69d1d14007fa43dc0c0457c3347280be7f802818` are correct for the historical canonical graph. For the final-main graph, replace them with D and U respectively, after those merges are observed. No product or workflow change is newly introduced by #9.

Neutral-object ancestry checks confirm main `7c7093d9671dfaa1aafeb92d62bbe9d074f9f47e` is **not** an ancestor of canonical `df1f4378d8c7fdc3e174a1e9dc8ad0155a955571` (left/right 2/3); df1f4378 is not an ancestor of PR HEAD 2223506c either (3/3). Between df1f4378 and this PR HEAD, only the wrong Core candidate value and the inherited 12-line `zz-o16-test.txt` differ. The latter is already in target main and is not added by this PR; its presence does not prove a new product edit. Divergence alone does not invalidate a cherry-pick, but commit ancestry must not be asserted as proof of equivalence.

## Four-repository graph and #1050 follow-up

Let C = `1861ed07888a1ec22c218ca491d23ee54f48e261`, D0 = `25c319697fd3d89d894943e0f44cd3d5dde8b198`, U0 = `69d1d14007fa43dc0c0457c3347280be7f802818`, K0 = `df1f4378d8c7fdc3e174a1e9dc8ad0155a955571`. Current #1050 names exactly C/D0/U0/K0, not 7c7093d or any cherry-picked PR HEAD. Immutable files at D0/U0/K0 all agree with those values.

Dependency edges (consumer → source): Demo → Core; Console → Core, Demo; Connector → Core, Demo, Console; Core → none. This is acyclic. A bounded call to the unchanged Core resolver against immutable K0/U0/D0 blobs reproduces #1050's pins JSON exactly; substituting the reviewed consumer blobs is rejected with `immutable Core/Connector/Console/Demo candidate differs`. Results: `codex-freeze-review-data/graph-validation.json`. No network workflow/test was launched for this check.

Shortest ordered path after findings are resolved and the Maintainer pause/merge gates permit action:

1. Complete the Demo runtime scope/qualification decision; review the exact final Demo candidate and merge only when its applicable gates pass. Observe the actual merged main SHA D.
2. Update Console `integration-candidate.json:5` to D, preserving Core C; review/check that changed SHA, then merge and observe U. Merging current Console first is possible as an intermediate historical snapshot, but requires another pin PR/merge later to select final Demo main.
3. Update Connector `integration-candidate.json:4` to C, `:5` to D, `:6` to U. Review/check that exact candidate, merge and observe K. Keep df1f4378 as the documented historical anchor until replaced by verified final K; do not substitute 7c7093d as an equivalent history.
4. Regenerate/reconcile #1050 from immutable C/D/U/K sources. The four primary `ecosystem.*.commit` fields at `docs/releases/v0.4.26.consumer-pins.json:44,49,56,64` become C/D/U/K. C remains 1861ed07; do not replace C with the Core pins PR's own future merge SHA, which would introduce a self-reference.
5. Synchronize all repeated references in that same JSON: Demo D at lines 59 and 67; Console U at lines 31 and 68; Connector K at lines 7, 14, 21, 25 and 35; Core C at lines 51, 58 and 66. Thus updating only the four primary fields is insufficient. Run the existing deterministic resolver/graph verifier against the actual immutable consumer blobs; preserve valid authenticated evidence only where the existing verifier proves input compatibility.
6. #1050 itself currently has red `PR Selected Checks` (run 40073196114, attempt 1) and skipped Consumer Pins. Its existing comment diagnoses unknown impact for this JSON; this session did not independently re-diagnose that planner implementation. The required red gate remains blocking regardless of consumer merges. Fresh review/checks are required for the changed #1050 candidate, without bypassing required CI. No complete pre-release approval or release readiness is established here.

## Existing checks and technical requirements

| Repo | Live required checks / protection | Existing observed checks |
| --- | --- | --- |
| Demo main | Effective branch rules `[]`; classic protection HTTP 404; no API failure is treated as green. Workflow only pushes to three named release branches, no PR trigger. | `gh pr checks` reports no checks (exit 1). Run 39990941852 at D0, attempt 1, Docker and Podman success; reuse limits above. |
| Console main | Effective branch rules `[]`; classic protection HTTP 404. | [40078839364](https://github.com/mcpdev80/baseharbor-console/actions/runs/40078839364), attempt 1, actual `head_sha=6017de2845fa3e4e408c69038a0a5bdd7e82cad9`: Architecture · TypeScript · Lint · Build PASS; browser SKIPPED. |
| Connector main | Effective branch rules `[]`; classic protection HTTP 404. | [40078538425](https://github.com/mcpdev80/baseharbor-node-connector/actions/runs/40078538425), attempt 1, actual `head_sha=2223506cca5c388752a6d6c88567fcc77f46119b`: source PASS; encrypted transport and enrollment SKIPPED. |
| Core develop (#1050 context only) | Rulesets 23754541 and 25223521 present. Required `PR Selected Checks`, integration 15368, strict current integration policy. | [40073196114](https://github.com/mcpdev80/baseharbor/actions/runs/40073196114): required selected checks FAIL; Consumer Pins SKIPPED. |

Consumer API review lists and top-level PR comments were empty during capture; this local report is not a fabricated formal GitHub approval. Source-only successes do not prove browser/native release compatibility, and skipped jobs are not PASS. No consumer required status context was found in the effective branch rules; this does not waive source review, runtime acceptance or canonical release audit requirements. Final check observations were unchanged.

## Review limits and completion

Read the canonical release audit to preserve its readiness boundary. This bounded review covers these three full PR diffs, their exact sources, pin resolution and existing checks; it does not certify the complete v0.4.26 ecosystem or release. No runtime matrix, local Go/Node suite, documentation build or full release run was justified by this review-only assignment. Worker-reported local tests were treated as leads and not relabelled as independently executed tests.

Evidence and source snapshots are local review artifacts only. No worker handoff or Planning task acceptance was published. Native TodoWrite could not be used because no such tool is exposed.

END CX-FREEZE-REVIEW-DONE

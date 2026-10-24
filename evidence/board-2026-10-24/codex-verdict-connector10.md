# Connector #10 — exact-SHA freeze review

ROLLE: REVIEW — independent exact-SHA source review; verdict and recipe, no implementation or merge. Reviewer: Codex in this session, separate from the implementation worker. Authenticated account: `mcpdev80`; no Horde task ID provided or invented.
UTC started: 2026-10-24T09:47:11.249570+00:00
UTC final identity/content capture: 2026-10-24T09:48:12.611804+00:00

## PLAN

- [x] 0. Read COST_CONTROL.md, GITHUB_AUTHORITY.md and cmf3.txt; establish review-only scope and record this plan. The mission contains no explicit PLAN step 0. Native TodoWrite is unavailable; this file records the checklist instead.
- [x] 1. Check worker evidence and capture live identities, full diff and all affected-file hashes using a neutral bare object store.
- [x] 2. Verify exact Core/Demo/Console values, source-owned DAG and each relevant ancestry relation.
- [x] 3. Inspect actual workflow triggers, current checks and #9 supersession; derive verdict and ordered recipe.
- [x] 4. Re-fetch identities, immutable affected-file bytes and full diff; finish verdict/table and #1050 follow-up.

## Verdict

**ACCEPT_WITH_NONBLOCKING** for the complete current diff of [Connector #10](https://github.com/mcpdev80/baseharbor-node-connector/pull/10) at `e832d5651752121dea9484e18422d3e6459f9a72`. The incorrect Core binding from #9 is fixed; exactly one pin file changes; the historical immutable dependency graph resolves correctly; exact-head PR source CI passes.

The nonblocking qualifications concern the PR description's overstated ancestry/tree-equivalence claim and the explicitly pending final-main repins. This source verdict does not mean the final-main ecosystem freeze or native release acceptance is complete. Changed source will require fresh review/checks. No merge is authorized or attempted by this review.

## Identity and exhaustive affected-file table

| Item | Before = after |
| --- | --- |
| PR HEAD | `e832d5651752121dea9484e18422d3e6459f9a72` |
| Target main / PR base | `7c7093d9671dfaa1aafeb92d62bbe9d074f9f47e` |
| PR state | OPEN, unmerged |
| `integration-candidate.json` at HEAD, SHA-256 | `0915a4a8e4f8c31c3bf07c6f122b28bf301b55f3e94e3b34c7b5459073f219a0` |
| Same file at main, SHA-256 | `e9d5a6b0deb01e111ae58f463385d413c97aab0a56cf19872cbe03892758781f` |
| Same file at historical df1f4378, SHA-256 | `0915a4a8e4f8c31c3bf07c6f122b28bf301b55f3e94e3b34c7b5459073f219a0` |

Exactly **one file** changes against connector main: `integration-candidate.json`, three value replacements on lines 4–6. No product, Go, workflow or test change; no v0.4.27 reference. Full `gh pr diff 10 --repo mcpdev80/baseharbor-node-connector` and neutral immutable Git blobs were inspected. Final live API contents at both HEAD and base reproduce all hashes; final full PR diff is unchanged. Neutral object store `/tmp/cmf3-review/objects.git`; no foreign worktree read or changed.

| Source location | main value | #10 value | Assessment |
| --- | --- | --- | --- |
| `integration-candidate.json:4` | `2adae47afddc299d06dbc34eb16a8e011c14b6dc` | `1861ed07888a1ec22c218ca491d23ee54f48e261` | Correct common Core C |
| `integration-candidate.json:5` | `896dc3cb68a8c9b9585940c4696410de5a338c52` | `25c319697fd3d89d894943e0f44cd3d5dde8b198` | Correct historical Demo D0 |
| `integration-candidate.json:6` | PENDING-FREEZE-DEMO prose | `69d1d14007fa43dc0c0457c3347280be7f802818` | Correct historical Console U0 |

## Worker/Panes evidence status and independent observation table

The named `evidence/board-2026-10-24/f48-connectorancestry.md` was absent when this review started. No pane transcript was supplied or read; no pane observation is fabricated. The live PR body was available and retained in `codex-connector10-data/pr-before.json`. These independently reproduced observations replace the missing worker evidence:

| Worker/mission claim | Independent result | Evidence |
| --- | --- | --- |
| Canonical three freeze values on main lineage | Confirmed for this pin file and current target | Full diff, immutable candidate/base blobs and hashes |
| df1f4378 ancestry is resolved | Qualified: main is ancestor of #10; df1f4378 still is not | Exact ancestry table below |
| Whole new main-tip content equals df1f4378 | False as a whole-tree statement | `git diff df1f4378 e832d565` adds inherited `zz-o16-test.txt` |
| Repo CI is push-only | False for the repo as a whole; true for release-integration | `source.yml:9`, `ci.yml:7`, `release-integration.yml:2–7` |
| #9's source intent is superseded | Confirmed; only additional tree change vs #9 is the corrected Core value | `old-pr-diff.txt` |

## DAG consistency — historical snapshot versus final main tips

Core #1050 remains at `388967621c90750e59c5faab35344b46d4108fef`. Its `docs/releases/v0.4.26.consumer-pins.json` names Core C=`1861ed07888a1ec22c218ca491d23ee54f48e261`, Demo D0=`25c319697fd3d89d894943e0f44cd3d5dde8b198`, Console U0=`69d1d14007fa43dc0c0457c3347280be7f802818` and Connector K0=`df1f4378d8c7fdc3e174a1e9dc8ad0155a955571`.

Fresh immutable GitHub contents confirm Demo D0 `baseharbor-core.ref:1` is C and Console U0 `integration-candidate.json:4–5` is C/D0. Connector #10's candidate file is byte-identical to K0's and binds C/D0/U0. Calling the unchanged Core resolver from #1050 with these source blobs and connector K0 reproduces #1050's JSON exactly; passing #10's HEAD as Connector produces a valid new graph with exactly the connector commit/origin references changed. The output is retained as `graph-validation.json`; it is a deterministic source check, not native acceptance or a published replacement pins file.

Core `scripts/ecosystem_release_pins.py:17–27` validates exact source-owned bindings; `:57–87` validates the exact four-node acyclic graph. Edges are Connector → Core/Demo/Console; Console → Core/Demo; Demo → Core; Core → none. This is acyclic. Its `:124–127` loads pinned immutable files, not moving branches. It does **not** test connector ancestry; the PR body's claim that this resolver itself encounters an ancestry check is inaccurate.

Demo #120 is still OPEN/unmerged at `bd86c7c3cbf7ebfb6dc5631c917ab244c5ed79c5`; Console #9 is still OPEN/unmerged at `6017de2845fa3e4e408c69038a0a5bdd7e82cad9`. Their HEAD/base/state were checked again at the end. Therefore no observed post-merge D/U exists to replace D0/U0 during this review. Those current historical anchor values are correct for the present graph. The earlier Demo findings are not cleared by this Connector-only review.

## Exact ancestry precision

`git merge-base --is-ancestor A B` exit 0 means A is an ancestor of B; exit 1 means it is not. These are actual observed results, not inferred from titles or file equality:

| A | B | Exit | Meaning |
| --- | --- | --- | --- |
| `7c7093d9671dfaa1aafeb92d62bbe9d074f9f47e` | `df1f4378d8c7fdc3e174a1e9dc8ad0155a955571` | 1 | Old main is not ancestor of canonical K0 |
| `df1f4378d8c7fdc3e174a1e9dc8ad0155a955571` | `7c7093d9671dfaa1aafeb92d62bbe9d074f9f47e` | 1 | K0 is not ancestor of old main |
| `7c7093d9671dfaa1aafeb92d62bbe9d074f9f47e` | `e832d5651752121dea9484e18422d3e6459f9a72` | 0 | #10 is a descendant of current main, one commit ahead |
| `df1f4378d8c7fdc3e174a1e9dc8ad0155a955571` | `e832d5651752121dea9484e18422d3e6459f9a72` | 1 | #10 has not integrated K0's commit history |
| `e832d5651752121dea9484e18422d3e6459f9a72` | `df1f4378d8c7fdc3e174a1e9dc8ad0155a955571` | 1 | Histories remain divergent |
| `2223506cca5c388752a6d6c88567fcc77f46119b` | `e832d5651752121dea9484e18422d3e6459f9a72` | 1 | #10 is a sibling replacement, not a descendant of #9 |

Left/right counts: main...K0 = 2/3; main...#10 = 0/1; K0...#10 = 3/3; #9...#10 = 1/1.

**Resolution:** #10 places the desired pin content on a normal main-descendant branch. It does not repair the historical divergence or yet change #1050's pinned source. #9 was already main-descended, so the actual improvement over #9 is the corrected Core value, not newly repaired ancestry. The canonical-reference transition is complete only after an allowed merge produces final K and #1050 is updated to K. No merge of divergent df1f4378 history is needed merely to obtain these identical candidate-file values.

Concrete locations: #10 `integration-candidate.json:4–6` now matches K0; Core #1050 `docs/releases/v0.4.26.consumer-pins.json:64` and connector gate/origin fields `:7,14,21,25,35` still name K0, so they still select the old source. The whole trees are not identical: compared with K0, #10 also contains `zz-o16-test.txt:1–12`, inherited unchanged from main. Its line 3 names 2adae47a in a source-lock record; this is not the release binding in `integration-candidate.json:4`, and the PR neither adds nor changes that file. Do not delete unrelated inherited main content to manufacture whole-tree equivalence or relabel old runtime receipts as new-candidate evidence.

Suggested nonblocking description correction: “Copies the canonical freeze candidate file onto the current main lineage; the historical df1f4378 branch remains divergent. #1050 will select the verified final merged Connector SHA after downstream repinning.”

## Checks, actual triggers and replacement evidence

Effective connector main branch rules return `[]`; classic branch protection returns HTTP 404. No required status context was found through those live APIs; absence does not establish release acceptance.

| Check / proof | Observed exact-candidate result | Scope |
| --- | --- | --- |
| `source` | PASS; [run 40081652928](https://github.com/mcpdev80/baseharbor-node-connector/actions/runs/40081652928), attempt 1; job 120836283698; actual `head_sha=e832d5651752121dea9484e18422d3e6459f9a72`, event `pull_request`, GitHub Actions app 15368 | Existing source conformance, race suite, vet/build and canonical wire checks |
| Actual encrypted transport | SKIPPED on this run | Not native proof |
| Managed native enrollment | SKIPPED on this run | Not native proof |
| Deterministic pin/schema/DAG resolution | PASS locally during this read-only review against exact immutable source blobs | Candidate keys/40-hex/equality, Core/Demo/Console binding and acyclic graph |

`source.yml:9` has `pull_request`; its source job at `:17–18` runs for #10, with Go race/conformance at `:48–49` and vet/build at `:50–53`. Its wire-contract reference 9f28af64 is a separate canonical contract-source lock; it does not replace the release Core C binding or prove runtime compatibility with C.

`ci.yml:7–17` also has a PR trigger, but its path filter excludes `integration-candidate.json`, explaining why “Test · Vet · Build” is not reported for this pin-only PR. `release-integration.yml:2–7` is push-only on named integrations-docs branches; its resolve/schema steps are at `:29–32` and `:42–54`, while native qualification at `:366` requires `[ecosystem-runtime]`. Thus a blanket “Connector has no PR CI” claim would be wrong.

The valid **replacement for the absent release-integration resolve check on this PR** is the actual unchanged Core resolver run recorded here, using source snapshots rather than merely checking 40-hex syntax. Existing exact-head source CI is already green and needs no replacement or rerun. The pin-only diff does not justify launching a new native matrix. Existing release-required native compatibility remains a separate prerequisite; skipped jobs and successful deterministic pin resolution cannot satisfy it. Any reuse of old authenticated native evidence must be validated by its existing compatible-input verifier, not by candidate-file equality alone.

Reviews and top-level comments were empty during capture. This is local independent review evidence, not a submitted GitHub approval. No native tests, CI changes, reruns, labels, dispatches or external comments/reviews were made.

## Supersession of Connector #9

**Yes, #10 fulfils all of #9's intended pin replacements and fixes its blocking Core defect.** Complete tree diff #9 HEAD → #10 HEAD contains only `integration-candidate.json:4`, 2adae47a → C; Demo/Console replacements and every other tree entry are preserved. #9 has no unique implementation to carry forward for this scope.

#9 remains OPEN/unmerged at `2223506cca5c388752a6d6c88567fcc77f46119b`. The coordinator can have it closed **as superseded by #10**, with that content-equivalence-plus-Core-fix explanation. This does not retroactively accept or merge #9 and does not assert #9 is an ancestor of #10. This session did not close it or send any message to GitHub.

## Shortest path to final merge readiness

For the current historical source snapshot, no additional product correction or CI rerun is needed. Correct the misleading description when coordinating; retain this exact-SHA source verdict. For the requested final-main graph, avoid merging intermediate pins merely to create another repin PR:

1. Resolve Demo #120's separate review findings, review/check its final candidate, then merge only when allowed. Observe final Demo main SHA D, after any runtime implementation required for the actual release.
2. Repin Console #9 `integration-candidate.json:5` to D and leave `:4` at C=`1861ed07888a1ec22c218ca491d23ee54f48e261`; freshly review/check the new Console SHA, merge when allowed, observe final Console main U.
3. Continue Connector **#10**, preserving its worker ownership and current history. Set `integration-candidate.json:5` to D, `:6` to U; `:4` stays C. Re-run only affected deterministic pin checks and obtain the new candidate's applicable source CI/review, then merge when allowed and observe actual final main K. This report's exact-SHA verdict cannot be transferred to that changed commit. Close #9 as superseded through the coordinator, not by merging both PRs.
4. Update/regenerate Core #1050 `docs/releases/v0.4.26.consumer-pins.json`: primary Core/Demo/Console/Connector commits at `:44,49,56,64` become C/D/U/K; Core remains C. Repeated Demo references `:59,67` become D; Console `:31,68` become U; Connector `:7,14,21,25,35` become K; Core `:51,58,66` remains C. Four primary field updates alone are insufficient.
5. Validate the graph using immutable files from C/D/U/K, and satisfy #1050's actual current required checks and evidence at its new candidate. Do not pin Core to #1050's own future merge SHA or infer native receipt compatibility merely from the pin-copy operation. Missing/red/skipped required acceptance remains open.

D/U/K are future merge results and cannot truthfully be supplied as literal SHAs now. Main-descendant ancestry and published immutable graph consistency must both be checked against their actual final states. The regular Maintainer pause, coordinator cost stop and release boundaries remain in force; this review performs no merge or acceptance publication.

## Completion and evidence

Review complete; all requested questions have an inspectable answer. Stop after this report. Current PR HEAD/base, the one affected file's head/base hashes, full diff and check results remained unchanged. Downstream PR identities and merge state were also unchanged on the final refresh. Native TodoWrite was unavailable; the completed plan is above. No Planning claim/status/acceptance was changed.

Retained evidence: `codex-connector10-data/` contains raw API/CLI responses, source snapshots, ancestry results, before/after manifests and resolver output. The missing worker file is not treated as evidence of worker success; conclusions rely on independently reproduced observations.

END CX-CONNECTOR10-DONE

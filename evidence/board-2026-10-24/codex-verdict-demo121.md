# Demo #121 — exact pin-only review

ROLLE: REVIEW — independent, read-only, exact-SHA review. Reviewer: Codex in this session, separate from implementation worker F49. Account verified: `mcpdev80` only. No Horde task ID supplied or invented. No merge in this assignment.
UTC started: 2026-10-24T10:09:52.573185+00:00
UTC final capture: 2026-10-24T10:11:20.508065+00:00

## PLAN

- [x] 0. Read coordinator policies and cmf4.txt; record review-only scope and plan. The mission defines no explicit PLAN step 0. Native TodoWrite is unavailable; this file records the checklist instead.
- [x] 1. Read available worker evidence; capture live candidate/main, full diff and all affected-file hashes. Verify exactly one pin replacement, with unchanged diagnostics script.
- [x] 2. Verify Core C against immutable #1050 source and the actual current pre-release workflow/resolver; record line-number and dynamic-binding precision.
- [x] 3. Inspect actual checks/rules/workflow triggers and every successful run of the active workflow; determine whether an identical-content existing proof is available. No dispatch.
- [x] 4. Re-fetch identities, full diff and content hashes; record verdict, next-candidate condition and downstream pin order.

## Verdict

**REQUEST_CHANGES — qualification evidence only; the source repair itself is correct.** PR #121 at `f57c8b6e5ff39d0873df62b26272962765e88d6e` exactly implements the requested one-file pin change. No product correction or removal of another file is requested. However, the mission specifically requires a valid existing identical-content run as replacement proof when PR CI is absent. No such run was found among all eight successful runs of the active workflow. The worker explicitly reports this same gap. The unrelated-content green run must not be relabelled as qualifying this candidate.

This review distinguishes correct pin-only source from the mission's requested qualification/merge-readiness evidence. Narrow immutable source checks below prove the pin delta, but cannot truthfully satisfy the additional “existing Run mit identischem Inhalt” requirement. No ACCEPT/merge-ready statement is issued while that requested proof is missing.

## Evidence inputs

`evidence/board-2026-10-24/f49-demorepin.md` was read as data and its candidate/CI claims independently checked. The on-disk `codex-verdict-demo120-repin.md` path is currently missing, matching the worker's observation. The earlier conversation contains that review's pin-only recipe; current conclusions rely on fresh immutable source/diff evidence rather than treating a missing artifact as inspectable proof.

## Identity and exhaustive changed-file table

PR: [baseharbor-demo #121](https://github.com/mcpdev80/baseharbor-demo/pull/121), target main, OPEN/unmerged before and after.

| Item | Before = after |
| --- | --- |
| HEAD | `f57c8b6e5ff39d0873df62b26272962765e88d6e` |
| Target main / base | `82d042bc9b2393733858fd5fc9dd42a7f5491ea9` |
| Candidate Git tree | `9a25907b7561d07c734022cf585ee43b06311c6a` |

Exactly **one affected file**, `baseharbor-core.ref`. `gh pr diff 121 --repo mcpdev80/baseharbor-demo` and the full neutral Git tree diff contain exactly:

```diff
-2adae47afddc299d06dbc34eb16a8e011c14b6dc
+1861ed07888a1ec22c218ca491d23ee54f48e261
```

| File / classification | main SHA-256 before = after | HEAD SHA-256 before = after |
| --- | --- | --- |
| `baseharbor-core.ref:1` — only affected file | `ec94c09ef271d04cba2291e3d11b71f0f322394d2713b1c30a55abbad4e05b66` | `a2c83515831054bb90a7df37e7b1c01c12463a3f6478a41b08f3b132a47f58b7` |
| `scripts/failure-runtime-diagnostics.sh` — supplemental exclusion check, unchanged | `650ac86e89d881bda640b49ae2ac177e7510da3df513aef762123b1579ccba8f` | `650ac86e89d881bda640b49ae2ac177e7510da3df513aef762123b1579ccba8f` |

No timeout, executable, workflow, test, documentation or v0.4.27 change appears in the PR diff. The Podman-budget change from #120 is absent. Initial hashes come from neutral `git show <full-sha>:<path>`; final hashes were rechecked through fresh immutable GitHub contents at HEAD and base. The final `gh pr diff` is byte-identical. Neutral store: `/tmp/cmf4-review/objects.git`. No foreign worktree was read or changed.

## Core-anchor justification and workflow precision

Core #1050 is at `388967621c90750e59c5faab35344b46d4108fef`. Its immutable `docs/releases/v0.4.26.consumer-pins.json:44` defines Core C=`1861ed07888a1ec22c218ca491d23ee54f48e261`; the Demo dependency at `:51`, Console dependency at `:58` and Connector dependency at `:66` repeat C. Candidate `baseharbor-core.ref:1` exactly equals C, with one newline. The exact Core commits API returns the same full C SHA, so the object is currently serveable.

The actual pre-release workflow was read at current Core develop `f39295680d41290fc505c0a8cc1769da51699293`:

- `.github/workflows/pre-release.yml:131–134` runs `scripts/ecosystem_release_pins.py` for v0.4.26, passing `--core "$(git rev-parse HEAD)"`, immutable consumer/demo inputs and the pins output path.
- `scripts/ecosystem_release_pins.py:21,25–27` requires exact common Core/Console/Demo bindings; `:124–127` fetches source-owned immutable candidate files and Demo core.ref, then resolves those bindings.
- The mission's `pre-release.yml:122` location is stale: line 122 is now the `else` of the candidate-ancestry check; the relevant resolver call is at 132–134.

**The workflow does not hardcode C.** It requires consumers to match the exact selected Core candidate. C is the correct value for #1050's current canonical snapshot; it is not proof that a later different Core candidate will pass with unchanged consumer pins. Any later Core candidate must have its common immutable consumer bindings reconciled. This precision does not change the correct one-line repair for this review.

## Rules, checks and exhaustive existing-run comparison

`gh pr checks 121 --repo mcpdev80/baseharbor-demo` returns exit 1 with “no checks reported” for `work/v0.4.26-demo-repin`. Effective main branch rules return `[]`; classic protection returns HTTP 404. No required PR status context was found; absence is not a green test result or an evidence waiver.

Actual repo entry points `AGENTS.md` and `CONTRIBUTING.md` were read from the immutable candidate. They require the repo's actual source/review/CI boundaries and do not assert a pin-specific successful runtime proof.

Live workflows API: only `Targeted Demo compatibility` is active, id 379825635, path `.github/workflows/v0424-targeted.yml`. Its `:2–4` triggers only on pushes to three named release branches, `:12` requires a runtime marker, and `:76–80` installs exact Core then runs Backup/Restore acceptance. It has no PR or workflow_dispatch trigger. The other three workflows are `disabled_manually`. No trigger/configuration change or CI launch was made.

The successful-runs query for that active workflow returned **total_count=8, returned=8**: the full successful-run inventory, not a truncated sample. GitHub records these source tree IDs:

| Existing successful run | head SHA | source tree | Equals #121 tree? |
| --- | --- | --- | --- |
| 39990941852 | `25c319697fd3d89d894943e0f44cd3d5dde8b198` | `1a2e53aaf7f228a2dc865133696434c012e4ff20` | No |
| 38557521398 | `d691f68307cbcb8333cdd33aa1af1a92ab41a69e` | `c1526e83dd08e80e105065022d774872896d0f76` | No |
| 38545162417 | `01e98f389b08a58579b8af80c663ddc2045ce7a3` | `f34ed98296fd2ecc7a902165f72056206da5bb4a` | No |
| 38541975235 | `47a34baa71afc3cff006645ab25f5756ba045ab6` | `ba4528a58682c644698be5def443cafc0b05527c` | No |
| 38009135327 | `23dbf9f4f985e175610cf620ef7c3503bc56679e` | `2dcfb4ba5513bfc19fdfbe1fff6a712aed6b5ae7` | No |
| 38005822167 | `b9d47cea12762d29b6f1770bab8eaf8e9185392f` | `0d059f247f301765d0ecfdcb1e1397a1a1c4bffe` | No |
| 38002436014 | `b65416a5c1aeb2dd2debe8ec18846b7e9f503f3b` | `ba914ca4225416393e1095210f1f36da5ff0c11f` | No |
| 37965138143 | `31b40af398ed6276562f5910a65793f1626be80c` | `72b90a138a940a766cf5477c70abebc418d05380` | No |

Candidate tree is `9a25907b7561d07c734022cf585ee43b06311c6a`; none matches. The closest v0.4.26 success is [39990941852](https://github.com/mcpdev80/baseharbor-demo/actions/runs/39990941852) at Demo `25c319697fd3d89d894943e0f44cd3d5dde8b198`. It has the same Core-pin bytes, but its diagnostics script hash is `f31bb1c0131d0f22c8449d696cd0c7bebd4e113211504e25bdf96c217625a276`, different from #121's `650ac86e...`; its workflow artifact handling also differs. Full old-run-source → #121 tree diff affects both the diagnostics script and workflow. It therefore cannot be named as an identical-content qualifying replacement.

The successful old run is context for installing C, not acceptance evidence for this source. No existing verifier proving compatible inputs was supplied or established. This source review's exact diff/hash/Core-availability checks constitute inspectable **pin-level** verification, but are not an existing identical-tree native run. This report does not invent the requested proof or interpret no-checks as success.

### Minimal resolution of the evidence finding

Keep the candidate source unchanged. Supply the required successful existing source-bound identical-content evidence if another qualifying source is known, with actual run/attempt, source tree/inputs and verified outcomes. Alternatively, any future qualification must be explicitly scoped to the changed risk and bind to this exact candidate content. The worker's suggestion of pushing equivalent content to the release branch would launch the existing Docker/Podman Backup/Restore job; this review does **not** select or execute that expensive route, since source pin verification alone does not justify a full Backup/Restore journey. Required native release compatibility remains separate and cannot be silently waived. No rerun, dispatch, release-branch push or matrix was initiated.

## Next merge assignment and downstream DAG

No executable next merge order is issued while the requested proof is missing. Once qualified and freshly rechecked, the next Demo candidate is exactly **PR #121, HEAD `f57c8b6e5ff39d0873df62b26272962765e88d6e`**, with an expected-HEAD guard and current applicable rules under a separately assigned merge mission. No merge authority is exercised by this review.

For the eventual dependency sequence:

1. Merge the qualified Demo candidate only when assigned/permitted; observe the **actual resulting Demo main SHA D**. Do not substitute f57c8b6, 25c31969 or another pre-merge commit for a squash merge result. If additional runtime work must land for the release, its separately qualified final result determines D. #121 itself does not carry the removed Podman-budget implementation.
2. Console #9 is currently OPEN/unmerged at `6017de2845fa3e4e408c69038a0a5bdd7e82cad9`. Set `integration-candidate.json:5` to D, leave `:4` at C=`1861ed07888a1ec22c218ca491d23ee54f48e261`; get fresh exact-candidate review/checks, then merge when assigned and observe actual final Console main U.
3. Connector #10 is currently OPEN/unmerged at `e832d5651752121dea9484e18422d3e6459f9a72`. Keep `integration-candidate.json:4` at C, set `:5` to D and `:6` to U; get fresh review/checks, merge when assigned and observe actual final Connector main K.
4. Reconcile baseharbor #1050 `docs/releases/v0.4.26.consumer-pins.json` from immutable C/D/U/K. Primary fields `:44,49,56,64` become C/D/U/K; repeated Demo refs `:59,67` become D; Console refs `:31,68` become U; Connector refs `:7,14,21,25,35` become K; common Core refs `:51,58,66` remain C for this snapshot. Update every repeated reference, not just four primary fields. Resolve source-owned bindings and acyclicity, and satisfy #1050's actual required checks/evidence. Do not introduce a Core self-reference.

D/U/K do not exist as observed final merge results in this review. The dependency order is Demo → Console → Connector → reconcile #1050. Review of pin-only source does not certify complete release readiness, native evidence, publication or promotion.

## Completion

All requested review questions are answered; source repair verified, required replacement-evidence gap made explicit. Stop after this report; no candidate edits, merge, PR closure, comment/review submission, CI dispatch, rerun, labels, Planning claim/acceptance or release operation. Native TodoWrite is not exposed; the completed plan is above.

Evidence snapshots and raw API/CLI captures: `codex-demo121-data/`. Current HEAD/base and affected-file hashes remained stable.

END CX-DEMO121-DONE — REQUEST_CHANGES for missing identical-content qualification evidence; pin-only source correct.

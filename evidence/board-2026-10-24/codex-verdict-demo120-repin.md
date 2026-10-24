# Demo #120 — exact delta and minimal repin recipe

ROLLE: REVIEW — bounded read-only source review; verdict and recipe. No implementation, merge, push, CI rerun, labels or dispatch.
Reviewer: Codex, separate from the implementation worker; authenticated account `mcpdev80`.
UTC started: 2026-10-24T09:01:17.923115+00:00
UTC final identity/hash capture: 2026-10-24T09:02:57.514767+00:00

## PLAN

- [x] 0. Read COST_CONTROL.md, GITHUB_AUTHORITY.md and cmf2.txt; record this review-only plan. The mission defines no explicit PLAN step 0. Native TodoWrite is unavailable in this session; this file records the checklist instead.
- [x] 1. Capture the live candidate/base, complete `gh pr diff`, immutable blobs and every affected file's SHA-256.
- [x] 2. Compare current main with the exact candidate using left/right history, file history, PR #117 and GitHub compare; determine whether executable changes are already integrated.
- [x] 3. Specify the smallest commit/file/value recipe and actual Demo CI or valid source-verification replacement.
- [x] 4. Record downstream order, #1050 updates and stop criteria; re-fetch candidate/base file contents and full diff.

## Decision

**REQUEST_CHANGES at `bd86c7c3cbf7ebfb6dc5631c917ab244c5ed79c5`. The timeout delta is genuinely absent from current main. For the requested minimal pin-only repair, retain only the Core-pin change and remove the executable delta from this PR.** The runtime change may be useful, but it requires separate qualification if retained or landed separately; it is not an already-integrated change that can be dismissed as commit-ID drift.

The preceding cmf1 verdict is confirmed and narrowed to an actionable recipe. This report does not claim that the timeout increase is inherently incorrect, or that runtime coverage must be launched for a pure pin edit.

## Identity and exhaustive affected-file table

PR: [baseharbor-demo #120](https://github.com/mcpdev80/baseharbor-demo/pull/120), OPEN, unmerged, target `main`.
HEAD before = after: `bd86c7c3cbf7ebfb6dc5631c917ab244c5ed79c5`.
Main/base before = after: `82d042bc9b2393733858fd5fc9dd42a7f5491ea9`.

`gh pr diff 120 --repo mcpdev80/baseharbor-demo` and neutral `git show bd86c7c:<path>` establish exactly these two modified files. The final fresh diff is byte-identical. Every SHA-256 below was checked again using GitHub contents at the immutable head/base; no candidate movement occurred.

| File | Class | main SHA-256 before = after | PR SHA-256 before = after |
| --- | --- | --- | --- |
| `baseharbor-core.ref` | (a) pure Core pin | `ec94c09ef271d04cba2291e3d11b71f0f322394d2713b1c30a55abbad4e05b66` | `a2c83515831054bb90a7df37e7b1c01c12463a3f6478a41b08f3b132a47f58b7` |
| `scripts/failure-runtime-diagnostics.sh` | (b) executable timeout behavior | `650ac86e89d881bda640b49ae2ac177e7510da3df513aef762123b1579ccba8f` | `f31bb1c0131d0f22c8449d696cd0c7bebd4e113211504e25bdf96c217625a276` |

(a) `baseharbor-core.ref:1`: `2adae47afddc299d06dbc34eb16a8e011c14b6dc` → **`1861ed07888a1ec22c218ca491d23ee54f48e261`**.

(b) `scripts/failure-runtime-diagnostics.sh:47`: Podman overall budget 60 → 90 seconds; `:50` and `:52–54`: default per-call ceiling 20 → 60 seconds for Podman. This applies to every default-limited call, including the Core inventory preview, rather than just the first native invocation. Docker remains at 60 seconds overall and 20 seconds per call. No test file or workflow file is changed in this PR. No v0.4.27 material appears in the diff.

## Is the executable change already on main?

No. Neutral bare object store: `/tmp/cmf2-review/objects.git`; no foreign worktree read or changed. Executed the requested comparison as `git --git-dir=... log --left-right --format=... main...bd86c7c...` and GitHub's compare endpoint `repos/mcpdev80/baseharbor-demo/compare/main...bd86c7c` (the `gh compare` operation is provided by `gh api`).

Left/right history contains only these right-side commits, with no left-side commits:

```text
> bd86c7c3cbf7ebfb6dc5631c917ab244c5ed79c5 chore(demo): pin Core 1861ed07 per v0.4.26 consumer-pins graph (#1050)
> 7f18fe79aaadde28a535757dfc66bdf5d9c1dc3d fix(demo): tolerate podman daemon cold-start stall in cleanup-preparation [demo-runtime] (o56b)
```

GitHub compare: `status=ahead`, `ahead_by=2`, `behind_by=0`, exactly the same two changed files. Thus main is an ancestor of this candidate, and the main-vs-candidate runtime file difference is real; no rebase or squash-history inference is needed.

[PR #117](https://github.com/mcpdev80/baseharbor-demo/pull/117) actually merged at `896dc3cb68a8c9b9585940c4696410de5a338c52` on 2026-10-22T18:16:21Z. Its Core pin was 2adae47a. That merge's runtime-file SHA-256 is `87b630925afe71ad8c65b4369396a93fb0665054f463dd93c7d2a162d3e0db48`, not the candidate's hash. Current main subsequently changed the runtime file in #118 at 82d042bc, but its current bytes still use the old timeout settings and differ from the PR. File history on main has no 7f18fe7/25c31969 timeout patch; direct content comparison confirms that no later main commit has supplied equivalent current contents.

The runtime blob at the candidate is the same as older release-branch commit `25c319697fd3d89d894943e0f44cd3d5dde8b198`. That confirms its origin, not integration into main.

## Minimal worker recipe — one Core-pin file

1. Refresh live main and #120 without modifying another session's checkout. Continue the owning worker's existing PR/branch and preserve its history. The desired main-based patch is exactly the single commit **`bd86c7c3cbf7ebfb6dc5631c917ab244c5ed79c5`**, cherry-picked by itself; **do not include its parent `7f18fe79aaadde28a535757dfc66bdf5d9c1dc3d`**. `git diff bd86c7c^ bd86c7c` changes only `baseharbor-core.ref:1`. The parent commit's old pin is byte-identical to current main's old pin, so the pin-only patch has matching input bytes on the observed main.
2. Since #120 already exists, do not force-push/rewrite it just to obtain that ideal history. The minimal normal-push correction is a new commit on its branch restoring **only** `scripts/failure-runtime-diagnostics.sh` to the freshly integrated main version. At the reviewed main, that is SHA-256 `650ac86e89d881bda640b49ae2ac177e7510da3df513aef762123b1579ccba8f`. A scoped revert of 7f18fe7 has the same effect at this observed head. If main advances, integrate normally and reassess; do not overwrite newer owned changes blindly.
3. Leave `baseharbor-core.ref:1` at **`1861ed07888a1ec22c218ca491d23ee54f48e261` plus newline**, SHA-256 `a2c83515831054bb90a7df37e7b1c01c12463a3f6478a41b08f3b132a47f58b7`. The resulting full target-branch diff must contain exactly that one file and one line replacement. No tests or workflows need changing for this correction.

Why 1861ed07: Core #1050 at `388967621c90750e59c5faab35344b46d4108fef`, `docs/releases/v0.4.26.consumer-pins.json:44,51,58,66`, requires this common exact Core anchor. Core `scripts/ecosystem_release_pins.py:21,25–27` requires Connector, Console and Demo to match that Core exactly. Demo `.github/workflows/v0424-targeted.yml:46–64` consumes a 40-hex `baseharbor-core.ref` and checks that the exact object is serveable; it does not require the old 2adae47a anchor. Authenticated `gh api .../commits/1861ed07...` returned that exact full SHA during this review. No repo contract requiring a different anchor was found.

This recipe removes the timeout feature from this pin-only candidate; it does not establish that the released Demo no longer needs it. If release acceptance requires the 25c31969 timeout behavior, land that separately with its own evidence first and select the final Demo SHA only afterward. Do not label a pin-only repair as carrying the old timeout implementation.

## Actual Demo gates and justified replacement evidence

There is **no PR-triggered Demo CI** in the current source. `.github/workflows/v0424-targeted.yml:2–4` triggers only on pushes to three named release branches; `:12` additionally requires a runtime marker in the pushed commit message. Its native job installs exact Core then runs `scripts/acceptance.sh backup-restore` (`:76–80`). There is no dispatch or PR trigger in that workflow. The live workflows API lists that workflow active and three other workflows manually disabled. `gh pr checks` reports no checks for #120, and the Actions runs query for its exact full head returns `total_count=0`. Effective main branch rules are `[]`. Absence of CI is recorded as absence, not PASS.

**For the repaired pin-only candidate**, the proportionate replacement is an inspectable source-verification record tied to its new full SHA and fresh main base:

- Complete `gh pr diff` / immutable Git object inspection confirms exactly `baseharbor-core.ref:1` changes, to the exact full C value above. Compare the full tree/diff, not just that file, to exclude retained timeout/test/workflow changes.
- Check the file's exact bytes/40-hex format and SHA-256; resolve the exact Core object through GitHub and require returned `sha == C`.
- Compare C to the canonical #1050 common Core binding and the consumer resolver's strict equality contract; record all immutable source SHAs and before/after identities. Source review of the new candidate must confirm this scope.

These checks prove the actual risks of the **one-file pin-only delta**: intended authoritative anchor, immutable object availability and absence of executable changes. They are a valid replacement for nonexistent PR source CI for this narrow review, not a claim of native compatibility, a named CI check, or release readiness. Record the worker's actual commands/results at the repaired SHA; this session cannot provide results for a commit that does not exist yet. Existing canonical runtime/consumer acceptance requirements still apply to the eventual final ecosystem, and missing required evidence blocks that acceptance. No new runtime journey is justified solely to test pin-file syntax or diff scope.

**If the timeout code is retained**, this is a separate runtime qualification question. Existing run [39990941852](https://github.com/mcpdev80/baseharbor-demo/actions/runs/39990941852), attempt 1, succeeded on `25c319697fd3d89d894943e0f44cd3d5dde8b198`; it is not a run on bd86c7c. The entire trees differ in `.github/workflows/v0424-targeted.yml:91` (retained cleanup scope receipt versus public-cache log), so neither commit identity nor full input equality is established. Reuse requires an actual verifier proving compatible relevant inputs and retained source-bound cleanup evidence; none is established here.

The minimum new behavioral proof, if reuse is unavailable, must exercise the exact changed script and selected Core on an isolated owned fixture: delayed Podman inventory that exceeds the old ceiling but succeeds within the new ceiling, a genuinely hung call that terminates within the configured budget, preserved ownership/foreign-container and cleanup accounting, with retained `failure-runtime/cleanup-preparation/scope-result.json`. Verify default Core-preview calls as well because the new per-call value applies there too. A bounded focused harness can cover timeout mechanics; real cold-runtime behavior requires the corresponding bounded Podman proof if claimed. The repo has no dedicated existing PR job for this slice. Do not represent a prospective harness as an existing passed gate, or automatically launch the heavy Backup/Restore job as a substitute. No runtime tests or CI were launched by this read-only mission.

Existing `tests/debug-cleanup-provenance-order/run.sh` checks mocked ownership/ordering, not the changed timeout threshold. `tests/debug-podman-cleanup-accounting/run.sh` explicitly suppresses the diagnostics exit with `|| true` and can print PASS without validating a successful scope receipt; it is not valid timeout qualification. Neither test's worker-reported success closes this runtime evidence gap.

## Downstream consequences and dependency order

Current Console #9 remains `6017de2845fa3e4e408c69038a0a5bdd7e82cad9`; Connector #9 remains `2223506cca5c388752a6d6c88567fcc77f46119b` at this capture. No existing PR was changed.

1. After the corrected Demo candidate is reviewed/accepted and a merge is permitted, observe its **actual final merged main commit D**, not the old release-branch SHA or the repaired PR HEAD. If a separately qualified runtime change must land, complete it before declaring final D.
2. Console #9 `integration-candidate.json:5` changes from `25c319697fd3d89d894943e0f44cd3d5dde8b198` to D; `:4` stays C = `1861ed07888a1ec22c218ca491d23ee54f48e261`. Review/check its new exact SHA, merge when permitted, observe actual final Console main U.
3. Connector #9 `integration-candidate.json:4` must change from `2adae47afddc299d06dbc34eb16a8e011c14b6dc` to C, `:5` from the old Demo pin to D, and `:6` from `69d1d14007fa43dc0c0457c3347280be7f802818` to U. Review/check that new SHA, merge when permitted, observe final Connector main K. Do not substitute divergent main `7c7093d` for the canonical historical Connector anchor `df1f4378` as if they were equivalent.
4. Regenerate #1050's `docs/releases/v0.4.26.consumer-pins.json` from the actual immutable C/D/U/K source files. Primary fields `:44,49,56,64` become C/D/U/K. Repeated references also change: Demo D at `:59,67`; Console U at `:31,68`; Connector K at `:7,14,21,25,35`. Core references `:51,58,66` stay C. Do not pin Core to #1050's own future merge SHA. Resolve and verify the acyclic graph against real source-owned bindings; four primary field edits alone are insufficient.

D/U/K are future merge results, so no exact literal is knowable yet. Use the actual 40-hex observed commits in that order. A cherry-pick or squash changes commit identity even when file contents match. Changed candidates require fresh source review and their applicable checks. Prior green native evidence remains reusable only if the existing verifier proves compatibility. This review does not clear #1050's independently reported required-check problem or authorize a release.

## Stop criterion

The review assignment is complete when the exact diff, main integration status, minimal recipe, applicable proof and downstream order are recorded and the candidate/base/hashes remain stable. Those conditions are satisfied. **Stop after this report; no implementation or merge.** Current #120 stays REQUEST_CHANGES until its owning worker supplies a reviewed new pin-only candidate plus the source-verification record, or supplies qualified runtime evidence for an explicitly broader candidate. Stop and refresh if either HEAD/base moves; do not transfer this verdict to an unreviewed SHA. The coordinator cost stop and Maintainer pause remain in force.

Raw read-only captures, exact patch, history and source snapshots: `codex-demo120-repin-data/`. No Planning task ID was assigned; no Planning ownership or acceptance changed. Native TodoWrite is not exposed and could not be used.

END CX-DEMO120-DONE

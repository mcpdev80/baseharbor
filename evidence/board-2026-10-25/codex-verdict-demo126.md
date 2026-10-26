# CMF25 — demo PR #126 independent review

ROLLE: REVIEW. Started UTC: 2026-10-26T14:07:08Z.
Candidate: `6cecb74f00637750a6ea771a4815325283fa9dd3`; requested base: `9493e82`.
Scope: read-only product review; no merge, push, implementation, or runtime dispatch.

## PLAN

- [x] 0. Read binding policies and mission; establish review scope and tooling limits.
- [x] 1. Confirm live candidate/base, applicable rules, and exact complete diff.
- [x] 2. Review both identity comparisons, preserved guards, set uniqueness, and o144 facts.
- [x] 3. Run isolated Python syntax/import and focused semantic checks; inspect remove/retention at `a39e6bf5`.
- [x] 4. Recheck candidate stability; record verdict, actionable recipe, and runtime qualification requirements.

The supplied mission contains no explicit PLAN step 0. The numbered plan above
implements the requested starting step. No native TodoWrite/update_plan tool is
exposed in this session (tool metadata searched); native TodoWrite cannot be
performed or truthfully claimed. Progress is recorded here.

## Bootstrap disposition

`basics-check.py` ran before mission review and reported pending events/reminders,
pool shortfall, and coordinator PLAN age/open steps. This is a separate bounded
Codex REVIEW lane, not a coordinator takeover. Pool dispatch/implementation and
merge are outside scope and remain stopped by binding policies. Existing
coordinator plans, sessions, and event cursors are preserved. The two observed
events concern discovery/idle of `e2adcc6a-b6ff-47ca-bbca-ad521c0ae22e`; neither
proves success. The 80 reminders concern coordinator lanes/PR #1077 and unknown
pane observations; no dispatch, merge, removal, or completion is inferred.
Shared queues are not acknowledged as completed by this unrelated reviewer.

## repo-rule compliance

Authenticated GitHub account verified as `mcpdev80`. Existing untracked files and
product worktrees preserved. Mission is explicitly review-only and no product
or Planning writes/pushes are authorized by it. Independent review is allowed
under COST_CONTROL.md; Maintainer pause and inactive production remain in force.

## Exact source and evidence

Completed UTC: 2026-10-26T14:08:14Z (final live API verification immediately
before the verdict). Reviewer: this independent Codex session, CMF25; no worker
implementation performed. No Horde task/model-backend identifier is exposed;
none is fabricated. This is a local source review, not merge-gate approval.

GitHub API verified PR #126 OPEN before and after checks, HEAD
`6cecb74f00637750a6ea771a4815325283fa9dd3`, base `main`
`9493e82faa66ade9c4bacbdb2c269d1f065adf39`, also the candidate's sole parent.
Complete diff: one file, +9/-2. Captured in [cmf25-demo126.diff](cmf25-demo126.diff),
SHA256 `8e9a74aeb5d651b30820b82d2846e3ffc3f6eb872101deee8afb1f22ffdffa10`.
Product tracked worktree untouched; initial/final untracked file lists agree.

Planning HEAD and GitHub main both resolve to
`f8157ecf9c349aec021c36537a9db9cebca9aec5`; local SSH remote lookup failed due
to system ssh_config permissions, so authenticated read-only GitHub API was
used instead. No Planning task ID is assigned in CMF25; existing ownership was
not changed. Historical Planning resume/authority text differs from the loaded
binding bootstrap policies; this review does not resolve or exploit that
conflict. The mission's explicit no-merge/no-push scope applies throughout.

## Findings and semantics

The implementation at `tests/data-capabilities/new_edit.py:217–220` is exactly
the requested `(Name, sorted Labels items, CreatedAt)` identity. Namespace,
Options, and other inspect-only fields do not participate. `None` and empty
Labels normalize to the same empty tuple, as in the proposed recipe.

Both comparison sites are changed:

- Lines 228–229 replace raw retained volume dictionaries with identity sets.
  Lines 226–227 preserve the preceding exact mount-name-set match, and lines
  232–234 preserve exactly-one-container detection.
- Diff hunk `@@ -354,7 +361,7 @@` changes only the final foreign-volume operand
  at line 364. The preceding container `Id`, `State.Paused is True`, and
  `Config.Labels` conditions are byte-for-byte unchanged (lines 362–363).
  No other assertion is removed or relaxed.

Two distinct valid named volumes can share labels and creation time but cannot
share the full tuple because `Name` is included. In the same runtime namespace,
two mounts of the same named volume refer to the same volume; `volumes()`
already deduplicated these through its name-keyed dictionary before this PR.
Two containers mounting the same set still produce two matches and fail the
unchanged uniqueness guard. Malformed inspect output with duplicate/missing
Names could collapse identity tuples, but that requires violating native
inspect-by-name semantics; this PR does not introduce such an output source.
No valid two-volume ambiguity was found.

**P2 — Required durable regression coverage missing.** o144 §4.3 item 2 explicitly
requires `retained_sql_identity_test` in
`tests/data-capabilities/new_edit_helpers_test.py`. That file exists but is
unchanged, and its ten tests do not exercise `_volume_identity` or retained SQL
metadata normalization. Review-only probes below verify current behavior but
do not satisfy the requested repository regression coverage.

o144 evidence qualifications: §4.2 itself names only the retained_sql site;
the second foreign-volume site is explicitly named by CMF25 and the q126
handoff. o144's broader claim that existing foreign guards already use only
three fields is incomplete: unchanged `verify_foreign:103–104` still compares
the full receipt volume dict during finalization. No speculative expansion is
requested here; the downstream runtime run must exercise finalization as well.
The precise Namespace/Options drift cause is not independently established by
this source review; it must not be reported as observed runtime evidence.

## Product fact check

Inspected immutable Core `a39e6bf5a72d5b5e651cf8fb1a69ad96331c736c`:

- `cmd/baha/application_provider_retirement.go:74` dispatches retirement;
  shared backend retirement calls `ConvergeExistingSharedBackendRuntime`.
- `internal/application/shared_backends_convergence_runtime.go:95` inspects
  existing resources and selects services; its volume loop at lines 140 onward
  verifies existence and records retention, without writing volume labels.
- `cmd/baha/application_native_backend_retirement.go:27` selects released
  containers only and calls `internal/runtime/service_retirement.go:21`
  `RetireOwnedServices`, which passes only exact containers to the mutation
  adapter and verifies that nonselected volumes remain.

These sampled remove/retention paths contain no volume-label update. This
supports the harness-scoped change; it is not a blanket proof about every
provider operation or native engine metadata stability.

## Verification

Reproduce with `python3 evidence/board-2026-10-25/scripts/cmf25-review.py`.
The script archives the exact SHA into a temporary directory and never invokes
a real container engine. [cmf25-checks.log](cmf25-checks.log) records:

- PASS `py_compile` and actual import of candidate plus guided helpers.
- PASS AST equality for six surrounding functions: `verify_foreign`,
  `foreign_finalize`, `owned_postgres`, `is_postgres_server`, `native`, `volumes`.
- PASS all 10 existing helper tests with docker selected and all 10 with podman
  selected (unit tests, not native runtime success).
- PASS focused checks for ignored Namespace/Options drift, label ordering,
  rejection of Name/Labels/CreatedAt changes, two-volume identity cardinality,
  duplicate-container rejection, and mismatched mount-set rejection.

Exact diff contains no added credentials, secret output, imports, or docker-only
paths. The helper is runtime-independent. GitHub reports zero check runs and
zero commit statuses; aggregate status `pending` is not green CI. At the
candidate SHA the sole workflow is `.github/workflows/v0424-targeted.yml`, with
push triggers restricted to three enumerated `work/v0.4.*-ecosystem-release`
branches, not this fix branch, and no pull_request trigger. No workflow or CI
policy was changed and no rerun was dispatched.

## Verdict and actionable recipe

**REQUEST_CHANGES** for the missing required regression test; no functional
defect found in the proposed tuple comparison under valid native inspect data.

Implementation owner recipe:

1. Edit only `tests/data-capabilities/new_edit_helpers_test.py` to add a
   retained-SQL identity regression using the existing imported fixture and
   `unittest.mock.patch`. Exercise the real `retained_sql`, mock only
   `owned_postgres`/`native` inspect responses, and preserve production helpers.
2. Assert equal Name/Labels/CreatedAt with differing Namespace/Options succeeds;
   reordered labels succeed; individual Name, Labels, and CreatedAt changes
   raise `retained SQL volume metadata changed`. Include two distinct names
   sharing labels/time and duplicate matching containers (uniqueness error).
3. Run the helper suite under both docker/podman environment selections and
   compile the changed Python files. Publish one coherent corrected candidate
   through its existing owner; obtain review of the new exact SHA.
4. Stop the correction round when those durable tests and source review pass;
   do not widen product scope or weaken further guards. After a separately
   authorized, gated merge into demo `main`, rerun baseharbor #1077's affected
   Podman data-capabilities job against the resolved new demo SHA as runtime
   qualification, including foreign finalization. Satisfy o144's affected Docker
   coverage too; reuse only demonstrably compatible green evidence, otherwise
   dispatch Docker and Podman gates in parallel. Missing/red runtime evidence
   prevents runtime acceptance. No full release matrix is requested.

This review performs no merge/push and claims no runtime qualification or
Planning acceptance. Native TodoWrite remains unavailable as recorded above.

END CMF25-REVIEW-DONE

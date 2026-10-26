# CMF26 — demo #126 independent review, round 2

ROLLE: REVIEW. Candidate: `55e6f87929cf659fda4f2a1f510305a56f3f48eb`.
Scope: local exact-source review and evidence only; no push or merge.

## PLAN

- [x] 0. Read binding policies and mission; establish scope and tool availability.
- [x] 1. Verify candidate, parent, current remote base/PR and complete diff.
- [x] 2. Map every CMF25 regression requirement to real test assertions; verify mock boundaries and unchanged production guards.
- [x] 3. Execute the requested suite, both runtime selections, and Python compilation; retain output.
- [x] 4. Verify unchanged candidate and publish verdict, owner recipe and conditional FF-push/merge guidance.

The mission contains no literal PLAN step 0; the user-requested starting step is
recorded above. Native TodoWrite/update_plan is not exposed in this session;
available tool metadata was inspected. Native TodoWrite cannot truthfully be
claimed. This evidence checklist records progress.

## Identity, source and scope

Review UTC: 2026-10-26T14:26:57Z through 2026-10-26T14:27:33Z (final remote
and candidate verification). Reviewer: this separate Codex CMF26 session,
not the F66 implementation worker. No Horde task or actual backend model ID
is exposed; neither is invented. Coordinator records CMF26 lane `d1a9c30c`;
this is contextual data, not an independently verified full session ID.

Authenticated account: `mcpdev80`. GitHub confirms demo #126 OPEN, remote branch
`fix/podman-volume-metadata-compare` at
`6cecb74f00637750a6ea771a4815325283fa9dd3`, base main at
`9493e82faa66ade9c4bacbdb2c269d1f065adf39`. Local candidate HEAD and branch
remain `55e6f87929cf659fda4f2a1f510305a56f3f48eb`, with exactly that remote
candidate as its sole parent. The candidate has not yet been pushed.

Local `main` is stale (`8b26585fa9c6b6c541216f7f4a63270fa4410462`); its literal
diff includes unrelated history. The authoritative current GitHub main and
local `origin/main` both resolve to `9493e82`; review uses that immutable base.
No refs were reset or branches switched. Planning local HEAD/origin/main and
GitHub main agree at `f8157ecf9c349aec021c36537a9db9cebca9aec5`.

Captured diffs:

- [Correction diff](cmf26-correction.diff): parent to candidate, only
  `tests/data-capabilities/new_edit_helpers_test.py`, +65 lines.
  SHA256 `021235dfef59f726dbd1f782b9ab0446a050cf1021cdc6cb6076f61b5f65638c`.
- [Complete diff](cmf26-complete.diff): current remote main to candidate,
  only that test file and `tests/data-capabilities/new_edit.py`.
  SHA256 `1d1ee99aa0f03235aa79c3b2583cc9010e76b4f629511ab244de7fa2ea0944c4`.

## Requirements and findings

**No blocking source-review findings.** New durable coverage closes CMF25's P2.
Line references below are in candidate `new_edit_helpers_test.py`.

| Requirement | Actual assertion | Result |
| --- | --- | --- |
| Ignore Namespace/Options drift | 251–254, same identity and different namespace/device/options, real retained_sql returns container ID | PASS |
| Label order irrelevant | 256–260, reversed insertion order, helper equality and real retained_sql success | PASS |
| Reject Name, Labels, CreatedAt individually | 262–274, three isolated subtests, exact metadata error substring | PASS |
| Two distinct Names with equal Labels/time | 276–284, identity cardinality two, successful two-volume binding, collapsed identity rejected | PASS |
| Duplicate matching containers | 286–291, two IDs sharing expected volume, uniqueness error | PASS |

`drive` (244–249) patches only `fixture.owned_postgres` and `fixture.native`.
It calls real `retained_sql`; real `volumes` parses native-shaped JSON and
real `_volume_identity` performs normalization. The native stub asserts
`volume inspect` and selects responses by requested volume name. Previous
tests and assertions are unchanged; mocks elsewhere belong to existing tests.
No production helper was patched or weakened by the correction.

Production file is byte-identical to `6cecb74`. Relative to live main,
`new_edit.py:217–220` adds exactly `(Name, sorted Labels items, CreatedAt)`;
228–229 use identity sets. The preceding mounted-name-set equality and
exactly-one-container guard are unchanged. Diff hunk `@@ -354,7 +361,7 @@`
changes only the foreign-volume comparison at 364; container Id, Paused and
Config.Labels guards at 362–363 are unchanged. AST comparison confirms unchanged
`verify_foreign`, `foreign_finalize`, `owned_postgres`, `is_postgres_server`,
`native`, and `volumes`. Only existing functions `retained_sql` and `main`
change relative to current remote main.

This meets o144 §4's tuple and unit-regression requirements, plus CMF25's
explicit second-site instruction. The complete production diff remains exactly
as reviewed in CMF25. CMF25's caveat still applies: `verify_foreign` compares a
full receipt volume dictionary during finalization; actual finalization must
be covered by downstream runtime qualification. No additional defect or scope
expansion is inferred from that unchanged code.

## Verification

[Raw checks](cmf26-checks.log) retain independently executed results:

- `python3 tests/data-capabilities/new_edit_helpers_test.py`: 15 tests, OK.
- Same suite with explicit docker selection and `-v`: 15 tests, OK.
- Same suite with explicit podman selection and `-v`: 15 tests, OK.
- `python3 -m py_compile tests/data-capabilities/new_edit.py tests/data-capabilities/new_edit_helpers_test.py`: exit 0.
- Complete diff whitespace check: clean.

These are Python unit tests with runtime environment selection, not native
Docker/Podman success. No engine or CI rerun was dispatched. Remote old HEAD
has zero check runs/status entries, aggregate pending; that is not green CI
and says nothing about hosted qualification of unpublished `55e6f87`.

Tracked product state remained unchanged before/after review. Pre-existing
untracked `tests/data-capabilities/__pycache__/` and `tests/guided/__pycache__/`
remain; Python tests/compilation may refresh cache contents. No cleanup of
foreign untracked files was attempted.

## repo-rule compliance and bootstrap disposition

Binding cost/authority policies were read first; basics-check then ran before
mission work. Independent bounded review is permitted. No implementation,
push, merge, acceptance, release or deployment occurred; production remains
inactive and the Maintainer pause is not lifted by this review.

Basics flagged two events, reminders, pool shortfall and 32 open coordinator
steps. Follow-up events-check found no unread events (shared coordinator had
advanced its cursor). Reminders concern coordinator PLAN age/open steps,
historical pane observations and #1077 triggers. Coordinator plan already
records F66 completion and CMF26 running. No reminder proves task completion
or grants merge authority. Existing coordinator plans, shared reminder cursors,
worker ownership and sessions are preserved; no pool refill or coordinator
takeover is within this review assignment. No task-ID/Planning write is assigned
by CMF26; its explicit no-push scope takes precedence over routine publication.
Historical Planning resumption text conflicts with loaded bootstrap stops;
this review does not use it to authorize implementation or merge.

## Verdict and actionable owner recipe

**ACCEPT** — exact source candidate
`55e6f87929cf659fda4f2a1f510305a56f3f48eb` satisfies this correction review.
This is source-review acceptance, not runtime or Planning acceptance.

1. **Explicit FF-push recommendation:** the existing authorized owner should
   publish `55e6f87` to `fix/podman-volume-metadata-compare` by ordinary
   fast-forward push, after rechecking remote HEAD is still `6cecb74` and the
   candidate/base remain as reviewed. No force/amend; preserve concurrent work.
   No further file changes are requested. A changed candidate needs fresh review.
2. A separate authorized MERGE lane must recheck current PR/base, ownership,
   effective repository rules, unresolved findings/threads and required CI for
   the exact published candidate. Missing required checks block merge. Source
   ACCEPT does not lift the Maintainer pause or waive protection. Do not infer
   demo-specific bypass authority from the core develop policy.
3. After separately authorized, gated demo merge, rerun baseharbor #1077's
   affected Podman data-capabilities job against the resolved merged demo SHA,
   including retained SQL and foreign finalization. This is mandatory runtime
   qualification; no success is claimed here. Qualify affected Docker coverage
   too, reusing only demonstrably valid evidence; if both gates need execution,
   dispatch them in parallel. Do not repeat unrelated green gates or a full
   release matrix.
4. Stop this correction round at this source ACCEPT. Runtime acceptance and
   subsequent core #1077 merge/Planning closure require their own complete
   exact-candidate evidence and authorized gates. Red, missing or uncertain
   required evidence stops that acceptance; workers diagnose any concrete failure.

END CMF26-REVIEW-DONE

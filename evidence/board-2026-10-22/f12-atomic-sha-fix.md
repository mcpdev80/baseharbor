# F12 — Atomic gate bare-SHA dispatch fix (workflow/scripts)

Lane: `f12-atomic-sha-fix` (session e3c18ad2-3e2b-4e24-8858-0ec00bc9666f)
Recorded: 2026-10-23 UTC
Mode: IMPLEMENT (owner-direct dispatch; no plan task id found via `plan.py list` grep atomic/gate/f12)
Prompt/contract: owner directive this turn + recipe in `o33-atomic-gate-c2.md` (verdict STILL-BROKEN(recipe))
Repository: baseharbor (core) — branch `fix/atomic-bare-sha-dispatch` from develop at
C2 = `2f005813dc6d7fa94de164388b2531d025af31bc` (verified == `refs/heads/develop` remote HEAD).
PR target: develop. No merges from this lane.

## Problem (deterministic, per o33 + checkout v7 source verification this lane)

`targeted-demo-acceptance.yml` advertises its `ref` dispatch input as "ref or commit SHA".
`actions/checkout@3d3c42e5` (v7.0.1) classifies the ref only in
`input-helper.ts`: a value matches `^(?:[0-9a-fA-F]{40}|[0-9a-fA-F]{64})$` and is fetched as a
commit; **anything else — including a short/bare SHA — is fetched via the refspec
`+refs/heads/<value>*`**, which matches nothing for a SHA. Run 39856259550 (podman
backup-restore `Checkout BaseHarbor candidate`): refspec `+refs/heads/2adae47a*`, 3 silent
retry back-offs, git exit 1; cleanup/evidence steps then fail as post-conditions.
Full 40-hex SHAs do work in this repo's practice (pre-release.yml passes `candidate_sha`
to the same checkout for every atomic gate) — the defect class is **unresolved bare/short
SHA and nonexistent ref names reaching checkout unvalidated**.

## Fix scope (owner recipe, o33 §Recipe)

1. Resolve-side normalization in the `resolve` job: a dispatch `ref` is resolved via the
   GitHub API **before** any gate checkout — short hex expands to a full commit SHA,
   branch/tag names are qualified to `refs/heads/...`/`refs/tags/...`, full SHAs are
   existence-checked, and anything unresolvable fails fast with an explicit
   `::error::` message instead of three silent retries. Implemented as
   `scripts/targeted-gate-ref.py` (pure stdlib, `gh` injectable for the selftest).
   The `resolve` job now checks out the run's own commit (`github.sha`) in both events so
   the script is available in the dispatch path too.
2. Suite-side fail-fast in `pre-release-runtime-suite.yml` before
   `Checkout BaseHarbor candidate`: reject empty / unresolvable bare-SHA-shaped inputs with
   the recipe message (defense in depth; the suite cannot use the script before checkout).
3. Evidence/diagnostics uploads in the suite: checkout failure no longer masks as
   "no diagnostics uploaded" — the two `if-no-files-found: warn` uploads get
   `steps.checkout.outcome == 'success'` guards and become `error`.
4. Regression proof (o33-prescribed dispatch-gate selftest): `scripts/test_targeted_gate_ref.py`,
   wired into the existing CI step that runs the scripts selftests; covers the exact
   o33 scenario `ref=2adae47a` plus the full classification table, mocks `gh` (no network),
   and pins the workflow wiring by parsing both YAML files (fail-fast step before checkout,
   no `warn` left on the guarded uploads, resolve wiring).

## PLAN — f12-atomic-sha-fix

1. [x] Branch + worktree: `fix/atomic-bare-sha-dispatch` from `2f005813` (develop HEAD,
       verified via `git ls-remote`), isolated worktree `worktrees/f12-atomic-bare-sha-dispatch`;
       no shared tree touched (main `baseharbor` clone left on its foreign docs branch — see findings).
2. [x] Add `scripts/targeted-gate-ref.py` (classification: refs/ passthrough, full-40/64
       existence check, short-hex branch→tag→commit-resolution, name→heads/tags,
       fail-fast `::error::` exit 2; stdout = checkout-safe ref).
3. [x] Wire `targeted-demo-acceptance.yml`: resolve-job tooling checkout on all events,
       dispatch ref through the script, input description corrected.
4. [x] Harden `pre-release-runtime-suite.yml`: inline Validate-candidate-ref step before
       checkout; two diagnostics uploads warn→error + checkout-outcome guard; demo
       diagnostics preserve step guarantees a summary file so `error` cannot spuriously fail.
5. [x] Add `scripts/test_targeted_gate_ref.py` selftest (o33 repro `2adae47a`, mock `gh`,
       YAML wiring assertions, executed inline guard) wired into BOTH ci.yml script steps:
       PR · Focused Python (executes on this PR) and release_source_preflight evidence step.
6. [x] Local checks: selftest 14/14; test_pre_release_resume, test_targeted_atomic_plan,
       test_artifact_gc, test_ecosystem_release_pins all OK; yaml parse OK; `bash -n` on the
       resolve run OK; ci-workflow-policy PASS (19 files). No product matrix (CI-only change).
7. [x] Commits `661b5e71` (fix + selftest) and `398bbdbd` (Focused-Python wiring); PR
       https://github.com/mcpdev80/baseharbor/pull/1042 → develop. Checks terminal (below).
8. [x] Stop conditions met: PR created, checks terminal, this file complete. No merge, no
       tag, no release, no gate dispatch performed by this lane.

## Findings / state

- checkout v7.0.1 source (`src/input-helper.ts` @ pinned commit, read this lane): only
  `^(?:[0-9a-fA-F]{40}|[0-9a-fA-F]{64})$` becomes a commit fetch; everything else, including
  a short SHA, uses the `+refs/heads/<value>*` refspec — o33's deterministic failure mode.
  o33's mechanism text ("bare 40-hex cannot resolve") is imprecise: full SHAs fetch by commit
  and demonstrably work (pre-release.yml passes `candidate_sha` to this same checkout across
  the whole atomic matrix); the real defect classes are (a) short/bare SHAs and names that
  are not refs, and (b) per o26's finding, malformed/nonexistent full SHAs falling into the
  same branch-prefix fallback. The fix covers both (existence check for full SHAs).
- Remote truth: `refs/heads/develop` == C2 == `2f005813dc6d7fa94de164388b2531d025af31bc`
  at dispatch; branch base verified against that.
- Host observation for coordinator (not touched): the main `baseharbor` clone's HEAD sits on
  `refs/remotes/origin/docs/v0426-notes-corrections` (a non-heads ref), which breaks
  `git worktree add -b` there ("fatal: HEAD not found below refs/heads!"); worked around with
  `--detach` + local `switch -c`. Root `evidence/` is not under git (workspace-local lane files).
- Turn-start flags processed: idle lanes o26-selected-checks and o33-atomic-gate-c2 verified
  complete (o26 report final; o33 has `O33-ATOMIC-DONE`) and deleted; events acked. Open
  coordinator items seen in events, NOT owned by this lane: q29-118-headcheck busy with no
  evidence/PLAN file; o33 Planning-Abschluss publish; REVIEW1038/NOTESQA pool work.
- PR 1042 run states (head `398bbdbd`, run 39876033945):
  - pass: Trusted Plan, ci-policy, ci-actionlint, ci-executor-verifier-tests,
    pr-planner-tests, Workflow policy, PR · Focused Python (selftest executed:
    `python3 scripts/test_targeted_gate_ref.py` → "Ran 14 tests … OK").
  - fail (as designed, deterministic): PR Selected Checks →
    `BLOCKED: coverage-unresolved` listing exactly the 5 CI-surface paths — the repo's
    conservative fail-closed planner contract ("unknown impact must block explicitly";
    workflow/script changes get resolved only by the #1040-style qualification binding that
    follows an independent review — deliberately NOT self-certified in this lane).
  - skipping: all matrix/consumer/HA gates (correct: plan selects zero gates for this diff).
- **Next trigger (review/merge lane):** CODEX-REVIEW of `398bbdbd` (diff, this evidence),
  then a separate worker qualification PR adding the exact-inventory binding to
  `scripts/pr-gate-plan.py` per the CORE_893/#1040 pattern (paths + statuses + reviewed SHA +
  clean merge-chain proof); after that "PR Selected Checks" can pass and merge may proceed.
- Live-gate stop criterion (o33 §5 recipe): after merge, dispatch
  `targeted-demo-acceptance.yml` once with `ref: develop` and once with a bare short SHA to
  confirm fail-fast/normalization; VM dispatch evidence only per task contract — not run here.

## Handoff

- Branch/worktree kept: `fix/atomic-bare-sha-dispatch` @ `398bbdbd` in
  `worktrees/f12-atomic-bare-sha-dispatch` for the correction round.
- PR: https://github.com/mcpdev80/baseharbor/pull/1042 (base develop @ `2f005813`).
- Commits: `661b5e71` fix(ci): resolve bare-SHA dispatch inputs before atomic gate checkout;
  `398bbdbd` ci(focused-python): execute the dispatch-gate ref selftest on PR checks.

## repo-rule compliance

- baseharbor/AGENTS.md: worktree isolation (no shared trees modified), branch from verified
  develop remote HEAD, smallest-fitting checks (scripts selftests + validations; CI-only diff
  → no product matrix, no heavyweight gates dispatched), batched candidate before push, no
  CI-loop pushes (one follow-up push for the missing PR-CI wiring, recorded above), checks
  read to terminal before reporting, no self-approval or binding self-certification.
- AGENTS.md (ecosystem): turn-start basics-check ran first; flags processed in-chain; events
  acked; idle lanes verified before removal; no merges/dispatch/releases; write-first PLAN
  kept native (todowrite) and in this file.

## repo-rule compliance

- baseharbor/AGENTS.md: worktree isolation, branch from current develop remote verified,
  targeted local checks before push, smallest-fitting CI (scripts selftests; this change is
  CI/workflow-only so no product matrix), PR flow with independent review, no self-merge.
- o33 recipe implemented without scope creep.

F12 lane end token: `F12-ATOMIC-FIX-DONE`

F12-ATOMIC-FIX-DONE

## OWNER HOLD (2026-10-23, this turn)

Owner directive: **HOLD on the #1042 selector failure** — q30-1039-checks now owns the
cross-branch selector fix (same class as #1039). Await explicit UNHOLD before any further
action on PR #1042 or its "PR Selected Checks" outcome.

- Complied: no pushes, no comments, no dispatches, no qualification/binding work on #1042.
  Watchdog event "PR #1042 MERGEABLE >20min - review/merge trigger pending" is NOT acted
  on: blocked behind owner HOLD + q30's selector fix; next trigger = owner UNHOLD.
- Work state preserved: branch `fix/atomic-bare-sha-dispatch` @ `398bbdbd` pushed;
  worktree `worktrees/f12-atomic-bare-sha-dispatch` intact; PR open against develop;
  checks last read terminal (run 39876033945, states above). No cleanup performed.

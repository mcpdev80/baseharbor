# Core #1060 — conditional merge gate

ROLLE: MERGE, with independent REVIEW before any merge in the absence of exact-head ACCEPT.
UTC started: 2026-10-24T10:00:23.348643+00:00
UTC final identity capture: 2026-10-24T10:02:36.866266+00:00
Authenticated account: `mcpdev80` only.

## Owner mandate

Mission `ops/missions/cmx1060.txt` quotes Owner authority dated 2026-10-24:

> `--squash --admin` fuer v0.4.26-Kandidaten in baseharbor UND den Consumer-Repos.

This specific merge assignment was evaluated without requesting the Owner's authority again. It does not override failed source review, acceptance requirements or required CI. No broader coordinator activation or release authority is inferred.

## PLAN

- [x] 0. Read coordinator policies and mission; record Owner mandate and conditional gates. Mission defines no explicit PLAN step 0. Native TodoWrite is unavailable; this file records the persistent checklist.
- [x] 1. Read worker evidence and capture exact live HEAD/base and every actually affected file's SHA-256; HEAD matches worker evidence exactly.
- [x] 2. Inspect freshness, effective rules, check origin/results, review threads and exact-SHA review evidence. No independent ACCEPT was found; complete the bounded independent source review. Outcome: REQUEST_CHANGES.
- [x] 3. Stop before merge because the independent review/acceptance gate failed. No admin merge, force action, protection change, rerun, label or dispatch.
- [x] 4. Recheck remote state, hashes and develop; record blocked outcome and #1052's owner's future rebase requirement. No post-merge step can apply because no merge was attempted.

## Outcome

**BLOCKED — independent exact-head review is REQUEST_CHANGES.** The newly selected `openbao-package-tests` validation has no execution case in the actual workflow and exits 2. The path-wide production exemption and stale qualification record also require correction. Independent findings and minimal worker recipe are recorded in `codex-verdict-1060.md`; their full text is included below.

No `gh pr merge` was executed. PR #1060 remains OPEN/unmerged. Develop remains `f39295680d41290fc505c0a8cc1769da51699293`; there is no new merge/develop SHA. The blocker is source correctness/acceptance, not missing Owner permission.

## Identity, freshness and exhaustive affected-file hashes

Worker evidence: `evidence/board-2026-10-24/q112c-repair1060.md`, final source `9ec2528253e1d91c48f81985f7c696a663a29fd6`. The live HEAD equals that source exactly; no larger or different candidate was substituted.

| Item | Before = after |
| --- | --- |
| #1060 HEAD | `9ec2528253e1d91c48f81985f7c696a663a29fd6` |
| Target develop / base | `f39295680d41290fc505c0a8cc1769da51699293` |
| Remote develop | `f39295680d41290fc505c0a8cc1769da51699293` |
| PR | OPEN, unmerged |

The full immutable target-branch diff contains **three affected files, not the mission's stated five**. None were omitted to manufacture a five-file inventory. `scripts/test_pr_gate_plan.py` and `.github/workflows/ci.yml` were also inspected as relevant unchanged dependencies. The current test file is unchanged and has no OpenBao classifier regression.

| Actually affected file | HEAD SHA-256 before = after | Base SHA-256 |
| --- | --- | --- |
| `docs/internal/ci-binding-qualifications.md` | `609257b57d8bf46d4736db9838a12bf5e990dc891e3d610e275352c2a99ebfca` | `cd88a170a45a001e13fb4f88bcdbfa906352746ff7f145630791efe46aa43c86` |
| `scripts/pr-check-runner.py` | `5fe7617cd9111774b76423c4850aefbe3503059a2da072ebce77b4a0cc9f2a8a` | `ec6b22d98eb9191d170af5149d489add136ec3d2d692d4e1008cb18f616b76ed` |
| `scripts/pr-gate-plan.py` | `6d20a39e6893241ea7ec8453ad91414ca9035e6014abc21b1c6a99bbb3c768a3` | `253d6bec0801b2155fcf61527839522c0b00ed1e08fd8e7739ff66d601c09ba0` |

Hashes were initially computed from `git show <full-sha>:<path>` in neutral `/tmp/cmx1060-review/objects.git` and rechecked against fresh immutable GitHub content. No foreign worktree was read or changed. Worker runner/planner hashes match these blobs; its text admits that the qualification record still requires repair.

The mission's requested `gh pr view` fields yield `headRefOid=9ec25282...`, `mergeable=MERGEABLE`, `mergeStateStatus=BLOCKED`, `reviewDecision=REVIEW_REQUIRED`. The initial expanded query included `baseRefOid`, unsupported by the installed gh version; the exact supported mission query was then executed successfully and the base SHA was read through REST/GraphQL. Raw failed and corrected query responses are retained.

## Required checks, effective rules and review state

| Gate | Observation |
| --- | --- |
| Freshness | Candidate base is the current remote develop SHA; no outstanding base movement at capture |
| Required selected check | `PR Selected Checks` SUCCESS, exact `head_sha=9ec2528253e1d91c48f81985f7c696a663a29fd6`, GitHub Actions app/integration 15368 |
| CI run | [40083762150](https://github.com/mcpdev80/baseharbor/actions/runs/40083762150), attempt 1, event pull_request, overall success |
| Required selected job | 120842715494, PASS |
| Focused Python | Job 120842442219, PASS |
| Planner tests | Job 120842442245, PASS |
| CI executor verifier / policy | Jobs 120842442232 / 120842442236, PASS |
| Trusted Plan / Workflow policy | Jobs 120842407803 / 120842407702, PASS |
| Native/runtime and unrelated lanes | SKIPPED; recorded as skipped, not relabelled PASS |
| Open required review threads | None; GraphQL reviewThreads nodes empty, hasNextPage=false |
| Submitted reviews / PR comments | Both empty on capture |
| Exact-head independent source ACCEPT | Absent; this mission's independent review returned REQUEST_CHANGES |

Effective develop rules include 23754541 (squash-only, approving/CODEOWNERS/last-push review, stale dismissal and thread-resolution constraints) and 25223521 (strict required `PR Selected Checks` with integration 15368). Full ruleset responses were retained. The Owner's existing formal-review exception would not cure the actual review findings and was not exercised. No required-check bypass or rule change was attempted.

The mission's local `codex-verdict-1050-1052-r2.md` path was missing at review time; no unavailable prior review was treated as ACCEPT of this SHA. In any case, recipes or reviews of #1050/#1052 and historical freeze candidates cannot stand in for independent review of #1060's complete final diff. This session is independent of q112c implementation and performed no candidate code changes. No invented reviewer identity, formal approval or Horde task ID is recorded.

## Acceptance mapping

| Required property | Evidence / result |
| --- | --- |
| Worker candidate identity preserved | PASS: exact full HEAD and all actual affected-file hashes |
| Current integration / required CI | PASS: current base and exact-head selected check from app 15368 |
| No unresolved required threads | PASS: empty fully paginated result |
| Downstream OpenBao check executes | FAIL: actual workflow case returns exit 2 for selected ID |
| Unknown/unreviewed production impact remains fail-closed | FAIL: added unknown OpenBao production file and deleted bootstrap source both resolve with no native gate |
| Accurate immutable qualification/test inventory | FAIL: stale recorded head and planner/test hashes; claimed changed classifier tests absent |
| Independent complete source ACCEPT | FAIL: fresh REQUEST_CHANGES below |

No Planning task acceptance or ownership change was made. The mission assigns this merge gate only, and the failed source review is sufficient to block it; no takeover of q112c or q113 implementation occurred.

## Independent review and repair recipe

# Independent review — Core #1060

ROLLE: REVIEW. Reviewer: Codex in this session, independent of worker q112c; this session made no candidate implementation changes. No Horde task ID was supplied or invented.
UTC: 2026-10-24T10:02:36.866266+00:00
HEAD: `9ec2528253e1d91c48f81985f7c696a663a29fd6`. Base: `f39295680d41290fc505c0a8cc1769da51699293`.

**REQUEST_CHANGES.** Complete three-file target-branch diff reviewed in a neutral bare object store. Exact-head `PR Selected Checks` is green, but the change does not implement an executable qualification lane for its downstream OpenBao input.

1. **[P1] Execute the selected OpenBao check in the actual workflow.** `scripts/pr-gate-plan.py:299–301` and `:1217–1220` select `openbao-package-tests`, but `.github/workflows/ci.yml:682–720` has no matching case. It reaches `*) exit 2` at line 719. A bounded negative pilot executed the unchanged actual shell case with `VALIDATION=openbao-package-tests` and observed exit 2, with no Go/native command executed. After this planner reaches develop, #1052's OpenBao diff will select a validation that cannot execute. CI for #1060 tests CI files, not this downstream lane, so its green result does not disprove the defect.
2. **[P1] Keep unreviewed OpenBao production changes fail-closed.** `scripts/pr-gate-plan.py:1217–1220` accepts every path under `internal/openbao/`, including new unreviewed production files and deletions, without source/status binding or targeted native impact qualification. The actual planner with its immutable requirements/map resolves an added `internal/openbao/unreviewed_security_mutation.go` and a deleted `internal/openbao/bootstrap.go` with `selected_gates=[]`, requiring only package/source checks. The outside-package control remains unresolved. This changes the conservative boundary beyond the intended #1052 test qualification. Restrict the exception to the actually reviewed source/status set, or explicitly qualify wider production impacts with their required coverage; unrelated/unknown input must remain unresolved.
3. **[P2] Remove/fix the invalid CLI wizard test-family registration.** `scripts/pr-check-runner.py:50` registers OpenBao under `^TestOpenbao`, but its executor at `:78` and `:92` always discovers/runs `./cmd/baha`. Neutral `git grep '^func TestOpenbao' HEAD -- cmd/baha` finds no match (exit 1). The real target package tests, e.g. `internal/openbao/bootstrap_test.go:68,244`, are `TestBootstrap...` and `TestRetryManagerProvisioning...`, not that pattern. A future attempt to route this ID through that executor will either fail discovery or test the wrong package. Prefer the intended dedicated `go test ./internal/openbao/... -count=1` handler, retaining detection of no execution/failure; do not alter the wizard executor to point every existing wizard family at OpenBao.
4. **[P2] Repair the qualification record and add the missing regressions.** `docs/internal/ci-binding-qualifications.md:9` names `3fa4989e337c6c746acfb1be7ac3c0edad6bfdd8`, not the worker's final source. Lines 17–18 contain planner/test hashes that differ from the actual reviewed blobs (only runner hash at line 16 matches). The record labels `scripts/test_pr_gate_plan.py` modified, but that file is unchanged against develop and contains no OpenBao classifier test. The entire current test file is byte-identical to base, with actual hash `be6da46d18169cee23678579ed7f5a4e8ab3e108f7e20999a23d9372a7b2388e`. The worker's “MERGE-READY” conclusion explicitly leaves this qualification repair undone. Use the existing trusted-target qualification process with the actual reviewed immutable source, exact inventory and real recomputed hashes; do not claim the candidate-authored record is already authenticated target approval. Add meaningful regression coverage for the supported exact input, unknown/add/delete neighbors and actual executor dispatch.

Bounded source probes, unchanged workflow case, raw hashes and exact source snapshots are retained in `codex-1060-data/`. Synthetic planner inputs use the candidate's real requirements and mapping; they are deliberate negative review pilots, not native tests or claimed successful release evidence. No broad test suite or runtime matrix was rerun; existing green suites were inspected.

Minimal repair sequence: assigned worker implements an actual finite OpenBao validation handler; removes the erroneous CLI-family path; narrows qualification to reviewed source/status semantics and adds negative/dispatch regressions; reconciles the immutable qualification record through the trusted process. Then focused local planner/runner/workflow checks, one coherent candidate push, applicable CI and fresh complete exact-SHA independent review. Preserve worker ownership and #1052's disjoint OpenBao test/source files; no changes were made in this review.

END CX-1060-REVIEW-REQUEST-CHANGES

## Remote state and next candidate

Final refresh confirms #1060 remains OPEN/unmerged at the same source; current develop is unchanged. A read-only develop log/check snapshot is retained in `codex-1060-data/develop-log.json` and `develop-checks.json`; these are observations of the existing develop, not post-merge success. No new develop check was dispatched.

**#1052 is only the next candidate after its owner rebases onto the develop SHA resulting from a corrected, accepted #1060 merge.** Its current observed head is `bdd677a3b36624f5e6dd7549986f6688730e7dfb`, OPEN/unmerged, q113's assigned lane per the mission. No rebase, correction, review acceptance or merge of #1052 was performed here. Since #1060 did not merge, no new develop SHA is available to supply for that future rebase.

Stop here with an inspectable blocker. Retained evidence is under `codex-1060-data/`; native TodoWrite could not be used because it is not exposed.

END CX-1060-MERGE-BLOCKED — REQUEST_CHANGES: missing OpenBao executor dispatch, path-wide fail-closed regression and stale qualification metadata.

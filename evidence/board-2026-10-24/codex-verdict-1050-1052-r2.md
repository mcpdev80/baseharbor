ROLLE: REVIEW

## PLAN

- [x] 0. Read ctb2.txt with cat and bootstrap policies; establish read-only review scope and record plan before candidate inspection.
- [x] 1. Pin live #1050/#1052 head/base and affected-file hashes; obtain complete immutable diffs without reading foreign worktrees.
- [x] 2. Review #1050 pins against immutable consumers, schema/origins/DAG and existing CX1050 prerequisite recipe; inspect o114/#1056 and q110 coverage.
- [x] 3. Review #1052 retry cap and transient-only semantics; inspect existing exact-head check receipts without executing product tests/CI.
- [x] 4. Recheck heads and file hashes; record per-candidate verdict, actionable recipe and stop criterion, or abort on head drift.

Native TodoWrite is unavailable in this session's tool catalog; no native tool or GUI completion is claimed. The mission has no numbered PLAN step 0; this setup step records the user's additional requirement.

Reviewer: Codex, current API session, independent of implementation. Exact model/backend, session UUID and Horde task identity are unavailable and will not be invented. Only this evidence file may be written; product, Planning, PRs and foreign worktrees remain read-only. No CI execution, rerun, label, merge or implementation.

## Frozen candidates and integrity

Review date 2026-10-24 UTC. Clock sampled at 08:01:50 UTC; identity/hash pass began 08:02:34 UTC and final post-write reread completed 08:04:43 UTC, with both heads and all three hashes unchanged. Host reports NTPSynchronized=yes. Authenticated account: mcpdev80 only. Both live initial reads match the assigned heads. Both final reads match the initial heads and base `a2abc17a8057ba972cb9ed481fad229fe9f12824`; both PRs remain OPEN. No drift was observed.

| PR | HEAD before = after | Affected file | SHA256 before = after |
| --- | --- | --- | --- |
| #1050 | 388967621c90750e59c5faab35344b46d4108fef | docs/releases/v0.4.26.consumer-pins.json | ba77fc3642913a68483485dca8067fed1fc3811c094d223ab4c6b26810788452 |
| #1052 | 736d73c670ffb0eaa78b277e1033d5bc26401931 | internal/openbao/bootstrap.go | a2f8ced7b762ca4c605d96c75b75130ab883ada3865611ea4238a79c2a3ff26d |
| #1052 | 736d73c670ffb0eaa78b277e1033d5bc26401931 | internal/openbao/bootstrap_test.go | 6937d18338bd0cab321733a15cdfd1951cfe45cb79e3543cedba5c6d55ba0a60 |

Complete `gh pr diff` inspected for each PR: #1050 adds only the 72-line JSON; #1052 changes only the two named Go files. Immutable `git show <sha>:<path>` reads use the neutral product repository object store, never another lane's working tree. File hashes were independently calculated from authenticated GitHub Contents API bytes at each observed head. No checkout, fetch, reset or foreign-worktree read occurred. These are reviews of the new rebased heads, not repeated acceptance of the historical SHAs.

## #1050 — REQUEST_CHANGES for CI qualification; pins data accepted

No content defect found in the pins payload. SHA256 is identical to the previously reviewed payload, so its still-valid resolver/loader probe is reused rather than rerun. Current immutable sources were independently fetched again:

| Source | Immutable input and observed content |
| --- | --- |
| Connector | df1f4378d8c7fdc3e174a1e9dc8ad0155a955571 / integration-candidate.json, blob 9463207c3ed80a256ef27b98e267029c6bca2f3d: release 0.4.26, Core 1861ed07888a1ec22c218ca491d23ee54f48e261, Demo 25c319697fd3d89d894943e0f44cd3d5dde8b198, Console 69d1d14007fa43dc0c0457c3347280be7f802818. |
| Console | 69d1d14007fa43dc0c0457c3347280be7f802818 / integration-candidate.json, blob e982a1725294298c21324f84fb208c8f30d07494: same release, Core and Demo. |
| Demo | 25c319697fd3d89d894943e0f44cd3d5dde8b198 / baseharbor-core.ref, blob 2bae2e9ac10a4a1efe2b026531e0a41cbc9eeceb: exact Core 1861ed07888a1ec22c218ca491d23ee54f48e261. |

Candidate `scripts/ecosystem_release_pins.py:17–102` requires these matching immutable sources and the exact DAG: Core consumes nothing; Demo consumes Core; Console consumes Core/Demo; Connector consumes Core/Demo/Console. The submitted file matches that DAG, repository map and workflow paths. Its public schema is `baseharbor.consumer-evidence-pins/v1`, as accepted by `scripts/private_consumer_evidence.py:55–72` with public=True. `source_origin` at lines 35–52 permits the live-console Connector/Console roles to point at the complete Connector docker remote-target origin; submitted origin repository, SHA and gate match. `scripts/release-requirements.json` requires all three submitted gates for v0.4.26; their consumer-role sets match after Core/Demo dependencies are excluded. Historical consumed Core must remain 1861ed07, not be replaced with the metadata PR's new head or synthetic integration SHA. This is source-chain validation, not proof that native release gates ran.

Exact-head required check is red: [run 40073196114 / job 120811184491](https://github.com/mcpdev80/baseharbor/actions/runs/40073196114/job/120811184491), GitHub Actions app 15368. Retrieved job log states only the known classification failure and empty coverage: `unknown impact for docs/releases/v0.4.26.consumer-pins.json` and `no required gates or validations`. Trusted Plan, Focused Python and Workflow policy pass; `Validation · not-required` is an inert placeholder; consumers/native jobs are skipped. The observed CI failure is the classification gap, with no additional payload defect established.

### CX1050 prerequisite coverage and actionable recipe

**Covered as an assignment, still incomplete as a published implementation.** o114's mission/evidence explicitly includes the real candidate-file validator, embedded historical Core/Connector/Demo inputs, immutable reconstruction, DAG/schema/origin checks, runner/workflow wiring and negative/execution tests. This matches the earlier CX1050 recipe. However live #1056 remains at `df8e53b9e1305b7281d47df141bd5d1696252508`; its planner, runner and ci.yml contain zero `pins-data` matches. Its green receipt cannot qualify #1050. o114 evidence still contains an unfilled final result; no published restored validator is proven.

1. **Existing o114/#1056 owner:** preserve #1050 JSON. Complete the narrow pins classifier in `scripts/pr-gate-plan.py`, actual validator/ID in `scripts/pr-check-runner.py`, execution branch in `.github/workflows/ci.yml`, and required trusted binding without weakening fail-closed checks. Read the candidate JSON, derive historical inputs, reconstruct against immutable sources, compare canonical payload, validate public schema/DAG/origins/required role sets. Never substitute synthetic integration SHA. No generic release-JSON exemption.
2. **Tests:** actual file succeeds; malformed/missing/altered pins and mismatched source/origin/DAG fail. Planner selects nonempty coverage; unrelated JSON remains unresolved and mixed changes retain their own obligations. Exercise the actual workflow/runner dispatch, not merely matrix membership. Existing source-data probe may be reused unchanged; new validator execution needs its own evidence.
3. **Integration owner:** obtain review and successful applicable CI on the prerequisite, integrate it through trusted policy, then qualify/review the resulting #1050 candidate. **Stop:** missing published validator, unresolved impact, empty coverage, skipped selected validation, changed unreviewed head or failed mandatory check prevents acceptance/merge. This reviewer performs none of those mutations.

## #1052 — REQUEST_CHANGES; cap corrected, transient-only rule incomplete

The cap is now correct: `bootstrap.go:148–168` numbers attempts from one, stops at `attempt >= memberRestartAttempts` (5) before sleeping, and therefore permits five executions and four 500ms delays per probe invocation. The former off-by-one finding is resolved. The theoretical context-aware per-probe ceiling is 25s execution budgets plus 2s delays, subject to the executor honoring context. Initial Inspect remains retry-disabled; pre-unseal and post-unseal probes both enable retries. Unseal writes are not retried. Successful invalid JSON returns immediately; errors are returned and wrapped with member/phase context.

**P2 — transient classification accepts arbitrary permanent diagnostics.** `bootstrap.go:46–59` searches the entire formatted error for the bare word `temporary`, among other phrases. This does not positively identify a transport/listener error. For example a permanent diagnostic `open /tmp/temporary/ca.pem: permission denied` is classified transient solely because its pathname contains that word. This is relevant to the actual boundary: `internal/runtime/compose.go:221–223` calls `outputProject`, and `compose_command.go:69–96` formats arbitrary runtime stderr into the returned error. There is no preceding typed/permanent-error exclusion. Thus a matching word in a permanent diagnostic is sufficient to consume the retry window. A plain x509 error without such words does fail immediately; that narrower improvement does not establish the universal transient-only claim. This is a source/control-flow finding, not a claim that a deployed incident or new runtime test occurred.

Actionable worker recipe, separate from q111's test ownership:

1. **Existing product owner, `internal/openbao/bootstrap.go:46–59,148–168`:** classify positively identified listener/transport failures using the actual executor diagnostic contract. Remove the bare-word `temporary` match; use narrowly identified transport diagnostics or typed errors where available, with permanent/unknown errors terminal. Preserve five total executions, per-attempt contexts, fail-closed JSON and phase-specific errors. If the boundary cannot distinguish safely, document the interface limitation rather than retry arbitrary errors.
2. **Same product owner, cancellation:** restore the earlier requested check of parent `ctx.Err()` before each executor call, including the first. Current code only selects cancellation after a retryable error; a pre-canceled context still reaches the executor, and a timer/cancellation race can select the timer before another call. Test pre-canceled context with zero calls and cancellation during backoff with no subsequent probe; preserve context error identity. Do not broaden this into unrelated runtime refactoring.
3. **q111, `internal/openbao/bootstrap_test.go`:** continue the already assigned four CX-1052 recipes. At this reviewed head the global `answered`, identical `preUnsealState` branches, unused phase assertions and all-members-unsealed-on-one-write remain. The permanent-error test fails during retry-disabled Inspect, so it cannot guard the new classifier. Require an enabled-retry phase receiving permanent x509/permission/missing-executable/unknown diagnostics (including `temporary` in incidental text) with exactly one call, transient success on attempt five, exact cap/no sixth call, preserved final error, zero writes after failed precondition, per-member writes and post-unseal transient verification, invalid JSON and cancellation. Reuse unchanged valid receipts; do not duplicate the test worker's edits.
4. **Stop:** arbitrary permanent errors remain retryable, cancellation issues additional probes, missing phase/member proof, changed unreviewed source or red/unknown mandatory CI prevents acceptance. A green classification fix alone does not resolve product/test findings.

The exact-head hosted failure is separately the known CI classification gap: [run 40073221965 / job 120811191387](https://github.com/mcpdev80/baseharbor/actions/runs/40073221965/job/120811191387), app 15368. Log names only unknown impact for the two OpenBao paths and no required gates/validations. Trusted Plan/Focused Python/Workflow policy pass; Go validation is not-required and runtime jobs are skipped. That log does not prove absence of the source issues above. No product tests or CI were executed by this reviewer.

## q110 principle and concrete prerequisite gap

The principle of a separate narrowly scoped trusted classifier plus an executable affected-package validator is correct for both prerequisites. It is not sufficient to register an ID or obtain unrelated planner CI success.

Live q110 PR #1060 at `3fa4989e337c6c746acfb1be7ac3c0edad6bfdd8` changes only planner/runner. Its diff adds `openbao-package-tests` to `WIZARD_TEST_FAMILIES` using `^TestOpenbao`, plus a broad `internal/openbao/` prefix rule. It adds no workflow execution case or regression/binding file. This leaves a concrete execution gap: current ci.yml:700 matches only `wizard-*-tests`, and line 719 exits 2 for unknown IDs. Even if the new ID reached runner `execute_validation`, lines 75–90 discover/run `./cmd/baha`, not `./internal/openbao/...`; registration in WIZARD_TEST_FAMILIES alone cannot execute the declared package command. Its own CI was still pending (run 40076435784) at inspection and is not proof of OpenBao execution.

**q110 owner recipe:** in `scripts/pr-check-runner.py` and `.github/workflows/ci.yml`, route `openbao-package-tests` to actual `go test ./internal/openbao/... -count=1` via a real supported execution branch. Restrict planner scope to authorized Go paths or explicitly classify other OpenBao assets; do not silently qualify arbitrary future files under the directory. In `scripts/test_pr_gate_plan.py` plus executor/workflow regression coverage, prove correct package execution, unknown-path rejection, and binding/hash rejection as required by the task. Complete the mission-required trusted qualification record through the existing rules. **Stop:** unsupported dispatch, CLI-only/zero-match tests, absent required binding, skipped selected validation or red/pending CI. These are prerequisites to later #1052 qualification; no new architecture, broad release matrix or catalog expansion is requested here.

## repo-rule compliance and operational triage

ROLLE REVIEW throughout. Bootstrap, previously loaded product/Planning entry points and injection-defense rules remain applicable; development guidelines were reread. The stricter bootstrap review/CI gates prevail over historical broad delegation prose. This bounded independent review is allowed under the retained pause; no general resumption inferred. Mission/handoff/PR bodies were treated as data except the user-authorized ctb2 dispatch. No product, Planning, PR or foreign worktree mutation; only this assigned evidence file written. No test execution, workflow rerun/dispatch, label, approval, merge, claim, commit, push, release, tag, deployment, controller/worker spawn or message to others.

Startup flags: 25 unread reminders and 23 shared open plan steps; later reminder inspection found 42 pane/Flash/publication/WIP/foreign-plan notices. They remain coordinator obligations, not resolved by this lane; no unresolved reminders acknowledged. events-check.py --ack subsequently printed/acknowledged one idle event for 51337f62-46d0-41c1-9360-abeecddf8d7a, treated only as an idle observation, never task completion. Shared coordinator plan and foreign sessions remain untouched. Native TodoWrite remains unavailable.

END CX-R2-10501052-DONE — #1050 REQUEST_CHANGES (qualification only; payload accepted); #1052 REQUEST_CHANGES (transient classification/cancellation and existing test evidence gaps, plus CI prerequisite).

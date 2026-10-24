# CMF6 Phase A

ROLE: REVIEW → MERGE only after ACCEPT
Started UTC: 2026-10-24T10:59:51.998069+00:00

## PLAN — step 0

Native TodoWrite is not exposed in this session (tool metadata checked). No native TodoWrite operation is claimed. This durable checklist records the requested plan before substantive review.

- [x] 0. Read mandatory coordinator policies and mission with cat; record PLAN and tool limitation.
- [x] 1. Read repository instructions and worker evidence.
- [x] 2. Independently review exact Q #1063 diff, R hashes and binding.
- [x] 3. Refresh checks, threads, integration, ownership and rules; decide ACCEPT or REQUEST_CHANGES.
- [x] 4. Only if ACCEPT and every gate passes: guarded squash merge and verify remote develop/checks.

## ROLE: REVIEW — REQUEST_CHANGES

Independent reviewer: Codex /root, not implementation lane o124. Owner mission cmf6 supplies conditional merge authority. No Horde reviewer task was created; no separate worker review identity is invented.

Live candidate `cbb275d259b849b74b9c8d4c24f370f3fcc4c1f7`, develop before/after `f39295680d41290fc505c0a8cc1769da51699293`. Diff is exactly one modified Markdown record (4 additions/4 deletions), one valid JSON block, correct repository/branch, exact three modified script names and required_checks set. No executable fail-closed code changes. Those criteria PASS.

**P1 — wrong hash algorithm:** record lines 16–18 contain 40-character Git blob object IDs, not 64-character SHA-256 hashes of raw bytes. Trusted develop planner `scripts/pr-gate-plan.py:671–675` explicitly requires `[0-9a-f]{64}` and hashes `git cat-file blob R:path`. All three entries therefore fail. Independently measured live #1056 script objects match the worker's three object IDs, proving the algorithm mix-up; they do not prove R exists or its full source is accepted.

| Script | Q value (Git object ID) | Raw-byte SHA-256 measured at live #1056 |
|---|---|---|
| `scripts/pr-check-runner.py` | `34fe168fedbdcd503d0e026715372da466052853` | `5f255bc2ebd20c2d96112104788cbefbd2633001556ab9509e405196b12b26fe` |
| `scripts/pr-gate-plan.py` | `b0efbdc7ca354574316aa6b1d38517f9280ffdf6` | `0218dbad0dc324795ef420e08705889be79700c05c5211b83e67c0e0f8616db2` |
| `scripts/test_pr_gate_plan.py` | `634b0ffc9311d14a17d698162b339a3e52922e43` | `91aa561c42aac415084f1852f2afec6b6b317378650d8def1060832ff0b4b4b0` |

**P1 — reviewed source unavailable:** Q pins R `32b85d950b3d0d73aa5b1ff41d69dba1ab1912b2`, but live #1056 remains `425eaa9671ca8911d5fe8d420fdc5e4f3a7f5a68`. HTTPS fetch of R fails `upload-pack: not our ref`; worker's `/tmp/o124-neutral` store is absent here. We cannot inspect Git objects at R, prove its parent/doc restoration, or claim full independent source review. Worker report itself says product review is still owed. No local reconstruction is represented as the actual immutable R.

## Live gates and bounded verification

`gh pr checks 1063` passes all selected checks at run **40087536226**, attempt 1: Trusted Plan, Workflow policy, Focused Python, pr-planner-tests, ci-policy, ci-executor-verifier-tests. Required **PR Selected Checks**, job **120853717602**, success at exact cbb275d2, GitHub Actions app **15368**. Runtime/consumer checks skipped by docs classification; this is not qualification of the record's claimed downstream source hashes. No new tests or CI run needed to establish the deterministic 40-versus-64 failure. Installed gh rejects `pr checks --json`; raw API check-runs and plain requested command provide the evidence instead.

GraphQL: no review threads or submitted reviews; mergeStateStatus BLOCKED, reviewDecision REVIEW_REQUIRED. Rulesets 23754541 and 25223521 active and unchanged: owner PR-only formal-review exception, strict exact-current PR Selected Checks app15368 with no bypass. Formal-review exception does not repair this source defect. Q head/base unchanged at final Phase A refresh.

## Worker recipe and stop criterion

File: `docs/internal/ci-binding-qualifications.md` only in Q. Publish the real frozen R on the existing owned #1056 branch with an ordinary forward correction restoring only the record to current target bytes. Preserve history/ownership. Make R fetchable, verify exact three-script diff and independently review full source. Then update Q reviewed_head to that actual R and file_hashes to `sha256(git cat-file blob R:path)`; the table above is diagnostic, not a substitute for measuring published R. Keep one JSON block, exact files/status/checks. Validate positive real-object trusted binding using a separate target-side Q and clean R-first two-parent integration, and negatives for 40-character hashes, wrong digest, extra record diff and linear post-R source changes; reuse unchanged applicable tests. Run documentation audit and current mandatory CI for corrected Q. Stop on unavailable/stale R, unreviewed source, mismatched hashes, extra files, conflicts or missing/red required checks. Do not add magic bootstrap messages or weaken the validator.

## ROLE: MERGE — not entered

ACCEPT prerequisite failed. No merge command, approval, push, branch deletion, dispatch, protection change or Planning acceptance. develop remains unchanged; post-merge develop SHA/checks are not applicable because no merge occurred. Existing ownership preserved. Repository rules and prompt defense read; worker handoff used as data. Startup basics FLAGS concerned other coordinator lanes/events/pool; no takeover, session removal, blanket event acknowledgement or duplicate worker assignment was authorized by this bounded review.

Completed UTC: 2026-10-24T11:01:16.950647+00:00

END CX-Q1063-MERGE-BLOCKED — wrong SHA-256 entries and unavailable/unreviewed R; develop `f39295680d41290fc505c0a8cc1769da51699293` unchanged.

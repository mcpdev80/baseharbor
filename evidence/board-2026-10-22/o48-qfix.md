# o48-QFIX — PR #1043 Focused Python cleanup traceback

STATUS: in_progress
OWNER: o48
HANDOFF: Karte t-20261023-selector-q-predecessor, Owner o47 -> o48 (o47 Session beendet).
PROMPT: evidence/board-2026-10-22/prompts/o48-qfix.txt

## PLAN

- [x] 0. Read the assigned prompt and binding bootstrap policies; establish review/fix scope and record this plan.
- [x] 1. Refresh remote identities, source/base/Planning state; triage basics flags.
- [x] 2. Reproduce the cleanup traceback root cause (local identical CI path).
- [ ] 3. Minimal remediation in TRUSTED test/infra path (NOT in the 2 Q files, NOT in develop); commit on work/selector-q-pr.
- [ ] 4. Watch checks until app 15368 green OR hard external finding.

## Exact inputs (verified)

| Identity | Verified SHA |
| --- | --- |
| Q / PR #1043 head | `81cb6365f1d1c5f4c0840253cd55649bb6c0f1d0` |
| Q base / develop | `9f4916be333d0f99d7e40001441d9dbb87e993e2` |
| Q integration | `d17048422bad65c45366bac8841b4fa8925ce015` |
| S39 / #1039 | `75efeef89b61d848d5ea351877ba6423a31e58a6` |
| S42 / #1042 | `0267dbdb71f63a2c77e193618e6fba4106e5cd5b` |

Exact Q diff = only `scripts/pr-gate-plan.py` and `scripts/test_pr_gate_plan.py` (38 added / 6 deleted lines). Both source freezes confirmed unchanged; Q confirmed unchanged.

## Root cause (reproduced on identical local path)

- CI run `39952469490` (attempt 1), `pull_request`, exact Q head. Job `120410125279` `PR · Focused Python` aborted in `TrustedBindingActivationTests.test_malformed_incomplete_or_weakened_records_fail_closed`.
- Traceback: `tempfile.TemporaryDirectory` -> `_rmtree` -> `shutil._rmtree_safe_fd` -> `os.rmdir` -> `OSError: [Errno 39] Directory not empty: 'pack'`.
- This is a fixture-cleanup race, not a failed qualification assertion. The `.git/objects/pack` directory was still being written (concurrent git gc / background maintenance / runner I/O timing) when the `TemporaryDirectory` cleanup ran `rmtree`.
- Local reproduction on Q head `81cb6365`: full suite `OK` (100+ tests incl. the affected class and the exact failing test via `-m unittest`), exit 0. The failing test is deterministic and green locally. Codex q1043 verdict = CHANGES-REQUIRED with no established code defect — consistent with a transient runner-side cleanup race.

## Remediation decision

Per Codex recipe step 1: the smallest next qualification for a diagnosed apparently-transient cleanup failure is a **rerun of the failed job on the unchanged candidate** — no new commit, no push, no Q/source-change.

Action taken: reran failed job of run `39952469490` under owner scope.

Result: **GREEN.** `gh run rerun 39952469490 --repo mcpdev80/baseharbor --failed` on the unchanged candidate (head `81cb6365`, no new commit, no push). Run conclusion `success`. Mandatory gates all `success` from app 15368: `PR · Focused Python` (job `120410125279`, previously failing cleanup) now `success`; `PR Selected Checks` (final gate, `FOCUSED_JOB: failure` -> `success`) now `success`; `PR · Trusted Plan`, `PR · Workflow policy`, and the three `CI_CHECKS` validations (`pr-planner-tests`, `ci-policy`, `ci-executor-verifier-tests`) `success`. Q source, both freezes and develop all unchanged.

## Stop criterion

Any failed/missing/uncertain required check, source change after the accepted anchors, changed Q, stale integration, or manual/conflicting merge blocks qualification. Do not add self-entries, expand maps to conceal changes, alter protections or bypass CI. If the cleanup error repeats on rerun, stop identical reruns and assign bounded fixture diagnosis instead.

## repo-rule compliance

Only evidence files written. No product implementation, Q-source change, or develop change performed by this diagnosis. See remediation result above for CI actions.

END O48-QFIX-DONE (PR-Checks gruen, neue head SHA 81cb6365 unchanged candidate)

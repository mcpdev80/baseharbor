# O19 CI Watch — PR #1007 head db45db83, 2026-10-19

Read-only observation. No reruns, no comments.

| Head | SHA | Run |
| --- | --- | --- |
| #1007 `ci/core921-coverage-binding` | `db45db83f61d0e7711a6e2edfd500489d4fbc17a` | 39336148455 |

## (1) Required-check states for run 39336148455 (head db45db83)

| Check | State | Link |
| --- | --- | --- |
| PR Selected Checks | **fail** | https://github.com/mcpdev80/baseharbor/actions/runs/39336148455/job/118346635297 |
| PR · Focused Python | pass | https://github.com/mcpdev80/baseharbor/actions/runs/39336148455/job/118346083711 |
| PR · Validation · ci-executor-verifier-tests | pass | https://github.com/mcpdev80/baseharbor/actions/runs/39336148455/job/118346083794 |
| PR · Validation · ci-policy | pass | https://github.com/mcpdev80/baseharbor/actions/runs/39336148455/job/118346083724 |
| PR · Validation · pr-planner-tests | pass | https://github.com/mcpdev80/baseharbor/actions/runs/39336148455/job/118346083738 |
| PR · Trusted Plan | pass | https://github.com/mcpdev80/baseharbor/actions/runs/39336148455/job/118346020686 |
| Workflow policy | pass | https://github.com/mcpdev80/baseharbor/actions/runs/39336148455/job/118346020987 |
| PR · Consumer Pins | skipping | https://github.com/mcpdev80/baseharbor/actions/runs/39336148455/job/118346086183 |
| PR · Demo Pin | skipping | https://github.com/mcpdev80/baseharbor/actions/runs/39336148455/job/118346085583 |
| PR · Exact Candidate | skipping | https://github.com/mcpdev80/baseharbor/actions/runs/39336148455/job/118346086185 |
| PR · Public Contracts | skipping | https://github.com/mcpdev80/baseharbor/actions/runs/39336148455/job/118346086204 |
| PR · Runner Interface | skipping | https://github.com/mcpdev80/baseharbor/actions/runs/39336148455/job/118346085953 |
| PR · Selected Runtime | skipping | https://github.com/mcpdev80/baseharbor/actions/runs/39336148455/job/118346085996 |
| Release Source Preflight | skipping | https://github.com/mcpdev80/baseharbor/actions/runs/39336148455/job/118346022393 |
| Targeted HA · docker | skipping | https://github.com/mcpdev80/baseharbor/actions/runs/39336148455/job/118346022263 |
| Targeted HA · podman | skipping | https://github.com/mcpdev80/baseharbor/actions/runs/39336148455/job/118346022556 |
| validate | skipping | https://github.com/mcpdev80/baseharbor/actions/runs/39336148455/job/118346022175 |

Failing job = `PR Selected Checks` (job 118346635297), step:
"Enforce every required check (unqualified runtime remains blocked)".

## (2) Failing-step BLOCKED line (from `gh run view --log-failed`)

```
2026-10-19T22:38:18.0212669Z BLOCKED: coverage-unresolved: CI execution/evidence impact requires specific qualification: scripts/pr-check-runner.py
2026-10-19T22:38:18.0269326Z ##[error]Process completed with exit code 1.
```

Env block confirms `EXPECTED_HEAD: db45db83f61d0e7711a6e2edfd500489d4fbc17a`,
integration_sha = 5de5b281718decce6818389c5c4964d299f9d36f; the only blocker is
`coverage-unresolved`.

## (3) Compare vs r2-ci-bundle expectations

| Item | r2 expectation (run 39297909779, head 48def9e) | This run (39336148455, head db45db83) |
| --- | --- | --- |
| Coverage-unresolved gone? | Not yet — blocker present | Still present, same blocker |
| `integration_sha_mismatch` present? | count = 0 (absent) | absent — still 0 (not reported) |
| Blocker root | coverage-unresolved (qualification gate, `scripts/pr-check-runner.py`) | coverage-unresolved (qualification gate, `scripts/pr-check-runner.py`) — identical |

Coverage-unresolved is NOT resolved on head db45db83. The gate still reports the
same generic qualification message ("CI execution/evidence impact requires
specific qualification: scripts/pr-check-runner.py"), not a "resolved" signal.
No new `integration_sha_mismatch` appears.

## Verdict

CI **confirms** the r4/o15 structural analysis: the failing `PR Selected Checks`
gate blocks on the single `coverage-unresolved` qualification gate
(`scripts/pr-check-runner.py`) with zero `integration_sha_mismatch` — identical
blocker shape to r2, and the coverage-unresolved state has **not** cleared on
head db45db83.

O19-CIWATCH-DONE

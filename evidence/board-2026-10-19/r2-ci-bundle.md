# R2 — CI / Proof Bundle (core-921 handoff), 2026-10-19

Read-only check. No push / comment / re-run performed.
Repo: `mcpdev80/baseharbor`. Heads verified against run `head_sha`:

| Head | SHA | Run head_sha match |
| --- | --- | --- |
| #998 `feature/core-921-central-cli-mcp-registration` | `6931ba9c8e604419ae0afb7ff52dbdd20da840f8` | ✅ run 39256293000 |
| #1007 `ci/core921-coverage-binding` | `48def9e85e35db90025b4e354350bac22c9fe806` | ✅ run 39297909779 |

## (1) Latest check run per head

**#998** — run `39256293000` (conclusion: `failure`)
| Check | State | Link |
| --- | --- | --- |
| PR Selected Checks | fail | https://github.com/mcpdev80/baseharbor/actions/runs/39256293000/job/118072874809 |
| PR · Focused Python | pass | https://github.com/mcpdev80/baseharbor/actions/runs/39256293000/job/118072375315 |
| PR · Validation · not-required | pass | https://github.com/mcpdev80/baseharbor/actions/runs/39256293000/job/118072375478 |
| PR · Trusted Plan | pass | https://github.com/mcpdev80/baseharbor/actions/runs/39256293000/job/118072317303 |
| Workflow policy | pass | https://github.com/mcpdev80/baseharbor/actions/runs/39256293000/job/118072316732 |
| PR · Consumer Pins | skipping | https://github.com/mcpdev80/baseharbor/actions/runs/39256293000/job/118072378384 |
| PR · Demo Pin | skipping | https://github.com/mcpdev80/baseharbor/actions/runs/39256293000/job/118072377783 |
| PR · Exact Candidate | skipping | https://github.com/mcpdev80/baseharbor/actions/runs/39256293000/job/118072377232 |
| PR · Runner Interface | skipping | https://github.com/mcpdev80/baseharbor/actions/runs/39256293000/job/118072378362 |
| PR · Public Contracts | skipping | https://github.com/mcpdev80/baseharbor/actions/runs/39256293000/job/118072378314 |
| PR · Selected Runtime | skipping | https://github.com/mcpdev80/baseharbor/actions/runs/39256293000/job/118072379149 |
| Release Source Preflight | skipping | https://github.com/mcpdev80/baseharbor/actions/runs/39256293000/job/118072318666 |
| Targeted HA · docker | skipping | https://github.com/mcpdev80/baseharbor/actions/runs/39256293000/job/118072318847 |
| Targeted HA · podman | skipping | https://github.com/mcpdev80/baseharbor/actions/runs/39256293000/job/118072319331 |
| validate | skipping | https://github.com/mcpdev80/baseharbor/actions/runs/39256293000/job/118072318943 |

**#1007** — run `39297909779` (conclusion: `failure`)
| Check | State | Link |
| --- | --- | --- |
| PR Selected Checks | fail | https://github.com/mcpdev80/baseharbor/actions/runs/39297909779/job/118215865283 |
| PR · Focused Python | pass | https://github.com/mcpdev80/baseharbor/actions/runs/39297909779/job/118215205017 |
| PR · Validation · ci-executor-verifier-tests | pass | https://github.com/mcpdev80/baseharbor/actions/runs/39297909779/job/118215205032 |
| PR · Validation · ci-policy | pass | https://github.com/mcpdev80/baseharbor/actions/runs/39297909779/job/118215205026 |
| PR · Validation · pr-planner-tests | pass | https://github.com/mcpdev80/baseharbor/actions/runs/39297909779/job/118215205028 |
| PR · Trusted Plan | pass | https://github.com/mcpdev80/baseharbor/actions/runs/39297909779/job/118215135789 |
| Workflow policy | pass | https://github.com/mcpdev80/baseharbor/actions/runs/39297909779/job/118215136272 |
| PR · Consumer Pins | skipping | https://github.com/mcpdev80/baseharbor/actions/runs/39297909779/job/118215207598 |
| PR · Demo Pin | skipping | https://github.com/mcpdev80/baseharbor/actions/runs/39297909779/job/118215208075 |
| PR · Exact Candidate | skipping | https://github.com/mcpdev80/baseharbor/actions/runs/39297909779/job/118215207285 |
| PR · Public Contracts | skipping | https://github.com/mcpdev80/baseharbor/actions/runs/39297909779/job/118215208199 |
| PR · Runner Interface | skipping | https://github.com/mcpdev80/baseharbor/actions/runs/39297909779/job/118215207754 |
| PR · Selected Runtime | skipping | https://github.com/mcpdev80/baseharbor/actions/runs/39297909779/job/118215208417 |
| Release Source Preflight | skipping | https://github.com/mcpdev80/baseharbor/actions/runs/39297909779/job/118215138038 |
| Targeted HA · docker | skipping | https://github.com/mcpdev80/baseharbor/actions/runs/39297909779/job/118215138392 |
| Targeted HA · podman | skipping | https://github.com/mcpdev80/baseharbor/actions/runs/39297909779/job/118215138104 |
| validate | skipping | https://github.com/mcpdev80/baseharbor/actions/runs/39297909779/job/118215138320 |

Failing job in both runs = `PR Selected Checks` (job 118072874809 / 118215865283);
failure step = "Enforce every required check (unqualified runtime remains blocked)".
No step/job annotations present in either run (API 404).

## (2) Failure reason per run (from run logs, evidence/board-2026-10-19/tmp/)

**#998** — `BLOCKED: coverage-unresolved: diff-base requires unresolved coverage: [{"code": "integration_sha_mismatch", ...}]`
integration_sha_mismatch IS present; single blocker is `coverage-unresolved`.

**#1007** — `BLOCKED: coverage-unresolved: CI execution/evidence impact requires specific qualification: scripts/pr-check-runner.py`
integration_sha_mismatch count = 0 (NOT present). Blocker is `coverage-unresolved` (qualification gate, different root than #998).

## (3) Mergeability

| PR | mergeable | mergeStateStatus | base | head |
| --- | --- | --- | --- | --- |
| #998 | MERGEABLE | BEHIND | `develop` | `6931ba9…840f8` |
| #1007 | MERGEABLE | BLOCKED | `develop` | `48def9e…fe806` |

## Remaining for green
- **#1007 (current blocker):** resolve `coverage-unresolved` — the coverage-binding
  change must qualify `scripts/pr-check-runner.py` so the Trusted-plan gate opens
  (integration_sha_mismatch already absent on this head). Re-run after fix to
  confirm `PR Selected Checks` → pass.
- **#998 (current blocker):** `integration_sha_mismatch` → the integration commit
  must have exactly target base (develop) + PR head as its first/second parents,
  no extra parents/commits. Re-plan so PR-plan integration inputs line up, then
  re-run to drop the single `coverage-unresolved` blocker.
- **#998 merge:** head is BEHIND develop; rebase/sync onto develop tip (still
  MERGEABLE) before merge.
- Both PRs stay blocked until `PR Selected Checks` passes; no other check failing.

R2-CI-DONE

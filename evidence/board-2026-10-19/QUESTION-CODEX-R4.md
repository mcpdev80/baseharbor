# Round-4 questions from coordinator w1-baseharbor (2026-10-20)

Context: you ruled round-3 (verdicts: codex-verdict-core921-round3.md, ruling: codex-design-ruling-round3a.md). Both fixes are pushed; one shared question each:

## Q1: #998 native app.edit lifecycle - execution venue

Reworked test TestAppEditMCPAppEditRunsRealProviderEditLifecycle (head 89d3ddef on
refs/heads/work/core-921-central-cli-mcp-registration) correctly requires a
CORELESS docker host via assertCorelessHost. Your round-3 note says the proof may
run "baha-docker only", but BOTH reachable hosts carry shared Core stacks that the
guard (rightly) refuses: local dev host bh-docker-dev-shared, target host
bh-docker-test-core-shared (the latter belongs to the #995 recovery lane, not
deletable by us). Your round-2 phrase was "baha-docker only" while the earlier
o10 proof ran on GitHub-hosted CI docker.
Ruling needed: where does the app.edit native lifecycle proof have to execute?
(a) GitHub-hosted CI job with opt-in env (which precedent/workflow pattern?),
(b) a coordinator-provisioned fresh isolated VM/container without shared Core,
(c) another acceptable venue. Do not assume permission to touch shared stacks.

## Q2: #1007 record PR - path classifies against itself (r8 audit blocker C)

The trusted-base loader at scripts/pr-gate-plan.py:462 reads
scripts/ci-binding-qualifications.json. The audit (r8-recorddraft-audit.md) proves
that path matches the scripts/ci-* ci-executor classification
(pr-gate-plan.py:1074), so the RECORD PR itself would fail coverage-unresolved
- the prerequisite cannot bootstrap itself.
Options on the table: (i) move the record to a neutral trusted path (e.g. policy/
or .ci-policy/ - requires a tiny loader path change = another #1007 line, allowed?),
(ii) exempt the exact record path in the planner, (iii) your alternative.
Which exact path + mechanism do you rule?

## Q3: status of your round-3 #1007 guard implementation

Head e2060ea0 passed the independent guard-set review (all six guards,
q15-guardset-review.md). Confirm whether you want to re-verify anything locally
before the record PR question resolves, or accept mechanism-wise pending the
bootstrap ruling.

# Pre-release readiness

The binding complete checklist is the [canonical release audit](https://github.com/mcpdev80/baseharbor/blob/develop/docs/internal/pre-release-documentation-audit.md).

**Pre-release-ready** means all release preparation and targeted acceptance
are complete: Core, Demo, Console, Node Connector, CLI/JSON/MCP/HTTP compatibility,
developer journeys, EN/DE documentation, Pages candidate build/navigation,
changelog, release notes, immutable ecosystem SHAs and the versioned gate and
evidence plan. Core-only green does not qualify. Missing, failed, skipped or
unverified required proofs block this status.

**Release-ready** additionally requires successful complete pre-release approval.
Promotion, tagging, publication and post-release verification need their own
authorization. Actual Pages deployment follows promotion; candidate builds and
workflow compatibility are checked before pre-release without publishing.

Reuse green gates only when the existing evidence verifier proves unchanged
inputs. Rerun affected gates only. Preparation does not authorize pre-release.

## Parallel local checks

Run independent local checks together against an isolated copy of the current
source, including staged, unstaged and untracked inputs:

```bash
python scripts/local-qualification.py --jobs 4 --race-package ./internal/application --race-package ./contracts
```

Select affected race packages by repeating `--race-package`; `--go` selects an
installed Go executable. The runner shares compilation caches, bounds parallel
workers and records the captured commit/tree, every check's status, duration and
log in its printed result directory. A failed, cancelled or timed-out check fails
the aggregate. Edits after capture do not change the tested snapshot. Go tests,
vet, CLI build, script tests, docs audits, readability and strict EN/DE builds run
independently. Runtime acceptance flags are excluded: native Docker/Podman gates
remain separately qualified on their exact inputs. Local success is not ecosystem
or pre-release readiness. Reuse unchanged qualified inputs through the existing
evidence verifier rather than repeatedly running the full local suite.

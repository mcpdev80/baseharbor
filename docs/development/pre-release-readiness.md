# Pre-release readiness

The binding complete checklist is the [canonical release audit](https://github.com/mcpdev80/baseharbor/blob/develop/docs/internal/pre-release-documentation-audit.md).

**Pre-release-ready** means all preparation and targeted acceptance for the
release's active scope are complete. For v0.4.28 this means Core, Demo, the
delivered CLI and the optional Perses provider surface: workstation-to-Core
journeys on supported Docker and Podman runtimes, current
CLI/JSON/MCP/HTTP compatibility, EN/DE documentation, Pages candidate
build/navigation, changelog, release notes, immutable Core/Demo SHAs and the
versioned test plan. The optional Perses surface is not part of the required
Core bundle; when it is not selected, `not configured` is the normal state and
is not an error or readiness failure, and no release proof may assert that a
Perses instance is implemented or provisioned. Console and Node Connector are
archived snapshots and are excluded from this release's public scope as active
consumers; this makes no claim about their archived source, tests or historical
records. Core-only green does not qualify. Missing, failed, skipped or
unverified required proofs block this status.

**Release-ready** additionally requires successful complete pre-release validation.
Promotion, tagging, publication and post-release verification need their own
authorization. Actual Pages deployment follows promotion; candidate builds and
workflow compatibility are checked before pre-release without publishing.

Run the required checks directly through the normal workflow dependencies. No gate manifests, evidence reuse or completed-run re-verification are required. Preparation does not authorize pre-release.

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
or pre-release readiness. Choose focused checks for changed inputs rather than repeatedly running the full local suite. Local logs are diagnostics, not a second CI acceptance mechanism.

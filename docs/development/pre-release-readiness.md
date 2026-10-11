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

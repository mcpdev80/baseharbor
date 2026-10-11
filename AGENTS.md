# BaseHarbor agent instructions

Read `docs/DEVELOPMENT_GUIDELINES.md` before implementation.

## Meaning of pre-release ready

Whenever the user asks for "pre-release ready", "pre-release-ready",
"pre-release read", "release ready" or "ready to start pre-release", read
`docs/internal/pre-release-documentation-audit.md` and apply its readiness
boundaries. This means the complete product ecosystem, not only Core code or
normal CI: BaseHarbor, Demo, Console, Node Connector, CLI/JSON/MCP contracts,
EN/DE docs, Pages build/navigation, changelog/release notes, immutable ecosystem
pins and the release-specific gate/evidence plan. Check actual current SHAs and
source-qualified evidence. Missing, failed, skipped or unverified checks block
readiness; report the exact blockers and complete authorized preparation.

Pre-release-ready excludes only the final complete pre-release approval and
subsequent promotion/tag/publication/post-release checks. Actual production
Pages deployment belongs after promotion; validate its candidate build and
workflow before pre-release. Do not publish Pages just to claim readiness.

Preparing readiness does not authorize launching the complete pre-release,
merging, tagging or publishing. Preserve the single shared release PR and
other sessions' commits. Never substitute historical green consumer tests for
compatibility with the current Core candidate. Reuse green evidence only when
the existing verifier proves unchanged inputs; rerun only affected gates.

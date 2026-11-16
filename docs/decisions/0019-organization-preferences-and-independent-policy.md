# ADR 0019: Organization preferences and independent policy

Status: Accepted for resolver/Target integration; full lifecycle acceptance pending.

## Context

The distribution foundation already pins organization configuration and shares
its output between CLI/JSON/MCP/HTTP. Treating policy as another preference list
allowed a later environment list to erase inherited mandatory policy.
Explicit Target selection also returned before consulting the active organization.

## Decision

Extend that existing resolver, without introducing a parallel configuration
system. Use deterministic builtin < organization < team < user < repository <
invocation preference order. Retain mandatory policy independently and evaluate
organization/team constraints against the final references. Emit non-secret
winning/overridden source provenance and typed actionable denials.

The [resolution specification](../spec/organization-resolution-v1.md) defines
field merging, absence, trust, canonical digests and consumer boundaries.
Requests cannot supply managed scopes or policies. Company names, product
choices, CA paths and distribution backends are not frozen by this metamodel.

## Standards and consequences

Use existing JSON/YAML types, SHA-256 content identity and immutable source
resolution. BaseHarbor defines only the minimal cross-surface scope/provenance
and constraint semantics. Portable requirements remain separate.
No IAM directory, legacy mode, migration layer or broader onboarding is added.
Tests cover precedence boundaries, independent conflicting constraints,
mandatory-policy retention, source-secret rejection and surface parity.

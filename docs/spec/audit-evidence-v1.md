# Audit and evidence v1

BaseHarbor exposes one secret-safe evidence model for lifecycle, policy, runtime observation, verification and recovery.

## Scope

The evidence bundle is a read-only projection of BaseHarbor's existing semantic models. It does not create a second control plane and it does not replace plan, policy, status, doctor or recovery state.

The bundle distinguishes desired state, enforced policy, observed state, verified results, explicit exceptions, unsupported controls, recovery evidence and bounded audit events.

The schema version is `v1`.

## Export boundary

Human inspection uses `baha app evidence`. Machine export uses `baha app evidence -o json`. MCP exposes the same typed result through `baseharbor.evidence`.

JSON stdout is the generic integration boundary. BaseHarbor does not provision SIEM products, compliance backends or vendor-specific evidence destinations.

## Audit event fields

A lifecycle audit event can carry, where applicable: UTC timestamp; actor interface and non-secret actor identity; Target, application and environment; operation; capability/resource; provider; placement/ownership; policy result; lifecycle result; verification result; outcome; and secret-safe diagnostic detail.

BaseHarbor records meaningful lifecycle completion events rather than every internal reconciliation step. CLI operations default to actor interface `cli` with local operator identity. MCP lifecycle operations use interface `mcp` with local-agent identity. Prompts, model reasoning and raw secret input are never audit content.

## Recovery evidence

Recovery evidence is derived from the typed recovery manifest recorded with a successful backup/restore. Each contributor records state class, logical resource, ownership, support state (`supported`, `unsupported`, or `external`), whether it was selected, whether durable state is involved, whether exclusion was explicit, and whether the selected contributor was verified.

Logical identities are portable. Physical Docker/Podman volume names, credentials and provider-private state are not the recovery contract.

## Retention and ownership

Audit history is Target-owned BaseHarbor state and is stored below the Target state root. The local JSONL history is bounded to the most recent 1000 events and an 8 MiB read limit. The directory is owner-only and the audit file is `0600`.

Application destroy removes application runtime/state but does not silently erase Target-level audit history for that application. External workload data remains externally owned and is represented as an explicit exception rather than copied into BaseHarbor evidence or recovery ownership.

## Integrity and tamper evidence

A machine evidence bundle is deterministically ordered and sealed with a SHA-256 digest over the bundle before the integrity field is populated.

This digest is tamper evidence for an exported bundle; it is not a digital signature, remote attestation or compliance certification. BaseHarbor does not claim that local root/admin compromise can be detected by a digest stored alongside the exported data.

## Secret-safety boundary

Evidence must not contain secret values, access keys, tokens, private keys, credential-bearing connection URLs, interactive prompt contents, or model/agent reasoning. Evidence reuses BaseHarbor's machine-safe status, doctor and policy models. New evidence fields must preserve the same boundary.

## Trust boundary

Evidence collection is read-only and does not add network listeners, provider credentials or cross-application access. Audit persistence uses existing protected Target-local state. Export does not grant access to provider-native APIs or application data.

# ADR 0006: Portable application contract before runtime providers

Status: Accepted

## Context

BaseHarbor v0.3.0 completes the Compose lifecycle for local/self-hosted operation. The next architecture step must preserve that working path while preparing the same logical application requirements for Kubernetes, OpenShift and external capability providers.

The current manifest v1 contains both portable application intent and deployment/provider-specific details. Examples of portable intent are named relational database requirements, named key-value/cache requirements and required secret names. Examples of non-portable details are `app.environment` as deployment context and the current repository Compose workload selection.

If runtime/provider details are allowed to become the application API, Kubernetes/OpenShift support would require applications to be re-operationalized instead of simply being realized by a different provider.

## Decision

BaseHarbor introduces an explicit provider-neutral `PortableContract` domain view between manifest compatibility and runtime/capability realization.

The contract contains application-owned logical identity and portable capability intent only. The initial v0.4 seam represents:

- relational SQL resources as `database.sql` capabilities;
- cache/key-value resources as `cache.key-value` capabilities;
- managed-secret intent and required/generated secret declarations without selecting a secret product.

The following do not belong in the portable contract:

- environment/deployment context;
- Compose project, service, network, volume or host-port names;
- Kubernetes namespaces, Deployments, StatefulSets, Services, PVCs, Ingress/Gateway objects or Secret objects;
- OpenShift Routes, SCC details or Operator-specific resource names;
- OpenBao/Vault paths, AppRoles or provider credentials;
- TLS source directories or other provider-local implementation state.

Manifest v1 remains a supported compatibility surface. `PortableContractFromManifest` translates it one-way into provider-neutral intent. v0.4 must evolve this seam incrementally rather than replacing the working v0.3 manifest/runtime path in one rewrite.

Runtime providers and capability providers are separate axes:

```text
Application source / manifest compatibility
                 |
                 v
        PortableContract
                 |
        +--------+---------+
        |                  |
        v                  v
 Capability providers   Runtime provider
 SQL/cache/secrets      Compose / Kubernetes / OpenShift
```

A runtime provider owns workload realization and provider-native objects. A capability provider owns implementation of a requested service capability. For example, an OpenShift workload may still use an external PostgreSQL provider, Vault and Ceph RGW.

Provider selection must preserve requested guarantees or fail clearly. It must never silently reduce security, durability or availability.

## Consequences

- Existing v0.3 Compose manifests and CLI behavior remain compatible.
- Compose becomes the first provider implementation rather than the permanent conceptual application model.
- Kubernetes/OpenShift support can map the same portable intent to native primitives without adding Kubernetes/OpenShift fields to the common application contract.
- Future contract schema work can evolve independently from provider object schemas.
- Provider-specific workload configuration remains outside `PortableContract` until a genuinely portable workload/exposure model is deliberately designed.
- New capabilities must first define their portable semantics before a default product is wired into the application contract.

## Initial implementation boundary

The first v0.4 implementation intentionally does not introduce a generic plugin framework, full provider interface hierarchy or Kubernetes/OpenShift implementation. It establishes and exercises the smallest domain seam needed to prevent further provider leakage.

`BuildPlan` begins consuming logical SQL, key-value and secret intent through `PortableContract`; current Compose workload handling remains on the existing path until the runtime-provider seam is introduced incrementally.

## Follow-up

Subsequent v0.4 work should:

1. classify remaining manifest-v1 fields as portable intent, deployment context or provider-specific compatibility data;
2. define the runtime-provider boundary around workload realization and lifecycle operations;
3. introduce capability negotiation with explicit unsupported-capability failures;
4. build the generic input resolver on top of application/deployment ownership boundaries;
5. add provider-conformance tests that can later be reused by Compose, Kubernetes and OpenShift implementations.

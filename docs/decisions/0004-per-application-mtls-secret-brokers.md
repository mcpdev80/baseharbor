# ADR 0004: Per-application mTLS secret brokers

## Status

Accepted for the MVP runtime-secret path.

## Decision

Each BaseHarbor application that uses dynamic managed secrets gets its own runtime secret broker. The broker is not a shared multi-tenant runtime API.

The broker:

- is bound to exactly one application and environment;
- is attached only to that application's backend network and the internal `baseharbor-secrets` network;
- has no Docker/Podman socket;
- has no BaseHarbor manager or root credentials;
- authenticates to OpenBao only with that application's AppRole;
- runs read-only, non-root, with all Linux capabilities dropped and `no-new-privileges`;
- requires client certificates issued by the BaseHarbor internal CA;
- requires the client certificate URI SAN `spiffe://baseharbor/apps/<app>/<environment>`;
- additionally requires the application-scoped runtime bearer token;
- exposes no host port for normal application traffic.

Applications receive the broker URL and mTLS file locations as runtime environment variables. BaseHarbor creates and projects the credentials automatically; applications do not configure PKI topology.

The internal CA private key is never persisted as a host-side key file. BaseHarbor generates/signs in memory and stores the CA material in a manager-only OpenBao namespace. Leaf client/server keys are stored only in the protected application runtime state and are mounted only into the workloads that require them.

## Rationale

A shared broker connected to several application networks would increase blast radius. Per-application brokers preserve network and secret-domain isolation even if one broker is compromised.

mTLS alone is not treated as authorization. Runtime access is layered:

1. network isolation;
2. CA validation and application-specific URI SAN;
3. application runtime token;
4. application-specific OpenBao AppRole;
5. OpenBao policy restricted to the application secret scope.

## Standalone compatibility

Applications remain runnable without BaseHarbor. The broker is an optional BaseHarbor runtime capability and does not replace standard database, Redis-protocol, HTTP, TLS or provider configuration contracts.

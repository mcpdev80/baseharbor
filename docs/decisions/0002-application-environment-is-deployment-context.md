# ADR 0002: Application environment is deployment context

- Status: Accepted
- Date: 2026-09-10

## Context

BaseHarbor v0.x is intentionally focused on a Compose-based developer and single-host runtime, while the long-term application lifecycle may later include Kubernetes and OpenShift providers.

The repository-owned `baseharbor.yaml` currently contains both the stable application identity and an `app.environment` value. Runtime resources use the pair to isolate deployments, for example `baseharbor-workload-<app>-<environment>`.

Without an explicit semantic boundary, `environment` could be mistaken for an intrinsic property of the application contract. That would make it harder to run the same application source concurrently as development, staging, production, or a customer-specific deployment.

## Decision

`app.name` is the stable logical application identity.

`app.environment` is the deployment-instance context for the current BaseHarbor realization of that application. It is not a business property of the application and must not be treated as permanently coupled to the source code or to one runtime provider.

For the Compose-focused v0.x implementation, keeping `app.environment` in `baseharbor.yaml` is an accepted compatibility and usability choice. It may continue to participate in Compose project names, runtime directories, backend isolation, backup identity, and lifecycle operations.

Future environment/profile work may move selection or override of deployment context outside the repository-owned application requirements without changing the logical application identity.

In particular, the architecture must allow the same application contract to be realized independently as, for example:

```text
mailflow / dev
mailflow / staging
mailflow / production
mailflow / customer-a-production
```

## Compose-first rule for v0.x

Compose is the current implementation target and should be made complete and consistent before additional providers are introduced.

Compose-specific implementation details are allowed inside the Compose/runtime layer. They must not become application requirements unless they represent a genuinely portable application need.

Examples of implementation details that must remain provider-owned:

- Compose project names
- Docker/Podman network names
- generated host ports
- container names
- volume names
- OpenBao internal paths and role names

Applications should continue to consume standard interfaces such as database URLs, Redis/Valkey URLs, secret files, service endpoints, and normal health semantics.

## Consequences

- No new Kubernetes/OpenShift feature is required for v0.2.0.
- Existing Compose behavior and manifest version 1 remain compatible.
- New v0.2 work must not introduce additional direct Docker/Podman assumptions into the portable application requirement model.
- Runtime-specific operations should stay behind the existing runtime/Compose boundary where practical.
- Multiple named resources remain logical resources and are not HA replicas.
- Future provider and environment-profile work can evolve behind the application contract instead of forcing applications to be re-operationalized.

## Release gate

For v0.2.0, a change is release-blocking only when it creates a security/data-loss problem, breaks the current Compose lifecycle, or hard-codes a provider/environment assumption that would materially prevent later provider evolution. Missing future Kubernetes/OpenShift, identity, policy, GUI, or HA capabilities are not v0.2.0 blockers.
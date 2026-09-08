# ADR 0003: Stable application backend network

- Status: Accepted
- Date: 2026-09-08

## Context

BaseHarbor provisions backend services such as PostgreSQL and Valkey for independent applications. The MVP reference applications (AWC, MailFlow and AI-Coding-System) already bring their own container topology and Compose networks.

BaseHarbor must connect those existing workloads to the backend services it manages without replacing application-owned networks, rewriting application architecture or becoming a generic container-hosting platform.

The application must also remain usable without BaseHarbor.

## Decision

Every BaseHarbor application environment has one stable logical **Application Backend Network**.

For the current Docker/Podman Compose provider the network name is deterministic:

```text
baseharbor-<application>-<environment>_default
```

BaseHarbor exposes that identity through `ApplicationBackendNetworkName` rather than allowing workload integration code to reconstruct provider naming rules independently.

BaseHarbor-managed PostgreSQL and Valkey services already run on this Compose network. Future workload integration attaches the application containers that require backend access to this network as an **additional** network.

Application-owned networks remain unchanged.

Example:

```text
MailFlow internal network
        |
   api / worker / web
        |
        +---------------- Application Backend Network ----------------+
                                  |                  |
                             PostgreSQL            Valkey
```

For AWC the same rule allows the coordinator or other selected workload containers to retain the existing `awc-runtime` relationships while additionally reaching BaseHarbor-managed services.

## Isolation

The Application Backend Network is scoped by application and environment.

Therefore:

- `mailflow/prod` and `mailflow/dev` do not share a backend network;
- `mailflow/prod` and `awc/prod` do not share a backend network;
- unrelated applications are not connected to each other's BaseHarbor backend services by default.

Normal LAN/Internet egress is not disabled by this decision. Application workloads such as AWC, MailFlow and AI-Coding-System must still be able to reach external services such as AgentGateway, mail servers, Git forges and other provider endpoints.

## Host and container access are different views of the same logical services

Host-run development keeps the existing loopback-only contract, for example:

```text
DATABASE_URL=postgresql://...@127.0.0.1:<allocated-port>/...
REDIS_URL=redis://...@127.0.0.1:<allocated-port>/0
```

Containerized workloads will receive container-routable URLs using service DNS names on the Application Backend Network.

Application code still consumes ordinary variables such as `DATABASE_URL` and `REDIS_URL`; BaseHarbor chooses the correct view when materializing/injecting the runtime contract.

## Ownership and lifecycle

The current Compose backend owns creation and removal of the network together with the BaseHarbor backend project. Lifecycle inspection treats the network as an owned runtime resource and validates the Compose project ownership label before destructive operations.

Once application workload attachment is implemented, shutdown order must detach/stop application workloads before removing the BaseHarbor backend network. BaseHarbor must not silently remove application-owned networks.

## Consequences

Positive:

- existing applications keep their own Compose topology;
- backend connectivity has one stable attachment point;
- app/environment isolation is explicit;
- host-side loopback access remains available;
- no BaseHarbor SDK or proprietary data protocol is introduced.

Trade-offs:

- the current physical network name contains a Compose-provider naming convention;
- workload lifecycle must coordinate network attachment before backend teardown;
- future providers such as Kubernetes may map the same logical boundary to a different implementation.

The logical Application Backend Network contract is stable even if a future provider changes the physical network implementation.

## MVP implication

This decision is a prerequisite for the MVP workload integration slice. The MVP acceptance tests for AWC, MailFlow and AI-Coding-System must prove that application-owned networks continue to work while selected workload containers can reach their own BaseHarbor-managed services and cannot implicitly reach another application's backend network.

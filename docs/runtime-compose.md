# Local Compose runtime

BaseHarbor's first operational runtime is intentionally small and single-node.

## Services

`baha up` materializes an embedded Compose definition and starts:

- PostgreSQL 18
- OpenBao 2.6.x

Both services bind to loopback by default. They are not exposed on all host interfaces.

## Generated state

Runtime files live below:

```text
.baseharbor/runtime/
├── compose.yaml
└── runtime.env
```

The directory is ignored by Git. Files are written with owner-only permissions. The PostgreSQL password is generated from cryptographically secure random bytes on the first `baha up` and preserved on subsequent runs.

The embedded Compose definition is rewritten from the current `baha` binary so runtime definitions can evolve with BaseHarbor upgrades without overwriting the generated secret environment.

## Commands

```bash
baha up
baha status
baha doctor
baha down
```

`baha up` validates the Compose configuration before starting containers.

`baha status` shows both container state and service readiness. A container being alive is not considered sufficient.

`baha doctor` verifies:

- supported host OS
- reachable Docker or Podman daemon
- working Compose integration
- PostgreSQL reachability once runtime state exists
- OpenBao initialization/seal state once runtime state exists

## OpenBao lifecycle

OpenBao deliberately does not run in development mode and BaseHarbor does not inject a static root token.

The first `baha up` therefore starts a persistent but uninitialized OpenBao server. Until initialization and unseal lifecycle commands are added, `baha status` and `baha doctor` will report OpenBao as not ready.

This is intentional: BaseHarbor must not claim a secrets service is healthy merely because its container process is running.

A following lifecycle change will add guided OpenBao initialization/unseal handling and connect generated application credentials to the existing credential broker.

## Scope

This runtime is the first single-node operational layer. It does not yet include:

- bundled OIDC provider
- automatic OpenBao initialization/unseal
- PostgreSQL schema/bootstrap execution from `baha up`
- TLS termination
- Kubernetes deployment

Those concerns remain separate so each security boundary can be tested before it becomes part of the default bootstrap path.

# Developer access

BaseHarbor provides a trusted-local developer access layer for day-to-day Compose workflows. The goal is to use logical application resource and workload names instead of generated ports, container names or OpenBao internals.

## Canonical development URLs

Each local development Target owns one development domain. The default is:

```text
baha.localhost
```

The guided setup asks for this value once. Change or inspect it later with:

```bash
baha dev domain
baha dev domain dev.example.internal
```

BaseHarbor derives browser-facing names automatically; applications do not carry these local names in `baseharbor.yaml`.

```text
<app>.<domain>
<app>-<service>.<domain>

pgadmin.<domain>
cache.<domain>
storage.<domain>
auth.<domain>
auth-admin.<domain>
secrets.<domain>
metrics.<domain>
```

A Target-scoped local HTTPS gateway owns the canonical browser entry points and routes to the already verified application/provider endpoints. The gateway certificate contains the active canonical hosts as DNS SANs and backend TLS is verified against BaseHarbor-managed trust. Random loopback ports remain runtime implementation details and are not normal developer-facing addresses.

For an unambiguous repository workload with one selected service and one detected HTTP port, BaseHarbor automatically publishes the canonical `<app>.<domain>` route. Ambiguous workloads are not guessed and require explicit exposure intent.

`baha status`, `baha doctor` and structured status output use canonical browser URLs in development. Internal loopback ports remain available only to lifecycle/readiness internals and verbose diagnostics.

## Development management login

A development Target also owns one management login when Identity or a management UI is selected. The default username is `developer`; BaseHarbor generates a strong password unless the guided setup receives an explicit password.

```bash
baha dev credentials
baha dev credentials --reset
```

The same development identity is reconciled through managed OIDC when Identity is present. Provider UIs that require native authentication receive a provider-specific adapter using the same development credentials. Database passwords, S3 credentials, runtime identities and other service credentials remain separate least-privilege credentials.

This convenience boundary applies only to `dev`. Test/prod continue to require individual operator OIDC identities and environment policy.

## Database and cache shells

Inside an application repository:

```bash
baha app psql
baha app redis
```

If an application declares multiple logical instances, select one explicitly:

```bash
baha app psql primary
baha app psql analytics
baha app redis cache
baha app redis sessions
```

`psql` uses the materialized owner-only PostgreSQL binding and passes the password through the child-process environment rather than a command-line argument. `redis` prefers `valkey-cli` and falls back to `redis-cli`; authentication is likewise supplied through the client environment.

The required client must be installed locally. BaseHarbor does not hide a missing client by opening an unrelated container shell.

For stored application state outside a repository, select the application explicitly:

```bash
baha app psql --app mailflow
baha app redis cache --app mailflow
```

## Connection metadata

Connection metadata is masked by default:

```bash
baha app creds postgres
baha app creds valkey cache
```

Output includes resource, instance, host, port and non-secret database/user metadata. Password and credential-bearing URI remain masked.

Explicit reveal is a separate action:

```bash
baha app creds postgres --reveal
```

`dev` remains trusted-local and does not require `baha login`. The same application operations against `test` or `prod` require the configured Target/Environment OIDC operator boundary. Use `baha login -e test`, `baha whoami -e test` and `baha logout -e test` explicitly when needed. Advanced RBAC/JIT/break-glass governance remains a later platform-access layer.

## Workload logs

```bash
baha app logs
baha app logs api
baha app logs api --follow
```

These commands operate on the application-owned Compose workload selected by `baseharbor.yaml`. Developers do not need to know the generated Compose project or container name.

## Workload shell and exec

Open a shell in a selected workload service:

```bash
baha app shell api
```

Run a non-interactive command:

```bash
baha app exec api env
baha app exec worker ./bin/worker --version
```

Workload commands are routed through BaseHarbor's Compose runtime boundary and use the same materialized workload overlays/bindings as lifecycle operations. BaseHarbor never rewrites the application's source Compose file for developer access.

## Safety model

- Resource access resolves logical application/instance names first.
- Generated host ports and container names remain implementation details.
- Database/cache passwords are not placed in client command arguments.
- Credential values are hidden by default.
- Explicit reveal is separate from normal connect/use actions.
- Workload access is limited to services selected by the application contract.
- Missing or ambiguous resource instances fail clearly instead of guessing.
- Test/prod require the configured OIDC operator boundary; trusted-local dev does not. Advanced approval, JIT elevation and break-glass governance remain future policy layers.

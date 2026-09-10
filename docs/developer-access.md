# Developer access

BaseHarbor v0.3 adds a trusted-local developer access layer for day-to-day Compose workflows. The goal is to use logical application resource and workload names instead of generated ports, container names or OpenBao internals.

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

Trusted-local v0.3 does not yet apply managed-production OIDC/RBAC/JIT policy. The command/action boundary is intentionally structured so later environment policy can govern the same action names without changing the normal developer workflow.

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
- Managed-production identity, approval and JIT elevation remain future policy layers, not local-development requirements.

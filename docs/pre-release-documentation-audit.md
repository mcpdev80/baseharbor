# v0.2.0 pre-release documentation audit

This audit is the documentation cleanup gate for BaseHarbor `v0.2.0`.

## Confirmed current behavior

- `baha up` supports guided first-run port selection plus `--yes`, `--postgres-port` and `--openbao-port`.
- control-plane runtime state is user-global by default and survives application checkout changes.
- `baha app init` creates a repository-owned `baseharbor.yaml` and is the preferred application workflow.
- repository app commands can resolve the nearest `baseharbor.yaml` without repeating the application name.
- PostgreSQL and Valkey support multiple named logical instances.
- named logical instances are independent services, not HA replicas.
- `baha app env` exposes the protected application environment/binding contract without making BaseHarbor a runtime SDK dependency.
- required and generated secrets gate workload startup correctly.
- static secret injection/file binding and dynamic app-scoped secret references are implemented.
- the per-application runtime broker uses scoped runtime identity and mTLS.
- `baha app backup` and `baha app restore` create and restore encrypted recovery units including application metadata, managed PostgreSQL instances and the application OpenBao scope.
- `baha app status` and `baha app doctor` verify real service/application boundaries rather than only container state.
- OpenBao bootstrap, status and manual unseal are implemented.
- Compose is the complete current workload/runtime provider and MailFlow acceptance validates the real MailFlow `main` branch.

## Architecture alignment

`app.name` is the stable logical application identity. `app.environment` is deployment context used by the current Compose implementation for isolation and naming.

Compose-specific project names, networks, host ports, volumes and generated overrides are implementation details. The public direction keeps the logical application requirements suitable for later Kubernetes and OpenShift providers without claiming those providers as `v0.2.0` features.

The intended lifecycle direction is:

```text
local development / homelab
        ↓
Compose production
        ↓
future Kubernetes / OpenShift
        ↓
enterprise deployment profiles
```

The application contract and standard application-facing interfaces should survive that progression.

## v0.2.0 documentation gate

Before tagging `v0.2.0`:

1. ensure README, English and German public documentation describe the current Compose-first scope consistently;
2. ensure release guidance points to the `0.2` compatibility line;
3. ensure architecture documentation distinguishes application identity from deployment context;
4. ensure future Kubernetes/OpenShift/HA/security capabilities are clearly marked as future work rather than current release features;
5. run the normal release and real-product acceptance gates on the final documentation commit;
6. tag the exact green `main` commit and verify release binaries, checksums, provenance and runtime-image version coupling.

The Pages site must render repository documentation rather than maintaining a separate independent prose source.

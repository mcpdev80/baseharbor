# Getting started

This tutorial gets an existing application running under BaseHarbor without requiring provider knowledge or manual manifest editing.

## Try it: a Go order API with SQL

Prerequisites: install the `baha` release binary, make a local Docker or Podman runtime available, and use a parent directory without an `orders-api` folder. Check the CLI and the effective Target first:

```bash
baha version
baha target show
```

If no Target is configured, follow [Target commands](../cli/targets.md) before deploying. Create a fresh application:

```bash
baha app new orders-api --stack go --http --sql
cd orders-api
baha plan
baha up -e dev
baha status
baha doctor
```

`app new` writes `main.go`, `go.mod`, `Dockerfile`, `compose.yaml`, `baseharbor.yaml`, repository metadata and an empty-value `.env.example`. The manifest requests SQL and exposes the `app` workload on port 8080. The Go scaffold includes `pgx`, uses the protected `DATABASE_URL` binding and checks database connectivity before serving HTTP.

On first `up`, answer the guided operator-owned decisions described below. Readiness is the expected result after successful deployment; creating files alone does not start a runtime. Open the canonical HTTPS application URL reported by `status` and request `/healthz`. Use the reported port, which can differ between Docker and rootless Podman.

The scaffold is a starting point: add your order endpoints and schema. The [PostgreSQL example](../how-to/postgres.md) shows how to insert and query a real sample order.

Stop only this application's runtime while preserving its persistent data:

```bash
baha app down
```

Run `baha up` again from the repository to resume it.

## Adopt an existing application instead

For a repository you already own, the normal path is inspection, guided adoption, then `baha up`. You do not need to create a new scaffold or manually rewrite Compose to use that path.

## 1. Optional: inspect the repository

```bash
baha app inspect .
```

Inspection is read-only. BaseHarbor reports detected workload, replaceable infrastructure and capability evidence without mutating the repository.

Use detailed evidence when needed:

```bash
baha app inspect . --verbose
```

## 2. Create the portable application contract

```bash
baha app init
```

BaseHarbor detects what it can and asks only for ambiguous or user-owned decisions.

The guided flow may ask you to:

- choose the authoritative workload source when multiple Compose, Quadlet or Kubernetes candidates exist;
- confirm application workload versus replaceable infrastructure;
- confirm provider-neutral SQL, cache, object-storage and observability intent;
- name application-owned secrets and mark them required or optional;
- choose whether an application secret is generated, entered during first apply, or configured later;
- confirm Runtime API permissions derived from concrete source evidence.
- choose the Target-scoped development domain once (default `baha.localhost`);
- accept or customize the single Target-scoped development management login used by selected local management surfaces.

Before writing `baseharbor.yaml`, BaseHarbor shows a human-readable adoption summary. Raw YAML is secondary detail available with `--verbose`.

For deterministic automation with unambiguous evidence:

```bash
baha app init --quick
```

`--quick` fails closed on source ambiguity and never silently promotes heuristic secret candidates. Unambiguous Compose, repository-authored Quadlet and raw Kubernetes YAML can be inspected/adopted without turning their source-native names into portable intent.

## 3. Start the application

```bash
baha up
```

On the first run BaseHarbor may ask for information it cannot safely invent, for example:

- whether to accept or override the proposed Target-scoped location for the operator-held OpenBao recovery file;
- a missing required application-secret value;
- confirmation of a safe port fallback.

For a development Target, BaseHarbor derives browser-facing URLs from the Target-scoped development domain. Internal random loopback ports remain runtime implementation detail. The default domain produces names such as `https://my-app.baha.localhost` and `https://secrets.baha.localhost`.

Interactive secret input disables terminal echo. Provider/runtime credentials are managed by BaseHarbor and are not requested from the developer.

The same `baha up` operation continues after these decisions and converges managed infrastructure, workload bindings and readiness. After successful OpenBao bootstrap, BaseHarbor persists only the recovery-file path reference on the effective Target. Later `baha up` runs automatically reuse that reference to unseal the shared OpenBao provider when the file is available.

## 4. Verify

```bash
baha status
```

```bash
baha doctor
```

A successful BaseHarbor operation means the relevant capability was verified, not merely that a container started.

For local development, `status` and `doctor` report canonical HTTPS URLs rather than internal `127.0.0.1:<port>` addresses. Inspect or change the Target-scoped local access settings explicitly with:

```bash
baha dev domain
baha dev credentials
```

`baha dev credentials` is an explicit secret-reveal command. Normal status, doctor, plan and evidence output never prints the password.

## Advanced and automation commands

These commands remain available, but are not required knowledge for the basic happy path:

```bash
baha plan
baha app preflight
baha app apply
baha app secret set APP_SECRET
```

Automation can use explicit non-interactive secret input:

```bash
printf '%s' "$APP_SECRET" | baha app secret set APP_SECRET --stdin
```

## Reference end-to-end demo

The external `mcpdev80/baseharbor-demo` repository is the release-facing proof of this journey. Its README documents a complete pristine-repository test from `baha app init` through `baha up`, READY verification, restart and cleanup.

Pre-release validation executes both:

- the guided human adoption scenario;
- deterministic CI/component scenarios.

The final release reuses that immutable pre-release evidence instead of rerunning the same expensive matrix.

## Next steps

- [Application contract](../explanation/application-contract.md)
- [Environments](../how-to/environments.md)
- [PostgreSQL](../how-to/postgres.md)
- [Secrets](../how-to/secrets.md)
- [Backup and restore](../how-to/backup-restore.md)
- [CLI documentation](../cli/index.md)

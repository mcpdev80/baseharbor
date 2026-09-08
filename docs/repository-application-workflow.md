# Repository-first application workflow

BaseHarbor treats `baseharbor.yaml` in an application repository as the preferred application contract.

The repository contains only declarative requirements. Secret values, generated credentials, runtime environment files and service state do not belong in Git.

## Repository layout

```text
myapp/
├── baseharbor.yaml
├── compose.yaml              # optional existing application workload
├── backend/
├── frontend/
└── .baseharbor/
```

`baseharbor.yaml` is intended to be committed. `.baseharbor/` is generated runtime state and is automatically protected by a nested `.gitignore`.

A normal repository manifest may use standard Git-friendly permissions such as `0644`; BaseHarbor rejects it when it is writable by group or others. Generated runtime files remain owner-only.

## Create a manifest

A deterministic non-interactive form is available for scripts and CI:

```bash
baha app init mailflow --postgres --redis
```

or for multiple logical services:

```bash
baha app init mailflow \
  --postgres-instance primary \
  --postgres-instance analytics \
  --redis-instance cache \
  --redis-instance sessions
```

When `NAME` is omitted, `baha app init` uses the current directory name when it is a valid application slug.

An interactive checkbox-based capability picker is planned on top of the same generator. The generated manifest contract is identical whether it came from flags, the guided UI or manual editing.

## Work from anywhere inside the repository

BaseHarbor searches the current directory and then parent directories for the nearest `baseharbor.yaml`.

This works from the repository root:

```bash
baha app plan
baha app apply
```

and from nested directories such as `frontend/src`:

```bash
baha app status
baha app doctor
```

Runtime state is always anchored next to the discovered manifest:

```text
<repo>/.baseharbor/apps/<application>/
```

BaseHarbor does not create another `.baseharbor` tree in a nested working directory.

## Existing Compose workloads

A repository may keep its normal Docker/Podman Compose application topology. BaseHarbor does not replace application-owned networks or require BaseHarbor-specific application code.

When exactly one conventional Compose file is present, BaseHarbor can detect it automatically. Recognized conventions include root-level `compose.yaml`, `compose.yml`, `docker-compose.yml`, `docker-compose.yaml` and the same names below `infrastructure/`.

If more than one plausible Compose file exists, BaseHarbor fails closed instead of guessing. The repository can then add only the disambiguation it needs:

```yaml
workload:
  compose: infrastructure/docker-compose.yml
```

Complex applications can additionally limit which services are required to be running and receive the BaseHarbor backend attachment:

```yaml
workload:
  compose: docker-compose.yml
  services:
    - coordinator
```

The workload path must be relative, normalized and remain inside the application repository.

During `baha app apply`, BaseHarbor:

1. provisions and verifies the requested backend services first;
2. creates the stable per-app/per-environment Application Backend Network;
3. generates an owner-only `.baseharbor/.../workload.override.yaml`;
4. adds that network to the selected application Compose services without removing their existing networks;
5. injects container-routable standard service URLs; and
6. starts and verifies the repository workload.

Application-owned Compose files are never rewritten.

### Host versus container service URLs

A host-run process continues to receive loopback-only endpoints through `baha app env`:

```text
DATABASE_URL=postgresql://...@127.0.0.1:<allocated-port>/...
REDIS_URL=redis://...@127.0.0.1:<allocated-port>/0
```

A containerized workload receives the same logical variables, but with service DNS on the isolated Application Backend Network:

```text
DATABASE_URL=postgresql://...@postgres:5432/...
REDIS_URL=redis://...@valkey:6379/0
```

Named instances follow the same alias rules, for example `DATABASE_ANALYTICS_URL` and `REDIS_SESSIONS_URL`.

The application code therefore continues to consume normal ecosystem variables and does not need to know whether it is running on the host or in Compose.

### Lifecycle order

To keep the shared network safe and deterministic:

```text
apply/up:   BaseHarbor backend -> repository workload
down:       repository workload -> BaseHarbor backend
destroy:    stop workload -> delete BaseHarbor-owned backend state
```

`down` and `destroy` preserve application-owned Compose volumes. `destroy` also preserves the committed `baseharbor.yaml`.

## Normal developer flow

```bash
git clone <application-repository>
cd <application-repository>
baha app plan
baha app apply
```

After apply, normal host-side application tooling can consume the generated standard runtime contract:

```bash
baha app env --path
```

The application itself does not need `baha`, a BaseHarbor login or a BaseHarbor SDK at runtime. It continues to consume normal values such as `DATABASE_URL`, named database URLs, Redis/Valkey URLs, OIDC metadata and binding files.

## Secrets

The repository manifest contains secret names only:

```yaml
secrets:
  required:
    - name: OPENAI_API_KEY
    - name: SMTP_PASSWORD
```

Inside the repository, the application name is inferred:

```bash
printf '%s' "$SMTP_PASSWORD" | baha app secret set SMTP_PASSWORD --stdin
baha app secret list
baha app secret delete SMTP_PASSWORD --yes
```

Values remain outside Git and are never accepted as command-line arguments.

## Explicit-name compatibility

Repository discovery is a convenience, not a new runtime dependency. Existing explicit-name workflows remain valid:

```bash
baha app status mailflow
baha app doctor mailflow
baha app env mailflow
```

## Source-of-truth semantics

For repository-managed applications:

```text
baseharbor.yaml
      ↓
parse + validate
      ↓
desired state
      ↓
protected internal runtime state
      ↓
PostgreSQL / Valkey / secrets / future services
      ↓
optional existing application Compose workload
```

The repository manifest is authoritative. BaseHarbor may keep protected generated files to support its runtime services, but changing the repository manifest and applying it again converges toward the new desired state without rotating unrelated existing credentials or state.

`baha app destroy --yes` deletes BaseHarbor-managed runtime resources and `.baseharbor` application state, but preserves the repository `baseharbor.yaml` and application-owned Compose volumes. Running `baha app apply` can therefore recreate the BaseHarbor backend from the committed contract.

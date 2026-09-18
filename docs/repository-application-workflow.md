# Repository-first application workflow

BaseHarbor treats `baseharbor.yaml` in an application repository as the preferred application contract.

The repository contract contains declarative desired state; provider-specific compatibility fields may exist in Manifest v1, while generated runtime/deployment state remains outside Git. Secret values, generated credentials, deployment TLS material, runtime environment files and service state do not belong in Git.

## Repository layout

```text
myapp/
├── baseharbor.yaml
├── compose.yaml              # optional existing application workload
├── backend/
├── frontend/
└── .baseharbor/              # protected generated runtime/deployment state
```

`baseharbor.yaml` is intended to be committed. `.baseharbor/` is generated runtime state and is automatically protected by a nested `.gitignore`.

A normal repository manifest may use standard Git-friendly permissions such as `0644`; BaseHarbor rejects it when it is writable by group or others. Generated runtime files remain owner-only.

## Create a manifest

The normal interactive path is:

```bash
baha app init
```

BaseHarbor analyzes the repository first, detects common Compose files, likely PostgreSQL/Redis/Valkey usage, application workload services and potential required secret names, then asks only for missing or ambiguous information. Detected choices are preselected and can be overridden. The generated manifest is previewed before it is written, and an existing manifest is never silently overwritten.

A non-interactive detect-first path is available when repository structure is unambiguous:

```bash
baha app init --quick
```

Deterministic explicit forms remain available for scripts and CI:

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

## Deployment/runtime initialization in v0.4

After Manifest v1 has been resolved into portable application intent, repository deployments may need Compose-specific operator/runtime inputs. v0.4 keeps those in protected deployment state and outside `PortableContract`.

Interactive setup may request a **Public FQDN** and deployment TLS mode. Existing/BYOC certificate mode accepts one certificate source directory, detects and validates a matching certificate/key pair including FQDN coverage, and normalizes the material into owner-only BaseHarbor state.

The same protected deployment state owns automatic workload host-port fallbacks selected when configurable Compose publishers conflict with an occupied local port. Explicit operator environment overrides remain authoritative.

These values are deployment realization, not portable application identity or capability requirements.

## Work from anywhere inside the repository

BaseHarbor searches the current directory and then parent directories for the nearest `baseharbor.yaml`.

This works from the repository root:

```bash
baha app plan
baha app apply
```

and from nested directories such as `frontend/src`:

```bash
baha app show
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

When exactly one conventional Compose file is present, BaseHarbor can detect it automatically. If more than one plausible Compose file exists, BaseHarbor fails closed instead of guessing. The repository can then add only the disambiguation it needs:

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

An explicit repository Compose workload may also be a valid **workload-only** application without requesting PostgreSQL or Valkey. BaseHarbor does not invent unused backend services, credentials or networks for that shape. A manifest with neither managed capabilities nor an explicit workload still fails validation.

During `baha app apply`, BaseHarbor:

1. validates desired state and required secrets;
2. provisions and verifies requested managed backend services when present;
3. creates the stable application backend network only when managed backends require it;
4. generates owner-only runtime/Compose override files;
5. attaches selected workload services without removing application-owned networks;
6. injects only the standard managed-service information actually requested by the application;
7. starts the repository workload; and
8. verifies selected service state/health and conventional application-owned HTTP/HTTPS exposure.

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

### Workload readiness and exposure

Selected Compose services are not considered READY merely because their containers are running. BaseHarbor distinguishes starting, unhealthy, exited and missing services when Compose exposes that state.

Conventional web publishers are additionally probed on the locally published host port. Redirects count as reachable application-owned HTTP exposure. A 5xx response or unreachable endpoint is NOT READY. Non-web ports such as PostgreSQL are not guessed as HTTP.

For hostname-bound HTTPS, BaseHarbor still dials the local published socket but uses the configured public FQDN as the HTTP Host/TLS ServerName. This keeps verification local while matching the application-owned TLS site.

### Lifecycle order

To keep shared state safe and deterministic:

```text
apply/up:   BaseHarbor backend -> repository workload -> readiness verify
down:       repository workload -> BaseHarbor backend
destroy:    stop workload -> delete BaseHarbor-owned backend/runtime state
restore:    validate -> stop -> restore -> regenerate identity -> restart -> verify
update:     preflight -> fast-forward source -> reconcile -> verify
```

`down` and `destroy` preserve application-owned Compose volumes. `destroy` also preserves the committed `baseharbor.yaml`.

## Normal developer flow

```bash
git clone <application-repository>
cd <application-repository>
baha app init        # only when the repository does not already contain baseharbor.yaml
baha up
baha app show
```

For an already initialized repository, the normal lifecycle remains:

```bash
baha app plan
baha app preflight
baha app apply
baha app status
baha app doctor
```

After apply, normal host-side application tooling can consume the generated standard runtime contract:

```bash
baha app env --path
```

Trusted-local convenience commands can connect through logical resources/services without exposing generated container names:

```bash
baha app psql
baha app valkey
baha app logs
baha app shell SERVICE
baha app exec SERVICE COMMAND
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

Values remain outside Git and are never accepted as normal command-line secret arguments.

## Backup, recovery and update

Interactive terminals can create/restore encrypted recovery units without placing passwords in argv:

```bash
baha app backup
baha app restore ./mailflow-production.bhbackup
```

Automation can use `--password-file` explicitly. Successful backup/recovery metadata is recorded under protected application state without persisting secret-bearing detail.

Git-backed application updates are inspectable before mutation:

```bash
baha app update --check
```

Mutation is strict fast-forward only. Dirty, ahead or diverged repositories fail closed. Durable applications require either an encrypted pre-update recovery point or explicit `--no-backup` acknowledgement. After source advancement, BaseHarbor reuses the normal application reconciliation/readiness lifecycle and only reports success when the updated application is READY.

## Existing/BYOC TLS updates

For a repository initialized with existing TLS certificates:

```bash
baha app tls update --check
baha app tls update
```

The check is read-only. Mutation validates the configured source pair/FQDN, refuses downgrades, installs protected normalized files, restarts the workload when required and verifies readiness. ACME automation and provider-neutral certificate lifecycle remain future work.

## Explicit-name compatibility

Repository discovery is a convenience, not a new runtime dependency. Existing explicit-name workflows remain valid where the command supports them:

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
portable desired state
      ↓
protected deployment/runtime realization
      ↓
managed capabilities + optional application Compose workload
      ↓
verified application boundary
```

The repository manifest is authoritative for portable desired state. BaseHarbor may keep protected generated/runtime files to realize that state, but changing the repository manifest and applying it again converges toward the new desired state without rotating unrelated existing credentials or state.

`baha app destroy --yes` deletes BaseHarbor-managed runtime resources and `.baseharbor` application state, but preserves the repository `baseharbor.yaml` and application-owned Compose volumes. Running `baha app apply` can therefore recreate BaseHarbor-owned runtime state from the committed contract, while application-owned data still requires its own appropriate recovery mechanism.

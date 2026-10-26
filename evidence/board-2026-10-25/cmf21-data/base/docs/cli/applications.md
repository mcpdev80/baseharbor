# Application commands

Application commands operate on the portable Application Contract and its deployment state.

Use [Create and edit application needs](../how-to/application-needs.md) for the interactive checklist, Target provider preferences and footprint preview.

## Authoring and adoption

| Command | Purpose |
| --- | --- |
| `baha new application` | Create a new ecosystem-native application |
| `baha app new` | Open the interactive needs wizard |
| `baha app edit` | Edit existing needs with preview and consent |
| `baha app footprint show` | Read the Target-bound footprint |
| `baha init` | Create/adopt a repository-owned `baseharbor.yaml` |
| `baha inspect` | Inspect repository/application evidence |
| `baha app show` | Show the resolved application contract |
| `baha list` | List known applications |
| `baha plan` | Show the application plan |
| `baha app preflight` | Validate before mutation |

v0.4.19 capability flags remain semantic rather than product-specific. Applications request capabilities such as durable key-value, document database or messaging; they do not request MongoDB/RabbitMQ by product name.

### Source-neutral repository adoption

`baha inspect` and `baha init` understand these v0.4.20 repository workload sources through the same source-adapter contract:

- Compose;
- repository-authored Podman Quadlet;
- raw Kubernetes YAML.

Human inspection reports the standardized source-resolution state. JSON/MCP expose `baseharbor.workload-source-resolution/v1`.

For deterministic non-interactive adoption, use logical component identity plus an explicit repository source when ambiguity exists:

```text
baha init my-app \
  --workload-source kubernetes:deploy/k8s \
  --workload-component api \
  --workload-component worker
```

`--quick` fails safely when source ambiguity cannot be resolved deterministically. Guided init asks once and persists `baseharbor.repository.yaml` only when a repository really needs an explicit source choice.

Helm and Kustomize are not v0.4.20 source adapters.

## Lifecycle

| Command | Purpose |
| --- | --- |
| `baha up` | Converge an application |
| `baha app apply` | Apply/reconcile through the application lifecycle |
| `baha down` | Stop application runtime state |
| `baha status` | Observe application state |
| `baha doctor` | Diagnose and optionally repair |
| `baha app update` | Update supported application source/state |
| `baha destroy` | Remove application-owned resources |

## Operations

Application operations also include:

- backup and restore;
- required application secrets;
- logs;
- shell / exec;
- environment selection;
- TLS;
- evidence;
- runtime identity;
- PostgreSQL and Valkey access helpers.

Use the top-level `baha status`, `baha plan`, `baha doctor`, `baha up` forms for the normal repository-local workflow. Use `baha app ...` when the explicit application namespace is useful for administration or automation.

## Machine parity

Where an operation has structured output, human CLI and machine consumers use the same underlying domain result rather than separate lifecycle implementations.

See [Automation and agents](automation-agents.md).

## Example: create and inspect an API with two capabilities

Run from a parent directory with no `shop-api` folder:

```bash
baha new application shop-api --stack go --http --sql --cache
cd shop-api
baha inspect .
baha app show
baha plan
```

The scaffold declares SQL and cache intent, adds Go clients, and records `DATABASE_URL`/`DATABASE_CA_FILE` and `REDIS_URL`/`REDIS_CA_FILE` binding names. It does not create shop-specific endpoints or tables. After deployment with `baha up`, inspect the workload with `baha app logs app` and the masked bindings with `baha app env --format json`.

For adoption, start inside your existing repository with `baha inspect .`, then `baha init`. Do not create a second scaffold over existing application files.

## Local workload without managed capabilities (v0.4.25)

A developer-authored `baseharbor.yaml` must omit `app.id`; `services` is optional:

```yaml
version: 1
app:
  name: my-app
  environment: dev
```

Place a supported, unambiguous Compose workload in the repository, then run
`baha app init --quick`, `baha app plan --json` and `baha app apply`.
Initialization assigns a stable ID through the protected repository application
store without rewriting authored YAML. Read commands never allocate an ID.
Absent or ambiguous source is an error, not a READY deployment.

For a genuine workload without requested managed capabilities, the existing
local lifecycle skips Core, managed gateway and broker provisioning. No profile
switch, implicit SQL or Core login is needed. Status and doctor use real
workload observations; stop/up and repeated destroy retain their existing
ownership checks. This does not authorize `baha serve`: its management security
requirements and explicit management Core installation remain unchanged.

Default local `baha app destroy` retires the owned runtime and application routes,
while keeping persistent data, credentials, recovery material and the canonical
deployment identity. `baha app apply` reactivates that deployment from the current
application intent. Runtime removal and complete data deletion are separate
choices. Provider consequences are displayed with `Achtung` and require two
explicit acknowledgements of the same current plan; `--yes` alone supplies
neither acknowledgement. The structured destroy result reports retained material
and the common provider-change receipt. A verified runtime removal does not
establish continued secret delivery, login, certificate renewal or recovery
usability; unverified consequences remain partial in the receipt.

Plan JSON and MCP/HTTP plan share `footprint` (`baseharbor.footprint/v1`). A
projected new binding is `requested/unbound`, never live evidence. Requested
managed capabilities resolve to selected providers and their genuine dependencies;
there is no unconditional SQL/Secrets/Identity bundle for every application.
An independently enabled management Core keeps its security requirements.
Memory/container quantities remain unknown when actual attribution is missing;
provider observations remain `unverifiable` until their native inventory is
verified. Non-interactive app `--yes` does not authorize management Core setup.

Compatible native placements are reused. Unsupported native BYO or replacement
adapters fail before provisioning; external OIDC/OTLP retain their supported
adapters. Plans disclose the actual execution limits without claiming a working
replacement. No legacy format migration is provided.

Removing a capability releases its app binding and converges the remaining
Target-wide dependency union. Owned runtime services that no consumer needs are
retired; another application's or independent management consumer's requirements
preserve shared services. Persistent data, credentials and recovery material
remain at their owned locations by default. Retained material is recovery state,
not proof of continued service access. A subsequent supported re-add reuses the
existing system identity and protected provider state.

Intentional provider changes show affected consumers, lost functions, selected
continuation and retained/deleted material. Alternative-less, destructive or
unknown consequences display **Achtung** and require two explicit acknowledgements
of the current plan. Ordinary `--yes` supplies neither these acknowledgements nor
data-deletion consent. Actual authority, ownership and post-mutation verification
remain required. Failed or interrupted execution stays partial and retryable.
Release readiness requires the applicable provider, mutation-path and native
ecosystem acceptance; individual green fixtures do not establish readiness.

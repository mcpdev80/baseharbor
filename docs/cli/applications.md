# Application commands

Application commands operate on the portable Application Contract and its deployment state.

## Authoring and adoption

| Command | Purpose |
| --- | --- |
| `baha app new` | Create a new ecosystem-native application |
| `baha app init` | Create/adopt a repository-owned `baseharbor.yaml` |
| `baha app inspect` | Inspect repository/application evidence |
| `baha app show` | Show the resolved application contract |
| `baha app list` | List known applications |
| `baha app plan` | Show the application plan |
| `baha app preflight` | Validate before mutation |

v0.4.19 capability flags remain semantic rather than product-specific. Applications request capabilities such as durable key-value, document database or messaging; they do not request MongoDB/RabbitMQ by product name.

### Source-neutral repository adoption

`baha app inspect` and `baha app init` understand these v0.4.20 repository workload sources through the same source-adapter contract:

- Compose;
- repository-authored Podman Quadlet;
- raw Kubernetes YAML.

Human inspection reports the standardized source-resolution state. JSON/MCP expose `baseharbor.workload-source-resolution/v1`.

For deterministic non-interactive adoption, use logical component identity plus an explicit repository source when ambiguity exists:

```text
baha app init my-app \
  --workload-source kubernetes:deploy/k8s \
  --workload-component api \
  --workload-component worker
```

`--quick` fails safely when source ambiguity cannot be resolved deterministically. Guided init asks once and persists `baseharbor.repository.yaml` only when a repository really needs an explicit source choice.

Helm and Kustomize are not v0.4.20 source adapters.

## Lifecycle

| Command | Purpose |
| --- | --- |
| `baha app up` | Converge an application |
| `baha app apply` | Apply/reconcile through the application lifecycle |
| `baha app down` | Stop application runtime state |
| `baha app status` | Observe application state |
| `baha app doctor` | Diagnose and optionally repair |
| `baha app update` | Update supported application source/state |
| `baha app destroy` | Remove application-owned resources |

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
baha app new shop-api --stack go --http --sql --cache
cd shop-api
baha app inspect .
baha app show
baha app plan
```

The scaffold declares SQL and cache intent, adds Go clients, and records `DATABASE_URL`/`DATABASE_CA_FILE` and `REDIS_URL`/`REDIS_CA_FILE` binding names. It does not create shop-specific endpoints or tables. After deployment with `baha up`, inspect the workload with `baha app logs app` and the masked bindings with `baha app env --format json`.

For adoption, start inside your existing repository with `baha app inspect .`, then `baha app init`. Do not create a second scaffold over existing application files.

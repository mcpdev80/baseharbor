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

Use the top-level `baha status`, `baha plan`, `baha doctor`, `baha up` and `baha down` forms for the normal repository-local workflow. Use `baha app ...` when the explicit application namespace is useful for administration or automation.

## Machine parity

Where an operation has structured output, human CLI and machine consumers use the same underlying domain result rather than separate lifecycle implementations.

See [Automation and agents](automation-agents.md).

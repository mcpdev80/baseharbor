# Application commands

Application commands operate on the portable Application Contract and its deployment state.

Use [Create and edit application needs](../how-to/application-needs.md) for the interactive checklist, Target provider preferences and footprint preview.

## Authoring and adoption

For the complete local, remote and team path, start with [Local, remote and team development](../how-to/local-remote-team-workflow.md).

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

The default `baha app new` path is create-only: it writes the project files and
prints the next steps, but does not register a deployment on the selected
Target. `baha app list` shows registered deployments, so the generated project
does not appear there until it is initialized and applied. In the generated
repository, run `baha app init --quick` and then `baha up`. If the selected
Target requires an operator session, sign in before initialization.

In `baha app new`, choosing `run` creates the project and then shows the exact
repository candidate identity and confirmation hash before deployment. Confirm
that candidate to continue. Declining leaves the project files in place and
prints the command needed to apply them later. Choosing `create-only` never
starts the lifecycle.

| Command | Purpose |
| --- | --- |
| `baha up` | Converge an application |
| `baha app apply` | Apply/reconcile through the application lifecycle |
| `baha app stop` / `baha down` | Stop application runtime while preserving persistent data |
| `baha status` | Observe application state, readiness and the unified version section |
| `baha doctor` | Diagnose and optionally repair |
| `baha app update` | Update supported application source/state |
| `baha app destroy NAME` | Remove one application and its owned resources |

## Operations

Application operations also include:

- backup and restore;
- required application secrets;
- logs;
- shell / exec;
- URL listing (`baha app urls`);
- environment selection;
- TLS;
- evidence;
- runtime identity;
- PostgreSQL and Valkey access helpers.

Use the top-level `baha status`, `baha plan`, `baha doctor`, `baha up` forms for the normal repository-local workflow. Use `baha app ...` when the explicit application namespace is useful for administration or automation.

## Guided observation and access

`baha app logs`, `baha app exec`/`baha app shell` and `baha app urls` guide a
missing choice from the same secret-safe read view that `baha app status`
renders; they never consult a second parallel source of truth.

- `baha app logs [COMPONENT] [--follow] [--tail N] [-c NAME]...` resolves the
  streamable components from the deployment log policy and the application
  contract. Without `COMPONENT`, a unique declared component is derived; in a
  terminal the options are listed; with `--no-input` or in scripts the command
  fails closed with the options instead of blocking. `--tail N` bounds the
  output and finishes. JSON output never follows an unbounded stream;
  `--tail` with `--json` is the bounded machine form. Managed (Loki) collection
  is reported explicitly with its machine result instead of a silent reroute.
- `baha app exec [--app NAME] [-e ENV|--environment ENV] [COMPONENT | -c NAME] [COMMAND [ARG...]]`
  opens the default shell when no command is given (same as `baha app shell`).
  Put selector options before the command; once `COMMAND` starts, its remaining
  arguments, including flags such as Python's `-c`, pass through unchanged.
  `--cmd` remains the separate form for one string run through the workload's
  POSIX shell. Positional commands remain direct argv and preserve their exit
  status.
- `baha app urls [NAME] [-c NAME]... [--print] [--json]` lists only the
  application's own URLs and the GUI URLs of its bound app/shared services,
  each with its verified/unverified accessibility. Nothing is guessed, invented
  or opened; opening the primary endpoint in a browser remains `baha open`.

Every path authorizes its operation category (`app.logs`, `app.exec`,
`app.urls`) fail-closed before any resolution or execution, following the
`secret.reveal` seam precedent: an unauthenticated boundary denies instead of
proceeding.

The same repository workflow applies to a selected remote Core Target:
`baha app init`, then `baha up`. Initialization registers the full repository
contract and configures deployment settings on the Core. The application name
and environment come from `baseharbor.yaml`; the Target alias does not encode
an environment. Interactive deployment displays the Core's provider-impact
summary of remaining changes from the Core plan. It distinguishes installation,
starting existing services, updates and unresolved checks; existing services
are listed compactly for reuse. One confirmation
approves these changes and the exact current application source. Noninteractive
clients receive the structured approval challenge. Remote work reports its
current phase and elapsed waiting time; JSON output stays machine-readable.
Enabling development management UIs creates their shared login on the Core
when none exists. Existing credentials are recovered and reused. A login explicitly
chosen in the initialization wizard is configured on the selected Core, including
its username and custom or generated password; it is not just saved locally.
The development domain selected in the wizard or with `baha dev domain DOMAIN`
is applied on the selected Core and projected locally. A new workstation offers
the Core's existing domain as its default. Canonical names require DNS pointing
to that Core; IP links remain independent of DNS. With an owned Core LAN address
and configured names outside `.localhost`, the canonical gateway uses that
address unless an explicit bind override is set. `.localhost` stays loopback-only.
The prepared pgAdmin connection opens its application database and limits the
browser tree to that database. Internal Core databases are separate resources.
Declining or cancelling the interactive confirmation stops the rollout. Run
`baha up` again to review and confirm; no manual source-hash flag is needed.
A successful remote deployment prints a short READY summary and application
links. IP links use the Core address and observed dynamic public exposure ports;
canonical URL links are also shown. Use `--json` for the complete verified receipt
or `--verbose` for additional human-readable checks.

For a remote Target, select a free published workload port explicitly with `baha app apply NAME --workload-port VARIABLE=PORT`. BaseHarbor verifies that the variable exists in the repository Compose workload and that the port is free on the Core host, then saves it for that deployment. For example, `--workload-port DEMO_HTTPS_PORT=8081` avoids a Core-host conflict on port 8080 without changing the repository source.

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

For a remote Target that is already active, run `baha app destroy NAME --yes`; the
selected Target and repository environment are inferred. The CLI displays the
current provider plan and asks for approval before making changes. If managed
workstation host entries are enabled, it reconciles them after removal. The
Target gateway CA remains trusted while that Core connection is retained.

`baha app stop NAME --yes` (also available as `baha down`) removes the active
application workload and its released, app-scoped provider runtimes while
retaining persistent data, credentials, recovery state and the deployment owner
for later reactivation. The current provider consequence plan still requires
its own acknowledgement. `baha app destroy NAME --yes` permanently removes
BaseHarbor-owned application state, credentials, runtime resources, deployment
registration and Compose data volumes whose exclusive application ownership is
verified. Shared, foreign, bind-mounted or otherwise ambiguous data is preserved
and reported; shared Target infrastructure, the repository source and workstation
trust for the still-connected Core are not application-owned data. Provider
consequences and data deletion are shown in the current plan and require explicit
approval. There is no routine `--full-reset` switch: destroying the application
means deleting its owned data.

Plan JSON and MCP/HTTP plan share `footprint` (`baseharbor.footprint/v1`). A
projected new binding is `requested/unbound`, never live evidence. Requested
managed capabilities resolve to selected providers and their genuine dependencies;
there is no unconditional SQL/Secrets/Identity bundle for every application.
An installation record retains recovery ownership, not a permanent runtime
lease. A valid, unexpired workstation or Connector certificate is an independent
Core consumer; while one exists, Core SQL, OpenBao and Identity stay available.
Expired or revoked certificates do not pin runtime services. If that inventory
cannot be verified, BaseHarbor keeps the uncertain Core services running. A
SQL-only application does not initialize OpenBao or Keycloak just because a Core
installation record exists.
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
unknown consequences display **Warning** and require two explicit acknowledgements
of the current plan. Ordinary `--yes` supplies neither these acknowledgements nor
data-deletion consent. Actual authority, ownership and post-mutation verification
remain required. Failed or interrupted execution stays partial and retryable.
Release readiness requires the applicable provider, mutation-path and native
ecosystem acceptance; individual green fixtures do not establish readiness.


### Deployment identity and recovery

Each application environment is registered exactly once per Target: the
`application_id` and environment identify one deployment, and a first
registration is serialized against concurrent registrations, so racing
`baha app init` runs cannot create a duplicate. Deployment names may repeat;
the identity is the UUID pair, not the name.

If a Target registered before this serialization already holds two deployments
for one application identity, every planning command on that Target fails
closed and names both colliding deployment UUIDs. The tolerant
`baha app list` still shows the twins; retire each one by UUID:

```bash
baha app destroy --uuid <deployment-uuid> --yes
```

Then verify the Target again with `baha app list`. Commands addressed by
`--uuid` keep working on such a Target; name-addressed commands do not, until
the duplicate is retired. The `(application_id, environment)` pair is the
system identity per ADR 0017.

Two selectors are often confused here, and they are not interchangeable.
`--uuid <deployment-id>` is the **identity** selector: it names the deployment
by its UUID and is understood only by the commands that read a deployment by
identity — `baha app show`, `baha app status`, `baha app plan`, and
`baha app destroy`. Each of them accepts `--uuid` without a positional `NAME`:
the selector is stripped before the one-`NAME` limit and routes directly at the
registry seam, so `baha app status --uuid <deployment-id>` (with no `NAME`)
resolves the exact deployment by its identity. `-e/--environment` is **not** an
identity selector: it sets or overrides the environment *value*, and it never
disambiguates a name-colliding deployment the way a UUID does. `baha app plan`
is the exception on a remote Target, where it refuses `--uuid` and asks for the
registered `NAME` instead. The identity selector is **not** part of `baha app env`:
that command selects its deployment by `NAME` alone (plus `--format`/`--reveal`/`--path`) and
rejects both `--uuid` and `-e/--environment` in its own parser (`unknown option`),
so neither selector is executable for `app env`; inside a duplicate situation `app env`
needs a distinct `NAME` (a different environment or a retired twin), not a
`--uuid` flag.

# Core workflow commands

These commands form the shortest normal BaseHarbor workflow.

| Command | Purpose | Safety |
| --- | --- | --- |
| `baha init` | Initialize or adopt the current repository, including required Core setup | Mutating |
| `baha up` | Start/reuse BaseHarbor and converge the current application | Mutating |
| `baha down` | Stop the current application, or the control plane outside a repository | Mutating |
| `baha status` | Show application status in a repository, otherwise control-plane status | Read-only |
| `baha plan` | Show the application plan | Read-only |
| `baha doctor` | Diagnose the current application/control plane | Read-only by default |
| `baha destroy` | Remove BaseHarbor-managed resources/state | Destructive |
| `baha update` | Update BaseHarbor and Core providers through the supported update path | Mutating |
| `baha version` | Show build version | Read-only |

## `baha up`

Use `baha up` as the normal convergence command.

Fresh targets use one native PostgreSQL server and one OpenBao server. Stable endpoints, administration and bootstrap services remain separate; the standard startup plan contains seven services, not a hidden HA cluster. OpenBao keeps authoritative PostgreSQL storage with native TLS. Memory planning counts the selected topology and reports estimates, not measured usage.

```bash
# Default single-server control plane outside an application repository
baha up --control-plane-only --yes
# Explicit HA on a separate fresh target
baha up --control-plane-only --ha --yes
```

Repository `ha: true` requests HA when creating its initial control plane. A retained target keeps its recorded topology; requesting HA on an existing single-server target fails before mutation. Use a separate target or deliberately destroy and recreate it. There is no pre-freeze state migration or legacy topology support. `status` and `doctor` report actual members and the effective availability guarantee.

Inside an application repository it resolves the effective Target, environment, provider placement and required input, then converges the application.

Outside an application repository it keeps the control-plane-only behavior.

Important modes include:

- `--yes` for deterministic approval where supported;
- `--no-input` / `--non-interactive` for automation;
- `--control-plane-only` for explicit operator/CI use;
- `--target NAME` to override Target resolution.

`baha up` is fail-closed: validation and preflight are expected before runtime mutation.

## `baha status`

Inside a repository, `baha status` is the shorthand for application status and can return structured JSON.

Outside a repository it reports control-plane status.

## `baha doctor`

Use `baha doctor` when a command fails or readiness is unclear.

Inside an application repository it uses the same application doctor semantics as `baha app doctor`.

`--fix` is an explicit mutating mode and cannot be treated as read-only automation.

## `baha destroy`

Inside a repository, `baha destroy` removes the owned application after confirmation. Outside a repository, it shows the ownership-safe destruction scope for the effective Target.

`baha destroy --all` is the explicit installation-cleanup path and removes BaseHarbor-owned installation state while preserving application source repositories and external infrastructure.

## Example: start, inspect and stop an order API

Inside a scaffold created with `baha new application orders-api --stack go --http --sql`, with a configured runtime Target:

```bash
baha plan -e dev
baha up -e dev
baha status -o json
baha doctor
baha down
```

The plan previews deployment; `up` converges it; status reports its result. `baha down` stops the application's runtime while preserving persistent data. Use `baha up` to resume it. Inside the repository, `baha down` stops the application; outside a repository it stops the selected control plane.

## Destroy inventory and preserved recovery material

```bash
baha destroy --all -o json
baha destroy --all --yes -o json
```

The plan names owned containers, networks and volumes, including older resources from the same target. Creation time alone never grants ownership. Runtime cleanup verifies remaining resources before deleting installation state. External recovery files are listed as `PRESERVED` and are never read for the report or deleted automatically. Decide separately whether the recovery copy is still needed before removing it.

JSON and the MCP destroy operations return `resources`, `preserved` and `results` from the same lifecycle. A successful container cleanup removes its unshared anonymous volumes; declared external repository volumes remain preserved.

## Provider update and point recovery

`baha update --check` inspects a published release without changing providers. A supported update requires `--yes`, immutable image identities and verified recovery points. v0.4.24 remains Unreleased; these commands do not publish a candidate.

`baha update --recover --version VERSION --yes` explicitly restores the owned HA PostgreSQL and DCS to that update’s verified backup point and retains displaced volumes. It does not replace the CLI. Transactions after the backup point are lost; an update failure never triggers this rewind automatically. Unknown commit/fencing states require reconciliation. See the [provider update contract](../spec/core-provider-update-v1.md) for topology and version boundaries.

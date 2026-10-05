# Core workflow commands

These commands form the shortest normal BaseHarbor workflow.

| Command | Purpose | Safety |
| --- | --- | --- |
| `baha init` | Explain initialization paths | Read-only |
| `baha up` | Start/reuse BaseHarbor and converge the current application | Mutating |
| `baha down` | Stop the local BaseHarbor control plane | Mutating |
| `baha status` | Show application status in a repository, otherwise control-plane status | Read-only |
| `baha plan` | Show the application plan | Read-only |
| `baha doctor` | Diagnose the current application/control plane | Read-only by default |
| `baha destroy` | Remove BaseHarbor-managed resources/state | Destructive |
| `baha update` | Update BaseHarbor/application state through the supported update path | Mutating |
| `baha version` | Show build version | Read-only |

## `baha up`

Use `baha up` as the normal convergence command.

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

Without `--all`, BaseHarbor shows the ownership-safe destruction scope for the effective Target.

`baha destroy --all` is the explicit installation-cleanup path and removes BaseHarbor-owned installation state while preserving application source repositories and external infrastructure.

## Example: start, inspect and stop an order API

Inside a scaffold created with `baha app new orders-api --stack go --http --sql`, with a configured runtime Target:

```bash
baha plan -e dev
baha up -e dev
baha status -o json
baha doctor
baha app down
```

The plan previews deployment; `up` converges it; status reports its result. `app down` stops the application's runtime while preserving persistent data. Use `baha up` to resume it. Top-level `baha down` stops the local control plane and is a different scope.

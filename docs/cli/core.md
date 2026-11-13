# Core workflow commands

Initialize the first tenant explicitly with `baha login -e dev --init-tenant` on
the locally owned Core. `baha up` never assigns the first tenant owner. See
[Security and trust commands](security-trust.md).

## Connect a developer workstation

On the Core VM, outside an application repository, run `baha up` and then
`baha onboard`. `baha up` starts the Core providers and checks for ORAS before
changing provider state. If ORAS is missing, Baha explains its OCI digest and
artifact-verification role and asks before installing a checksum-verified
user-local copy. Before starting Core, it resolves and pulls the configured
BaseHarbor Runtime image by immutable digest, so the first application deploy
does not discover a missing image. `baha onboard` waits for Core readiness, configures the
managed LAN HTTPS endpoint and writes a private, one-use JSON invitation. Do
not start a separate `baha serve` process.

Transfer the invitation securely. On the developer workstation, run
`baha onboard --import FILE`, accept the suggested workstation alias such as
`dev-docker-1` or pass `--target-name NAME`, then explicitly select it with
`baha use NAME`. The alias does not rename the Core Target. The workstation's
own local Core Target remains available. Subsequent application lifecycle
commands run on the workstation through the selected Core Target. See the normative
[Core-to-workstation onboarding flow](../spec/zero-friction-workstation-onboarding-v0427.md).

This device enrollment is separate from interactive human-operator OIDC login.
See [Security and trust commands](security-trust.md) for operator authentication.

These commands form the shortest normal BaseHarbor workflow.

| Command | Purpose | Safety |
| --- | --- | --- |
| `baha init` | Initialize or adopt the current repository, including required Core setup | Mutating |
| `baha up` | Start/reuse BaseHarbor and converge the current application | Mutating |
| `baha down` | Stop the current application, or the control plane outside a repository | Mutating |
| `baha status` | Show application status in a repository, otherwise control-plane status | Read-only |
| `baha plan` | Show the application plan | Read-only |
| `baha doctor` | Diagnose the current application/control plane | Read-only by default |
| `baha destroy` | Remove the complete installation across configured Targets | Destructive |
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
For a remote Target, Doctor compares the developer CLI and Core build versions.
A mismatch fails the version check and prints the exact `baha update --version
<CLI_VERSION> --yes` command for the Core host. Development builds have no
published release command, so Doctor says to install the matching build there.

## `baha destroy`

`baha destroy` always removes the BaseHarbor installation across all configured
Targets, regardless of the current directory. It removes registered applications,
owned providers and runtime resources, Core services, BaseHarbor configuration,
and local state. Application source repositories and external infrastructure are
preserved. Use `baha app destroy NAME` to remove just one application while
keeping the installation. `--all` remains accepted as a compatibility alias.

The command displays the concrete impact plan and requires its consequence
acknowledgements; `--yes` does not skip them. Owned data is selected for permanent
deletion and requires a separate data-deletion consent. External recovery files
and resources whose ownership cannot be verified are preserved. If a Target
cannot be inventoried, destruction stops before mutation and retains ownership
state so the issue can be fixed and retried.

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

## Destroy inventory and recovery material

```bash
baha destroy -o json
baha destroy --yes -o json
```

The plan names owned containers, networks and volumes, including older resources from the same target. Creation time alone never grants ownership. Runtime cleanup verifies remaining resources before deleting installation state. Existing OpenBao recovery files are listed for deletion and are removed only after the acknowledged permanent installation destroy. The file contents are never read into the report. Keep a separate backup if the recovery material is still needed.

If one Target runtime cannot be reached, BaseHarbor leaves that runtime untouched and reports it as `PRESERVED`, while continuing cleanup on reachable Targets. Local configuration and ownership state are removed after the acknowledged workstation destroy; remote Target services are never implied to have been removed when they could not be inspected.

JSON and the MCP destroy operations return `resources`, `preserved` and `results` from the same lifecycle. A successful container cleanup removes its unshared anonymous volumes; declared external repository volumes remain preserved.

When installation cleanup removes one deployment, the remaining consumer plan
can change. Before the next deployment, BaseHarbor displays that current remainder
and requires two new confirmations plus separate data-deletion consent. In JSON,
`approval_required` / `impact-confirmation-required` includes the remaining plan
and `continuation` with the previous plan ID and completed deployment IDs.
`destroyed: false` and `results` preserve actual progress. Confirm the new plan to
continue; do not replay a failed mutation or reuse the earlier acknowledgement.

## Provider update and point recovery

`baha update --check` inspects a published release without changing providers. A supported update requires `--yes`, immutable image identities and verified recovery points.

`baha update --recover --version VERSION --yes` explicitly restores the owned HA PostgreSQL and DCS to that update’s verified backup point and retains displaced volumes. It does not replace the CLI. Transactions after the backup point are lost; an update failure never triggers this rewind automatically. Unknown commit/fencing states require reconciliation. See the [provider update contract](../spec/core-provider-update-v1.md) for topology and version boundaries.

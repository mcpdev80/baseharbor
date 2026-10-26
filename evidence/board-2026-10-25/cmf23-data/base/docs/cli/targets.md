# Target commands

Targets select where and how BaseHarbor realizes a deployment.

```text
baha target
baha target list
baha target show
baha target create
baha target delete
baha target activate
baha target deactivate
```

## Target resolution

A Target can be selected through:

1. explicit global `--target NAME`;
2. activated shell Target;
3. organization/platform defaults;
4. configured default Target.

Portable Application Intent does not contain Docker/Podman/Kubernetes-specific Target mechanics.

## Important distinction

Target selection is deployment context. It is not a capability provider choice and not a delivery-provider choice.

```text
runtime != capability != delivery
```

Use `baha target show` to inspect the effective Target and provenance before mutation.

## Detach and restore a Target definition

`baha target delete NAME` removes the selected configuration entry after **Achtung**
and two explicit acknowledgements of its current impact plan. Existing deployments,
provider bindings, native processes, volumes, data and access definitions remain.
The command preserves a protected copy of the original Target and access references
and returns its `recovery_file`. Retention does not establish working service access,
certificate renewal or authentication continuity; unknown consumer results remain partial.

If the removed entry supplied automatic Target selection, that selection remains
blocked. BaseHarbor does not silently choose another runtime or access policy.
Restore the original definition with its displayed recovery file, then explicitly
select the restored Target when appropriate:

```bash
baha target delete docker-dev -o json
baha target create docker-dev --from-recovery /path/from/recovery_file -o json
baha target activate docker-dev
```

JSON invocation returns the current structured plan when acknowledgement is missing.
Use `BASEHARBOR_PROVIDER_CHANGE_CONSENT` in a trusted CLI invocation, or the MCP
`provider_change_consent` object, with that plan's ID and both independent stages.
Restoration also requires its own current plan. `--yes` does not grant these stages.
Restoration cannot override the original runtime/access/authentication references,
overwrite an existing Target or access owner, or silently move the retained data
to another data root. It does not activate the restored route automatically.
Partial preparation retains protected recovery for inspection and a fresh retry.

## Runtime and Target Access are separate axes

A Target selects both:

```text
Target
├── Runtime Provider
└── Target Access Provider
```

They do not have to be the same provider.

Local Docker example:

```bash
baha target create docker-dev \
  --runtime-provider docker \
  --access local-docker \
  --access-provider local \
  --reference local
```

Remote Docker/Podman hosts use the Node Connector enrollment workflow rather than hand-authoring access records:

```bash
baha node add node-a
baha node list
baha node status node-a
```

`baha node add` is guided on a terminal. It creates the remote Target and writes one owner-only enrollment bundle containing the short-lived one-use authorization. Copy that file to the remote host and run:

```bash
baha node connect /path/to/node-a.json
```

The remote command consumes the bundle through the existing Connector bootstrap API, keeps the private key local, installs the rootless user service, and establishes outbound mTLS. Token and nonce values are never accepted as process arguments or written to logs. For automation, provide the explicit `node add` options shown by `--help`; non-TTY mode never prompts.

To disconnect a node, inspect its state and review both explicit acknowledgements of the concrete current plan:

```bash
baha node status node-a
baha node disconnect node-a --yes
```

Disconnect revokes Connector admission and submits its current certificate for issuer revocation. It preserves the Target definition, node registration, provider bindings and deployment recovery owners. Native workloads, provider data and foreign infrastructure are not destroyed; retained material does not prove continued service access.

Review the current concrete plan before confirming. `--yes` and MCP `approval=true` provide ordinary operation approval only. Unknown remote consequences and loss of control access require **Achtung** and two independently explicit acknowledgements of that plan. JSON clients supply `BASEHARBOR_PROVIDER_CHANGE_CONSENT` for a trusted CLI invocation; MCP uses `provider_change_consent`. Missing consent returns the structured plan without revocation. Core HTTP callers obtain the authoritative plan through `POST /api/v1/connectors/disconnect-plan` and acknowledge its current scope ID through `POST /api/v1/connectors/disconnect`; the plan route never executes supplied consent.

The result separates admission revocation, certificate revocation, retained owners and unverified remote data/access. A partial issuer or recovery failure remains partial and retains the owner needed for a newly reviewed retry. CLI/MCP aggregate known local recovery owners with the authenticated Core plan; direct Core HTTP covers its own authoritative owners. Reconnection needs existing authorized enrollment/renewal and never bypasses mTLS.

Kubernetes/OpenShift can use native API access without a Node Connector:

```text
runtime = kubernetes|openshift
access.provider = native-api
```

The legacy `--provider` flag remains an alias for `--runtime-provider`.
Non-local access must declare `--access-provider` explicitly.

## Example: inspect deployment context before applying

After creating `docker-dev` as above, with a working Docker runtime:

```bash
baha target list
baha target show docker-dev -o json
baha --target docker-dev plan -e dev
baha --target docker-dev up -e dev
```

Run the last two commands inside an application repository. Confirm `runtime_provider`, access settings and the plan's environment before applying. An explicit `--target` applies to that invocation without relying on the shell's active Target. Target registration alone does not install Docker or validate every provider capability.

## Local Docker engine selection

Rootless Docker is the default. The provider resolves and verifies one Unix socket before any lifecycle operation, then uses `docker --host SOCKET` for Compose, inspection, execution, volume recovery and cleanup. A failed or rootful selection never falls back to System Docker.

For a fresh Target the precedence is explicit Target endpoint or context, `DOCKER_CONTEXT`, `DOCKER_HOST`, then the active Docker context. With the `default` context and no explicit rootful intent, BaseHarbor selects `/run/user/<uid>/docker.sock`. An active rootless context is resolved to its actual socket. Application environment variables cannot override this engine selection.

```bash
baha target create local-dev --runtime-provider docker --access local-docker --reference local --docker-context rootless --default
baha target show local-dev --json
baha --target local-dev status --json
baha --target local-dev doctor --json
```

These read results include `docker_engine.endpoint`, `mode`, `daemon_id`, `selection_origin` and `verified`. Human output exposes the socket and mode too. If Rootless Docker is unavailable, start that daemon or correct the explicitly selected Target; BaseHarbor does not use `/var/run/docker.sock` instead.

The first owned lifecycle operation records a protected `runtime/docker-engine.json` binding under the Target state directory. Subsequent CLI and MCP operations use that binding even when their ambient Docker contexts differ. A changed endpoint, mode or daemon identity refuses mutation, duplicate bootstrap and cleanup. Existing Core state without a binding must have provable owned resources on the selected engine. When System Docker is reachable, an existing same-Target project there also blocks a fresh duplicate bootstrap on Rootless Docker.

Existing rootful installations remain intact. Maintaining one requires explicit Target configuration with `docker-endpoint: unix:///var/run/docker.sock` and `docker-mode: rootful`; these values also have `--docker-endpoint` and `--docker-mode` Target-create options and `docker_endpoint`/`docker_mode` machine/MCP fields. This is explicit maintenance intent, not a migration. Do not remove protected state or switch a recorded Target's socket to move existing data. Use a separate Target for a genuinely separate installation.

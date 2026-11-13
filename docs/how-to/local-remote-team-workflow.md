# Local, remote and team development

Use the same repository workflow whether the selected BaseHarbor Core is on your workstation or on a remote host. A management connection identifies how the CLI reaches a Core; a Target identifies where that Core deploys an application; the application environment selects that deployment's configuration and state.

## Local development

From an existing application repository, initialize or adopt its contract once, inspect the plan, then converge the selected environment:

```bash
baha init
baha plan -e dev
baha up -e dev
baha status -e dev
```

Ordinary trusted-local development does not require `baha login`. `baha init` records the repository-owned application contract; `baha up` performs the lifecycle. Neither an environment name nor a Target name is a substitute for the other. See [application commands](../cli/applications.md) and [environment selection](environments.md).

## One remote Core

On the machine that owns the Core, prepare it and issue one private workstation invitation:

```bash
baha up
baha onboard --invite
```

Transfer the invitation securely. On the developer workstation, redeem it and select the resulting connection:

```bash
baha onboard --import /path/to/invitation.json
baha use <connection>
baha connections
baha target list
baha target show
```

Then work from the application repository using the selected remote Target:

```bash
baha init
baha plan -e dev
baha up -e dev
baha status -e dev
```

Enrollment attaches this workstation to the Core and creates a device identity. It does not grant a human operator role. The connection alias does not rename a Core Target, and a failed remote operation never falls back to a local runtime. Keep the local Core and remote connection distinct when selecting a Target. See the [workstation enrollment contract](../spec/zero-friction-workstation-onboarding-v0427.md) and [Target commands](../cli/targets.md).

## Shared team access

Create a person and their first workstation on the owning Core:

```bash
baha onboard --invite
```

For another workstation belonging to an existing person, issue a separate invitation with `baha workstation new`. Each device has its own revocable identity. Manage devices with `baha workstation list` and `baha workstation revoke NAME`; manage people with `baha user list` and the explicit `baha user offboard LOGIN` flow.

After the person signs in where policy requires it, assign the intended access profile and Targets through the guided command:

```bash
baha access assign PERSON
baha whoami
```

Access assignment is scoped to the selected installation and Targets. A shared organization configuration can provide versioned defaults, but it is not a grant, a synchronized global authority or a copy of live deployment state. See [security and trust commands](../cli/security-trust.md) and [organization configuration](../explanation/organization-configuration.md).

## Profiles, credentials and recovery

The one-use workstation invitation is credential-bearing device enrollment. It is not a general profile-sync bundle. For a selected application's eligible local developer credentials, the CLI currently provides an explicit encrypted export:

```bash
baha app secure-profile export --password-file /path/to/owner-only-password
```

There is no `baha app secure-profile import` command. Do not copy remote provider secrets or treat a connection profile as permission to use a Target. Use native Git, SSH, kubeconfig and cloud credential chains when they already work; use verified system or explicitly configured CA trust for TLS. Never work around a trust failure by disabling certificate verification. If the exact image is already in the destination's native image store, that store can be used without a registry. When artifacts must move between hosts, use an existing OCI-compatible registry and its configured authentication/trust; BaseHarbor does not silently install a registry or make GitOps/CI mandatory for local development. See [security and trust](../cli/security-trust.md), the [delivery contract](../spec/delivery-provider-v1.md) and [platform support](../reference/platform-support.md).

Application recovery is separate from workstation/profile portability:

```bash
baha app backup
baha app restore
```

Backup does not prove that a new application schema is compatible with an older image. The application and its existing migration tool own schema changes and compatibility evidence; BaseHarbor does not execute a generic migration engine. See [backup and restore](backup-restore.md) and [candidate contract](../spec/candidate-contract-v1.md).

## Exact versions and promotion boundary

Readable release or component versions help people identify a choice. Inspect the recorded view with `baha app version`; use `baha app versions --all-targets` to compare deployments across Targets. Wanted, applied and observed references are separate recorded dimensions. A missing dimension is shown as unclear; an observation that differs from the last applied value is stale. Component rows show image repository/tag and observed image digest when available. Use `baha app workspace resolve -o json` to inspect component-to-repository provenance.

The immutable applied identity must resolve to a Git commit or OCI digest; a mutable Git branch/tag or image tag alone is not an exact candidate. A missing Git revision stays unknown, and OCI annotations alone do not prove Git provenance. Each record is scoped to an application, Target and environment, with intended/applied/observed values kept independent. `stage` is a readable label, not another deployment identity or policy boundary. Exact-candidate consumption does not build or migrate. The generalized cross-environment `baha promote` operation is deferred to v0.4.28 and is not a supported v0.4.27 command. `baha app update` updates supported application source; it is not promotion or database rollback. See the [candidate contract](../spec/candidate-contract-v1.md), [application commands](../cli/applications.md) and [PostgreSQL migration boundary](../reference/postgresql-and-migrations.md).

### Further reading

- [First application](../tutorials/getting-started.md)
- [Remote workstation details](remote-workstation.md)
- [Target concepts](../explanation/targets.md)
- [Machine-readable command and operation mapping](../reference/cli-machine-coverage.md)

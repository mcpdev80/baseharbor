# Remote workstation enrollment

For the end-to-end local, remote and team path, see [Local, remote and team development](local-remote-team-workflow.md). This page records the Core-specific invitation details. Workstation enrollment is separate from Node Connector enrollment and human operator login.

## Issue an invitation on the Core host

On the machine that owns the Core, run:

```bash
baha up
baha onboard --invite
```

The command creates a private, one-use device invitation. Transfer it securely and keep its contents private. For another device belonging to an existing person, use `baha workstation new` on the Core host.

## Redeem it on the developer workstation

```bash
baha onboard --import /path/to/invitation.json
baha use NAME
```

BaseHarbor verifies the Core and invitation, creates the device private key locally, and registers the connection. The invitation does not grant a human operator role. The connection alias is distinct from the Core Target. Select and inspect the deployment Target with `baha target list` and `baha target show`. See [security and trust commands](../cli/security-trust.md) for CA trust, revocation and offboarding details.

Continue from the application repository using [the canonical local, remote and team workflow](local-remote-team-workflow.md).

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
baha target list
baha use <workstation-target>
baha target show <workstation-target>
baha status
baha doctor
```

The import output names the new Workstation Target; use that exact name with `baha use` and `baha target show`. `baha target list` shows configured Targets, while `baha status` and `baha doctor` verify the selected Core and its version relationship. BaseHarbor verifies the Core and invitation, creates the device private key locally, and registers the device connection. This does not grant a human operator role; when policy requires an operator session, use `baha login -e dev` as well. The Workstation Target is distinct from the Core Target. See [security and trust commands](../cli/security-trust.md) for CA trust, revocation and offboarding details.

Continue from the application repository using [the canonical local, remote and team workflow](local-remote-team-workflow.md).

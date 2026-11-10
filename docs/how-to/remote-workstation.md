# Connect a workstation to BaseHarbor Core

Run BaseHarbor Core on one machine and develop on another. The workstation connects directly to Core; Node Connector is not needed for this connection.

## On the Core machine

Install BaseHarbor, then run:

```bash
baha up
baha onboard
```

`baha onboard` creates a short-lived, one-use invitation file for a workstation. Transfer the file securely to the developer's machine. Keep its contents private.

## On the developer workstation

Install the `baha` CLI and import the invitation:

```bash
baha onboard --import /path/to/invitation.json
```

BaseHarbor verifies the Core, enrolls this workstation using a locally generated private key, and registers the remote Target.

To check the selected Target:

```bash
baha target show
```

## Run an application

From the application repository:

```bash
baha inspect .
baha init
baha plan
baha up
baha status
```

For advanced options and troubleshooting, see [Target commands](../cli/targets.md) and [Security and trust](../cli/security-trust.md).

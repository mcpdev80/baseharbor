# Connect a developer workstation to BaseHarbor Core

BaseHarbor separates the machine that operates Core from the workstation used to develop applications. Core remains the authority for state, security and deployment decisions.

## On the Core host

Install BaseHarbor and initialize the control plane:

```bash
baha up --control-plane-only
baha onboard --init-tenant
```

The initial operator follows the protected login and identity flow. Set up the Core's reachable management endpoint using the normal TLS and Core configuration.

## On the workstation

Install the BaseHarbor CLI and start the guided connection flow:

```bash
baha onboard
baha target list
baha target show
```

The onboarding wizard discovers existing connections and Target choices, reuses a trusted connection when possible, and avoids reinstalling Core on a workstation that is joining an existing installation.

For a known registered Target, the explicit form is `baha onboard --target NAME`. The wizard may consume a trusted enrollment bundle; do not substitute a manually fabricated TLS identity. The private workstation identity belongs on the workstation.

## Deploy an application

Once the workstation has a verified Target and an application repository:

```bash
baha inspect .
baha init
baha plan
baha up
baha status
```

The optional Node Connector is for separate remote Docker/Podman runtime hosts. It is not the link between the workstation and Core.

See [Targets](../cli/targets.md), [Security and trust](../cli/security-trust.md), and [Getting started](../tutorials/getting-started.md).

# Connect a workstation to BaseHarbor Core

Use this guide when Core runs on one machine and you develop on another. The workstation connects directly to Core; Node Connector is only needed for separate remote runtime hosts.

## 1. Set up Core

On the Core host, install BaseHarbor and run:

```bash
baha up --control-plane-only
baha onboard --init-tenant
```

Complete the operator login and configure a reachable, TLS-protected management endpoint.

## 2. Connect the workstation

Install `baha` on the workstation, then run:

```bash
baha onboard
baha target show
```

Follow the prompts to connect to the existing Core. A trusted enrollment bundle may be required. BaseHarbor reuses an existing verified connection.

## 3. Start an application

From your application repository:

```bash
baha inspect .
baha init
baha plan
baha up
baha status
```

For available options and recovery procedures, see [Target commands](../cli/targets.md) and [Security and trust](../cli/security-trust.md).

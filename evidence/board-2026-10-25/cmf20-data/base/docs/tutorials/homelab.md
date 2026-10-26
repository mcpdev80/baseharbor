# Homelab: Core + Console + remote Podman

A minimal setup:

```text
Developer PC
  -> BaseHarbor Core + Console
       -> Node Connector
            -> remote Podman node
```

## 1. Core VM

Install BaseHarbor:

```bash
curl -fsSL https://raw.githubusercontent.com/mcpdev80/baseharbor/main/scripts/install.sh | bash
baha up --control-plane-only
baha status
```

This starts the Core: PostgreSQL, OpenBao and Keycloak.

Optional: run the [BaseHarbor Console](https://github.com/mcpdev80/baseharbor-console) next to this Core.

## 2. Developer PC

Install `baha` and connect to the Core using the configured HTTPS/OIDC access.

Check:

```bash
baha target list
baha status
```

For AI/MCP use:

```bash
baha mcp serve
```

## 3. Remote Podman VM

Install rootless Podman and the [BaseHarbor Node Connector](https://github.com/mcpdev80/baseharbor-node-connector).

Enroll the Connector with the Core, then run it with the Core address and Target identity. The Connector opens an outbound mTLS session; no inbound management port is required.

Create the Target on the Core side:

```bash
baha target create app-node-01 \
  --runtime-provider podman \
  --access node-01 \
  --access-provider baseharbor-node-connector \
  --reference node-01
```

## 4. Use it

Inside an application repository:

```bash
baha --target app-node-01 plan
baha --target app-node-01 up
baha --target app-node-01 status
```

The same Target can be operated from the Console.

## AI prompt

Give this to an AI client connected to BaseHarbor MCP:

```text
Work only through the BaseHarbor MCP tools.

First inspect the current Core, Target, application and status.
Use read-only tools first and create a plan before mutation.

Target: app-node-01

Do not access Docker, Podman or the Node Connector directly.
Do not bypass BaseHarbor policy, ownership, preflight or verification.
Do not expose secrets.

If a mutation is needed, explain the planned change briefly and wait for approval.
After each mutation, verify status/doctor/evidence and report the result.
```

BaseHarbor Core stays authoritative. Console and MCP are clients; the Node Connector is only the bounded remote Target Access path.

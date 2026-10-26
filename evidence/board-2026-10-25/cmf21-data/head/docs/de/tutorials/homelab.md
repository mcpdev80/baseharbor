# Homelab: Core + Console + Remote Podman

Minimaler Aufbau:

```text
Developer PC
  -> BaseHarbor Core + Console
       -> Node Connector
            -> remote Podman node
```

## 1. Core-VM

BaseHarbor installieren:

```bash
curl -fsSL https://raw.githubusercontent.com/mcpdev80/baseharbor/main/scripts/install.sh | bash
baha up --control-plane-only
baha status
```

Damit läuft der Core: PostgreSQL, OpenBao und Keycloak.

Optional: die [BaseHarbor Console](https://github.com/mcpdev80/baseharbor-console) neben diesem Core betreiben.

## 2. Entwicklungsrechner

`baha` installieren und über den konfigurierten HTTPS/OIDC-Zugang mit dem Core verbinden.

Prüfen:

```bash
baha target list
baha status
```

Für KI/MCP:

```bash
baha mcp serve
```

## 3. Remote-Podman-VM

Rootless Podman und den [BaseHarbor Node Connector](https://github.com/mcpdev80/baseharbor-node-connector) installieren.

Den Connector am Core einschreiben und mit Core-Adresse und Target-Identität starten. Der Connector baut selbst eine ausgehende mTLS-Verbindung auf; ein eingehender Management-Port ist nicht nötig.

Target im Core anlegen:

```bash
baha target create app-node-01 \
  --runtime-provider podman \
  --access node-01 \
  --access-provider baseharbor-node-connector \
  --reference node-01
```

## 4. Benutzen

Im Application-Repository:

```bash
baha --target app-node-01 plan
baha --target app-node-01 up
baha --target app-node-01 status
```

Dasselbe Target kann über die Console bedient werden.

## KI-Prompt

Diesen Prompt einer KI mit verbundenem BaseHarbor-MCP geben:

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

Der BaseHarbor Core bleibt autoritativ. Console und MCP sind Clients; der Node Connector ist nur der begrenzte Remote-Target-Access-Pfad.

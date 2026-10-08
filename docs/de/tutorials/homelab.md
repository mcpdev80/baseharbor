# Homelab: Core + Console + Remote Podman

Minimaler Aufbau:

```text
Entwicklungsrechner
  -> BaseHarbor Core + Console
       -> Node Connector
            -> Remote-Podman-VM
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
Arbeite ausschließlich über die BaseHarbor-MCP-Tools.

Prüfe zuerst Core, Target, Application und Status.
Nutze zuerst Read-only-Tools und erstelle vor Änderungen einen Plan.

Target: app-node-01

Greife niemals direkt auf Docker, Podman oder den Node Connector zu.
Umgehe niemals BaseHarbor-Policy, Ownership, Preflight oder Verifikation.
Gib keine Secrets aus.

Wenn eine Änderung nötig ist, erkläre sie kurz und warte auf Freigabe.
Prüfe nach jeder Änderung Status/Doctor/Evidence und melde das Ergebnis.
```

Der BaseHarbor Core bleibt autoritativ. Console und MCP sind Clients; der Node Connector ist nur der begrenzte Remote-Target-Access-Pfad.

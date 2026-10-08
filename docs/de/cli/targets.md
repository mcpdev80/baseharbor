# Target-Befehle

Targets wählen Deployment-Ziel, Runtime und Zugriffsweg. Sie sind keine Capability- oder Delivery-Provider-Auswahl.

`baha target`, `list`, `show`, `create`, `delete`, `activate` und `deactivate` bilden den Target-Bereich.

Die Auflösung verwendet zuerst globales `--target NAME`, dann das aktivierte Shell-Target, Organisations-/Plattform-Defaults und das konfigurierte Standardtarget. Vor Mutation zeigt `baha target show` Ziel und Herkunft.

## Lokaler Docker-Zugriff

```bash
baha target create docker-dev --runtime-provider docker --access local-docker --access-provider local --reference local
baha target show docker-dev -o json
```

Im Application-Repository mit funktionierender Runtime:

```bash
baha --target docker-dev plan -e dev
baha --target docker-dev up -e dev
```

Eine Registrierung installiert keine Runtime und beweist nicht alle Capabilities.

## Remote-Zugriff

Runtime Provider und Target Access Provider sind getrennt. Ein Connector-Target kann registriert werden mit:

```bash
baha target create edge-a --runtime-provider docker --access node-a --access-provider baseharbor-node-connector --reference node-a
```

Registrierung allein stellt keine authentifizierte Verbindung her. Der vollständige Remote-Application-Lifecycle wird für v0.4.23 noch qualifiziert; ein Remote-Target darf niemals ersatzweise lokal ausgeführt werden. Native Kubernetes-/OpenShift-API-Zugriffe benötigen nicht grundsätzlich einen Connector; diese Runtime-Realisierungen folgen später.

`--provider` bleibt Alias von `--runtime-provider`; nicht lokaler Zugriff braucht einen ausdrücklichen `--access-provider`.

Weiter: [Target-Konzept](../explanation/targets.md), [exakte Befehle (EN)](https://mcpdev80.github.io/baseharbor/cli/targets/).

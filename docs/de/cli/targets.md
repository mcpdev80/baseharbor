# Target-Befehle

Targets wählen Deployment-Ziel, Runtime und Zugriffsweg. Sie sind keine Capability- oder Delivery-Provider-Auswahl.

`baha target`, `list`, `show`, `create`, `delete`, `activate` und `deactivate` bilden den Target-Bereich.

Die Auflösung verwendet zuerst globales `--target NAME`, dann das aktivierte Shell-Target, Organisations-/Plattform-Defaults und das konfigurierte Standardtarget. Vor Mutation zeigt `baha target show` Ziel und Herkunft.

## Lokaler Docker-Zugriff

```text
baha target
baha target list
baha target show
baha target create
baha target delete
baha target activate
baha target deactivate
```

Im Application-Repository mit funktionierender Runtime:

```text
runtime != capability != delivery
```

Eine Registrierung installiert keine Runtime und beweist nicht alle Capabilities.

## Remote-Zugriff

Runtime Provider und Target Access Provider sind getrennt:

```text
Target
├── Runtime Provider
└── Target Access Provider
```

Entfernte Docker-/Podman-Hosts werden nicht mehr durch manuelles Erstellen des Connector-Access-Eintrags aufgenommen. Auf dem Core-System wird der geführte Node-Workflow verwendet:

```bash
baha node add node-a
baha node list
baha node status node-a
```

`baha node add` erzeugt das Remote-Target und genau eine owner-only Enrollment-Datei mit kurzlebiger Einmal-Autorisierung. Diese Datei wird auf den Remote-Host kopiert und dort konsumiert:

```bash
baha node connect /pfad/zu/node-a.json
```

Der Connector erzeugt seinen privaten Schlüssel ausschließlich lokal, verwendet die bestehende Bootstrap-API und baut anschließend die outbound-initiierte mTLS-Verbindung als rootless User-Service auf. Token und Nonce werden weder als Prozessargumente noch in Logs ausgegeben. Non-TTY-Automation verwendet die expliziten Optionen aus `baha node add --help` und fragt niemals interaktiv nach.

Zum Entfernen wird zuerst der Zustand geprüft und anschließend die Sperrung ausdrücklich bestätigt:

```bash
baha node status node-a
baha node disconnect node-a --yes
```

Der Core widerruft die Connector-Zulassung, bevor das leere Target entfernt wird. Ein altes, weiterhin CA-vertrauenswürdiges Zertifikat kann sich dadurch nicht unbemerkt erneut verbinden. Native Kubernetes-/OpenShift-Zugriffe verwenden weiterhin normalerweise ihre nativen APIs und benötigen dafür keinen Node Connector.

`--provider` bleibt Alias von `--runtime-provider`; nicht lokaler Zugriff braucht einen ausdrücklichen `--access-provider`.

Weiter: [Target-Konzept](../explanation/targets.md), [exakte Befehle (EN)](https://mcpdev80.github.io/baseharbor/cli/targets/).


## Zusätzliche Befehlsbeispiele

```bash
baha target create docker-dev \
  --runtime-provider docker \
  --access local-docker \
  --access-provider local \
  --reference local
```

```text
runtime = kubernetes|openshift
access.provider = native-api
```

```bash
baha target list
baha target show docker-dev -o json
baha --target docker-dev plan -e dev
baha --target docker-dev up -e dev
```


Technische Bezeichner: `runtime_provider`.

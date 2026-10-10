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

```text
Target
├── Runtime Provider
└── Target Access Provider
```

## Remote-Zugriff

Runtime Provider und Target Access Provider sind getrennt:

```bash
baha target create docker-dev \
  --runtime-provider docker \
  --access local-docker \
  --access-provider local \
  --reference local
```

Entfernte Docker-/Podman-Hosts werden nicht mehr durch manuelles Erstellen des Connector-Access-Eintrags aufgenommen. Auf dem Core-System wird der geführte Node-Workflow verwendet:

```bash
baha node add node-a
baha node list
baha node status node-a
```

`baha node add` erzeugt das Remote-Target und genau eine owner-only Enrollment-Datei mit kurzlebiger Einmal-Autorisierung. Diese Datei wird auf den Remote-Host kopiert und dort konsumiert:

```bash
baha node connect /path/to/node-a.json
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

## Lokale Docker-Engine auswählen

Rootless-Docker ist der Standard. Vor jeder Lifecycle-Aktion löst der Provider genau einen Unix-Socket auf und verifiziert ihn. Compose, Inspektion, Exec, Volume-Recovery und Cleanup verwenden danach `docker --host SOCKET`. Eine nicht erreichbare oder unerwartet Rootful laufende Engine führt zum Abbruch; es gibt keinen Wechsel zu System-Docker.

Bei einem neuen Target gilt die Reihenfolge: expliziter Target-Endpoint oder -Kontext, `DOCKER_CONTEXT`, `DOCKER_HOST`, aktiver Docker-Kontext. Beim Kontext `default` ohne ausdrücklichen Rootful-Wunsch wählt BaseHarbor `/run/user/<uid>/docker.sock`. Ein aktiver Rootless-Kontext wird auf seinen tatsächlichen Socket aufgelöst. Anwendungsvariablen können diese Auswahl nicht überschreiben.

```bash
baha target create local-dev --runtime-provider docker --access local-docker --reference local --docker-context rootless --default
baha target show local-dev --json
baha --target local-dev status --json
baha --target local-dev doctor --json
```

Die Leseergebnisse enthalten `docker_engine.endpoint`, `mode`, `daemon_id`, `selection_origin` und `verified`. Auch die Textausgabe zeigt Socket und Modus. Ist Rootless-Docker nicht erreichbar, muss dessen Daemon gestartet oder die explizite Target-Auswahl korrigiert werden. BaseHarbor verwendet dann nicht ersatzweise `/var/run/docker.sock`.

Die erste eigene Lifecycle-Aktion speichert eine geschützte Bindung `runtime/docker-engine.json` im Target-Zustandsverzeichnis. Weitere CLI- und MCP-Aktionen verwenden diese Bindung auch bei unterschiedlichen geerbten Docker-Kontexten. Ein geänderter Endpoint, Modus oder eine andere Daemon-ID verhindert Änderungen, einen zweiten Bootstrap und Cleanup. Vorhandener Core-Zustand ohne Bindung verlangt nachweislich eigene Ressourcen auf der gewählten Engine. Ist System-Docker erreichbar, verhindern dort vorhandene Ressourcen desselben Targets ebenfalls einen zweiten Core-Bootstrap auf Rootless-Docker.

Vorhandene Rootful-Installationen bleiben erhalten. Ihre Wartung erfordert die explizite Target-Konfiguration `docker-endpoint: unix:///var/run/docker.sock` und `docker-mode: rootful`. Dafür gibt es auch die Target-create-Optionen `--docker-endpoint` und `--docker-mode` sowie die Machine-/MCP-Felder `docker_endpoint` und `docker_mode`. Das ist eine ausdrückliche Wartungsentscheidung, keine Migration. Geschützten Zustand nicht löschen und den Socket eines gebundenen Targets nicht ändern, um Daten zu verschieben. Für eine getrennte Installation ein eigenes Target verwenden.

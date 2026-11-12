# Core-Workflow

Den ersten Tenant richtest du auf dem eigenen Core ausdrücklich mit
`baha login -e dev --init-tenant` ein. `baha up` weist den ersten Tenant-Owner
nicht automatisch zu. Siehe [Befehle für Sicherheit und Vertrauen](security-trust.md).

## Entwicklungsrechner verbinden

Führe auf dem Core-Rechner außerhalb eines Anwendungsordners `baha up` und danach
`baha onboard` aus. `baha up` prüft ORAS und lädt das BaseHarbor-Runtime-Image
anhand seines unveränderlichen Digests, bevor es den Core startet. Fehlt ORAS,
erklärt Baha den Zweck und fragt vor der Installation.
`baha onboard` wartet auf den startbereiten Core, richtet den verwalteten
LAN-HTTPS-Endpunkt ein und schreibt eine private Einmal-Einladung als JSON-Datei.
Ein separater `baha serve`-Prozess ist nicht nötig.

Übertrage die Datei sicher. Führe auf dem Entwicklungsrechner
`baha onboard --import FILE` aus. Übernimm den vorgeschlagenen Target-Namen, etwa
`dev-docker-1`, oder gib `--target-name NAME` an. Wähle das Target anschließend
mit `baha use NAME` aus. Der Name ändert nicht das Core-Target; ein lokales
Target bleibt verfügbar. Anwendungsbefehle laufen danach auf dem
Entwicklungsrechner über das ausgewählte Core-Target. Der verbindliche Ablauf steht in
[Core-zu-Workstation-Onboarding](../spec/zero-friction-workstation-onboarding-v0427.md).

Die Geräteeinschreibung ist von der interaktiven OIDC-Anmeldung eines Operators
getrennt. Siehe [Befehle für Sicherheit und Vertrauen](security-trust.md).

## Core gezielt abbauen

Außerhalb eines App-Repositories entfernt ein bestätigtes `baha destroy` nur
die kanonischen PostgreSQL-/OpenBao-Container und ihre Startdefinitionen. Aktive
Verbraucher verhindern die bewusst gewählte Entscheidung nicht; die konkreten
Ausfallfolgen benötigen zwei Bestätigungen des aktuellen Plans. Volumes,
Netzwerke, geschützte Zugangsdaten/Konfiguration, Bindings, Workloads, andere
Provider und Sicherheitsrichtlinien bleiben für Wiederaufnahme oder Retry.
Erhalt beweist keine laufende Secret-Zustellung, Anmeldung oder Erneuerung.

`--delete-data` wählt zusätzlich die eigenen Core-Volumes zur Löschung aus und
erzeugt einen anderen Plan. Nach beiden Planbestätigungen ist separate
Löschzustimmung erforderlich. Erhaltene Zugangsdaten stellen gelöschte Volume-
Inhalte nicht wieder her. `--yes` ersetzt keine dieser Zustimmungen. MCP verwendet
`delete_data` für die Auswahl und `provider_change_consent` für die Zustimmung
zum konkreten Plan.

| Befehl | Aufgabe | Eingriff |
| --- | --- | --- |
| `baha init` | Aktuelles Repository übernehmen und benötigten Core einrichten | Verändernd |
| `baha up` | Core wiederverwenden/einrichten und Application abgleichen | Verändernd |
| `baha down` | Im Repository die Application, sonst die Control Plane stoppen | Verändernd |
| `baha status` | Im Repository Application-, sonst Control-Plane-Status | Lesend |
| `baha plan` | Application-Plan anzeigen | Lesend |
| `baha doctor` | Application/Control Plane diagnostizieren | Standardmäßig lesend |
| `baha destroy` | Eigene Ressourcen entfernen | Destruktiv |
| `baha update` | Unterstützten Update-Pfad ausführen | Verändernd |
| `baha version` | Build-Version anzeigen | Lesend |

## Control-Plane-Topologie

```bash
# Default single-server control plane outside an application repository
baha up --control-plane-only --yes
# Explicit HA on a separate fresh target
baha up --control-plane-only --ha --yes
```

## Einrichtung und Konvergenz

Der Core benötigt SQL, Secrets und Identity mit PostgreSQL, OpenBao und Keycloak. Ohne Repository/Application ist `baha up --control-plane-only` der explizite Core-only-Pfad. Beim ersten Application-Ablauf wird fehlender Core angeboten und nach geprüfter Readiness derselbe Ablauf fortgesetzt. TLS und geschützte Zugangsdaten bleiben verpflichtend.

Frische Standard-Targets verwenden einen nativen PostgreSQL- und einen OpenBao-Server; Endpunkte, Administration und Bootstrap sind zusätzliche Dienste. Identity kann weitere Abhängigkeiten benötigen. Capability-Anzahl ist keine Container-Anzahl. [Gemessene Topologien](../explanation/core-resources.md) sind von Planungsbudgets zu unterscheiden.

`--ha` ist explizite HA-Auswahl. Bestehende Targets behalten ihre aufgezeichnete Topologie; ein inkompatibler HA-Wechsel scheitert vor Mutation. Es gibt keinen impliziten Migrationspfad. Status und Doctor zeigen effektive Mitglieder und Garantien.

`--target NAME` setzt das Ziel ausdrücklich. `--no-input`/`--non-interactive` unterdrücken Prompts; `--yes` genehmigt unterstützte Entscheidungen, ersetzt aber keine fehlenden Secrets oder Policy.

Bei einem Remote-Target vergleicht `doctor` die Versionen von Entwickler-CLI
und Core. Eine Abweichung schlägt den Versionscheck fehl und nennt den exakten
Befehl `baha update --version <CLI_VERSION> --yes` für den Core-Host.
Entwicklungs-Builds haben kein veröffentlichtes Release-Kommando; Doctor weist
dann auf die Installation desselben Builds auf dem Core-Host hin.

## Application prüfen und stoppen

In einem eingerichteten Application-Repository:

```bash
baha plan -e dev
baha up -e dev
baha status -o json
baha doctor
baha down
```

Mit `baha up` wird sie fortgesetzt. `doctor --fix` ist ausdrücklich verändernd.

## Vollständiges Entfernen

Im Repository betrifft `baha destroy` die eigene Application; außerhalb zeigt es den sicheren Scope des effektiven Targets. `baha destroy --all` betrifft alle eigenen Installationsressourcen, bewahrt Source-Repositories und fremde Infrastruktur. Ohne ausdrückliche Freigabe wird nicht gelöscht.

```bash
baha destroy --all -o json
baha destroy --all --yes -o json
```

Diese Ausgabe zeigt Ressourcen und bewahrtes Recovery-Material. Erst `baha destroy --all --yes -o json` führt das genehmigte Entfernen aus. Externe Recovery-Dateien werden als `PRESERVED` aufgeführt, nicht gelesen oder automatisch gelöscht. Container-Alter oder rekonstruierte Namen beweisen keinen Besitz.

Exakte Referenz: [Core-Befehle (EN)](https://mcpdev80.github.io/baseharbor/cli/core/).


Technische Bezeichner: `ha: true`, `baha app doctor`, `baha new application orders-api --stack go --http --sql`, `preserved`, `results`.

Entfernt der Installationsabbau ein Deployment, kann sich der verbleibende
Verbraucherplan ändern. Vor dem nächsten Deployment zeigt BaseHarbor diesen
aktuellen Restplan und verlangt zwei neue Bestätigungen sowie separate Zustimmung
zur Datenlöschung. JSON liefert dafür `approval_required` /
`impact-confirmation-required` mit Restplan und `continuation`: vorherige Plan-ID
und tatsächlich abgeschlossene Deployment-IDs. `destroyed: false` und `results`
erhalten den bisherigen Fortschritt. Bestätige den neuen Plan zur Fortsetzung;
wiederhole keine fehlgeschlagene Mutation mit der alten Zustimmung.

## Provider-Update und Point-Recovery

`baha update --check` prüft ein veröffentlichtes Release, ohne Provider zu verändern. Ein unterstütztes Update benötigt `--yes`, unveränderliche Image-Identitäten und verifizierte Recovery-Punkte.

`baha update --recover --version VERSION --yes` stellt den eigenen HA-PostgreSQL-/DCS-Zustand ausdrücklich zum verifizierten Backup-Punkt dieses Updates wieder her und bewahrt verdrängte Volumes. Die CLI wird nicht ersetzt. Transaktionen nach dem Backup-Punkt gehen verloren; ein Update-Fehler löst diesen Rücksprung nie automatisch aus. Unklare Commit-/Fencing-Zustände benötigen Reconciliation. Topologie- und Versionsgrenzen stehen im [Provider-Update-Vertrag](../spec/core-provider-update-v1.md).

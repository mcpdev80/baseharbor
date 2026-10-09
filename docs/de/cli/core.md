# Core-Workflow

| Befehl | Aufgabe | Eingriff |
| --- | --- | --- |
| `baha init` | Initialisierungspfade erklären | Lesend |
| `baha up` | Core wiederverwenden/einrichten und Application abgleichen | Verändernd |
| `baha down` | Lokale Control Plane stoppen | Verändernd |
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

## Application prüfen und stoppen

In einem eingerichteten Application-Repository:

```bash
baha plan -e dev
baha up -e dev
baha status -o json
baha doctor
baha app down
```

Mit `baha up` wird sie fortgesetzt. `doctor --fix` ist ausdrücklich verändernd.

## Vollständiges Entfernen

`baha destroy` zeigt den sicheren Scope des effektiven Targets. `baha destroy --all` betrifft alle eigenen Installationsressourcen, bewahrt Source-Repositories und fremde Infrastruktur. Ohne ausdrückliche Freigabe wird nicht gelöscht.

```bash
baha destroy                 # preview this target after its applications are destroyed
baha destroy --all -o json   # inventory the full installation without mutation
baha destroy --all --yes -o json
```

Diese Ausgabe zeigt Ressourcen und bewahrtes Recovery-Material. Erst `baha destroy --all --yes -o json` führt das genehmigte Entfernen aus. Externe Recovery-Dateien werden als `PRESERVED` aufgeführt, nicht gelesen oder automatisch gelöscht. Container-Alter oder rekonstruierte Namen beweisen keinen Besitz.

Exakte Referenz: [Core-Befehle (EN)](https://mcpdev80.github.io/baseharbor/cli/core/).


Technische Bezeichner: `ha: true`, `baha app doctor`, `baha app new orders-api --stack go --http --sql`, `preserved`, `results`.

## Provider-Update und Point-Recovery

`baha update --check` prüft ein veröffentlichtes Release, ohne Provider zu verändern. Ein unterstütztes Update benötigt `--yes`, unveränderliche Image-Identitäten und verifizierte Recovery-Punkte. v0.4.24 bleibt Unreleased; diese Befehle veröffentlichen keinen Kandidaten.

`baha update --recover --version VERSION --yes` stellt den eigenen HA-PostgreSQL-/DCS-Zustand ausdrücklich zum verifizierten Backup-Punkt dieses Updates wieder her und bewahrt verdrängte Volumes. Die CLI wird nicht ersetzt. Transaktionen nach dem Backup-Punkt gehen verloren; ein Update-Fehler löst diesen Rücksprung nie automatisch aus. Unklare Commit-/Fencing-Zustände benötigen Reconciliation. Topologie- und Versionsgrenzen stehen im [Provider-Update-Vertrag](../spec/core-provider-update-v1.md).

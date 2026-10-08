# Einstieg

Dieser Einstieg bringt eine bestehende Anwendung unter BaseHarbor zum Laufen, ohne dass du Provider-Interna verstehen oder das Manifest manuell bearbeiten musst.

## Core-Einrichtung

Vor der ersten Application benötigt BaseHarbor seinen Core: **SQL + Secrets + Identity**
(PostgreSQL, OpenBao und Keycloak). Die Web Console ist optional.

Beim ersten `app init` oder Application-Start bietet BaseHarbor die Einrichtung an:

```text
BaseHarbor needs its Core services before the first application can run.
Set them up now? [Y/n]
```

Danach fragt der Wizard nach der Maschinenrolle:

```text
Is this installation running on a machine where you write code?
  1. Yes, this is a development machine
  2. No, this is a deployment machine
```

Die Rolle steuert nur Workspace-/Source-Defaults; TLS und geschützte Zugangsdaten
bleiben verpflichtend. Nach geprüfter Readiness aller drei Core-Capabilities setzt
derselbe Application-Ablauf fort. Retry verwendet dieselbe eigene Installation
und erzeugt keinen zweiten Core.

Core-only ohne Repository/Application verwendet den bestehenden Pfad
`baha up --control-plane-only`; `baha status` außerhalb eines Application-Repositories zeigt den Zustand.
Der endgültige CLI-Namensraum bleibt Gegenstand des Pre-Freeze-Reviews.
Maschinenpfade liefern ohne ausdrücklichen Bootstrap-Auftrag den typisierten
Fehler `core_required`; sie richten keinen Core stillschweigend ein.

Shared ist der einfache Standard. Pro Application isolierte Provider können
zusätzliche Instanzen und Ressourcen verbrauchen. Die [gemessenen Core-Ressourcen](../explanation/core-resources.md)
beschreiben die geprüfte Referenz-Topologie. Planungsbudgets sind keine
Messwerte. Core-Idle, Startup-Peak und gleichzeitiger Gesamtverbrauch werden erst
nach Messung der konkreten Runtime/Topologie als solche ausgewiesen.

## Konkretes Beispiel: Go-API mit SQL

Voraussetzungen: `baha` ist installiert, Docker oder Podman ist lokal verfügbar und der aktuelle Ordner enthält noch keinen Unterordner `orders-api`. Prüfe zuerst CLI und Target:

```bash
baha version
baha target show
```

Fehlt ein Target, konfiguriere es anhand der [Target-Befehle](../cli/targets.md). Erzeuge danach eine neue Anwendung:

```bash
baha app new orders-api --stack go --http --sql
cd orders-api
baha plan
baha up -e dev
baha status
baha doctor
```

`app new` erzeugt `main.go`, `go.mod`, `Dockerfile`, `compose.yaml`, `baseharbor.yaml`, Repository-Metadaten und `.env.example` mit leeren Werten. Das Manifest fordert SQL an und beschreibt den HTTP-Workload `app` auf Port 8080. Der Go-Code verwendet `pgx` und die geschützte Bindung `DATABASE_URL`; vor dem HTTP-Start prüft er die Datenbankverbindung.

Beim ersten `up` beantwortest du die unten beschriebenen Betreiberentscheidungen. Erst eine erfolgreiche Bereitstellung führt zu READY; erzeugte Dateien allein starten keine Runtime. Öffne danach die von `status` angezeigte HTTPS-Adresse und rufe `/healthz` auf. Übernimm den angezeigten Port, da Docker und rootless Podman unterschiedliche Ports verwenden können.

Eigene Bestell-Endpunkte und Tabellen ergänzt du im erzeugten Code. Das [PostgreSQL-Beispiel](../how-to/postgres.md) zeigt das Einfügen und Lesen einer konkreten Bestellung.

Nur die Anwendung stoppen und ihre persistenten Daten behalten:

```bash
baha app down
```

Mit `baha up` im selben Repository startest du sie wieder.

## Bestehendes Repository übernehmen

Bei einer vorhandenen Anwendung folgt auf die optionale Inspektion `baha app init` und danach `baha up`. Dafür brauchst du kein neues Scaffold und keine manuelle Compose-Umschreibung.

## 1. Optional: Repository prüfen

```bash
baha app inspect .
```

Die Inspektion ist schreibgeschützt. BaseHarbor zeigt den erkannten Workload, ersetzbare Infrastruktur und die erkannten Fähigkeiten, ohne das Repository zu verändern.

Detaillierte Nachweise:

```bash
baha app inspect . --verbose
```

## 2. Portablen Anwendungsvertrag erstellen

```bash
baha app init
```

BaseHarbor erkennt so viel wie sicher möglich und fragt nur bei Mehrdeutigkeit oder echten Benutzerentscheidungen nach.

Der Wizard kann unter anderem fragen nach:

- der maßgeblichen Workload-Quelle bei mehreren Compose-, Quadlet- oder Kubernetes-Kandidaten;
- Anwendungs-Workload gegenüber ersetzbarer Infrastruktur;
- provider-neutralen Anforderungen an SQL, Cache, Objektspeicher und Observability;
- anwendungseigenen Geheimnissen einschließlich Pflicht-/Optional-Status;
- automatischer Erzeugung, Eingabe beim ersten Anwenden oder späterer Konfiguration;
- Berechtigungen der Runtime-API aus konkreten Quellnachweisen.

Vor dem Schreiben von `baseharbor.yaml` zeigt BaseHarbor eine menschenlesbare Zusammenfassung. Rohes YAML erscheint nur als Zusatzdetail unter `--verbose`.

Deterministische Automatisierung bei eindeutiger Erkennung:

```bash
baha app init --quick
```

`--quick` bricht bei Mehrdeutigkeit der Quelle sicher ab und übernimmt heuristisch erkannte Geheimnis-Kandidaten niemals stillschweigend. Eindeutige Compose-, im Repository gepflegte Quadlet- und rohe Kubernetes-YAML-Quellen können geprüft und übernommen werden, ohne quellenspezifische Namen in den portablen Anwendungsvertrag zu übernehmen.

## 3. Anwendung starten

```bash
baha up
```

Beim ersten Lauf kann BaseHarbor nur Informationen abfragen, die es nicht sicher selbst bestimmen darf, zum Beispiel:

- Annahme oder Anpassung des vorgeschlagenen, Target-bezogenen Pfads für die vom Betreiber verwahrte OpenBao-Wiederherstellungsdatei;
- Wert eines fehlenden verpflichtenden Anwendungsgeheimnisses;
- Bestätigung eines sicheren Ausweichports.

Interaktive Eingabe von Geheimnissen erfolgt ohne Terminal-Echo. Zugangsdaten für Provider und Runtime werden von BaseHarbor verwaltet und nicht vom Entwickler abgefragt.

Derselbe `baha up`-Lauf konvergiert danach weiter bis READY. Nach erfolgreichem OpenBao-Bootstrap speichert BaseHarbor am effektiven Target nur die Pfadreferenz auf die Wiederherstellungsdatei. Spätere `baha up`-Läufe verwenden diese Referenz automatisch, um den gemeinsam genutzten OpenBao-Provider zu entsperren, wenn die Datei vorhanden ist.

## 4. Ergebnis prüfen

```bash
baha status
```

```bash
baha doctor
```

Erfolg bedeutet verifizierte Capability-Bereitschaft, nicht nur einen gestarteten Container.

## Fortgeschrittene und Automation-Befehle

Diese Befehle bleiben verfügbar, gehören aber nicht zum notwendigen Standardablauf:

```bash
baha plan
baha app preflight
baha app apply
baha app secret set APP_SECRET
```

Automatisierung kann weiterhin ausdrücklich `--stdin` verwenden:

```bash
printf '%s' "$APP_SECRET" | baha app secret set APP_SECRET --stdin
```

## Durchgängige Referenzdemo

Das externe Repository `mcpdev80/baseharbor-demo` ist der Release-seitige Nachweis dieses Ablaufs. Seine README beschreibt den vollständigen Test vom unveränderten Repository über `baha app init` und `baha up` bis READY, Neustart und Aufräumen.

Die Vorabprüfung validiert sowohl den geführten Benutzerablauf als auch deterministische Komponentenpfade. Der finale Release verwendet diese unveränderlichen Nachweise wieder, statt dieselbe aufwendige Matrix erneut auszuführen.

## Nächste Schritte

- [Architektur](../explanation/architecture.md)
- [Anwendungsvertrag](../explanation/application-contract.md)
- [Provider](../explanation/providers.md)
- [Sicherheit](../explanation/security.md)
- [Umgebungen](../how-to/environments.md)
- [PostgreSQL](../how-to/postgres.md)
- [Secrets](../how-to/secrets.md)
- [Backup und Restore](../how-to/backup-restore.md)
- [CLI-Überblick](../cli/index.md)

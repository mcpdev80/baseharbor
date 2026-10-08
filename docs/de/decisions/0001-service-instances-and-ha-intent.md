# ADR 0001: Stabile Service-Instanzen und Verfügbarkeitsabsicht

- Status: angenommen
- Datum: 2026-09-08
- Implementation update: v0.4.22 wählt einen nativen PostgreSQL-Server und einen OpenBao-Server für neue Standardziele aus, mit autoritativem PostgreSQL-Speicher und nativem TLS. Explizit HA behält die dreiköpfige Patroni/etcd/OpenBao-Topologie. Aufgezeichnete Topologie ist unveränderlich; Pre-Gefrierzustand wird direkt ersetzt, ohne Migration oder Legacy-Unterstützung.

## Kontext

BaseHarbor ist eine Backend-Laufzeit für unabhängige Anwendungen. Anwendungen sollten die Backend-Funktionen erklären, die sie benötigen, während BaseHarbor Bereitstellung, Sicherheit, Topologie und Lebenszyklus besitzt.

Ein einziger Antrag kann berechtigterweise mehrere Dienste desselben Typs benötigen, z. B.:

- eine transaktionale PostgreSQL-Datenbank und eine separate Analyse PostgreSQL-Datenbank
- eine Valkey-Instanz für Cache und eine andere für Sessions

BaseHarbor benötigt auch einen Pfad zu Multi-Node- und hochverfügbaren Bereitstellungen, ohne das Applikationsmanifest in Docker Compose, Kubernetes, Patroni oder Sentinel-Konfiguration umzuwandeln.

Dieselbe Anforderung gilt für BaseHarbor selbst. Die Steuerungsebene muss in der Lage sein, in einer einfachen Standardtopologie für Entwicklung und kleine Installationen zu laufen und gleichzeitig einen klaren Weg zu einer hochverfügbaren Topologie für Produktionsumgebungen zu haben. Betreiber sollten das gewünschte Verfügbarkeitsniveau ausdrücken, ohne die Steuerungs-Plane-Topologie von Hand erstellen zu müssen.

## Entscheidung

### 1. Service-Instanzen haben stabile logische Namen

Wiederholte Dienste werden als benannte logische Instanzen modelliert, nicht als Integer-Replica-Anzahl.

```yaml
services:
  postgres:
    instances:
      primary: {}
      analytics: {}

  redis:
    instances:
      cache: {}
      sessions: {}
```

Namen wie `primary`, ` analytics `, ` cache `und` sessions`sind anwendungsorientierte logische Identitäten. Sie bleiben stabil, auch wenn sich die zugrunde liegende Laufzeittopologie ändert.

Die kompakte Single-Instance-Form bleibt erhalten:

```yaml
services:
  postgres:
    enabled: true
  redis:
    enabled: true
```

Dies löst sich intern auf eine Instanz namens `default` und bewahrt den Standard `DATABASE_URL`, ` REDIS_URL `und` VALKEY_URL`Vertrag.

### 2. Instanzenzahl und HA Replikenzahl sind unterschiedliche Konzepte

Zwei PostgreSQL-Instanzen bedeuten zwei unabhängige logische Datenbanken mit unabhängigen Anmeldeinformationen, Speicher und Lebenszyklus.

Hohe Verfügbarkeit bedeutet nicht, zusätzliche logische Instanzen zu erstellen. HA ist eine Implementierungstopologie hinter einer logischen Instanz.

Zum Beispiel könnte die Zukunftsabsicht wie folgt aussehen:

```yaml
services:
  postgres:
    instances:
      primary:
        availability: high
```

Die Bewerbung erhält immer noch einen stabilen `primary` endpoint. BaseHarbor kann diesen Endpunkt mit mehreren Knoten, einem Proxy, Leader-Wahl, Replikation und Failover implementieren.

### 3. Das Manifest drückt Absicht, nicht Topologie

Die Anwendungsmanifestierung darf keine infrastrukturspezifischen Bereiche erfordern, wie z. B.:

```text
patroni
sentinel_count
etcd_nodes
quorum_size
pod_anti_affinity
synchronous_standby_names
```

Das sind Laufzeit-Beschlüsse.

Das beabsichtigte künftige Verfügbarkeitsvokabular ist klein und semantisch, zum Beispiel:

- `standard`
- `high`
- `critical`

Explizite Anbieter HA wurde in v0.4.21 eingeführt.Die v0.4.22 Laufzeitrealisierung ehrt den Standardstandard sowie explizite HA und berichtet über die ausgewählte Topologie; eine Ein-Host-HA-Topologie verspricht keine Host-Failure-Toleranz.

### 4. Verfügbarkeitsabsicht gilt auch für die BaseHarbor-Steuerebene

BaseHarbor selbst folgt der gleichen Intent-over-Topologie-Regel wie Application Services.

Eine künftige Bedienerkonfiguration kann nur die gewünschte Verfügbarkeit der Steuerebene ausdrücken, z. B.:

```yaml
baseharbor:
  availability: standard
```

or:

```yaml
baseharbor:
  availability: high
```

`standard` ist die einfache Standardtopologie für die Entwicklung, Bewertung und kleinere Anlagen. Sie sollte bewegliche Teile minimieren und gleichzeitig die gleichen Sicherheits- und Laufzeitverträge einhalten.

`high` bedeutet, dass die BaseHarbor-Steuerebene ohne vermeidbaren einzigen Ausfallpunkt für die Komponenten eingesetzt werden muss, die für den Betrieb der Plattform erforderlich sind. Die genaue Topologie bleibt eine BaseHarbor/Provider-Verantwortung.

Die HA-Steuerplane soll mindestens Folgendes abdecken:

- die BaseHarbor Control-Plane API
- Steuerflugzeug PostgreSQL
- OpenBao
- stabile Endpunkte der Steuerungs-Plane-Netzwerke
- Zertifikat/PKI-Verfügbarkeit und Rotationswege
- jede durch die gewählte Implementierung eingeführte Koordinierungs-, Routing- oder Leader-Auswahl-Komponente

Ein Anbieter darf nicht geschlossen werden, wenn er das geforderte Niveau der Verfügbarkeit der Steuerebene nicht erfüllen kann. Er darf niemals stillschweigend eine Standard-One-Node-Topologie starten, wenn `high` wurde angefordert.

### 5. Das `baha` CLI-Oberfläche bleibt über Standard- und HA-Modi hinweg stabil

Betreiber sollten unabhängig von der Topologie weiterhin denselben primären Workflow verwenden:

```text
baha up
baha status
baha doctor
```

Die CLI kann explizite Setup- oder Migrationsoptionen für die Auswahl eines Verfügbarkeitsmodus freigeben, aber die täglichen Befehle dürfen von den Betreibern nicht verlangen, die zugrunde liegende HA-Implementierung zu verstehen.

`baha status ` und`baha doctor ` muss topologie-bewusst werden: in`high` Sie sollten den effektiven HA-Vertrag validieren und nicht nur melden, dass einzelne Prozesse laufen.

Beispiele für zukünftige HA-aware Diagnosen sind:

- erforderliche Steuerungs-Plane-Mitglieder sind erreichbar
- quorum ist gegebenenfalls gesund
- der stabile Schreibendpunkt ist verfügbar
- Nachbildungen sind entsprechend dem gewählten HA-Vertrag ausreichend aktuell
- OpenBao ist über die unterstützte HA-Topologie verfügbar
- Zertifikat/PKI-Pfade bleiben nutzbar
- nicht angefordert `high` Die Komponente hat sich zu einem nicht unterstützten Ein-Knoten-Zustand stumm degradiert

Das `baha` executable selbst ist ein Operator-Tool, nicht eine Laufzeitabhängigkeit. Eine laufende BaseHarbor-Installation, einschließlich einer HA-Installation, muss fortgesetzt werden, wenn `baha` Der Prozess läuft.

### 6. Anwendungsorientierte Endpunkte bleiben über topologische Veränderungen hinweg stabil

Anwendungen verbrauchen native Protokolle und normale Umgebungsvariablen oder verbindliche Dateien.

Eine zukünftige HA-Migration darf keine Änderungen des Anwendungscodes erfordern.

```text
DATABASE_PRIMARY_URL=postgresql://stable-endpoint/...
REDIS_CACHE_URL=redis://stable-endpoint/...
```

Der Endpunkt kann später an einem lokalen Proxy, virtuellen Dienst, verwaltetem Datenbankendpunkt oder Kubernetes Service enden. Das ist transparent für Anwendungscode.

Das gleiche Prinzip gilt für BaseHarbors eigene Bediener-zugewandte Endpunkte.`standard ` to ` high`darf Anwendungscode nicht zwingen, ein BaseHarbor SDK oder einen Runtime Login Flow anzunehmen.

### 7. Laufzeitanbieter zeigen die Absicht zur Implementierung

Die gleiche logische Anwendung manifest sollte portabel bleiben über:

- Einknoten-Docker/Podman
- Remote- oder Multi-Node-Hosts
- Kubernete
- externe verwaltete PostgreSQL/Redis-Anbieter

Jeder Laufzeitanbieter ist für die Übersetzung der logischen Service-Instanz und Verfügbarkeitsabsicht in eine unterstützte Topologie verantwortlich.

Die gleiche Regel gilt für die Steuerebene: Ein Anbieter kann `high` anders auf einer Multi-Host Docker-Umgebung, Kubernetes oder einer zukünftigen verwalteten Umgebung, unter Beibehaltung der BaseHarbor Betreibervertrag.

Ein Anbieter muss nicht geschlossen werden, wenn er den gewünschten Verfügbarkeitsvertrag nicht erfüllen kann. Er darf nicht stillschweigend herabgestuft werden `high` zu einer einzigen Instanz.

## Folgen

### Positiv

- Anwendungen können heute mehrere unabhängige PostgreSQL- oder Valkey-Instanzen anfordern
- Instanz Identitäten bleiben über Upgrades und zukünftige HA-Implementierungen stabil
- einfache Anwendungen halten ein sehr kleines Manifest
- BaseHarbor vermeidet das Aufdecken von implementationsspezifischen Orchestrierungsdetails
- Anwendungscode bleibt unabhängig von der `baha` CLI und BaseHarbor SDKs
- zukünftiger Application-Service HA kann hinzugefügt werden, ohne zu definieren, was eine logische Service-Instanz bedeutet
- future BaseHarbor Steuerungs-Plane HA folgt dem gleichen kleinen semantischen Verfügbarkeitsmodell
- Betreiber halten den gleichen Kern `baha` Workflow in Standard- und HA-Modi
- `baha` bleibt eher ein Bediener-Tool als ein erforderlicher Laufzeitprozess

### Handelshemmnisse

- Laufzeitabgleich muss Ressourcen pro logische Instanz verfolgen
- generische Umgebungsvariablen werden zweideutig, wenn mehrere Instanzen existieren
- Die Entfernung oder Umbenennung einer Service-Instanz ist ein Stateful-Lifecycle-Betrieb und muss sicher behandelt werden
- HA erfordert separate Arbeit für die Platzierung, Failover, Quorum, Backup/Recovery und die Validierung der Providerfähigkeit
- Control-Plane HA erfordert explizite Gesundheitssemantik jenseits von Prozesslebendigkeit
- Wanderung zwischen `standard` und `high` muss Identitäten, Geheimnisse und stabile Endpunkte ohne unsichere implizite Downgrade-Pfade bewahren

## Regeln für die Laufzeit der Verträge

Für eine einzige Standardinstanz bleiben Kompatibilitätsvariablen bestehen:

```text
DATABASE_URL
REDIS_URL
VALKEY_URL
```

Benannte Instanzen erhalten explizite Variablen wie:

```text
DATABASE_PRIMARY_URL
DATABASE_ANALYTICS_URL
REDIS_CACHE_URL
REDIS_SESSIONS_URL
VALKEY_CACHE_URL
VALKEY_SESSIONS_URL
```

Wenn mehrere PostgreSQL-Instanzen existieren,`default ` ist das bevorzugte Generikum`DATABASE_URL ` Ziel. Wenn nein`default ` gibt es aber ein`primary ` Instanz existiert,`primary` kann den generischen Alias erhalten. Andernfalls erfindet BaseHarbor keine mehrdeutige generische Datenbank-URL.

Dieselbe Präferenzregel gilt für Cache-Bindungen. Wenn keine eindeutige bevorzugte Instanz existiert, werden nur benannte Variablen emittiert.

## Nichtziel dieser Entscheidung

Dieser ADR wählt nicht:

- Patroni im Vergleich zu einer anderen PostgreSQL HA-Implementierung
- Redis Sentinel versus Redis Cluster oder eine andere Valkey Topologie
- OpenBao HA Speicher/Topologie Details
- ein Kubernetes-Betreiber
- einen spezifischen Proxy/Lastausgleicher
- synchrone oder asynchrone Replikationseinstellungen
- Herstellung HA SLOs
- genaue Quorumgrößen oder Knotenzahlen
- die konkrete CLI-Syntax zum Umschalten einer bestehenden Installation von `standard` to ` high`

Diese Entscheidungen erfordern ihre eigene Umsetzung und operative Beweise.
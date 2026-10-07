# Architektur

BaseHarbor übersetzt portable Anforderungen einer Anwendung in verifizierte Infrastruktur.

```text
Anwendung
   ↓
Portable Anforderungen
   ↓
Umgebung + Richtlinie
   ↓
BaseHarbor Core
   ↓
Runtime- + Fähigkeits- + Auslieferungs-Provider
   ↓
Verifiziertes Ergebnis
```

## Core-Installation

Der BaseHarbor Core besteht verbindlich aus **SQL + Secrets + Identity**:
PostgreSQL, OpenBao und Keycloak. Die Web Console bleibt optional und verbindet sich
mit genau einem ausgewählten Core derselben Installation/Sicherheitsgrenze.
Same-Origin-HTTPS ist die Standardtopologie.

Der Core kann ohne Application und Repository eingerichtet werden. Eine verbindliche
Installations-ID, getrennte Capability-Zustände und echte Readiness-Prüfungen machen
Teilfehler sichtbar. Retry reconciliert eigene Ressourcen; fremde oder mehrdeutige
Zustände werden weder übernommen noch ersetzt.

Der erste Application-Flow bietet die Einrichtung bei Bedarf an und läuft danach
weiter. Die Maschinenrolle Development/Deployment steuert Workspace-/Source-Defaults;
TLS und geschützte Zugangsdaten gelten in beiden Fällen.
Gewöhnliche lokale Entwicklungsarbeit erfordert keinen interaktiven Login.

Die ausgewählte Installation bleibt für Core-verwaltete Zugangsdaten, Service-PKI
und Runtime-Berechtigungsbroker zuständig. Application-Workloads haben eine eigene
Runtime-Grenze. Vor der Ausführung wird die ausgewählte Core-Konfiguration erneut
geprüft. Veränderte oder nicht verfügbare Bindungen blockieren bereits vor nativer
Workload-Inspektion oder Mutation. Ein ausgewählter Remote-Core wird niemals
implizit durch einen lokalen Core ersetzt.

Die Core-Capabilities sind verpflichtend; Provider können shared oder pro Application
isoliert platziert sein. Zusätzliche Isolation kann zusätzliche Instanzen und
Ressourcenverbrauch erzeugen. Messwerte müssen Topologie/Placement, stabilisierten
Idle-Verbrauch, Startup-/Konvergenz-Peak und gleichzeitigen Core-Gesamtverbrauch nennen.
Fehlende Messungen werden ausdrücklich als nicht verfügbar ausgewiesen.

## Portable Anforderungen

Die Anwendung beschreibt, was sie braucht. Sie schreibt nicht vor, welches Infrastrukturprodukt das umsetzen muss.

Beispiele sind SQL, Cache, Object Storage, Secrets, Identity, HTTP-Exposition und
Telemetrie.

## Repository-Workload-Quellen

Repository-Syntax wird vor Application Intent über einen Workload Source Adapter
interpretiert und ist keine eigene Provider-Achse. Compose, Repository-Quadlet und
Raw Kubernetes YAML liefern normalisierte Evidenz für Inspektion und Adoption.
Ihre Source-Identität bleibt Herkunft; logische Workload-Komponenten bilden den
portablen Vertrag. Diese Grenze wurde in v0.4.20 nachgewiesen.

Helm und Kustomize folgen als spätere Source Adapter. Kubernetes und OpenShift
bleiben spätere Runtime Provider. Siehe [Workload-Quellen](workload-sources.md).

## Drei Provider-Achsen

```text
runtime != capability != delivery
```

- Runtime-Provider: wo Workloads laufen.
- Fähigkeits-Provider: wie eine fachliche Infrastruktur-Anforderung umgesetzt wird.
- Auslieferungs-Provider: wie der gewünschte Runtime-Zustand ausgerollt und abgeglichen wird.

Aktuell verwendet Docker Docker Compose. Podman übersetzt dieselben Workload-/Runtime-Anforderungen in native Quadlets und verwaltet sie ohne Root-Rechte über `systemd --user`. `podman-compose` ist dafür nicht erforderlich. Kubernetes und OpenShift folgen später.

## Besitz und Platzierung

Wo anwendbar:

```text
application
shared
external
```

BaseHarbor verändert nur Ressourcen, die es besitzt.

Shared Placement ist eine eigene Lebenszyklusgrenze. Das Target beziehungsweise der Provider besitzt Infrastruktur; jede Application besitzt isolierte logische Ressourcen, Zugangsdaten und Bindungen. Eine PostgreSQL-Instanz kann dadurch getrennte Datenbanken bedienen, ohne Application Intent an die Topologie zu binden.

## Umgebung und Policy

Die Umgebung beschreibt Deployment-Risiko und Policy-Kontext, weder Runtime noch Provider-Produkt. Der Abgleich vergleicht Soll- und Ist-Zustand und scheitert bei Mehrdeutigkeit, Besitzkonflikten oder nicht unterstützten Anforderungen sicher.

## Lokales HTTPS-Routing

Ein Target-weiter Dev-Gateway liefert kanonische Browser-URLs. Loopback-Ports und interne Provider-Endpunkte bleiben Implementierungsdetails. Die Standarddomain `baha.localhost` ist pro Target konfigurierbar.

Applications verwenden `<app>.<domain>`, Application-Management `<app>-<service>.<domain>` und shared Provider semantische Hosts wie `pgadmin`, `cache`, `storage`, `auth`, `secrets` oder `metrics`. Routen folgen dem Provider-Placement und dessen Besitzgrenze. Externe Provider behalten ihre URLs. Gateway-TLS stammt aus der Service-PKI; HTTPS-Upstreams werden gegen projiziertes Vertrauensmaterial geprüft.

## Lebenszyklus

```text
planen -> vorprüfen -> anwenden -> verifizieren
```

CLI, JSON und MCP benutzen dieselbe Semantik.

Keine Schnittstelle darf Policy, Besitzprüfung, Verifikation oder Secret-Schutz umgehen. Menschliche Doku erklärt Nutzung und Konzepte; technische Referenzen und Spezifikationen definieren exaktes beziehungsweise normatives Verhalten. ADRs begründen Entscheidungen, GitHub Issues planen zukünftige Arbeit und Release Notes dokumentieren ausgeliefertes Verhalten.

Normative Details stehen ausschließlich in den englischen [Spezifikationen](https://mcpdev80.github.io/baseharbor/spec/).

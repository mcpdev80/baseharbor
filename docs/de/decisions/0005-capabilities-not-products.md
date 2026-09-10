# ADR 0005: Application Contracts beschreiben Capabilities statt Produkte

Status: Akzeptiert

Datum: 2026-09-10

## Kontext

BaseHarbor soll dieselbe logische Anwendung von lokaler Entwicklung und Homelab über Compose-basierte Produktion bis zu späteren Kubernetes-/OpenShift-Enterprise-Deployments begleiten.

Konkrete Infrastrukturprodukte können sich dabei ändern. Lokal können PostgreSQL, Valkey, OpenBao und Caddy verwendet werden; bei einem Enterprise-Kunden stattdessen kundeneigenes PostgreSQL, Managed Redis, Vault, Ceph RGW und OpenShift Routes.

Würden Produktnamen Bestandteil des portablen Application Contracts, würde jeder Provider-Wechsel zu einer App-Migration. Das widerspricht dem Ziel eines stabilen Anwendungsvertrags über verschiedene Environments hinweg.

## Entscheidung

BaseHarbor Application Contracts MÜSSEN Infrastruktur-Capabilities und portable Anforderungen beschreiben, nicht konkrete Infrastrukturprodukte.

Konkrete Implementierungen MÜSSEN über austauschbare Provider außerhalb des portablen Application Contracts ausgewählt werden.

```text
Application Contract
        ↓
Capabilities / portable Anforderungen
        ↓
Environment + Policy
        ↓
Provider-Auswahl
        ↓
Konkrete Implementierung
```

Beispiele für portable Capabilities:

```text
database.sql
cache.key-value
object-storage.s3
secrets
identity.oidc
ingress.http
tls.certificate
telemetry.otel
metrics.openmetrics
logs
```

Beispiele für konkrete Provider:

```text
PostgreSQL
Valkey
OpenBao
SeaweedFS
Caddy
cert-manager
Keycloak
Prometheus
Loki
```

### Application und Environment bleiben getrennt

Die Application-Definition beantwortet:

> Was braucht die Anwendung?

Die Environment-/Plattformkonfiguration beantwortet:

> Wie wird diese Anforderung hier erfüllt?

### Default Provider sind Implementierungsentscheidungen

BaseHarbor DARF Default- oder Referenzprovider mitliefern bzw. empfehlen. Diese sind aber kein Bestandteil der App-Identität.

Jeder Default-Provider MUSS haben:

1. eine definierte Capability-/Provider-Grenze;
2. einen dokumentierten Austauschpfad;
3. stabile Anwendungsschnittstellen, wenn ein etablierter Standard existiert;
4. klare Capability-Verhandlung, falls ein alternativer Provider Anforderungen nicht vollständig erfüllen kann.

Ein Provider-Wechsel DARF Security-, Haltbarkeits-, Verfügbarkeits- oder Protokollgarantien niemals still reduzieren.

### Provider-Auswahl gehört zum Environment

Provider-Auswahl, Credentials, Endpunkte, Cluster-spezifische Einstellungen und Produkttuning gehören zur Environment-/Plattformseite bzw. in operatorverwalteten State.

Apps dürfen nicht gezwungen werden, Produktkonfiguration einzubauen, nur weil ein Environment einen anderen Provider verwendet.

### Standards sind bevorzugte Grenzen

Wo möglich nutzt BaseHarbor bestehende Ökosystemstandards:

- SQL bzw. PostgreSQL-kompatible Schnittstellen für relationale Datenbanken;
- Redis-Protokoll für passende Cache-/Key-Value-Fälle;
- S3 API für Object Storage;
- OIDC/OAuth2 für Identity;
- OpenTelemetry/OpenMetrics für Telemetrie und Metrics;
- Standard-TLS/X.509 und ACME/PKI für Zertifikate;
- native Secret-Dateien, Environment-Projektion oder Workload Identity je nach Provider und Policy.

## Kompatibilität mit v0.2.0

Diese Entscheidung fügt v0.2.0 keine neue Funktion hinzu und ändert Manifest v1 nicht.

Das aktuelle v0.2.0-Manifest verwendet weiterhin PostgreSQL-/Redis-/Valkey-orientierte Felder, weil Compose die erste vollständige Implementierung ist. Diese Felder sind der aktuelle v1-Contract, aber keine Vorgabe dafür, dass spätere providerneutrale Contracts Produktnamen verwenden müssen.

Compose bleibt der Fokus. PostgreSQL, Valkey und OpenBao bleiben die aktuellen konkreten Implementierungen.

## Konsequenzen

Vorteile:

- Apps können von Homelab zu Enterprise-Infrastruktur wachsen, ohne produktspezifische Neuschreibung;
- Default-Komponenten können bei Lizenz-, Security-, Support- oder Projektänderungen ersetzt werden;
- kundeneigene Managed Services werden möglich, ohne App-Manifeste zu forken;
- Kubernetes/OpenShift können native Provider verwenden und trotzdem denselben App-Intent erfüllen.

Kosten:

- Capabilities müssen präzise definiert werden;
- Provider brauchen Conformance- und Capability-Tests;
- nicht jedes Produkt ist für jede Capability tatsächlich austauschbar;
- Environment-/Provider-Konfiguration wird ein eigener versionierter Bereich.

## Nicht-Ziele

Dieses ADR verlangt nicht, jetzt mehrere Provider zu implementieren. Es legt die Architekturgrenze für alle zukünftigen Provider- und Capability-Arbeiten fest.

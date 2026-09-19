# ADR 0005: Application Contracts beschreiben Capabilities statt Produkte

Status: Akzeptiert

Datum: 2026-09-10

## Kontext

BaseHarbor soll dieselbe logische Anwendung von lokaler Entwicklung und Homelab ueber Compose-basierte Produktion bis zu spaeteren Kubernetes-/OpenShift-Enterprise-Deployments begleiten.

Konkrete Infrastrukturprodukte koennen sich dabei aendern. Lokal koennen PostgreSQL, Valkey und OpenBao verwendet werden; bei einem Enterprise-Kunden stattdessen kundeneigenes PostgreSQL, Managed Redis, Vault, Ceph RGW und OpenShift Routes.

Wuerden Produktnamen Bestandteil des portablen Application Contracts, wuerde jeder Provider-Wechsel zu einer App-Migration. Das widerspricht dem Ziel eines stabilen Anwendungsvertrags ueber verschiedene Environments hinweg.

## Entscheidung

BaseHarbor Application Contracts MUESSEN Infrastruktur-Capabilities und portable Anforderungen beschreiben, nicht konkrete Infrastrukturprodukte.

Konkrete Implementierungen MUESSEN ueber austauschbare Provider ausserhalb des portablen Application Contracts ausgewaehlt werden.

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

Beispiele fuer portable Capabilities:

```text
database.sql
cache.key-value
object-storage.s3
secrets
identity.oidc
ingress.http
tls.certificate
telemetry.otlp
metrics.openmetrics
logs
```

Beispiele fuer konkrete Provider:

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

> Wie wird diese Anforderung hier erfuellt?

### Default Provider sind Implementierungsentscheidungen

BaseHarbor DARF Default- oder Referenzprovider mitliefern bzw. empfehlen. Diese sind aber kein Bestandteil der App-Identitaet.

Jeder Default-Provider MUSS haben:

1. eine definierte Capability-/Provider-Grenze;
2. einen dokumentierten Austauschpfad;
3. stabile Anwendungsschnittstellen, wenn ein etablierter Standard existiert;
4. klare Capability-Verhandlung, falls ein alternativer Provider Anforderungen nicht vollstaendig erfuellen kann.

Ein Provider-Wechsel DARF Security-, Haltbarkeits-, Verfuegbarkeits- oder Protokollgarantien niemals still reduzieren.

### Provider-Auswahl gehoert zum Environment

Provider-Auswahl, Credentials, Endpunkte, Cluster-spezifische Einstellungen und Produkttuning gehoeren zur Environment-/Plattformseite bzw. in operatorverwalteten State.

Apps duerfen nicht gezwungen werden, Produktkonfiguration einzubauen, nur weil ein Environment einen anderen Provider verwendet.

### Standards sind bevorzugte Grenzen

Wo moeglich nutzt BaseHarbor bestehende Oekosystemstandards:

- SQL bzw. PostgreSQL-kompatible Schnittstellen fuer relationale Datenbanken;
- Redis-Protokoll fuer passende Cache-/Key-Value-Faelle;
- S3 API fuer Object Storage;
- OIDC/OAuth2 fuer Identity;
- OpenTelemetry/OpenMetrics fuer Telemetrie und Metrics;
- Standard-TLS/X.509 und ACME/PKI fuer Zertifikate;
- native Secret-Dateien, Environment-Projektion oder Workload Identity je nach Provider und Policy.

## Aktuelle Kompatibilitaet mit v0.3.0

Diese Entscheidung fuehrt auch in v0.3.0 noch **nicht** das zukuenftige generische Capability-/Provider-Manifest ein.

Manifest v1 verwendet weiterhin PostgreSQL-/Redis-/Valkey-orientierte Felder, weil Compose die erste vollstaendige Implementierung ist. Diese Felder sind der aktuelle pre-v1-Contract, aber keine Vorgabe dafuer, dass spaetere providerneutrale Contracts Produktnamen verwenden muessen.

v0.3 fuegt konkrete Compose-Deployment-Funktionen hinzu: Public-FQDN/TLS-Runtime-State, Existing/BYOC-Zertifikats-Lifecycle, app-eigene HTTP/TLS-Readiness und Host-Port-Fallback. Diese Details bleiben absichtlich ausserhalb des portablen Manifests und bestaetigen damit die Architekturentscheidung statt sie aufzuweichen.

Compose bleibt der aktuelle Runtime-Fokus. PostgreSQL, Valkey und OpenBao bleiben die derzeitigen konkreten Managed-Implementierungen.

## Konsequenzen

Vorteile:

- Apps koennen von Homelab zu Enterprise-Infrastruktur wachsen, ohne produktspezifische Neuschreibung;
- Default-Komponenten koennen bei Lizenz-, Security-, Support- oder Projektveraenderungen ersetzt werden;
- kundeneigene Managed Services werden moeglich, ohne App-Manifeste zu forken;
- Kubernetes/OpenShift koennen native Provider verwenden und trotzdem denselben App-Intent erfuellen.

Kosten:

- Capabilities muessen praezise definiert werden;
- Provider brauchen Conformance- und Capability-Tests;
- nicht jedes Produkt ist fuer jede Capability tatsaechlich austauschbar;
- Environment-/Provider-Konfiguration wird ein eigener versionierter Bereich.

## Nicht-Ziele

Dieses ADR verlangt nicht, jetzt mehrere Provider zu implementieren. Es legt die Architekturgrenze fuer alle zukuenftigen Provider- und Capability-Arbeiten fest.

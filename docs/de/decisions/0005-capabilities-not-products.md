# ADR 0005: Application-Verträge beschreiben Fähigkeiten, keine Produkte

> Kompatibilität: Frühere Formulierungen zu Legacy-Unterstützung und Migration wurden durch [ADR 0018](0018-public-contract-namespace-and-compatibility.md) abgelöst. Die historische Begründung bleibt erhalten. v0.4 übernimmt keine Legacy- oder Migrationspflicht.

Status: Akzeptiert

Datum: 2026-09-10

## Kontext

BaseHarbor soll dieselbe logische Application von lokaler Entwicklung und Homelab über Compose-Produktion bis zu späteren Kubernetes-/OpenShift-Installationen betreiben können.

Die eingesetzten Infrastrukturprodukte werden sich ändern: Lokal können PostgreSQL, Valkey und OpenBao gebündelt sein; im Unternehmen kommen eventuell kundeneigene PostgreSQL-Dienste, verwaltetes Redis, Vault, Ceph RGW oder OpenShift Routes zum Einsatz.

Würden Produktnamen Teil des portablen Application-Vertrags, wäre jeder Produktwechsel eine Migration der Application. Repositories würden so an BaseHarbor-Implementierungen gebunden und die Stabilität des Vertrags über Umgebungen hinweg verloren gehen.

## Entscheidung

BaseHarbor-Application-Verträge MÜSSEN Infrastruktur-Fähigkeiten und portable Application-Anforderungen beschreiben, NICHT konkrete Produkte.

Konkrete Realisierungen MÜSSEN über austauschbare Provider außerhalb des portablen Application-Vertrags ausgewählt werden.

```text
Application Contract
        |
        v
Capabilities / portable requirements
        |
        v
Environment + policy
        |
        v
Provider selection
        |
        v
Concrete implementation
```

Beispiele für portablen Capability-Intent:

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

Konkrete Provider können PostgreSQL, Valkey, OpenBao, SeaweedFS, Caddy, cert-manager, Keycloak, Prometheus oder Loki sein.

### Application- und Umgebungskonfiguration sind getrennt

Die repositoryeigene Application-Definition beantwortet: *Was benötigt die Application?*

Die Umgebungs-/Plattformkonfiguration beantwortet: *Wie werden diese Anforderungen hier umgesetzt?*

Konzeptionelles zukünftiges Beispiel:

```yaml
# baseharbor.yaml
apiVersion: baseharbor.io/v1
kind: Application
metadata:
  name: mailflow
spec:
  resources:
    database:
      main:
        type: sql
    cache:
      main:
        type: key-value
    objectStorage:
      attachments:
        type: s3
  secrets:
    - OPENAI_API_KEY
  tls:
    required: true
```

Eine lokale Umgebung könnte Folgendes auswählen:

```yaml
providers:
  database: postgres
  cache: valkey
  objectStorage: seaweedfs
  secrets: openbao
  ingress: caddy
```

In einer Unternehmensumgebung wäre möglich:

```yaml
providers:
  database: customer-postgres
  cache: managed-redis
  objectStorage: ceph-rgw
  secrets: vault
  ingress: openshift-route
```

Die portable Application-Definition bleibt dabei unverändert.

### Standard-Provider sind Implementierungsentscheidungen

BaseHarbor DARF für eine Fähigkeit Standard-Provider ausliefern oder empfehlen. Dieser Standard ist kein Bestandteil des Application-Vertrags.

Für jede gebündelte Standardkomponente MUSS Folgendes vorhanden sein:

1. eine definierte Capability-/Provider-Grenze;
2. ein dokumentierter Austauschpfad;
3. stabile anwendungsseitige Schnittstellen, soweit ein Ökosystemstandard besteht;
4. ausdrückliche Capability-Aushandlung, wenn ein anderer Provider die Anforderung nicht erfüllen kann.

Ein Providerwechsel DARF zugesicherte Sicherheit, Dauerhaftigkeit, Verfügbarkeit oder Protokollgarantien NICHT stillschweigend abschwächen.

### Providerkonfiguration gehört der Umgebung

Providerwahl, Zugangsdaten, Endpunkte, clusterspezifische Einstellungen und produktspezifische Optimierungen gehören in die Plattform-/Umgebungskonfiguration oder den vom Operator verwalteten Zustand.

Applications DÜRFEN NICHT allein deshalb produktspezifische Konfiguration benötigen, weil eine Umgebung einen anderen Provider verwendet.

Providerspezifische Erweiterungen können später existieren, müssen aber optional und klar namensraumgebunden bleiben und dürfen das portable Capability-Modell nicht verändern.

### Standards haben Vorrang

BaseHarbor bevorzugt verbreitete Verträge vor proprietären Datenprotokollen:

- SQL-/PostgreSQL-kompatible Verbindungsschnittstellen für relationale Datenbanken;
- Redis-Protokoll für Key-Value-/Cache-Zugriff, soweit passend;
- S3-API für Objektspeicher;
- OIDC/OAuth2 für Identität;
- OpenTelemetry/OpenMetrics für Telemetrie und Metriken;
- standardisiertes TLS/X.509-Material und ACME-/PKI-Anbindung;
- Secret-Dateien, Umgebungsvariablen und Workload-Identitäten nach Provider-Policy.

Die Orchestrierung und Policy von BaseHarbor können projektspezifisch sein; der Datenzugriff der Applications sollte es nicht sein.

## Kompatibilität mit v0.3.0

Diese Entscheidung führt nicht bereits das zukünftige allgemeine Capability-/Provider-Manifestschema ein.

Manifest v1 aus v0.3.0 benennt noch PostgreSQL-/Redis-/Valkey-orientierte Dienste, weil Compose die erste vollständige Implementierung ist. Diese Felder gehören zum damaligen Pre-v1-Vertrag und verpflichten zukünftige providerneutrale Schemas nicht zu Produktnamen.

v0.3 ergänzt deploymentspezifisches Verhalten wie öffentliche FQDN-/TLS-Zustände, BYOC-Zertifikate, Application-eigene HTTP-/TLS-Readiness und Compose-Host-Port-Fallback. Diese Details bleiben absichtlich außerhalb des portablen Manifests.

Spätere Vertragsentwicklung muss den in ADR 0018 geregelten Weg für öffentliche Vertragsänderungen respektieren.

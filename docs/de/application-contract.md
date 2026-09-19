# Anwendungsvertrag

Eine Anwendung beschreibt, welche Backend-Faehigkeiten und Secret-Namen sie benoetigt. BaseHarbor uebernimmt Bereitstellung, Isolation, Readiness und Lebenszyklus. Die Anwendung soll keine OpenBao-AppRole-Namen, internen KV-Pfade, Compose-Netzwerknamen oder proprietaere BaseHarbor-IDs kennen muessen.

## Anwendungsidentitaet und Deployment-Kontext

`app.name` ist die stabile logische Identitaet der Anwendung. `app.environment` beschreibt den Deployment-Kontext und ist **kein** Bestandteil der fachlichen Anwendungsidentitaet.

Dieselbe Anwendung kann deshalb als `dev`, `test`, `staging`, `production` oder kundenspezifische Instanz betrieben werden, ohne als neue Anwendung definiert zu werden. In der aktuellen Compose-Implementierung wird der Environment-Wert fuer Isolation und Runtime-Namen verwendet. Spaetere Provider wie Kubernetes oder OpenShift duerfen dieselben logischen Anforderungen anders realisieren.

Provider-spezifische Details wie Compose-Projektnamen, Netzwerke, Host-Ports, Volumes oder OpenBao-Pfade gehoeren nicht zum portablen Anwendungsvertrag.

## Portabler Contract und Deployment-State

`baseharbor.yaml` Manifest v1 ist der unterstuetzte repository-eigene Kompatibilitaetsvertrag. BaseHarbor uebersetzt daraus den portablen Anwendungs-Intent in den providerneutralen `PortableContract`; Compose-spezifische Kompatibilitaetsfelder gehoeren nicht in diesen portablen View.

Dazu gehoeren in v0.4 beispielsweise:

- Public FQDN des aktuellen Deployments;
- Deployment-TLS-Modus;
- Existing/BYOC-Zertifikatsquelle und normalisierte geschuetzte Zertifikat-/Key-Dateien;
- automatisch ausgewaehlte Host-Port-Fallbacks;
- generierte Compose-Overrides und Runtime-Identity-Material.

Diese Werte duerfen nicht nur deshalb in den portablen Anwendungs-Intent wandern, weil Compose sie aktuell benoetigt.

Seit v0.4.2 werden auch Provider-Platzierung und Lifecycle-Ownership separat in einer geschuetzten Provider-Registry gehalten. Shared, application-scoped und externe Provider bleiben Operator-State und werden nicht Teil des portablen Application Contracts.

```yaml
version: 1

app:
  name: mailflow
  environment: production

services:
  postgres:
    enabled: true
  redis:
    enabled: true
  secrets:
    enabled: true

secrets:
  required:
    - name: OPENAI_API_KEY
    - name: SMTP_PASSWORD
```

Secret-Werte gehoeren niemals in das Manifest.

## Mehrere Instanzen

Der einfache Fall bleibt eine Instanz namens `default`. Mehrere unabhaengige logische Dienste werden stabil benannt:

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

Jede Instanz erhaelt eigene Zugangsdaten, persistenten Zustand und Bindings. Mehrere Instanzen sind **kein HA**. Zukuenftiges HA liegt hinter einem stabilen logischen Dienst-Endpunkt.

## Workload-only Anwendungen

Ein explizit deklarierter repository-eigener Compose-Workload kann auch ohne kuenstliche PostgreSQL- oder Valkey-Abhaengigkeit eine gueltige Anwendung sein. BaseHarbor erfindet fuer diesen Fall keine Backend-Services, Credentials oder Netzwerke.

Ein Manifest ohne verwaltete Capability und ohne expliziten Workload bleibt ungueltig. Managed-Secrets-only bleibt dort ungestuetzt, wo der aktuelle Runtime Broker ein materialisiertes Managed Backend benoetigt.

## Standard-Schnittstellen

BaseHarbor erzeugt normale Verbindungsinformationen wie:

```text
DATABASE_URL=postgresql://...
DATABASE_PRIMARY_URL=postgresql://...
REDIS_URL=redis://...
REDIS_SESSIONS_URL=redis://...
VALKEY_URL=redis://...
```

sowie geschuetzte Datei-Bindings. Ein Anwendungsprozess braucht zur Laufzeit weder `baha`, einen BaseHarbor-Login noch ein BaseHarbor-SDK.

Trusted-local CLI-Komfort wie `baha app psql`, `baha app valkey`, `logs`, `shell` oder `exec` ist optional und keine Runtime-Abhaengigkeit der Anwendung.

## Required Secrets

`baha app apply` und `baha app up` starten den Workload nicht, solange ein deklariertes Required Secret fehlt oder unbrauchbar ist. `show`, `status` und `doctor` zeigen nur Readiness-/Presence-/Usability-Metadaten und niemals Secret-Werte.

Die konkrete Secret-Auslieferung ist Provider-Sache. Compose kann heute Environment-/Datei-Bindings oder Runtime Identity verwenden; spaetere Kubernetes-/OpenShift-Provider koennen native Secret-Projektion oder Workload Identity verwenden, ohne den logischen Secret-Vertrag der Anwendung zu aendern.

## Workload-Readiness

Ein laufender Container ist nicht automatisch READY. BaseHarbor bewertet ausgewaehlte Compose-Services health-aware und prueft konventionelle app-eigene HTTP/HTTPS-Publisher aktiv.

Redirects gelten als erreichbare Exposition, 5xx oder nicht erreichbare Endpunkte als NOT READY. Bei hostname-gebundenem HTTPS wird der lokale Port geprueft, aber Public FQDN als HTTP Host/TLS ServerName verwendet.

## Deployment-TLS ist kein App-Produktvertrag

Die aktuelle Compose-Implementierung unterstuetzt den Existing/BYOC-Zertifikats-Lifecycle fuer das aktuelle Repository-Compose-Deployment. `baha app tls update --check` ist read-only. `baha app tls update` validiert Quelle, Key-Pair und FQDN, verhindert Downgrades, installiert owner-only Dateien, startet bei Bedarf den Workload neu und verifiziert Readiness.

Das macht Zertifikatsverzeichnisse, Caddy-Details oder Compose-TLS-Dateien nicht zu portablen App-Anforderungen. BaseHarbor-gesteuertes ACME, OpenBao-PKI-Issuance, automatische Rotation und providerneutrale TLS-Capabilities bleiben Future Work.

BaseHarbor trennt Infrastruktur-/Secret-Eingaben von normaler Anwendungskonfiguration. Fachliche Einstellungen wie Sprache, Batch-Groesse oder Schwellenwerte bleiben Eigentum der Anwendung.


## Verwaltete HTTP-Exposition

Endpoint-Erkennung und Exposition sind absichtlich getrennte Konzepte.

Ein app-eigener Compose-Publisher bleibt Eigentum der Anwendung. BaseHarbor darf ihn erkennen, beobachten und verifizieren, uebernimmt aber weder Provisionierung noch Loeschung.

Verwaltete Exposition ist expliziter portabler Intent:

```yaml
workload:
  compose: compose.yaml
  services:
    - web

exposure:
  http:
    - name: public
      service: web
      port: 8080
      protocol: https
      visibility: public
```

Portable Felder beschreiben nur die Anwendungsanforderung: logischer Exposure-Name, logischer Workload-Service, Zielport, HTTP/HTTPS-Transport und Sichtbarkeit `public|internal`. Konkreter FQDN, publizierter Host-Port, Zertifikatsquelle, Compose-Netz und Reverse-Proxy-Konfiguration bleiben Deployment-/Provider-State.

In v0.4.4 ist Caddy der Compose-Referenzprovider. Verwaltetes HTTPS verwendet den bestehenden Existing/BYOC-Deployment-TLS-State. Managed ACME, OpenBao-PKI-Issuance und automatische Zertifikatserneuerung bleiben Future Work.

`visibility: public` ist der kanonische Default. `visibility: internal` bindet der aktuelle Compose-Referenzprovider nur lokal/auf Loopback.

## Secure Bindings und Workload Identity in v0.4.5

v0.4.5 fuehrt unterhalb des Application Manifests ein providerneutrales Secure-Binding-Modell ein. Es beschreibt die Security-Metadaten fuer die Verbindung eines Workloads mit einer Capability, ohne Provider-Interna oder Secret-Werte in den portablen Application Intent zu verschieben.

Ein Secure Binding kann enthalten:

- Workload-Identitaet;
- opake Credential-Referenzen;
- Trust-/CA-Referenzen;
- Least-Privilege-Authorization-Metadaten;
- opake Referenzen auf Required Secrets;
- deklarierte Unterstuetzung fuer Renewal, Rotation und Revocation;
- maschinenlesbare Security-Diagnostik.

Der bestehende Managed-Secrets-Pfad bildet seine Runtime Identity auf das SPIFFE-Subject `spiffe://baseharbor/apps/<application>/<environment>` ab. Das Binding enthaelt nur logische Referenzen. OpenBao-AppRole-Namen, RoleIDs, SecretIDs, KV-Pfade, Policy-Namen, Private Keys und Secret-Werte bleiben geschuetzter Provider-/Runtime-State.

Das Modell liegt bewusst unter Manifest v1. Anwendungen deklarieren weiterhin nur Secret-Namen und Capability-Anforderungen; SPIFFE, OpenBao, Zertifikatsdateien oder BaseHarbor-Credential-Referenzen werden nicht zu neuen Manifest-Feldern.

Secure-Binding-Metadaten werden vor dem Provider-Preflight validiert. Ungueltige oder mehrdeutige Referenzen scheitern damit vor jeder Provider-Mutation.


## S3-kompatibler Object Storage in v0.4.6

Object Storage ist expliziter portabler Application Intent. Anwendungen deklarieren logische Bucket-Identitaeten und kein Storage-Produkt:

```yaml
services:
  object_storage:
    buckets:
      attachments: {}
      exports: {}
```

Der deterministische CLI-Pfad ist gleichwertig:

```bash
baha app init mailflow \
  --s3-bucket attachments \
  --s3-bucket exports
```

Mit `--s3` kann ein einzelner Default-Bucket angefordert werden.

Jeder logische Bucket wird auf `object-storage.s3/v1` abgebildet. SeaweedFS ist der aktuelle shared Compose-Referenzprovider, aber `baseharbor.yaml` enthaelt weder SeaweedFS-Image/Port noch physischen Bucket-Namen, IAM-User oder Credential-Werte. Diese Details bleiben Provider-/Deployment-State.

Fuer den bevorzugten Bucket materialisiert BaseHarbor normale S3/AWS-kompatible Variablen:

```text
S3_ENDPOINT=http://...
S3_BUCKET=...
S3_REGION=us-east-1
AWS_ENDPOINT_URL=http://...
AWS_REGION=us-east-1
AWS_ACCESS_KEY_ID=...
AWS_SECRET_ACCESS_KEY=...
```

Benannte Buckets erhalten zusaetzlich `S3_<NAME>_ENDPOINT`, `S3_<NAME>_BUCKET`, `S3_<NAME>_REGION`, `S3_<NAME>_ACCESS_KEY_ID` und `S3_<NAME>_SECRET_ACCESS_KEY`.

File-Bindings liegen unter `bindings/object-storage-s3/<bucket>/` und enthalten Endpoint, Bucket, Region, Access Key und Secret Key. Credential-Dateien sind owner-only; normales `baha app env` maskiert Access-/Secret-Keys.

Der aktuelle SeaweedFS-Provider erstellt fuer jeden logischen Bucket eine eigene bucket-scoped Identity und verifiziert Readiness mit einem realen authentifizierten SigV4-Put/Get. Ein shared Provider bedeutet damit keine geteilte Authorization zwischen Anwendungen oder Buckets.

Repository-Workloads verwenden den internen Compose-Endpoint ueber das BaseHarbor-eigene Object-Storage-Integrationsnetz; Host-Prozesse verwenden den geschuetzten Loopback-Endpoint. Beide Adressen sind Deployment-State und keine portable Application Identity.

Backup/Restore erfasst Object-Inhalte noch nicht. v0.4.6 verweigert deshalb `baha app backup` und `baha app restore` fuer Anwendungen mit Managed Object Storage, statt eine unvollstaendige Recovery-Einheit zu erzeugen oder zu akzeptieren.

## OTLP-Telemetrie-Export in v0.4.7

Anwendungen koennen providerneutralen OTLP-Export deklarieren:

```yaml
workload:
  compose: compose.yaml
  services:
    - api
    - worker

telemetry:
  otlp:
    signals:
      - traces
      - metrics
```

Die ausgewaehlten Workload-Services erhalten normale OpenTelemetry-Environment-Variablen. Es gibt keine BaseHarbor-spezifische Telemetrie-API und der Application Contract nennt weder Collector noch Tempo, Prometheus, Loki oder Grafana.

Das aktuelle v1-Binding verwendet OTLP HTTP/Protobuf Export. BaseHarbor kann es ueber den shared Managed OpenTelemetry Collector oder einen externen OTLP-Endpunkt aus Deployment-State erfuellen. Provider-Endpunkte und Authorization-Material sind kein portabler Application Intent.

Ein externes Ziel kann ueber `BASEHARBOR_OTLP_ENDPOINT` im Deployment-Environment gesetzt werden. Optionale sensitive OTLP-Header verwenden `BASEHARBOR_OTLP_HEADERS` und werden nur an der vertrauenswuerdigen Workload-/Provider-Grenze injiziert; sie landen nicht in `baseharbor.yaml`.

OTLP-Transport allein provisioniert niemals automatisch Prometheus, Loki, Tempo oder Grafana.

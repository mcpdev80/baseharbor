# Capability- und Provider-Modell

BaseHarbor trennt konsequent **was eine Anwendung braucht** von **welches Produkt diese Anforderung erfuellt**.

Dieses Dokument ist die Arbeitsmatrix fuer die Plattformarchitektur. Es haelt die portable Capability, den aktuellen oder geplanten BaseHarbor-Default-Provider und dokumentierte Austauschpfade fest.

Die verbindliche Architekturregel steht in ADR [0005-capabilities-not-products](decisions/0005-capabilities-not-products.md).

## Regeln

1. Application Contracts beschreiben Capabilities und portable Anforderungen.
2. Die Provider-Auswahl gehoert in Environment-/Plattformkonfiguration.
3. Ein mitgelieferter oder bevorzugter Provider ist eine Implementierungsentscheidung, keine Eigenschaft der Anwendung.
4. Fuer jeden Default-Provider muss mindestens ein dokumentierter Austauschpfad existieren.
5. Ein Provider-Wechsel muss den angeforderten Contract erfuellen oder klar fehlschlagen; Security, Haltbarkeit oder Verfuegbarkeit duerfen nie still reduziert werden.
6. Compose bleibt in v0.4 die vollstaendige Runtime-Implementierung, jetzt hinter einer expliziten Runtime-Provider-/Capability-Negotiation-Grenze. Kubernetes/OpenShift folgen spaeter und muessen denselben logischen Intent erhalten.
7. Deployment-spezifische Umsetzung erzeugt nicht automatisch eine neue portable Application Capability; `PortableContract` wird nur bewusst und versioniert erweitert.

## Komponentenmatrix

| Capability | Portable Schnittstelle / Intent | BaseHarbor-Default | Stand v0.4.0 | Austauschpfade / Alternativen | Architekturhinweis |
| --- | --- | --- | --- | --- | --- |
| Relationale SQL-Datenbank | Manifest-v1-PostgreSQL-Kompatibilitaetsinput, intern als `database.sql` im `PortableContract` normalisiert | PostgreSQL | implementiert | externe PostgreSQL-Instanz, Managed PostgreSQL/RDS-artige Dienste, Enterprise-PostgreSQL-Plattformen; andere SQL-Engines nur bei passender Semantik | PostgreSQL ist aktueller Referenzprovider, nicht der dauerhafte Capability-Name |
| Cache / Key-Value | Manifest-v1-Redis/Valkey-Kompatibilitaetsinput, intern als `cache.key-value` im `PortableContract` normalisiert | Valkey | implementiert | Redis, Dragonfly, Managed Redis/Valkey; andere KV-Systeme nur mit passender Semantik | Protokoll-/Feature-Anforderungen muessen echte Austauschbarkeit absichern |
| Secrets | `secrets` / benoetigte Secret-Namen + policy-gesteuerte Auslieferung | OpenBao | implementiert | Vault, Cloud Secret Stores, externe Provider-Adapter | Der App-Contract kennt Secret-Anforderungen, aber keine OpenBao-Pfade oder AppRoles |
| HTTP-Ingress / Reverse Proxy | spaeter `ingress.http` | kein BaseHarbor-managed Ingress-Provider in v0.4; app-eigene Compose-Exposition wird beobachtet | portable Capability geplant; app-eigene HTTP/HTTPS-Readiness implementiert | Caddy, Traefik, nginx, HAProxy, Kubernetes Gateway/Ingress, OpenShift Route | v0.4 prueft konventionelle app-eigene Publisher, provisioniert aber keinen BaseHarbor-Ingress |
| Object Storage | spaeter `object-storage.s3` / S3 API | SeaweedFS als geplanter Referenz-/Default-Provider | geplant | Garage, Ceph RGW, AWS S3 und kompatible Managed Services | S3 ist die Anwendungsgrenze; Topologie und Implementierung bleiben Provider-Sache |
| TLS-Zertifikats-Lifecycle | spaeter `tls.certificate` / X.509-Identitaet | providerabhaengig | Existing/BYOC fuer Repository-Compose-Deployment implementiert; portable Capability geplant | vorhandene/BYOC-Zertifikate, OpenBao PKI, ACME-Provider, cert-manager, OpenShift Service CA, Cloud-Zertifikatsdienste | v0.4 validiert/importiert/aktualisiert Existing-Zertifikate als Deployment-State; ACME/PKI/providerneutraler Intent bleiben Future Work |
| Externe Secret-Projektion | Provider-Integration, kein portabler App-Produktname | kein globaler Pflichtprovider | geplant/optional | External Secrets Operator, Secrets Store CSI, Vault/OpenBao Workload Identity, plattformnative Secret-Projektion | ESO darf niemals Teil des Application Contracts werden |
| Identity / SSO | spaeter `identity.oidc` / OIDC/OAuth2 | kein fest verdrahtetes Produkt | geplant | Authentik, Zitadel, Entra ID, Google Workspace, GitHub oder andere OIDC-Provider | BaseHarbor wertet Identity Claims aus; Apps duerfen nicht von einem konkreten IdP-Produkt abhaengen |
| Metrics | spaeter `metrics.openmetrics` | Prometheus als Referenz-/Default-Kandidat | geplant | VictoriaMetrics, Mimir und kompatible Backends | Collection, Query und Storage muessen austauschbar bleiben |
| Traces / Telemetrie-Transport | spaeter `telemetry.otel` / OpenTelemetry | OpenTelemetry | geplant | vendor-spezifische Backends hinter OTel-kompatiblen Exportern | OTel ist Standard-Schnittstelle, nicht nur ein Produkt |
| Logs | strukturierte App-/Runtime-Logs mit providerdefiniertem Transport | Loki als Referenz-/Default-Kandidat | Trusted-local Compose-Logzugriff implementiert; Backend-Abstraktion geplant | OpenSearch, Elasticsearch, VictoriaLogs und kompatible Stacks | `baha app logs` ist ein lokaler Operator-Workflow und bindet nicht an Loki |

## Runtime Provider und Capability Provider sind getrennte Achsen

Ein **Runtime Provider** entscheidet, wo und wie Workloads laufen:

```text
Compose
Kubernetes
OpenShift
```

Ein **Capability Provider** entscheidet, welches Produkt eine Ressourcenanforderung erfuellt:

```text
database.sql        -> PostgreSQL / Managed PostgreSQL
cache.key-value     -> Valkey / Redis / Managed Redis
object-storage.s3   -> SeaweedFS / Ceph RGW / AWS S3
secrets             -> OpenBao / Vault / Cloud Secret Store
ingress.http        -> Caddy / Kubernetes Gateway / OpenShift Route
```

Ein OpenShift-Runtime-Provider bedeutet deshalb nicht, dass jede Capability OpenShift-native sein muss. Ein Enterprise-Kunde kann OpenShift-Workloads mit einem externen PostgreSQL-Cluster, Vault und Ceph RGW kombinieren.

## Application Contract und Environment-Definition

Konzeptionelles Zielmodell:

```yaml
# zukuenftiger portabler Application Contract
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

Environment-/Provider-Mapping:

```yaml
providers:
  database: postgres
  cache: valkey
  objectStorage: seaweedfs
  secrets: openbao
  ingress: caddy
```

Enterprise-Mapping:

```yaml
providers:
  database: customer-postgres
  cache: managed-redis
  objectStorage: ceph-rgw
  secrets: vault
  ingress: openshift-route
```

Das spaetere Environment-/Provider-Policy-Schema wird in v0.4.0 bewusst noch nicht festgeschrieben. Deployment-eigener Runtime-Provider/Profile-State existiert bereits; breitere Environment-Policy bleibt Future Work.

## Provider-Conformance

Ein zukuenftiges Provider-Interface muss mehr ausdruecken als einen Produktnamen. Provider muessen Capabilities deklarieren und dagegen getestet werden, zum Beispiel:

- Protokoll-/API-Kompatibilitaet;
- Persistenz/Haltbarkeit;
- Backup-/Restore-Unterstuetzung;
- HA-/Replikationsunterstuetzung;
- Verschluesselung in Transit/at Rest, wenn gefordert;
- Identity-/Credential-Modell;
- Rotation und Revocation;
- Observability-Hooks;
- Lifecycle-Operationen;
- unterstuetzte Runtime-/Environment-Bedingungen.

Kann ein gewaehlter Provider eine angeforderte Garantie nicht erfuellen, muss BaseHarbor den Plan ablehnen statt die Garantie still abzusenken.

## v0.4.1-Grenze

v0.4.1 bleibt zur Laufzeit Compose-only. Zusaetzlich zum `PortableContract` und der Runtime-Provider-Grenze aus v0.4.0 fuehrt es den gemeinsamen Capability-/Provider-/Resource-/Binding-Core, fail-closed Capability-Provider-Negotiation, maschinenlesbare Lifecycle-Ergebnisse und geschuetzte Runtime-Metadaten fuer aufgeloeste PostgreSQL-, Valkey- und OpenBao-Bindings ein. Ein neues generisches oeffentliches Capability-/Provider-Manifest wird weiterhin nicht eingefuehrt.

Die operative Vollstaendigkeit des Compose-Providers bleibt erhalten; neu sind die Architekturgrenzen fuer spaetere Provider ohne Bruch der Manifest-v1-Kompatibilitaet.

Diese Seams sind keine Kubernetes/OpenShift-Unterstuetzung. Neues oeffentliches Capability-Schema, weitere Capability-Provider, HA, Managed Ingress, ACME/PKI-Automation und Kubernetes/OpenShift-Runtime-Provider bleiben Future Work.

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

| Capability | Portable Schnittstelle / Intent | BaseHarbor-Default | Stand v0.4.6 | Austauschpfade / Alternativen | Architekturhinweis |
| --- | --- | --- | --- | --- | --- |
| Relationale SQL-Datenbank | Manifest-v1-PostgreSQL-Kompatibilitaetsinput, intern als `database.sql` im `PortableContract` normalisiert | PostgreSQL | implementiert | externe PostgreSQL-Instanz, Managed PostgreSQL/RDS-artige Dienste, Enterprise-PostgreSQL-Plattformen; andere SQL-Engines nur bei passender Semantik | PostgreSQL ist aktueller Referenzprovider, nicht der dauerhafte Capability-Name |
| Cache / Key-Value | Manifest-v1-Redis/Valkey-Kompatibilitaetsinput, intern als `cache.key-value` im `PortableContract` normalisiert | Valkey | implementiert | Redis, Dragonfly, Managed Redis/Valkey; andere KV-Systeme nur mit passender Semantik | Protokoll-/Feature-Anforderungen muessen echte Austauschbarkeit absichern |
| Secrets | `secrets` / benoetigte Secret-Namen + policy-gesteuerte Auslieferung | OpenBao | implementiert; v0.4.5 bildet Identity/Credentials/Trust/Authorization/Secret-Referenzen ueber `secure-binding/v1` ab | Vault, Cloud Secret Stores, externe Provider-Adapter | Der App-Contract kennt Secret-Anforderungen, aber keine OpenBao-Pfade oder AppRoles; Security-Wiring ist providerneutral |
| HTTP/HTTPS-Exposition | `exposure.http/v1` | Caddy als Compose-Referenzprovider | in v0.4.4 implementiert; app-eigene Publisher bleiben beobachtet und werden nicht lifecycle-seitig uebernommen | Traefik, Kubernetes Gateway API/Ingress, OpenShift Route, Cloud-Traffic-Provider | Managed Exposure ist expliziter portabler Intent; Host-Ports, FQDNs, TLS-Dateien, Netze und Proxy-Konfiguration bleiben Deployment-/Provider-State |
| Object Storage | `object-storage.s3/v1` / S3 API | SeaweedFS als shared Compose-Referenzprovider | in v0.4.6 implementiert | Ceph RGW, AWS S3 und konforme S3-kompatible Managed Services | Logische Buckets und S3-Semantik sind Application-facing; SeaweedFS-Topologie, physische Bucket-/User-Identitaet und Endpoint-Platzierung bleiben Provider-State |
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
exposure.http       -> Caddy / Traefik / Kubernetes Gateway / OpenShift Route
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
  exposure: caddy
```

Enterprise-Mapping:

```yaml
providers:
  database: customer-postgres
  cache: managed-redis
  objectStorage: ceph-rgw
  secrets: vault
  exposure: openshift-route
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

## v0.4.6-Grenze

v0.4.6 bleibt zur Laufzeit Compose-only. Die v0.4-Linie umfasst jetzt den gemeinsamen Capability-/Provider-/Resource-/Binding-Core, geschuetzte Provider-Platzierung und Ownership, den Provider Integration Contract v1, deterministische Repository-Inspection, Managed Traffic ueber `exposure.http/v1`, mit `secure-binding/v1` eine gemeinsame Security-Grenze sowie `object-storage.s3/v1` mit SeaweedFS als lazy shared Compose-Referenzprovider.

Manifest v1 bleibt die unterstuetzte Kompatibilitaetsoberflaeche. Managed Exposure ist additiv und explizit; app-eigene Publisher bleiben app-eigener Observation-/Readiness-State.

Diese Seams sind keine Kubernetes/OpenShift-Unterstuetzung. Weitere S3/Object-Storage-Provider und oeffentliche Provider-Auswahl, HA-Profile, Managed ACME/OpenBao-PKI-Zertifikatsausstellung und Kubernetes/OpenShift-Runtime-Provider bleiben Future Work.


## Provider-Registry in v0.4.2

Provider-Platzierung bleibt geschuetzter Deployment-/Operator-State und ist kein portabler Application Intent.

- `shared`: von BaseHarbor verwalteter Provider fuer mehrere Anwendungen.
- `application`: von BaseHarbor verwalteter Provider exklusiv fuer eine Anwendung.
- `external`: bestehender/BYO Provider, den BaseHarbor referenziert, aber lifecycle-seitig nicht besitzt.

Aktuelle Referenzabbildung:

```text
OpenBao              -> shared
SeaweedFS            -> shared
PostgreSQL-Instanzen -> application-scoped
Valkey-Instanzen     -> application-scoped
```

Logische Ressourcen bleiben unabhaengig vom Provider-Scope Eigentum der Anwendung. Shared Provider bleiben bei Application-Lifecycle-Operationen bestehen; externe Provider werden von BaseHarbor nicht lifecycle-seitig veraendert.


## Provider Integration Contract v1

Alle nach v0.4.2 hinzukommenden Provider implementieren den gemeinsamen [Provider Integration Contract v1](provider-integration-contract.md).

Aktuelle versionierte Reference Claims:

- PostgreSQL: `database.sql/v1`
- Valkey: `cache.key-value/v1`
- OpenBao: `secrets/v1`
- Caddy: `exposure.http/v1`
- SeaweedFS: `object-storage.s3/v1`

Weitere S3-, Telemetrie-, Observability-, Messaging-, AI/MCP- und Vector-Provider definieren bzw. implementieren versionierte Capability Specifications. Produktdetails duerfen dadurch nicht in den portablen Application Contract gelangen.

Fuer kuenftige externe Provider ist gRPC/Protocol Buffers als Transport und OCI als Distribution vorgesehen. BaseHarbor bleibt fuer Capability-Semantik und Conformance autoritativ.

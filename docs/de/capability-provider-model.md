# Capability- und Provider-Modell

BaseHarbor trennt konsequent **was eine Anwendung braucht** von **welches Produkt diese Anforderung erfüllt**.

Dieses Dokument ist die Arbeitsmatrix für die Plattformarchitektur. Es hält die portable Capability, den aktuellen oder geplanten BaseHarbor-Default-Provider und dokumentierte Austauschpfade fest.

Die verbindliche Architekturregel steht in ADR [0005-capabilities-not-products](../decisions/0005-capabilities-not-products.md).

## Regeln

1. Application Contracts beschreiben Capabilities und portable Anforderungen.
2. Die Provider-Auswahl gehört in Environment-/Plattformkonfiguration.
3. Ein mitgelieferter oder bevorzugter Provider ist eine Implementierungsentscheidung, keine Eigenschaft der Anwendung.
4. Für jeden Default-Provider muss mindestens ein dokumentierter Austauschpfad existieren.
5. Ein Provider-Wechsel muss den angeforderten Contract erfüllen oder klar fehlschlagen; Security, Haltbarkeit oder Verfügbarkeit dürfen nie still reduziert werden.
6. Compose bleibt jetzt der Implementierungsfokus. Kubernetes/OpenShift werden später als Provider ergänzt, ohne die logische Capability der Anwendung neu zu definieren.

## Komponentenmatrix

| Capability | Portable Schnittstelle / Intent | BaseHarbor-Default | Stand v0.2.0 | Austauschpfade / Alternativen | Architekturhinweis |
| --- | --- | --- | --- | --- | --- |
| Relationale SQL-Datenbank | `database.sql` / Connection URL + nativer SQL-Treiber | PostgreSQL | implementiert | externe PostgreSQL-Instanz, Managed PostgreSQL/RDS-artige Dienste, Enterprise-PostgreSQL-Plattformen; andere SQL-Engines nur wenn deren Semantik zum deklarierten Contract passt | PostgreSQL ist Referenzprovider, nicht der dauerhafte Produktname der Capability |
| Cache / Key-Value | `cache.key-value` / Redis-kompatibles Protokoll, wenn angefordert | Valkey | implementiert | Redis, Dragonfly, Managed Redis/Valkey; andere KV-Systeme nur mit passender Semantik | Protokoll- und Feature-Anforderungen müssen präzise genug sein, damit keine falsche Austauschbarkeit vorgetäuscht wird |
| Secrets | `secrets` / benötigte Secret-Namen + policy-gesteuerte Auslieferung | OpenBao | implementiert | Vault, Cloud Secret Stores, externe Provider-Adapter | Der App-Contract kennt Secret-Anforderungen, aber keine OpenBao-Pfade oder AppRoles |
| HTTP-Ingress / Reverse Proxy | `ingress.http` | Caddy | geplanter Default | Traefik, nginx, HAProxy, Kubernetes Gateway/Ingress, OpenShift Route | Caddy ist der bevorzugte Compose-Provider, keine App-Abhängigkeit |
| Object Storage | `object-storage.s3` / S3 API | SeaweedFS | geplanter Default | Garage, Ceph RGW, AWS S3 und kompatible Managed Services | S3 ist die Anwendungsgrenze; Topologie und Implementierung bleiben Provider-Sache |
| TLS-Zertifikats-Lifecycle | `tls.certificate` / X.509-Identität | providerabhängig: Caddy/OpenBao/ACME bei Compose; cert-manager auf Kubernetes | geplant | vorhandene/BYOC-Zertifikate, OpenBao PKI, ACME-Provider, cert-manager, OpenShift Service CA, Cloud-Zertifikatsdienste | cert-manager ist ein Kubernetes-Provider und keine globale BaseHarbor-Vertragsabhängigkeit |
| Externe Secret-Projektion | Provider-Integration, kein portabler App-Produktname | kein globaler Pflichtprovider; ESO kann Adapter sein | geplant/optional | External Secrets Operator, Secrets Store CSI, Vault/OpenBao Workload Identity, plattformnative Secret-Projektion | ESO darf niemals Teil des Application Contracts werden |
| Identity / SSO | `identity.oidc` / OIDC/OAuth2 | kein fest verdrahtetes Produkt; Keycloak als bevorzugte Self-Hosted-Referenz | geplant | Authentik, Zitadel, Entra ID, Google Workspace, GitHub oder andere OIDC-Provider | BaseHarbor wertet Identity Claims aus; Apps dürfen nicht von Keycloak-spezifischen APIs abhängen |
| Metrics | `metrics.openmetrics` / OpenMetrics-kompatibles Scraping/Export | Prometheus als Referenz-/Default-Kandidat | geplant | VictoriaMetrics, Mimir und kompatible Backends | Collection, Query und Storage müssen austauschbar bleiben |
| Traces / Telemetrie-Transport | `telemetry.otel` / OpenTelemetry | OpenTelemetry | geplant | vendor-spezifische Backends hinter OTel-kompatiblen Exportern | OTel ist Standard-Schnittstelle, nicht nur ein weiteres austauschbares Produkt |
| Logs | `logs` / strukturierte App-/Runtime-Logs mit providerdefiniertem Transport | Loki als Referenz-/Default-Kandidat | geplant | OpenSearch, Elasticsearch, VictoriaLogs und kompatible Stacks | Log-Storage und Query-Backend sind Environment-Sache |

## Runtime Provider und Capability Provider sind getrennte Achsen

Ein **Runtime Provider** entscheidet, wo und wie Workloads laufen:

```text
Compose
Kubernetes
OpenShift
```

Ein **Capability Provider** entscheidet, welches Produkt eine Ressourcenanforderung erfüllt:

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
# baseharbor.yaml -- gehört der Anwendung
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
# konzeptionelle, environment-eigene Konfiguration
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

Der genaue Dateiname bzw. das Schema der Environment-Konfiguration wird von v0.2.0 bewusst noch nicht festgeschrieben. Die Architekturtrennung ist dagegen ab jetzt verbindlich.

## Provider-Conformance

Ein zukünftiges Provider-Interface muss mehr ausdrücken als einen Produktnamen. Provider müssen Capabilities deklarieren und dagegen getestet werden, zum Beispiel:

- Protokoll-/API-Kompatibilität;
- Persistenz/Haltbarkeit;
- Backup-/Restore-Unterstützung;
- HA-/Replikationsunterstützung;
- Verschlüsselung in Transit/at Rest, wenn gefordert;
- Identity-/Credential-Modell;
- Rotation und Revocation;
- Observability-Hooks;
- Lifecycle-Operationen;
- unterstützte Runtime-/Environment-Bedingungen.

Kann ein gewählter Provider eine angeforderte Garantie nicht erfüllen, muss BaseHarbor den Plan ablehnen statt die Garantie still abzusenken.

## v0.2.0-Grenze

v0.2.0 bleibt auf Compose und den bestehenden Implementierungen PostgreSQL, Valkey und OpenBao fokussiert. Dieses Dokument fügt keine Runtime-Features hinzu und ändert Manifest v1 nicht. Es definiert lediglich, wie neue Capabilities und zukünftige Provider-Arbeit nach v0.2.0 aufgebaut werden müssen.

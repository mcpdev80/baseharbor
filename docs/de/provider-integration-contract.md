# Provider Integration Contract v1

BaseHarbor standardisiert **Capability-Semantik und Provider-Lifecycle**, nicht einzelne Infrastrukturprodukte.

Das langfristige Ziel:

> Ein Provider kann von BaseHarbor, der Community oder einem Hersteller implementiert werden, ohne den portablen Application Contract zu aendern.

## Architektur

```text
Application Intent
      |
      v
Versionierte Capability Specification
      |
      v
Provider Registry / Auswahl
      |
      v
Provider Integration Contract v1
      |
      +-- eingebauter Reference Driver
      |
      +-- kuenftiger External Driver
              |
              +-- gRPC / Protocol Buffers
                      |
                      +-- Vendor/Community Provider
```

BaseHarbor definiert die verbindliche Semantik. Ein Provider darf erklaeren, dass er eine BaseHarbor Capability Specification implementiert; er darf diese Semantik nicht selbst neu definieren.

## Offene Standards als Bausteine

BaseHarbor verwendet offene Standards dort, wo sie Infrastruktur-Plumbing bereits sinnvoll loesen:

- **gRPC + Protocol Buffers** fuer die sprachneutrale externe Provider-API.
- **OCI Artifacts und OCI Distribution** fuer registry-neutrales Packaging und Distribution.
- **Open Service Broker API Konzepte** als Input fuer Provision/Update/Bind/Deprovision-Semantik.
- **Service Binding Konzepte** als Input fuer Workload Bindings.

Diese Bausteine ersetzen nicht die BaseHarbor-Regeln fuer Capabilities, Lifecycle, Ownership, Security und Verification.

Der oeffentliche Compatibility Contract darf nicht von HashiCorp go-plugin, Kubernetes, Docker, GitHub, einem Cloud-Anbieter oder einer proprietaeren BaseHarbor Registry abhaengen.

## Versionierung

Provider Protocol und Capability Specifications werden unabhaengig versioniert:

```text
Provider Protocol:         baseharbor.provider/v1
Capability Specification: database.sql/v1
```

Breaking Protocol-Semantik benoetigt eine neue Protocol-Major-Version.

Breaking application-facing Capability-Semantik benoetigt eine neue Capability-Specification-Major-Version.

## Lifecycle

Der bestehende Lifecycle bleibt autoritativ:

```text
resolve
  -> preflight aller Ressourcen
  -> provision/apply
  -> bind
  -> verify
```

Der Contract definiert ausserdem das vollstaendige Vokabular fuer kuenftige Provider:

```text
Describe
Preflight
Provision
Bind
Verify
Status
Update
Backup
Restore
Destroy
```

Optionale Operationen muessen explizit als supported/unsupported beschrieben werden. Unsupported darf niemals stillschweigend Erfolg bedeuten.

## Bestehende Reference Provider

| Provider | Capability Specification |
| --- | --- |
| PostgreSQL | `database.sql/v1` |
| Valkey | `cache.key-value/v1` |
| OpenBao | `secrets/v1` |

Neue Integrationen wie S3, OTLP, Prometheus, Loki, Tempo, Grafana, Messaging, AgentGateway, MCP und Vector Search muessen dieselbe Grenze verwenden.

## Externe Provider

Die versionierte Protocol-Spezifikation liegt unter:

```text
spec/provider/v1/provider.proto
```

Sie beschreibt die kuenftige sprachneutrale gRPC-Grenze.

Diese Architektur-Voraussetzung implementiert bewusst noch keinen externen Loader, OCI-Download oder gRPC-Runtime-Client. Eigene Provider bleiben zunaechst eingebaut und dienen als Reference Implementations.

## OCI Distribution

Kuenftige externe Provider sollen ueber normale OCI Registries verteilt werden koennen, z. B. GHCR, Quay, Harbor, Artifactory oder private OCI-kompatible Registries.

Ein BaseHarbor-Login oder eine zentrale proprietaere Registry darf nicht erforderlich sein.

## Secrets

Plaintext-Secrets gehoeren weder in Provider-Metadaten, Diagnostics, Registry-State noch in den portablen Application Contract.

Bindings mit Secret-Bedarf verwenden stabile Credential-/Secret-Referenzen, die BaseHarbor an einer vertrauenswuerdigen Grenze aufloest.

## Ownership

Die v0.4.2 Provider Registry bleibt autoritativ fuer shared, application-scoped und external/BYO Provider sowie fuer BaseHarbor-vs.-external Lifecycle Ownership.

Ein kompatibler externer Provider wird dadurch nicht automatisch von BaseHarbor lifecycle-seitig besessen.

## Conformance

Kompatibilitaet bedeutet nicht nur, dass ein Prozess startet.

Provider muessen statische Contract-Conformance und spaeter capability-spezifische reale Tests bestehen, z. B. SQL Query, Valkey PING, isolierter Secret-Zugriff, S3 put/get, OTLP Export oder Messaging publish/consume.

## Regel fuer alle folgenden Provider

Jede neue Capability-/Provider-Integration muss:

1. eine versionierte BaseHarbor Capability Specification definieren oder erweitern;
2. genau diese Specification deklarieren;
3. den gemeinsamen Lifecycle und die Provider Registry wiederverwenden;
4. Produktdetails aus dem portablen Application Intent heraushalten;
5. capability-spezifische Conformance Tests hinzufuegen;
6. spaetere Ersetzbarkeit durch einen konformen externen Provider erhalten.

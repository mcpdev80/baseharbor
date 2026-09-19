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
- **OCI Image/Distribution Specifications** fuer registry-neutrales Provider-Packaging und Distribution.
- **JSON Schema 2020-12** fuer Provider-Konfiguration und Validierung.
- **Open Service Broker API Konzepte** als Input fuer Provision/Update/Bind/Deprovision-Semantik.
- **Service Binding Konzepte** als Input fuer Workload Bindings.

Diese Bausteine ersetzen nicht die BaseHarbor-Regeln fuer Capabilities, Lifecycle, Ownership, Security und Verification.

Der oeffentliche Compatibility Contract darf nicht von HashiCorp go-plugin, Kubernetes, Docker, GitHub, einem Cloud-Anbieter oder einer proprietaeren BaseHarbor Registry abhaengen.

## GraphQL-Grenze

GraphQL ist **nicht** das Provider-Lifecycle-Protokoll von BaseHarbor.

Provider-Lifecycle benoetigt klar definierte Commands, Deadlines, Cancellation, Transport-Status, Idempotenz und Long-Running-Operations. Dafuer bleibt gRPC/Protocol Buffers die externe Provider-Grenze.

GraphQL kann spaeter fuer eine benutzerorientierte Control-Plane-/Web-UI-Query-API evaluiert werden, bleibt dann aber nur Adapter ueber dem gemeinsamen BaseHarbor Core.

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
| Caddy | `exposure.http/v1` |

Neue Integrationen wie S3, OTLP, Prometheus, Loki, Tempo, Grafana, Messaging, AgentGateway, MCP und Vector Search muessen dieselbe Grenze verwenden.

## Externe Provider

Die versionierte Protocol-Spezifikation liegt unter:

```text
spec/provider/v1/provider.proto
```

Sie beschreibt die kuenftige sprachneutrale gRPC-Grenze.

Diese Architektur-Voraussetzung implementiert bewusst noch keinen externen Loader, OCI-Download oder gRPC-Runtime-Client. Eigene Provider bleiben zunaechst eingebaut und dienen als Reference Implementations.

## RPC-Zuverlaessigkeit und Long-Running Operations

- Jeder RPC bekommt eine explizite Deadline.
- Cancellation wird propagiert.
- Read-only Calls duerfen kontrolliert retryt werden.
- Mutierende Calls werden nicht blind retryt und benoetigen einen Idempotency Key.
- Gleicher Input + gleicher Idempotency Key darf keine doppelten Ressourcen erzeugen.
- Transport-/Protocol-Fehler verwenden gRPC Status Codes; BaseHarbor-Diagnostics bleiben strukturierte Domain-Daten.
- Externe Provider stellen den standardisierten gRPC Health Service bereit.
- Lokale Provider bevorzugen Unix Domain Sockets.
- Remote Provider verwenden TLS und nach Moeglichkeit mTLS bzw. gleichwertige Workload Identity.

Provision, Bind, Unbind, Update, Backup, Restore und Destroy duerfen asynchron laufen und liefern eine stabile Operation-ID fuer `GetOperation`; `CancelOperation` ist best-effort.

## Protocol-Buffer-Evolution

Innerhalb `baseharbor.provider.v1` werden Field Numbers niemals geaendert oder wiederverwendet. Entfernte Felder/Enum-Werte werden reserviert. Neue v1-Felder sind additiv. Breaking Wire- oder Semantik-Aenderungen benoetigen eine neue Protocol-Major-Version.

## OCI Distribution

Kuenftige externe Provider werden ueber normale OCI Registries verteilt, z. B. GHCR, Quay, Harbor, Artifactory oder private OCI-kompatible Registries. Ein BaseHarbor-Login oder eine zentrale proprietaere Registry ist nicht erforderlich.

OCI wird **digest-first** behandelt: Tags dienen nur als veraenderliche Discovery-Aliase; installierter/gelockter State speichert und prueft den aufgeloesten Manifest-Digest.

Lauffaehige Container-Provider verwenden bevorzugt normale OCI Images. Multi-Platform Provider verwenden einen OCI Image Index.

Generische Nicht-Container-Pakete koennen OCI Artifact Guidance mit eigenem RFC-6838-Media-Type/`artifactType` nutzen.

Signaturen, SBOMs und Provenance werden ueber OCI `subject`/Referrers assoziiert; Clients beachten den OCI-Fallback, falls die Referrers API nicht verfuegbar ist. Fuer Supply-Chain-Verifikation werden offene Mechanismen wie Sigstore/cosign bzw. Notation und in-toto/SLSA bevorzugt, keine proprietaere BaseHarbor-Signatur.

## Provider-Konfigurationsschema

Provider-spezifische Operator-Konfiguration verwendet **JSON Schema 2020-12** und wird ueber `Describe` bekanntgegeben. Sie bleibt Deployment-/Operator-State und wird nicht Teil des portablen Application Intent. Secret-Werte gehoeren nicht in das Schema oder normale Config-Payloads.

## Secrets

Plaintext-Secrets gehoeren weder in Provider-Metadaten, Diagnostics, Registry-State noch in den portablen Application Contract.

Bindings mit Secret-Bedarf verwenden stabile Credential-/Secret-Referenzen, die BaseHarbor an einer vertrauenswuerdigen Grenze aufloest. Das Proto-`oneof` erzwingt die Trennung zwischen oeffentlichem Wert und Credential-Referenz.

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


## HTTP-Exposure-Provider-Grenze

`exposure.http/v1` ist die erste Traffic-Capability, die ueber diesen Provider Contract umgesetzt wird.

BaseHarbor trennt zwei Pfade bewusst:

```text
app-eigener Publisher  -> erkennen -> beobachten -> verifizieren
managed Exposure Intent -> aufloesen -> preflight -> provisionieren -> binden -> verifizieren
```

Observation uebertraegt niemals Lifecycle-Ownership an BaseHarbor. Nur expliziter Managed-Exposure-Intent wird als application-scoped Provider-Ressource registriert.

Der aktuelle Compose-Referenzprovider ist Caddy. Host-Ports, TLS-Dateien und generierte Proxy-Konfiguration bleiben geschuetzter Provider-/Deployment-State. Das stabile Exposure-Integrationsnetz gehoert dagegen zum generierten BaseHarbor-Workload-Override und wird vom Exposure-Provider nur als externes Netz konsumiert.

### Capability-eigene Binding-Parameter

Provider Protocol v1 transportiert capability-eigene, provider-neutrale Binding-Semantik durch Preflight, Provision und Bind. Fuer `exposure.http/v1` enthaelt das typisierte Binding:

- logischen Workload-Service;
- Zielport;
- HTTP/HTTPS-Transport;
- Sichtbarkeit `public|internal`.

Diese Felder werden durch die Capability Specification definiert, nicht durch Caddy oder einen anderen Provider. Provider-spezifische Konfiguration bleibt separate Operator-/Deployment-Konfiguration. Dadurch kann ein spaeterer konformer Provider denselben Application Intent verarbeiten, ohne `baseharbor.yaml` selbst lesen oder von der Go-Implementierung BaseHarbors abhaengen zu muessen.

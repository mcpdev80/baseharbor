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

Die v0.4.2 Provider Registry bleibt autoritativ fuer `shared`, `application` und `external`/BYO Provider sowie fuer BaseHarbor-vs.-external Lifecycle Ownership.

Die Scopes haben strikte Semantik. In der Compose-Runtime bedeutet `application` eine dedizierte Provider-Instanz als eigener Container/Project mit eigenem Provider-State fuer genau eine Application/Environment; sie wird niemals von einer anderen Application wiederverwendet. `shared` bedeutet eine BaseHarbor-eigene Platform-/Core-Runtime-Provider-Instanz, die lazy erzeugt wird und eine oder mehrere explizit autorisierte Applications bedienen darf. Die Anzahl aktueller Consumer aendert den Scope nicht. `external` bedeutet, dass der Provider-Lifecycle ausserhalb von BaseHarbor bleibt.

Ein kompatibler externer Provider wird dadurch nicht automatisch von BaseHarbor lifecycle-seitig besessen. Das Teilen einer Provider-Instanz bedeutet niemals automatisch geteilte Credentials, Datenzugriffe oder Cross-Application-Netzwerkfreigaben.

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

## Secure-Binding-Semantik

Ab v0.4.5 duerfen Provider-Lifecycle-Bindings das gemeinsame Modell `secure-binding/v1` tragen. Es ist die einheitliche providerneutrale Darstellung fuer Workload-Identitaet, Credential-Referenzen, Trust-Material, Authorization-Metadaten, Secret-Referenzen sowie deklarierte Renewal-/Rotation-/Revocation-Unterstuetzung.

Regeln:

- Plaintext-Credentials, Tokens, Private Keys und Secret-Werte sind im Provider-Protokoll verboten;
- Provider-spezifische Security-Interna wie OpenBao-Pfade/AppRoles/Policies, Kubernetes-Secret-Namen oder Cloud-Secret-Objekt-IDs bleiben Provider-/Deployment-State;
- Secure-Binding-Metadaten werden vor dem Provider-Preflight und damit vor Mutation validiert;
- Shared Provider Infrastructure bedeutet niemals automatisch Shared Authorization;
- Provider uebersetzen Least-Privilege-Authorization-Metadaten in ihr natives ACL-/Policy-Modell;
- Security-Diagnostik bleibt maschinenlesbar und secret-safe;
- `WorkloadBinding.security` in `baseharbor.provider/v1` bildet dieselbe Semantik ab.

Die bestehende OpenBao-/Runtime-Broker-/mTLS-Implementierung ist die erste Referenzrealisierung. v0.4.5 extrahiert deren stabile Semantik, ersetzt sie aber nicht.


## S3-Object-Storage-Provider-Grenze in v0.4.6

`object-storage.s3/v1` ist die portable Capability-Grenze. Die Anwendung besitzt die logische Bucket-Identitaet; Provider-Platzierung, physische Bucket-Namen, IAM-Objekte, Endpoint-Platzierung, Storage-Topologie und implementierungsspezifischer State sind kein Application Intent.

Die aktuelle Compose-Referenzimplementierung verwendet einen lazy shared SeaweedFS-Provider. Er wird nur materialisiert, wenn eine Anwendung S3-Object-Storage explizit anfordert. Logische Buckets bleiben Application-owned Ressourcen, obwohl Provider-Prozess und Storage Plane geteilt werden.

Jeder logische Bucket erhaelt unabhaengige bucket-scoped Credentials, die ueber `secure-binding/v1` beschrieben werden. Intern verwendet der Provider SeaweedFS IAM; Klartext-Credentials werden nur an der trusted Binding-/Runtime-Grenze aufgeloest und gelangen weder in Provider-Registry-Metadaten noch in portable Capability-Diagnostik.

Conformance fuer diese Capability prueft authentifiziertes S3-Verhalten einschliesslich Put/Get und nicht nur Prozess-Liveness. Ein spaeterer Ceph-RGW-, AWS-S3- oder anderer konformer Provider muss dieselbe Application-facing Semantik ohne Manifest-Umschreibung erfuellen.

Die aktuelle oeffentliche CLI bietet noch keine allgemeine Auswahl/dynamisches Laden externer Provider. Das bleibt Operator-/Provider-Plattform-Arbeit; Capability und Provider-Protokoll sind bereits so geschnitten, dass der Application Contract spaeter nicht geaendert werden muss.

## OTLP-Provider-Grenze in v0.4.7

`telemetry.otlp/v1` verwendet denselben Provider Integration Contract wie alle anderen Capabilities. Die OTLP-Protokollsemantik gehoert BaseHarbor; der OpenTelemetry Collector ist eine Referenzprovider-Implementierung.

Der aktuelle Managed-Compose-Provider ist lazy/shared. Ein externer OTLP-Endpunkt wird als externe Provider-Platzierung abgebildet und bleibt lifecycle-seitig extern owned. Provider-spezifische Endpunkte, Authorization-Header und Collector-Konfiguration bleiben Deployment-/Provider-State.

Conformance verlangt fail-closed Preflight, idempotente Provisionierung/Bindings und einen echten OTLP-HTTP/Protobuf-Export, den der gewaehlte Endpunkt akzeptiert. Ein nur laufender Collector-Prozess reicht nicht.

Das OTLP-Binding ist Capability-owned und in Provider Protocol v1 typisiert. Die Anforderung darf weder Prometheus, Loki, Tempo, Grafana noch andere nicht angeforderte Observability-Produkte implizit provisionieren.


## Metrics-/Prometheus-Conformance in v0.4.8

Prometheus ist der erste Reference Provider fuer `metrics/v1`. Die Provider-Grenze bleibt dabei dieselbe wie fuer spaetere VictoriaMetrics-/Mimir- oder Community-Implementierungen.

Conformance verlangt mindestens:

- fail-closed Preflight fuer Direction, Signalformat und Source-Endpunkt;
- idempotente shared Provider-Provisionierung;
- automatische Target-Registrierung ohne manuelle Prometheus-Konfiguration;
- Application-/Environment-/Service-/Source-Attribution;
- Isolation gleichnamiger Workload-Services verschiedener Anwendungen;
- echte Scrape-/Ingestion-Verifikation mit erfolgreichem `up=1`;
- Entfernen nur der zur betroffenen Application gehoerenden Target-Bindings;
- keine Credentials oder Secret-Werte in Target-State oder normalen Diagnostics;
- kein implizites Provisioning von Grafana, Loki oder Tempo.

Collection-Policy ist Deployment-/Operator-State. Eine deklarierte `metrics/v1`-Source autorisiert nicht automatisch Collection in jeder Umgebung.

## Provider-Placement, Sharing Boundaries und Runtime-Isolation

Provider-Placement ist eine BaseHarbor-weite Deployment-/Operator-Entscheidung. Sie ist unabhaengig von Application Intent, Runtime-Topologie und konkreter Produktauswahl.

```text
Application Intent
        |
        v
Capability
        |
        v
Provider-Aufloesung
        |
        v
Provider-Placement
   +----+------------------+
   |                       |
application             shared ---------------- external
                           |
                           +-- optionale Sharing Boundary
        |
        v
Isolation / Deployment Boundary
        |
        v
Runtime-/Provider-Implementierung
```

Die kanonischen Placement-Scopes bleiben exakt `application`, `shared` und `external`. Eine Sharing Boundary ist eine optionale Eigenschaft von `shared` und kein vierter Scope.

Ein Shared Provider ist niemals automatisch fuer alle Applications erreichbar. Zugriff bleibt explizit, least-privilege und deny-by-default. Eine Sharing Boundary erlaubt es dem Operator, genau eine Provider-Instanz bewusst fuer eine ausgewaehlte Gruppe von Applications gemeinsam zu nutzen, waehrend andere Applications ausserhalb dieser Trust Boundary bleiben.

Provider-Implementierungen deklarieren, welche Placements sie unterstuetzen. Wenn die Policy ein Placement aufloest, das der ausgewaehlte Provider nicht erfuellen kann, bricht BaseHarbor vor jeder Mutation fail-closed ab, statt still auf ein anderes Placement auszuweichen.

Der portable Application Contract enthaelt weder Provider-Placement noch Sharing Boundary, Lifecycle Ownership oder Runtime-Isolationsmechanik. Der Entwickler beschreibt weiterhin nur die benoetigten Capabilities. BaseHarbor und Deployment Policy loesen die Infrastrukturdetails auf.

Placement bleibt ausserdem von Runtime-spezifischer Isolation getrennt. Heute kann Compose Grenzen ueber Projekte, Netze und Volumes realisieren. Spaetere Kubernetes-/OpenShift-Runtimes koennen dieselben logischen Grenzen auf Namespaces/Projects, clusterweite Infrastruktur, Helm Releases, Operators, NetworkPolicies oder andere native Mechanismen abbilden, ohne den Application Intent zu aendern.

Der Installations-Scope eines spaeteren Operators ist nicht dasselbe wie Provider-Placement oder Resource-Scope. Ein clusterweit installierter Operator kann application-scoped oder sharing-boundary-scoped Ressourcen verwalten.

Mehrere BaseHarbor-Installationen sind daher nicht notwendig, nur weil Gruppen von Applications bestimmte Provider gemeinsam nutzen. Getrennte BaseHarbor-Control-Planes bleiben echten administrativen, Trust-Domain-, Infrastruktur- oder Compliance-Grenzen vorbehalten.

Der aktuelle Implementierungsumfang bleibt Docker/Podman Compose. Kubernetes-/OpenShift-Abbildungen sind hier nur Architektur-Kompatibilitaetsanforderungen und noch keine implementierte Runtime-Funktionalitaet.

## Progressive Disclosure und explizite Kontrolle

BaseHarbor muss standardmaessig einfach sein, ohne dadurch unflexibel zu werden.

Der normale Entwicklerpfad soll nur Application Intent benoetigen und sichere, nachvollziehbare Defaults verwenden:

```text
Entwickler deklariert Capability
        |
        v
BaseHarbor erkennt/loest sinnvolle Defaults auf
        |
        v
plan -> preflight -> apply -> verify
```

Fortgeschrittene Nutzer und Operatoren muessen Deployment-Entscheidungen weiterhin explizit festlegen koennen, soweit die Plattform sie unterstuetzt. Dazu gehoeren insbesondere Provider-Auswahl, Provider-Placement, optionale Sharing Boundary, Lifecycle Ownership soweit anwendbar, externe Provider-Referenzen, Isolation-/Deployment-Policy sowie unterstuetzte Provider-/Runtime-Optionen.

Das Bedienmodell folgt damit Progressive Disclosure:

```text
einfacher Pfad
  -> automatische sichere Defaults

fortgeschrittener Pfad
  -> explizite Deployment-/Operator-Policy

Expertenpfad
  -> vollstaendig spezifizierte unterstuetzte Provider-/Runtime-Realisierung
```

Explizite Kontrolle darf nicht dazu fuehren, dass Infrastrukturdetails in den portablen Application Contract gelangen. Portabler Application Intent bleibt produktneutral; konkrete Infrastrukturentscheidungen gehoeren in Deployment-/Operator-Konfiguration und die entsprechenden Control Surfaces.

BaseHarbor muss den aufgeloesten Plan vor der Mutation sichtbar machen, damit Nutzer erkennen koennen, welche Defaults gewaehlt wurden, und unterstuetzte Entscheidungen bewusst ueberschreiben koennen. Explizite Nutzer-/Operator-Konfiguration hat Vorrang vor Defaults, darf aber Capability-Conformance, Security Boundaries, Validierung oder Fail-Closed-Verhalten niemals umgehen.

Das Ziel lautet: einfach, wenn Infrastrukturdetails egal sind; praezise steuerbar, wenn sie wichtig sind.

## Convention by default, Configuration by choice

BaseHarbor folgt ueber alle Capabilities und Runtimes hinweg einem gemeinsamen UX- und Architekturprinzip:

> **Convention by default, configuration by choice.**

Der Default-Pfad minimiert Entscheidungen. BaseHarbor erkennt, was sicher erkennbar ist, waehlt sichere und nachvollziehbare Defaults, zeigt den aufgeloesten Plan und verwendet danach den normalen Validierungs- und Lifecycle-Pfad.

Wer mehr Kontrolle moechte, kann unterstuetzte Deployment-Entscheidungen schrittweise explizit ueberschreiben, ohne den portablen Application Intent zu veraendern.

```text
Default
  -> nur Capabilities
  -> sichere automatische Provider-/Placement-/Runtime-Defaults

Advanced
  -> Provider / Placement / Sharing / externe Referenzen explizit

Expert
  -> unterstuetzte Naming-, Topology-, Runtime- und Provider-Realisierungs-Hints
```

Optionale Expert-Control kann zum Beispiel stabile Resource-Prefixes, logische Hostnamen, Compose-Projekt-/Netz-/Volume-Namen, DNS-Aliase und spaeter Kubernetes-/OpenShift-Namespace-/Project-Naming umfassen. Ephemere runtime-generierte Identitaeten wie Replica- oder Pod-Instanznamen bleiben Runtime-eigen, solange die Runtime keinen sicheren stabilen Override ausdruecklich unterstuetzt.

Jedes konfigurierbare Feld muss klare Semantik besitzen:

- stabil und sicher ueberschreibbar;
- nur Hint/Template;
- generiert/runtime-owned und nicht ueberschreibbar.

Overrides werden nur akzeptiert, wenn aktive Runtime und Provider sie sicher und deterministisch umsetzen koennen. Security, Ownership, Reconciliation, Conformance und Fail-Closed-Validierung duerfen dadurch niemals umgangen werden.

Einfacher und Experten-Pfad verwenden denselben Core. Erweiterte Flexibilitaet darf weder einen zweiten Application Contract noch einen parallelen Lifecycle erzeugen.

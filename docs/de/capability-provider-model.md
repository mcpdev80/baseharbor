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

| Capability | Portable Schnittstelle / Intent | BaseHarbor-Default | Stand bis v0.4.10 | Austauschpfade / Alternativen | Architekturhinweis |
| --- | --- | --- | --- | --- | --- |
| Relationale SQL-Datenbank | Manifest-v1-PostgreSQL-Kompatibilitaetsinput, intern als `database.sql` im `PortableContract` normalisiert | PostgreSQL | implementiert | externe PostgreSQL-Instanz, Managed PostgreSQL/RDS-artige Dienste, Enterprise-PostgreSQL-Plattformen; andere SQL-Engines nur bei passender Semantik | PostgreSQL ist aktueller Referenzprovider, nicht der dauerhafte Capability-Name |
| Cache / Key-Value | Manifest-v1-Redis/Valkey-Kompatibilitaetsinput, intern als `cache.key-value` im `PortableContract` normalisiert | Valkey | implementiert | Redis, Dragonfly, Managed Redis/Valkey; andere KV-Systeme nur mit passender Semantik | Protokoll-/Feature-Anforderungen muessen echte Austauschbarkeit absichern |
| Secrets | `secrets` / benoetigte Secret-Namen + policy-gesteuerte Auslieferung | OpenBao | implementiert; v0.4.5 bildet Identity/Credentials/Trust/Authorization/Secret-Referenzen ueber `secure-binding/v1` ab | Vault, Cloud Secret Stores, externe Provider-Adapter | Der App-Contract kennt Secret-Anforderungen, aber keine OpenBao-Pfade oder AppRoles; Security-Wiring ist providerneutral |
| HTTP/HTTPS-Exposition | `exposure.http/v1` | Caddy als Compose-Referenzprovider | in v0.4.4 implementiert; app-eigene Publisher bleiben beobachtet und werden nicht lifecycle-seitig uebernommen | Traefik, Kubernetes Gateway API/Ingress, OpenShift Route, Cloud-Traffic-Provider | Managed Exposure ist expliziter portabler Intent; Host-Ports, FQDNs, TLS-Dateien, Netze und Proxy-Konfiguration bleiben Deployment-/Provider-State |
| Object Storage | `object-storage.s3/v1` / S3 API | SeaweedFS als shared Compose-Referenzprovider | Deployment-time Buckets seit v0.4.6; autorisiertes Runtime Create/Get/Delete nach v0.4.7 ueber den Runtime Provider Executor implementiert | Ceph RGW, AWS S3 und konforme S3-kompatible Managed Services | Logische Buckets und S3-Semantik sind Application-facing; SeaweedFS-Topologie, physische Bucket-/User-Identitaet und Endpoint-Platzierung bleiben Provider-State; provider-globale Credentials bleiben an der Executor-Grenze |
| TLS-Zertifikats-Lifecycle | spaeter `tls.certificate` / X.509-Identitaet | providerabhaengig | Existing/BYOC fuer Repository-Compose-Deployment implementiert; portable Capability geplant | vorhandene/BYOC-Zertifikate, OpenBao PKI, ACME-Provider, cert-manager, OpenShift Service CA, Cloud-Zertifikatsdienste | v0.4 validiert/importiert/aktualisiert Existing-Zertifikate als Deployment-State; ACME/PKI/providerneutraler Intent bleiben Future Work |
| Externe Secret-Projektion | Provider-Integration, kein portabler App-Produktname | kein globaler Pflichtprovider | geplant/optional | External Secrets Operator, Secrets Store CSI, Vault/OpenBao Workload Identity, plattformnative Secret-Projektion | ESO darf niemals Teil des Application Contracts werden |
| Identity / SSO | spaeter `identity.oidc` / OIDC/OAuth2 | kein fest verdrahtetes Produkt | geplant | Authentik, Zitadel, Entra ID, Google Workspace, GitHub oder andere OIDC-Provider | BaseHarbor wertet Identity Claims aus; Apps duerfen nicht von einem konkreten IdP-Produkt abhaengen |
| Metrics | `metrics/v1` / OpenMetrics-kompatible HTTP-Exposition | Prometheus 3.14.0 als Compose-Referenzprovider; shared Default, benannte Shared Boundaries und application-scoped Placement | v0.4.10: Application-Sources plus generische, policy-autorisierte Metrics-Signale verwalteter Provider; automatische Registrierung und echte Scrape-Verifikation | VictoriaMetrics, Mimir und kompatible Backends | Applications deklarieren nur ihre Metrics-Source. Provider koennen eigene sichere Observability-Signale ueber den Provider Integration Contract anbieten; Prometheus enthaelt keine produktspezifischen Scrape-Sonderfaelle |
| OTLP-Telemetrie-Transport | `telemetry.otlp/v1` / OTLP HTTP-Protobuf Export | OpenTelemetry Collector 0.161.0 als shared Compose-Referenzprovider | in v0.4.7 implementiert; externe OTLP-Endpunkte ohne Lifecycle-Ownership werden unterstuetzt | jeder konforme OTLP-HTTP/Protobuf-Endpunkt, managed oder extern | OpenTelemetry ist das Oekosystem; OTLP ist die portable Protokollgrenze; der Collector ist Provider-Implementierung und keine Application Identity |
| Logs | `logs/v1` Platform-Log-Source-Lifecycle; Application Intent bleibt produktneutral | Loki 3.7.8 + Alloy 1.19.2 als Compose-Referenzprovider | v0.4.9: Policy-gesteuerte Workload-Collection, shared/application Placement, geschuetzte Registry-Ownership und echte Query-Verifikation | OpenSearch, Elasticsearch, VictoriaLogs und kompatible Stacks ueber konforme Provider | Loki bleibt Provider-State statt Application Intent; lokales `baha app logs` bleibt ein unabhaengiger Trusted-local Operator-Pfad |
| Trace Storage | `traces/v1` als Platform-Facility fuer ueber `telemetry.otlp/v1` transportierte Trace-Signale | Tempo 3.0.2 als shared Compose-Referenzprovider | v0.4.10: policy-gesteuerter shared Trace Store, Managed OTel-to-Tempo-Routing und echte Query-Verifikation | Jaeger und kompatible Trace-Backends ueber konforme Provider | Instrumentierung/Transport bleiben vom Storage getrennt. Tempo-Topologie und -Storage sind Provider-State und niemals Application Intent |

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

## v0.4.10-Grenze

v0.4.10 bleibt zur Laufzeit Compose-only. Die v0.4-Linie umfasst den gemeinsamen Capability-/Provider-/Resource-/Binding-Core, geschuetzte Provider-Platzierung und Ownership, den Provider Integration Contract v1, deterministische Repository-Inspection, Managed Traffic ueber `exposure.http/v1`, `secure-binding/v1`, `object-storage.s3/v1`, providerneutrales `telemetry.otlp/v1`, `metrics/v1`, `logs/v1` und `traces/v1`. Provider-Descriptoren koennen Observability-Signale generisch deklarieren; Collection-Policy und Placement entscheiden, welche Signale verbunden werden. Explizite gerichtete Cross-Application-Connectivity bleibt eine getrennte deny-by-default Platform-Policy.

Manifest v1 bleibt die unterstuetzte Kompatibilitaetsoberflaeche. Managed Exposure und Metrics-Sources sind additiv und explizit; app-eigene Publisher bleiben app-eigener Observation-/Readiness-State. Provider-Sharing erzeugt niemals automatisch Cross-Application-Connectivity.

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
- OpenTelemetry Collector: `telemetry.otlp/v1`

Weitere S3-, Telemetrie-, Observability-, Messaging-, AI/MCP- und Vector-Provider definieren bzw. implementieren versionierte Capability Specifications. Produktdetails duerfen dadurch nicht in den portablen Application Contract gelangen.

Fuer kuenftige externe Provider ist gRPC/Protocol Buffers als Transport und OCI als Distribution vorgesehen. BaseHarbor bleibt fuer Capability-Semantik und Conformance autoritativ.

## Provider-Placement, Sharing Boundaries und Runtime-Realisierung

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
Runtime-Realisierung des gewaehlten Placements
```

Die kanonischen Placement-Scopes bleiben exakt `application`, `shared` und `external`. Eine Sharing Boundary ist eine optionale Eigenschaft von `shared` und kein vierter Scope.

Placement hat strikte Provider-Instanz-Semantik:

- `application`: genau eine von BaseHarbor verwaltete Provider-Instanz fuer exakt eine Application/Environment. In der Compose-Runtime bedeutet das einen dedizierten Provider-Container/-Project mit eigenem Provider-State; die Instanz wird niemals von einer anderen Application wiederverwendet.
- `shared`: genau eine BaseHarbor Platform-/Core-Runtime-Provider-Instanz, die lazy beim ersten Bedarf erzeugt wird und von einer oder mehreren explizit autorisierten Applications wiederverwendet werden kann. Ein Provider bleibt `shared`, auch wenn ihn aktuell nur eine Application nutzt.
- `external`: eine ausserhalb von BaseHarbor betriebene Provider-Instanz. BaseHarbor kann sie anbinden, besitzt oder provisioniert ihren Lifecycle aber nicht.

Ein Shared Provider ist niemals automatisch fuer alle Applications erreichbar. Zugriff bleibt explizit, least-privilege und deny-by-default. Eine Sharing Boundary erlaubt es dem Operator, genau eine Platform-Provider-Instanz bewusst fuer eine ausgewaehlte Gruppe von Applications gemeinsam zu nutzen, waehrend andere Applications ausserhalb dieser Trust Boundary bleiben. Das Teilen des Provider-Prozesses bedeutet niemals automatisch geteilte logische Application-Ressourcen, Credentials, Daten oder Netzwerkzugriffe.

Shared Provider sind on-demand Platform-Infrastruktur und keine pauschalen Bootstrap-Abhaengigkeiten. Existiert bereits eine kompatible Shared-Instanz in der BaseHarbor Platform-/Core-Runtime, wird sie wiederverwendet statt eine weitere Provider-Instanz zu starten.

Provider-Implementierungen deklarieren, welche Placements ihr aktueller Adapter tatsaechlich unterstuetzt. Wenn eine Policy ein Placement aufloest, das der ausgewaehlte Provider-Adapter nicht wahrheitsgemaess realisieren kann, bricht BaseHarbor vor jeder Mutation fail-closed ab, statt still auf ein anderes Placement auszuweichen.

Der portable Application Contract enthaelt weder Provider-Placement noch Sharing Boundary, Lifecycle Ownership oder Runtime-Realisierungsmechanik. Der Entwickler beschreibt weiterhin nur die benoetigten Capabilities. BaseHarbor und Deployment Policy loesen die Infrastrukturdetails auf.

Die Placement-Semantik bleibt runtime-unabhaengig, auch wenn die Realisierung unterschiedlich ist. Compose realisiert einen `application` Provider als dedizierten Container/Project und einen `shared` Provider als BaseHarbor Platform-/Core-Runtime-Infrastruktur. Spaetere Kubernetes-/OpenShift-Runtimes koennen dieselbe Semantik mit dedizierten/geteilten plattformnativen Ressourcen, Namespaces/Projects, Operatoren oder anderen Isolationsmechanismen umsetzen, ohne den Application Intent zu veraendern.

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

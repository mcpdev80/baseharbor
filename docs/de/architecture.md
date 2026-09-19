# Architektur

BaseHarbor ist eine sichere, modulare und selbst gehostete Plattformgrundlage für unabhängige Anwendungen. Es verwaltet Backend-Infrastruktur und Anwendungslebenszyklus, ohne Anwendungen an ein BaseHarbor-SDK zu binden.

Das langfristige Ziel ist ein durchgängiger Weg von lokaler Entwicklung und Homelab über strengere Produktionsumgebungen bis zu Kubernetes-/OpenShift-Enterprise-Deployments, ohne den logischen Anwendungsvertrag neu definieren zu müssen.

## Grundprinzipien

- Anwendungen bleiben unabhängig und besitzen ihren eigenen Code sowie ihre fachliche Konfiguration.
- `app.name` ist die stabile logische Anwendungsidentität; `app.environment` ist Deployment-Kontext.
- Application Contracts beschreiben Capabilities, nicht konkrete Infrastrukturprodukte.
- Die Provider-Auswahl gehört zum Environment bzw. zur Plattform und bleibt austauschbar.
- Jeder BaseHarbor-Default-Provider braucht eine definierte Provider-Grenze und einen dokumentierten Austauschpfad.
- BaseHarbor besitzt nur die von ihm bereitgestellten Infrastrukturressourcen und Lifecycle-Mechaniken.
- Standardprotokolle und Standardvariablen bleiben die Anwendungsgrenze.
- Sicherheitsgrenzen werden fail-closed und mit Least Privilege umgesetzt.
- Compose ist der vollständige aktuelle Provider und bleibt first-class.
- Spätere Kubernetes-/OpenShift-Provider müssen dieselben logischen Anforderungen abbilden, statt einen neuen App-Vertrag zu erzwingen.
- Environment/Risiko und Deployment-Topologie sind getrennte Konzepte.
- CLI, API/WebGUI und Operator sind Adapter ueber denselben Domain-/Lifecycle-Core und duerfen keine getrennten Wahrheiten fuer Plan, Status, Readiness oder Security entwickeln.

## Gemeinsamer Core und mehrere Bedienoberflaechen

BaseHarbor wird als **ein gemeinsamer Application-/Lifecycle-Core mit mehreren Control Surfaces** aufgebaut.

```text
                         BaseHarbor Core
              +--------------------------------+
              | PortableContract                |
              | Input Resolution                |
              | Plan / Preflight / Apply        |
              | Verify / Status / Diagnostics   |
              | Recovery / Update               |
              | Provider Selection/Capabilities |
              +---------------+----------------+
                              |
          +-------------------+-------------------+
          |                   |                   |
          v                   v                   v
       baha CLI            HTTP API        Operator Controller
                              |
                              v
                         schlanke WebGUI
```

`baha` bleibt die primaere lokale Developer-/Operator-Oberflaeche. Zukuenftige HTTP-API, WebGUI und Kubernetes/OpenShift-Operatoren muessen dieselben Domain-Modelle und Lifecycle-Semantiken verwenden, statt sie separat zu implementieren.

Die WebGUI bleibt bewusst schlank und nutzt die BaseHarbor-API. Sie darf weder `baha` per Shell aufrufen noch Validation, Policy, Secret-Schutz oder Readiness-Regeln umgehen.

Ein spaeterer Kubernetes/OpenShift-Operator reconciled BaseHarbor Desired State ueber dasselbe Application-/Provider-Modell. Er verwendet idempotente Reconciliation und provider-native Observation statt imperative CLI-Kommandos einzuwickeln.

Lifecycle- und Diagnoseergebnisse sollen zuerst maschinenlesbar sein. CLI, API, WebGUI und Operator-Status sind verschiedene Darstellungen derselben Runtime-Wahrheit.

Siehe ADR [0009](https://github.com/mcpdev80/baseharbor/blob/main/docs/decisions/0009-shared-core-multiple-control-surfaces.md).

## Ebenen

```text
Application Repository
  └── baseharbor.yaml
        ↓
Capabilities / portable Anforderungen
        ↓
baha CLI / Control Plane
        ↓
Environment / Policy / Provider-Auswahl
        ↓
Runtime Provider
  ├── Compose       (aktuell)
  ├── Kubernetes    (später)
  └── OpenShift     (später/Enterprise)
        ↓
Capability Provider + Application Workload
```

Die Control Plane ist heute benutzer-/maschinenbezogen. Anwendungsressourcen sind nach Anwendung und Umgebung isoliert.

## Capability ist nicht Produkt

Eine App soll beispielsweise SQL-Datenbank, Key-Value-Cache, S3-Object-Storage, Secrets oder TLS anfordern können, ohne dauerhaft an PostgreSQL, Valkey, SeaweedFS, OpenBao oder Caddy gekoppelt zu sein.

Ein lokales Environment kann diese Capabilities mit BaseHarbor-Defaults erfüllen. Ein Enterprise-Environment kann stattdessen Kundendienste wie externes PostgreSQL, Managed Redis, Ceph RGW, Vault oder OpenShift Routes verwenden. Die Application-YAML soll dafür nicht neu geschrieben werden müssen.

Die vollständige Matrix steht unter [Capability- und Provider-Modell](capability-provider-model.md). Die verbindliche Architekturentscheidung ist ADR [0005](decisions/0005-capabilities-not-products.md).

## Portabler Vertrag und Deployment-State

`baseharbor.yaml` Manifest v1 bleibt der repository-eigene Desired-State-Kompatibilitaetsvertrag. v0.4 uebersetzt daraus portablen Anwendungs-Intent in den providerneutralen `PortableContract`; Compose-spezifische Kompatibilitaets- und Deployment-Details bleiben ausserhalb dieses Views und werden in geschuetztem BaseHarbor-State gehalten.

Zu diesem Deployment-State gehoeren in v0.4 unter anderem:

- Public FQDN für die aktuelle Compose-Realisierung;
- TLS-Modus des Deployments;
- normalisierte Existing/BYOC-Zertifikat-/Key-Dateien;
- automatisch gewählte Host-Port-Fallbacks;
- generierte Compose-Overrides und Runtime-Identity-Material.

Diese Werte sind Provider-/Operator-State und keine neuen portablen Manifest-Anforderungen.

## Mehrere Instanzen und HA

Mehrere benannte PostgreSQL- oder Valkey-Instanzen sind mehrere unabhängige logische Dienste. Sie sind nicht automatisch Replikate. Zukünftiges HA wird als Topologie hinter einem stabilen logischen Dienst modelliert, damit der Anwendungsvertrag gleich bleibt.

## Provider-Grenze

Compose-Projektnamen, Netzwerke, Host-Ports, Volumes und generierte Overrides sind Implementierungsdetails des aktuellen Providers. Spätere Kubernetes-Ressourcennamen, Ingresses oder OpenShift-Routes wären ebenfalls Providerdetails und dürfen nicht zu fachlichen Abhängigkeiten der Anwendung werden.

Manifest v1 enthaelt aus Kompatibilitaetsgruenden weiterhin produktorientierte PostgreSQL/Redis/Valkey-Felder. v0.4 normalisiert deren portablen Intent intern zu `database.sql` und `cache.key-value`; die Produktnamen sind keine Vorgabe fuer spaetere Contract-Versionen.

## Runtime-Truth in v0.4

BaseHarbor bewertet nicht nur Containerstatus:

- PostgreSQL und Valkey werden per echtem Protokoll geprüft;
- ausgewählte Compose-Services unterscheiden Running/Healthy, Starting, Unhealthy, Exited und Missing;
- konventionelle app-eigene HTTP/HTTPS-Publisher werden lokal aktiv geprüft;
- Redirects gelten als erreichbare Exposition, 5xx oder nicht erreichbare Endpunkte als NOT READY;
- bei hostname-gebundenem HTTPS wird weiter lokal verbunden, aber Public FQDN als HTTP Host/TLS ServerName verwendet;
- Restore- und Update-Pfade melden Erfolg erst nach erfolgreicher Post-Verifikation.

`baha app show`, `status` und `doctor` verwenden diese gemeinsame Wahrheit.

## Recovery und Updates

Backup gilt nur zusammen mit verifiziertem Restore als unterstützt. Restore validiert vor destruktiver Mutation, hält Workloads während unsicherem Recovery gestoppt und meldet READY erst nach erfolgreicher Verifikation von Backend, Secrets/Runtime-Identity und Application Boundary.

Git-basierte Application Updates sind ausschließlich strict fast-forward. BaseHarbor setzt keine lokalen Änderungen zurück, stasht nicht und merged/rebased keine divergierten Branches automatisch. Für dauerhaften BaseHarbor-State gilt vor Mutation eine explizite Recovery-Policy.

BaseHarbor-Self-Update prüft Release-Artefakte, ersetzt die CLI atomar und hält eine Recovery-Binary für Rollback bereit.

## TLS-Grenze in v0.4

v0.4 behaelt fuer Repository-Compose-Deployments den Existing/BYOC-Zertifikats-Lifecycle: Zertifikat/Key/FQDN-Prüfung, Downgrade-Schutz, geschützte Installation, Workload-Restart und Readiness-Verifikation.

Nicht Teil von v0.4 sind ein providerneutraler `tls.certificate`-Manifest-Contract, BaseHarbor-gesteuertes ACME, OpenBao-PKI-Issuance, automatische Rotation oder Kubernetes/OpenShift-TLS-Provider.

## Bewusste Grenzen von v0.4.2

Der aktuelle Standard bleibt Single-Node/Compose. HA, Kubernetes, OpenShift, providerneutrale Ingress-/TLS-Capabilities, Object Storage und Managed-Production-Identity/Policy werden separat entwickelt und versioniert. Diese zukuenftigen Faehigkeiten sind Architekturziele, keine impliziten Versprechen fuer v0.4.2.


## Provider Integration Contract v1

Alle neuen Capability Provider nach v0.4.2 muessen den gemeinsamen Provider Integration Contract v1 verwenden.

BaseHarbor definiert die versionierte Capability-Semantik. Eingebaute Reference Provider und kuenftige externe Provider verwenden denselben Lifecycle-, Ownership-, Diagnostics- und Conformance-Vertrag.

Die kuenftige externe Transportgrenze ist sprachneutral mit gRPC/Protocol Buffers; OCI ist die Richtung fuer registry-neutrales Packaging und Distribution. Ein dynamischer Plugin Loader ist in dieser Architektur-Voraussetzung bewusst noch nicht implementiert.

Siehe [Provider Integration Contract v1](provider-integration-contract.md).


## Repository Inspection in v0.4.3

v0.4.3 verschiebt Repository-Verstaendnis in einen gemeinsamen, read-only Core. CLI-Inspection und gefuehrtes `app init` verwenden dieselbe deterministische Detector Engine mit Detected/Suggested/Possible-Confidence und maschinenlesbaren Ergebnissen fuer spaetere Control Surfaces.

Inspection ist keine Mutation und keine AI-Abhaengigkeit. Spaetere Capability-Releases erweitern die Engine ueber registrierbare Detektoren statt eigene CLI-Sonderlogik aufzubauen.


## Endpoint- und Exposure-Fundament in v0.4.4

v0.4.4 zieht HTTP/HTTPS-Endpoint-Identitaet und Readiness in einen gemeinsamen Endpoint-Core. Die logische Identitaet basiert auf Workload-Service-Namen und Protokoll-/Adressmetadaten; generierte Compose-Container-Namen sind niemals anwendungsseitige Identitaet.

Endpoint-Erkennung bedeutet keine Exposure-Ownership:

```text
app-eigener Endpoint
  -> erkennen
  -> beobachten
  -> verifizieren

explizites exposure.http/v1
  -> Provider aufloesen
  -> alle Preflights
  -> provisionieren
  -> logischen Endpoint binden
  -> Ende-zu-Ende verifizieren
```

Der erste verwaltete Compose-Exposure-Provider ist Caddy. Er besitzt ein separates application-scoped Proxy-Projekt. Das stabile Exposure-Integrationsnetz gehoert dagegen zum von BaseHarbor erzeugten Workload-Override; nur explizit exponierte Workload-Services werden daran angebunden und Caddy konsumiert dieses Netz extern. Die Compose-Quelldatei und app-eigene Volumes werden nicht uebernommen.

Derselbe `exposure.http/v1`-Intent soll spaeter ohne Aenderung am Application Contract auf andere Provider abgebildet werden koennen.

## Secure-Binding-Fundament in v0.4.5

v0.4.5 fuehrt unterhalb von CLI- und Runtime-spezifischem Code eine gemeinsame Security-Binding-Domain ein. Capability Bindings koennen nun providerneutral Workload-Identitaet, Credential-Referenzen, Trust-Referenzen, Authorization-Metadaten, Secret-Referenzen, Lifecycle-Unterstuetzung und Security-Diagnostik tragen.

Der bestehende Compose-/OpenBao-Pfad bleibt autoritativ. Sein Client-Zertifikat verwendet bereits das SPIFFE-Subject `spiffe://baseharbor/apps/<application>/<environment>`; v0.4.5 bildet diese stabile Identitaet semantisch ab, ohne Zertifikatspfade, AppRoles, SecretIDs, Policies oder Secret-Werte offenzulegen.

Der Lifecycle validiert Secure-Binding-Metadaten bereits bei der Plan-Erstellung vor jedem Provider-Preflight und vor Mutation. Spaetere SQL-, S3-, Messaging-, Vector-, AI- und MCP-Provider verwenden damit dieselbe Security-Grenze statt eigenes Credential-Plumbing zu erfinden.

Human OIDC/RBAC/MFA/JIT/Breakglass bleibt ein separates v0.6-Thema fuer Plattformzugriff. Vollstaendige Cross-Provider-Rotation bleibt spaetere Lifecycle-Arbeit.


v0.4.6 fuehrt die erste providerneutrale S3-Object-Storage-Implementierung auf denselben gemeinsamen Lifecycle- und Secure-Binding-Grundlagen ein. Logische Buckets werden auf `object-storage.s3/v1` abgebildet; SeaweedFS ist ein lazy shared Compose-Referenzprovider und keine Application Identity. Provider-State besitzt physische Bucket-/IAM-/Topologie-Details; Application-facing Readiness wird mit einem authentifizierten SigV4-Put/Get verifiziert.

## OTLP-Telemetrie-Fundament in v0.4.7

v0.4.7 fuehrt `telemetry.otlp/v1` als providerneutrale Transport-Capability ein. OpenTelemetry ist Oekosystem und Instrumentierungsmodell; OTLP ist die portable Protokollgrenze. Der OpenTelemetry Collector ist nur der erste verwaltete Compose-Referenzprovider.

Anwendungen deklarieren die exportierten Telemetrie-Signale und verwenden normale OpenTelemetry-Konfiguration:

```text
OTEL_EXPORTER_OTLP_ENDPOINT
OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf
OTEL_SERVICE_NAME
OTEL_RESOURCE_ATTRIBUTES
```

Managed-Collector-Platzierung ist lazy/shared. Externe OTLP-Ziele verwenden dieselbe Capability und bleiben lifecycle-seitig extern owned. BaseHarbor verifiziert einen echten OTLP-HTTP/Protobuf-Export statt nur einen laufenden Collector-Prozess.

Die gemeinsame Resource Identity verwendet Standard-OpenTelemetry-Attribute fuer Service und Environment sowie BaseHarbor-Attribute fuer Application, logische Telemetrie-Ressource und Provider. OTLP-Transport startet nicht implizit Prometheus, Loki, Tempo, Grafana oder andere Observability-Produkte.

## Metrics-Collection und Prometheus-Provider in v0.4.8

Metrics werden in drei unabhaengige Ebenen getrennt:

```text
von der Anwendung bereitgestellte Signalquelle
        !=
Deployment-Collection-Policy
        !=
Metrics-Provider-Implementierung
```

`metrics/v1` beschreibt eine von der Anwendung bereitgestellte OpenMetrics-kompatible HTTP-Source. Das Manifest enthaelt logischen Source-Namen, Workload-Service, Ziel-Port und Pfad. Prometheus oder ein anderer Backend-Produktname gehoeren nicht in diesen Contract.

Die aktuelle Compose-Policy aktiviert Collection standardmaessig nur in Development-Umgebungen. Test/Staging/Produktion benoetigen ein explizites Operator-Opt-in. Die Policy wird vor Provider-Mutation aufgeloest.

Prometheus 3.14.0 ist der erste shared, von BaseHarbor verwaltete Compose-Provider. Targets werden ueber file-based Service Discovery aus geschuetztem BaseHarbor-State erzeugt; manuelle Scrape-Target-Pflege ist nicht erforderlich. Nur deklarierte Source-Services treten dem internen `baseharbor-metrics`-Netz bei. App/Environment/Service erhalten deterministische kollisionsresistente DNS-Aliase, sodass identische Compose-Service-Namen verschiedener Anwendungen nicht kollidieren.

Target-Labels enthalten Application, Environment, Workload-Service und logische Source-Identitaet. Readiness verlangt einen echten erfolgreichen Scrape, der in Prometheus als `up=1` sichtbar ist; ein nur laufender Prometheus-Prozess reicht nicht.

Der Provider hat keinen Docker-/Podman-Socket. Seine API wird nur auf Loopback fuer lokale Lifecycle-Verifikation/Queries publiziert. Grafana, Loki und Tempo bleiben getrennte Provider-Tracks und werden nicht als Nebenwirkung von Metrics-Collection provisioniert.

## Fundament fuer kontinuierliche Application-Evolution

BaseHarbor behandelt Application Intent als dauerhaft abgleichbaren Desired State.

```text
Source Repository
      |
      v
read-only Inspection
      |
      v
typisierte Capability-Evidenz
      |
      v
Vergleich mit explizitem Contract
      |
      +-- satisfied
      +-- new
      +-- ambiguous
      +-- stale (nur Information)
      |
      v
minimales explizites Contract-Delta
```

Das Modell deckt vier Lebensphasen mit derselben Capability-Architektur ab:

1. **Bootstrap** — neues Repository verstehen und initialen Intent festlegen;
2. **Evolution** — neue Capability-Anforderungen erkennen, wenn sich der Code weiterentwickelt;
3. **Runtime** — explizit autorisierte Application-Time-Resource-Operationen erlauben, wenn die Capability sie unterstuetzt;
4. **Retirement** — Capability Intent nur durch eine ausdrueckliche Entwickler-/Operator-Entscheidung entfernen.

Capability-Evidenz traegt eine Richtung (`consume`, `provide`, `export`, `receive`, `provision`) und kann Runtime-Hinweise wie `runtime.create` enthalten. Detection ist niemals Authorization.

Deployment-Time- und Application-Time-Ressourcen verwenden dieselbe logische Capability-/Provider-Grenze. Eine spaetere Anforderung der laufenden Anwendung, S3-Ressourcen anzulegen, wird deshalb ueber `object-storage.s3` aufgeloest und fuehrt nicht zu einem separaten SeaweedFS-, AWS- oder Ceph-Lifecycle.

Repository Inspection liefert bereits erste konkrete Evidenz fuer dieses Modell:

- PostgreSQL- und Redis/Valkey-Konsum;
- S3-kompatiblen Object-Storage-Konsum und moegliche Runtime-Bucket-Erzeugung;
- von der Anwendung bereitgestelltes OpenMetrics `/metrics`;
- OTLP-Export.

Repository-first `baha up` verwendet denselben Reconciliation-Pfad und meldet neu erkannte/unklare Capability-Aenderungen sowie Runtime-Operations-Hinweise vor der normalen Convergence, ohne den Contract zu veraendern.

Dieses Fundament implementiert bewusst noch keine grosse oeffentliche Runtime-Resource-API. Es definiert die Semantik, die eine solche API spaeter wiederverwenden muss. Siehe ADR 0010.


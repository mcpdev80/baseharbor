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
- Runtime-, Capability- und Delivery-Provider sind getrennte Achsen.
- Placement bleibt, wo anwendbar, durchgehend `application | shared | external`.
- Pro Ressourcensatz existiert genau ein Reconciliation Owner.
- Contracts, nicht Tools: etablierte OSS-Mechanismen bleiben hinter BaseHarbor-Contracts austauschbar.
- Der Entwickler bedient primaer `baha`; BaseHarbor bedient oder delegiert an die darunterliegenden Tools.

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

## Runtime-, Capability- und Delivery-Provider

BaseHarbor besitzt drei unabhängige Provider-Achsen:

```text
                         BaseHarbor Core
                               |
             +-----------------+-----------------+
             |                 |                 |
             v                 v                 v
       Runtime Provider  Capability Provider  Delivery Provider
             |                 |                 |
          Compose           SQL / S3         direct
        Kubernetes          Secrets          delegated/GitOps
         OpenShift          Telemetry
```

Harte Regel:

```text
runtime != capability != delivery
```

Runtime Provider realisieren Workload-/Runtime-Primitiven. Capability Provider realisieren logische Application Capabilities. Delivery Provider bestimmen, wie die gewünschte Runtime-Realisierung zur Ziel-Runtime gelangt und dort reconciled wird.

Auswahl und Placement gehören zu Deployment-/Operator-State, nicht zum portablen Application Intent.

Wo anwendbar verwenden alle Provider-Familien dieselben kanonischen Placement-Semantiken:

```text
application
shared
external
```

### Direct und delegated delivery

Bei direct delivery besitzt BaseHarbor Mutation/Reconciliation des verwalteten Runtime-Ressourcensatzes.

Bei delegated delivery erzeugt BaseHarbor die gewünschte Realisierung; ein Delivery Provider bzw. externer Reconciler besitzt die Runtime-Mutation. BaseHarbor behält Policy, Lifecycle-Semantik, Observation, semantische Verifikation und Evidence.

Für einen Ressourcensatz darf genau ein aktiver Reconciliation Owner existieren. BaseHarbor darf nicht gegen einen externen Reconciler arbeiten.

Argo CD kann Referenzprovider für GitOps sein. Flux oder andere konforme Provider müssen ohne Änderung des portablen Contracts möglich bleiben.

### Contracts, not tools

BaseHarbor besitzt Contract und semantische Wahrheit. Runtime, Provider und Delivery-System wählen den Mechanismus.

Reifes OSS, offene Standards, Standard-APIs/SDKs, Controller/Operatoren und CRDs sollen hinter Provider-Grenzen wiederverwendet werden, wenn sie den BaseHarbor-Contract erfüllen. Sie werden nicht zur Application API.

Der normale Entwickler-/Agentenpfad bleibt BaseHarbor-first: `baha`, JSON und MCP bedienen die gewählten Provider/Tools. Native Tool-UIs und CLIs bleiben für Platform Engineers und Experten verfügbar.

Siehe ADR [0011](decisions/0011-delivery-providers-and-tool-neutrality.md).

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

Human OIDC/RBAC/MFA/JIT/Breakglass bleibt ein separater Post-v0.5-Track fuer Plattformzugriff und ist nicht Teil der v0.6-Availability-/Guarantee-Phase. Vollstaendige Cross-Provider-Rotation bleibt spaetere Lifecycle-Arbeit.


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

Prometheus 3.14.0 ist der erste von BaseHarbor verwaltete Compose-Metrics-Provider. Das sichere Default-Placement ist `shared`, dieselbe generische Provider-Placement-Schicht unterstuetzt aber auch eine application-scoped Prometheus-Instanz. Shared Placement kann optional eine benannte Sharing Boundary verwenden, sodass ausgewaehlte Applications genau eine Provider-Instanz teilen, waehrend andere Applications ausserhalb dieser Provider-Trust-Boundary bleiben. Targets werden ueber file-based Service Discovery aus geschuetztem BaseHarbor-State erzeugt; manuelle Scrape-Target-Pflege ist nicht erforderlich. Jede teilnehmende Application erhaelt ihr eigenes isoliertes Metrics-Netz und die ausgewaehlte Prometheus-Instanz wird nur an die explizit registrierten Application-Netze angebunden. App/Environment/Service erhalten deterministische kollisionsresistente DNS-Aliase, sodass identische Compose-Service-Namen verschiedener Anwendungen nicht kollidieren.

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

Placement hat konkrete Ownership- und Runtime-Folgen:

```text
shared
  -> eine BaseHarbor Platform-/Core-Runtime-Provider-Instanz
  -> gehoert keiner einzelnen Application
  -> kann eine oder mehrere explizit autorisierte Applications bedienen
  -> wird lazy erzeugt, sobald eine Capability sie benoetigt

application
  -> eine dedizierte Provider-Instanz fuer genau eine Application/Environment
  -> bei Compose bedeutet das einen dedizierten Provider-Container/-Project mit eigenem State
  -> wird niemals von einer anderen Application wiederverwendet

external
  -> Provider-Instanz wird ausserhalb von BaseHarbor betrieben
  -> BaseHarbor bindet sie an, besitzt/provisioniert aber nicht ihren Lifecycle
```

Ein Provider wird **nicht** deshalb von `shared` zu `application`, weil ihn aktuell nur eine einzige Application nutzt. `shared` beschreibt Platform-Ownership und Wiederverwendungsgrenze der Provider-Instanz, nicht die aktuelle Anzahl der Consumer. Ein shared Prometheus-, PostgreSQL-, OpenBao-, Object-Storage- oder Telemetry-Provider gehoert deshalb in den BaseHarbor Platform-/Core-Runtime-Bereich, auch wenn ihn momentan nur eine Application verwendet. Umgekehrt bedeutet `application` immer eine dedizierte Provider-Instanz fuer genau diese Application.

Shared Provider sind on-demand Platform-Infrastruktur und keine pauschalen Bootstrap-Abhaengigkeiten. Die minimale BaseHarbor-Control-Plane bleibt klein; ein optionaler shared Provider rueckt erst dann in die Platform-/Core-Runtime, wenn eine Application-Capability auf diesen shared Provider aufgeloest wird. Existiert dort bereits eine kompatible shared Provider-Instanz, wird sie wiederverwendet statt eine zweite Instanz zu starten.

Ein Shared Provider ist niemals automatisch fuer alle Applications erreichbar. Zugriff bleibt explizit, least-privilege und deny-by-default. Eine Sharing Boundary erlaubt es dem Operator, genau eine Provider-Instanz bewusst fuer eine ausgewaehlte Gruppe von Applications gemeinsam zu nutzen, waehrend andere Applications ausserhalb dieser Trust Boundary bleiben. Das Teilen einer physischen Provider-Instanz bedeutet niemals, dass logische Application-Ressourcen, Credentials, Daten oder Netzwerkzugriff geteilt werden.

Provider-Implementierungen deklarieren, welche Placements sie unterstuetzen. Wenn die Policy ein Placement aufloest, das der ausgewaehlte Provider nicht erfuellen kann, bricht BaseHarbor vor jeder Mutation fail-closed ab, statt still auf ein anderes Placement auszuweichen.

Der portable Application Contract enthaelt weder Provider-Placement noch Sharing Boundary, Lifecycle Ownership oder Runtime-Realisierungsmechanik. Der Entwickler beschreibt weiterhin nur die benoetigten Capabilities. BaseHarbor und Deployment Policy loesen die Infrastrukturdetails auf.

Die Placement-Semantik steht vor der Runtime-Realisierung fest. Eine Runtime darf plattformnative Mechanismen zur Umsetzung waehlen, die Bedeutung aber nicht neu interpretieren: `application` bleibt genau eine dedizierte Provider-Instanz fuer eine Application/Environment, `shared` bleibt BaseHarbor Platform-/Core-Runtime-Infrastruktur und `external` bleibt extern lifecycle-owned. Compose realisiert diese Garantien aktuell ueber dedizierte/geteilte Projects, Netze und Volumes. Spaetere Kubernetes-/OpenShift-Runtimes koennen Namespaces/Projects, Operators, NetworkPolicies oder andere plattformnative Mechanismen verwenden, ohne die Placement-Bedeutung oder den Application Intent zu veraendern.

Der Installations-Scope eines spaeteren Operators ist nicht dasselbe wie Provider-Placement oder Resource-Scope. Ein clusterweit installierter Operator kann application-scoped oder sharing-boundary-scoped Ressourcen verwalten.

Mehrere BaseHarbor-Installationen sind daher nicht notwendig, nur weil Gruppen von Applications bestimmte Provider gemeinsam nutzen. Getrennte BaseHarbor-Control-Planes bleiben echten administrativen, Trust-Domain-, Infrastruktur- oder Compliance-Grenzen vorbehalten.

Der aktuelle Implementierungsumfang bleibt Docker/Podman Compose. Kubernetes-/OpenShift-Abbildungen sind hier nur Architektur-Kompatibilitaetsanforderungen und noch keine implementierte Runtime-Funktionalitaet.

## Explizite Cross-Application-Connectivity

Provider-Placement und Application-zu-Application-Kommunikation sind getrennte Dinge. `shared` darf niemals als Abkuerzung benutzt werden, um ansonsten isolierte Applications miteinander zu verbinden.

BaseHarbor kennt die aufgeloesten Applications, Services, Capabilities, Provider-Bindings, Runtime-Identitaeten, Netze und Endpoints bereits. Eine Cross-Application-Policy beschreibt deshalb nur die Verbindung, die vom deny-by-default-Grundzustand abweicht:

```text
app-a/api -> app-b/sql
```

Das ist genau ein gerichteter Policy-Eintrag und eine einzige Source of Truth. Der Operator traegt keine Ports, URLs, Netzwerknamen, Provider-Placements, Credentials oder spiegelbildliche Definitionen in beiden Applications doppelt ein, wenn BaseHarbor diese Informationen aus dem aufgeloesten Zustand ableiten kann.

Das semantische Modell bleibt bewusst minimal:

```text
Quell-Application/Service -> Ziel-Application/Service-oder-Resource
```

BaseHarbor loest daraus die konkreten Connectivity-Details auf und validiert vor jeder Mutation, dass Quelle und Ziel existieren und kompatibel sind.

Default ist keine Cross-Application-Connectivity. Eine explizite Policy erlaubt nur den genannten Source-to-Target-Pfad; sie verbindet nicht pauschal Application-Netze, exponiert keine unbeteiligten Services und erzeugt keinen Rueckkanal.

Runtime Provider setzen dieselbe Policy mit ihren nativen Isolationsmechanismen um:

```text
BaseHarbor Connectivity Policy
        |
        +-- Compose
        |     -> dediziertes Source-Link-Netz
        |     -> gehaerteter BaseHarbor-TCP-Relay
        |     -> Target bleibt in seinem eigenen Netz
        |
        +-- Kubernetes
        |     -> NetworkPolicy
        |
        +-- OpenShift
              -> NetworkPolicy / plattformnative Entsprechung
```

Die Compose-Realisierung erhaelt die Richtung technisch. BaseHarbor haengt Source und Target **nicht** gemeinsam an dasselbe Bridge-Netz. Nur der Source-Service kommt in ein verbindungsspezifisches Link-Netz. Ein gehaerteter Relay aus dem versionsgleichen BaseHarbor-Runtime-Image haengt an diesem Source-Link sowie an genau einem vorhandenen Target-Netz und leitet nur auf den aufgeloesten Target-TCP-Port weiter. Der Target-Service kommt niemals in das Source-Link-Netz; dadurch entsteht kein reziproker Netzwerkpfad. Der Relay besitzt keinen Host-Port und keinen Container-Runtime-Socket, laeuft non-root, verwendet ein read-only Root-Filesystem, droppt Linux-Capabilities und setzt `no-new-privileges`.

Die Policy ist unabhaengig vom Provider-Placement. Zum Beispiel koennen beide Applications ihre PostgreSQL-/OpenBao-Provider `application`-scoped behalten, waehrend nur `app-a/api -> app-b/sql` als Cross-Application-Pfad erlaubt wird. Umgekehrt erzeugt ein `shared` Provider niemals automatisch Application-zu-Application-Connectivity.

Auch hier gilt derselbe Security-Grundsatz wie im restlichen BaseHarbor: **deny by default; nur die minimale Ausnahme deklarieren; alles Weitere aus dem vorhandenen Plattformwissen ableiten.**

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

## Zentrale Logs und Workload-Security in v0.4.9

Log-Collection ist Plattform-/Deployment-Policy. Anwendungen nennen Loki nicht im portablen Intent. BaseHarbor leitet logische `logs/v1`-Sources aus den ausgewaehlten Repository-Workload-Services ab, loest Provider-Placement ueber das bestehende Modell `shared | application | external` auf und speichert Application-owned Log-Ressourcen in der geschuetzten Provider-Registry.

Die aktuelle Compose-Referenz verwendet Loki 3.7.8 plus Alloy 1.19.2. Shared Placement ist der sichere Default und kann eine benannte Sharing Boundary verwenden; Application Placement erzeugt einen dedizierten Provider. Der aktuelle Adapter bewirbt External Placement nicht. Loki/Alloy erhalten keinen Docker-/Podman-Socket.

Die Provider-Topologie trennt Isolation und Host-Publishing bewusst. Loki und Alloy teilen sich ein rein internes Provider-Netz fuer ihren internen Datenverkehr. Ein zweites provider-eigenes Bridge-Netz wird nur dort verwendet, wo Docker/Podman fuer Host-Port-Publishing ein nicht-internes Netz benoetigt. Die publizierten API-/Syslog-Ports bleiben dabei strikt an `127.0.0.1` gebunden; es entsteht keine Wildcard-Host-Exposition. Generierte Workload-Logging-Overrides verwenden Loopback-Syslog/RFC5424; Loki-Readiness und echte Queries beweisen die End-to-End-Ingestion.

Vor dem Start eines Repository-Workloads analysiert BaseHarbor die vollstaendig gerenderte Compose-Konfiguration auf Privileged Mode, Runtime-Sockets, Host-Network/PID/IPC, gefaehrliche Linux-Capabilities, Devices und kritische Host-Bind-Mounts. Managed Environments brechen fail-closed ab, wenn diese Einstellungen die dokumentierte Isolation umgehen. Development kann erlaubte Ausnahmen explizit bestaetigen; Diagnosen bleiben maschinenlesbar.

Provider Integration Contract v1 besitzt jetzt ausfuehrbare Conformance-Tests. Ein wiederverwendbarer Harness und ein deterministischer Fake Provider pruefen side-effect-freien Preflight, Idempotenz, CREATE/NOOP, Drift/Repair, Lifecycle-Fehler, Verify, Retry-Convergence, secret-sichere Diagnosen und ownership-sicheres Destroy.

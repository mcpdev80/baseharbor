# Roadmap

BaseHarbor entwickelt sich zu einer developer-first Plattformgrundlage, die eine Anwendung vom ersten lokalen/Homelab-Start bis zum Enterprise-Betrieb begleiten kann.

Ziel ist ein durchgaengiger Weg:

```text
Idee / lokale Entwicklung
        |
        v
Homelab / Single Host
        |
        v
Compose-Produktion
        |
        v
spaeter Kubernetes / OpenShift
        |
        v
Enterprise-Deploymentprofile
```

Die Anwendung deklariert logische Anforderungen. BaseHarbor loest, provisioniert, sichert und betreibt diese Anforderungen ueber getrennte Runtime- und Capability-Provider, waehrend die Anwendung Standardprotokolle und native Clients verwendet.

## Aktueller Stand v0.4.12

Docker/Podman Compose bleibt die vollstaendige Runtime-Implementierung. v0.4 fuehrt die Architekturgrenzen ein, die spaetere Provider ermoeglichen, ohne den logischen Anwendungsvertrag neu zu definieren.

Umgesetzt sind unter anderem:

- `baha` als zentrale Lifecycle-CLI;
- Manifest v1 `baseharbor.yaml` als unterstuetzter oeffentlicher Kompatibilitaetsvertrag;
- providerneutraler `PortableContract` fuer portablen Anwendungs-Intent;
- ein oder mehrere benannte logische PostgreSQL- und Valkey/Redis-Ressourcen;
- ein oder mehrere logische S3-Buckets ueber `object-storage.s3/v1`;
- SeaweedFS als aktueller lazy shared Compose-S3-Referenzprovider mit bucket-scoped Credentials und authentifizierter Put/Get-Readiness;
- providerneutrales `telemetry.otlp/v1`-Export-Binding mit Standard-OpenTelemetry-Workload-Konfiguration;
- OpenTelemetry Collector als aktueller lazy shared Compose-OTLP-Referenzprovider plus externe OTLP-Endpunkt-Bindings;
- echte OTLP-HTTP/Protobuf-Export-Verifikation ohne implizites Prometheus/Loki/Tempo/Grafana-Provisioning;
- `metrics/v1` mit Prometheus 3.14.0 als erstem Compose-Referenzprovider, shared/application Placement, isolierten Metrics-Netzen und echter Scrape/Ingestion-Verifikation;
- `logs/v1` Platform-Log-Source-Lifecycle mit Loki 3.7.8 + Alloy 1.19.2, shared/application Placement und echter Query-Verifikation;
- Rendered-Compose Workload-Security-Preflight sowie ausfuehrbare Provider-Contract-Conformance/Fault-Injection;
- explizite gerichtete Cross-Application-Connectivity ueber `baha connect`, getrennt von Provider-Sharing und in Compose ueber einen gehaerteten BaseHarbor-Relay realisiert;
- Managed Required/Generated Secrets ohne Secret-Werte im Contract;
- providerneutrale `secure-binding/v1`-Semantik fuer Workload Identity, Credential-/Trust-/Secret-Referenzen, Least-Privilege-Authorization und Security-Lifecycle-Deklarationen;
- Compose als expliziter Runtime Provider;
- deployment-eigener Runtime-Provider- und Profil-State;
- Runtime-Capability-Negotiation mit fail-closed Verhalten;
- zentrale Runtime-Guards, damit spaetere Provider nicht versehentlich in Compose-spezifischen Code fallen;
- deklarative Input-Aufloesung fuer Defaults, generierte, externe und bedingte Werte;
- gemeinsame Deployment-Input-Logik fuer `baha app init` und repository-aware `baha up`;
- explizite non-secret Automation-Inputs ueber `--input NAME=VALUE`;
- Trusted-local Developer Access;
- health-aware Workload-Truth inklusive app-eigener HTTP/HTTPS-Readiness;
- gemeinsame logische Endpoint-/Exposure-Semantik fuer app-eigene Publisher und Managed Exposure;
- expliziter providerneutraler `exposure.http/v1`-Intent mit application-scoped Caddy als aktuellem Compose-Referenzprovider;
- verschluesseltes, verifiziertes Backup/Restore;
- strict fast-forward Application Updates und abgesichertes BaseHarbor-Self-Update;
- Existing/BYOC-TLS-Lifecycle fuer Compose-Deployments;
- realer MailFlow-Acceptance-Pfad inklusive Backup -> Destroy -> Restore.

Mehrere logische Service-Instanzen sind nicht HA. HA ist eine spaetere Topologie hinter einer stabilen logischen Ressource.

## Architekturregel

Compose bleibt first-class, ist aber nicht mehr die konzeptionelle Anwendungs-API. Runtime Provider und Capability Provider sind getrennte Achsen.

Delivery Provider bilden eine dritte, unabhängige Achse. Sie bestimmen, wie die gewünschte Runtime-Realisierung zur Ziel-Runtime gelangt und dort reconciled wird. Direct Delivery und delegated/GitOps Delivery müssen dieselbe BaseHarbor-Semantik erhalten, ohne Argo CD, Flux, Git oder Kubernetes-Objekte in den portablen Application Contract zu ziehen.

Wo anwendbar verwenden Runtime-, Capability- und Delivery-Provider dieselben kanonischen `application | shared | external` Placement-/Ownership-Semantiken bei weiterhin getrennten Verantwortlichkeiten.

BaseHarbor bleibt die normale Entwickler-/Agenten-Oberfläche. Reifes OSS und offene Standards werden hinter Provider-Grenzen wiederverwendet statt neu implementiert; native Tool-UIs/CLIs bleiben für Platform-/Expert-Drill-down verfügbar.

Provider-spezifische Details wie Compose-Projektnamen, Host-Ports, generierte Overrides, Kubernetes-Objektnamen oder OpenShift Routes/SCCs duerfen nicht in den providerneutralen `PortableContract` leaken.

Provider-Substitution muss den angeforderten Contract erfuellen oder klar fehlschlagen. Sicherheit, Haltbarkeit oder Verfuegbarkeit duerfen niemals still reduziert werden.

BaseHarbor verwendet einen gemeinsamen Domain-/Lifecycle-Core mit mehreren Control Surfaces. `baha`, eine spaetere HTTP-API/WebGUI und ein spaeterer Kubernetes/OpenShift-Operator muessen dieselben Plan-, Validation-, Lifecycle-, Readiness-, Diagnose- und Policy-Semantiken verwenden statt eigene Implementierungen zu entwickeln.

## v0.4-Grenze

In v0.4 umgesetzt:

- Manifest-v1-Kompatibilitaetsadapter -> `PortableContract`;
- Runtime-Provider-Seam und Capability-Negotiation;
- geschuetzter Provider-/Profil-State;
- fail-closed Guards fuer Application-Runtime-Operationen;
- deklarativer Input Resolver;
- gemeinsamer TLS/FQDN-Deployment-Input-Referenzpfad;
- vollstaendig erhaltener Compose-Developer-Journey und v0.3-State-Kompatibilitaet.

Weiterhin Future Work:

- neue oeffentliche capability-orientierte Contract-Syntax;
- weitere Capability-Provider und zusaetzliche S3/Object-Storage-Provider/Provider-Auswahl;
- BaseHarbor-Recovery fuer Object-Storage-Inhalte (v0.4.6 bricht Backup/Restore fuer S3-Anwendungen fail-closed ab statt unvollstaendige Recovery zu behaupten);
- weitergehende Enterprise-Policy-/Profil-Komposition ueber die v0.4.13-Core-Semantik hinaus;
- OIDC/RBAC/JIT fuer Managed Production;
- HA-/Topologieprofile;
- weitere Traffic-/Exposure-Provider und breiterer providerneutraler TLS-/Zertifikats-Lifecycle ueber den aktuellen `exposure.http/v1`- und Existing/BYOC-Pfad hinaus;
- Kubernetes Runtime Provider;
- OpenShift Runtime Provider / Enterprise-Spezialisierung.

## v0.4.3 Repository Inspection

v0.4.3 macht Repository-Analyse zu einer gemeinsamen, strikt read-only Core-Funktion.

- `baha app inspect [PATH]` zeigt nachvollziehbare Evidence;
- `--json` liefert dasselbe strukturierte Ergebnis fuer spaetere API/WebUI/Operator-Adapter;
- Findings werden als Detected / Suggested / Possible klassifiziert;
- spaetere Capabilities koennen eigene Detektoren registrieren;
- `app init` nutzt dieselbe Detection Engine;
- Environment-Werte werden verworfen und Symlinks nicht verfolgt;
- Inspection veraendert weder Repository noch Runtime-State.

## v0.4.8 Metrics-/Prometheus-Track

Die Voraussetzungen fuer kontinuierliche Application-Evolution und Runtime-Resource-Ausfuehrung sind abgeschlossen. v0.4.8 fuegt den ersten Metrics-Collection-/Storage-Provider hinzu und behaelt die Trennung zwischen Application Intent und Provider bei.

Im v0.4.8-Development-Track umgesetzt:

- versionierte `metrics/v1` Capability Specification;
- providerneutrale Metrics-Source-Deklarationen mit logischem Source-Namen, Workload-Service, Ziel-Port und Pfad;
- OpenMetrics-kompatible HTTP-Exposition als v1-Signalformat;
- Repository Inspection mappt konventionelle `/metrics`-Evidenz auf die kanonische `metrics`-Capability;
- Deployment-eigene Collection-Policy statt einer portablen `prometheus: true`-Anforderung;
- Collection in Development standardmaessig aktiv, Test/Staging/Produktion nur per explizitem Operator-Opt-in;
- Prometheus 3.14.0 als erster lazy shared Compose-Referenzprovider;
- file-based automatische Target Discovery aus BaseHarbor-State ohne manuelle Prometheus-Target-Pflege;
- pro Application isolierte Metrics-Netze mit expliziter Prometheus-Anbindung nur an registrierte Application-Trust-Boundaries;
- deterministische kollisionsresistente Target-DNS-Aliase, sodass identische Service-Namen verschiedener Anwendungen getrennt bleiben;
- Attribution von Application/Environment/Service/Source auf gescrapten Serien;
- Readiness durch echten erfolgreichen Scrape mit `up=1` statt nur Prozess-Health;
- manual-only Docker-Acceptance mit zwei isolierten Applications auf einem gemeinsamen Prometheus-Provider;
- kein implizites Provisioning von Grafana, Loki oder Tempo.

OTLP-Metrics-Export bleibt ein getrenntes `telemetry.otlp/v1`-Transportthema. v0.4.8 definiert OTLP nicht neu und macht Prometheus nicht zur Application Identity.

## v0.4.9 Logs-/Loki- und Hardening-Track

In v0.4.9 umgesetzt:

- zentrale Application-Workload-Log-Collection bleibt Deployment-/Plattform-Policy statt Loki-Application-Intent;
- ausgewaehlte Repository-Workload-Services sind logische `logs/v1`-Ressourcen mit Application-Ownership in der geschuetzten Provider-Registry;
- Loki 3.7.8 und Alloy 1.19.2 bilden den Compose-Referenzprovider/-Collector mit shared oder application Placement und optionalen benannten Shared Boundaries;
- Loki/Alloy verwenden keinen Docker-/Podman-Socket und Host-Listener bleiben loopback-only;
- Readiness prueft echte Loki-Queries fuer Application/Environment/Service-Streams;
- gerendertes Repository-Compose wird vor Mutation auf Isolation-Bypaesse geprueft;
- Provider Integration Contract v1 besitzt ausfuehrbare Lifecycle-Conformance, Fake Provider und Fault Injection;
- Kubernetes/OpenShift bleiben spaetere Runtime-Tracks.

## v0.4.11 Developer Experience und Repository Adoption

In v0.4.11 veroeffentlicht/abgeschlossen. Die vorhandene providerneutrale Runtime ist fuer bestehende Repositories leichter nutzbar, ohne die fuer v0.4.12 geplante Agent-/MCP-Schicht vorwegzunehmen.

Umgesetzt:

- lokale und Remote-Git-Inspection ueber denselben deterministischen Evidence-Core;
- `baha up -e/--environment` als Deployment-Kontext ohne Umschreiben des portablen Intents;
- repository-aware `baha plan`, `baha status` und `baha doctor`;
- secret-safe strukturierte Ausgabe fuer inspect/plan/status/doctor;
- optionale begrenzte/idempotente `AGENTS.md`-Integration;
- lokale Compose-Runtime als first-class BaseHarbor Playground;
- Five-Minute-Onboarding.

Die strukturierte Ausgabe aus v0.4.11 bildet die Grundlage fuer die Machine-Schicht.

## v0.4.12 Agent-native Machine Interface und MCP

In v0.4.12 umgesetzt/abgeschlossen:

- versionierter BaseHarbor Machine Contract `v1` fuer Inspect/Plan/Status/Doctor;
- strukturierte secret-safe Fehler-Envelopes fuer JSON-Automation;
- explizite Safety-Metadaten fuer semantische Operationen;
- `baha agent describe` / `baha agent describe -o json`;
- offizielles Model Context Protocol Go SDK v1.8.0;
- MCP `2026-07-28` als aktuelles Ziel mit ausgehandelter `2025-11-25`-Kompatibilitaet;
- lokales stdio-only `baha mcp serve`;
- exakt vier read-only Tools: `baseharbor.inspect`, `baseharbor.plan`, `baseharbor.status`, `baseharbor.doctor`;
- MCP Read-only-/Destructive-/Open-world-Annotations;
- gemeinsame typisierte Status+TLS- und Doctor-Result-Pfade fuer CLI, TUI und MCP;
- Acceptance ueber einen generischen MCP-Client ohne BaseHarbor-spezifisches Plugin;
- Secret-Leak-Tests und explizit keine generischen Shell-/Docker-/Compose-Ausfuehrungsprimitiven;
- erweiterte begrenzte/idempotente `AGENTS.md`-Guidance fuer strukturierte BaseHarbor-Interfaces.

Remote-MCP/Auth, mutierende/destruktive MCP-Tools, eingebettete LLM-Logik, vendor-spezifische Agent-Integrationen und Application-MCP-Capabilities bleiben bewusst spaeter.


## Verbleibende v0.4-Sequenz

v0.4 schliesst die runtime-neutrale BaseHarbor-Sprache und die Lifecycle-Semantik vor dem Compatibility-Freeze ab.

### v0.4.13 - Environment- und Policy-Semantik

v0.4.13 macht Deployment-Environment und effektive Policy zu expliziten gemeinsamen Semantiken statt zu CLI-Sonderlogik.

- deterministische Auswahl entweder des kompatiblen Root-`baseharbor.yaml` oder genau eines vollstaendigen `envs/<environment>/baseharbor.yaml`; Environment-Manifeste sind vollstaendige Intents und keine versteckten Overlays;
- `-e/--environment` wird zentral fuer Repository-Lifecycle-/Read-Operationen aufgeloest und veraendert niemals das portable Manifest;
- Workload-Aufloesung bleibt am echten Repository-Root verankert, auch wenn das gewaehlte Manifest unter `envs/` liegt;
- geschuetzter Application-, Deployment-Input-, TLS- und generierter Runtime-State wird pro Environment unter `.baseharbor/environments/<environment>/` getrennt; unveraenderte Single-Environment-Repositories behalten fuer Kompatibilitaet ihren v0.4.12-State-Pfad;
- typisierte Policy-Ergebnisse verwenden `allow`, `warn` und `deny`; `baha policy check` und `baha policy explain` besitzen passende read-only MCP-Tools;
- vorhandene Compose-Workload-Isolation speist dasselbe Policy-Ergebnis statt eine zweite Policy-Engine zu erzeugen;
- sichere Policy ist fail-closed: Managed Environments koennen nicht per Operator-Variable auf Development abgeschwaecht werden; ueberschreibbar bleibt nur die explizit begrenzte Host-Device-Bestaetigung in Development;
- Environment ist ausschliesslich Deployment-/Risiko-Kontext und kodiert weder Runtime, Provider-Placement, Topologie, Availability noch spaetere Kubernetes-Namespace-Details.

Kubernetes/OpenShift bleiben Future Work. Diese Semantik ist bewusst so gebaut, dass ein spaeterer Namespace-only Runtime Target keine Kubernetes-spezifischen Felder im Application Intent erfordert.

### v0.4.14 - Reconciliation-, Security- und Lifecycle-Semantik

In v0.4.14 umgesetzt/abgeschlossen:

- runtime-neutrales typisiertes Desired/Observed/Diff/Ownership-Reconciliation-Modell im gemeinsamen Core;
- typisierte Zustaende `missing`, `in_sync`, `drift`, `conflict`, `foreign_ownership`, `unsupported` und `degraded`;
- typisierte Reconciliation-Aktionen `create`, `noop`, `repair`, `destroy`, `observe` und `blocked`;
- provider-native Observation vor Mutation und eine zweite Observation nach der Verifikation;
- fail-closed Behandlung von Conflict, Unsupported, Degraded und Foreign Ownership vor jeder Provider-Mutation;
- Core-seitige NOOP-Erkennung ohne unnoetige Provider-Mutation;
- minimale Reparatur bei BaseHarbor-owned Drift unter Erhalt stabiler logischer Resource-Identitaet;
- extern verwaltete Ressourcen bleiben observe-only und werden niemals zu BaseHarbor-Mutationszielen;
- die vorhandene Provider-Integration-Contract-Conformance prueft jetzt die gemeinsame typisierte Reconciliation-Semantik;
- ownership-sichere Destroy-Semantik bleibt explizit und wiederverwendbar;
- keine Kubernetes-/OpenShift-Runtime-Implementierung und kein produktspezifischer Delivery-Mechanismus.

Das Modell liegt bewusst unterhalb von CLI-/Runtime-Praesentation. Spaetere Compose-, Kubernetes-, OpenShift- und delegierte Delivery-Adapter koennen dieselbe Semantik verwenden, ohne Runtime-spezifische Felder in den portablen Application Intent aufzunehmen.

### v0.4.15 - Audit- und Evidence-Semantik

- secret-safe Lifecycle-/Policy-/Verification-Events;
- klare Trennung von Desired/Enforced/Observed/Verified;
- generische Evidence-Export-Grenze ohne Vendor-Lock-in.

### v0.4.16 - Capability Provider SDK, Starter Kit und Conformance

- praktischer Third-Party-Capability-Provider-Pfad;
- Conformance gegen die nun vollstaendige Lifecycle-, Ownership-, Security- und Evidence-Semantik;
- kein Runtime-Provider-SDK und keine Kubernetes-spezifischen Typen.

### v0.4.17 - Runtime-Grenze und semantische Full-Stack-Acceptance

- alle wichtigen States als portable, Deployment/Operator, Runtime, Provider oder Protected/Generated klassifizieren;
- den vollstaendigen Compose-Referenz-Lifecycle beweisen;
- CLI/JSON/MCP-Semantik angleichen;
- alle Blocker vor dem v0.5-Freeze schliessen.

## Richtung des Provider-Oekosystems

Das offene Provider-Oekosystem bleibt strategisch. gRPC/Protocol Buffers bilden bei Bedarf die sprachneutrale externe Prozessgrenze; OCI bleibt der registry-neutrale Distributionsweg fuer unabhaengig implementierte Community-/Hersteller-/Unternehmens-Provider.

Provider duerfen intern reifes OSS, Standard-APIs/SDKs, Controller/Operatoren/CRDs oder Managed-Service-APIs verwenden. Diese Mechanismen bleiben hinter der Provider-Grenze; der BaseHarbor Core darf nicht zum Katalog produktspezifischer Integrationen werden.

## v0.5 - Contract Freeze und Compatibility

v0.5 fuegt keine neuen Plattformprimitiven hinzu. Die in v0.4 abgeschlossenen Contracts werden eingefroren, versioniert und bewiesen.

- **v0.5.0** kompletter agent-nativer Core und Contract Freeze;
- **v0.5.1** State-Versionierung und Migration-Compatibility;
- **v0.5.2** Cross-Component-Compatibility-Contracts;
- **v0.5.3** Upgrade-, Recovery- und Deprecation-Verhalten;
- **v0.5.4** Portability- und Compatibility-Acceptance.

Vor Abschluss von v0.5 muss die Deployment-/Runtime-Grenze einen spaeteren eingeschraenkten Runtime-Target, secret-safe Runtime-Access-Referenzen und platform-owned Resource-Referenzen ausdruecken koennen, ohne Kubernetes-Felder in den portablen Application Intent aufzunehmen.

Die BaseHarbor-MCP-/JSON-Control-Surface muss vor dem Freeze ausserdem den vollstaendigen sinnvollen Compose-Lifecycle abdecken.

## v0.6 - Availability, Topologie und portable Garantien

v0.6 definiert portable Availability-Semantik vor jeder Kubernetes-Implementierung.

- **v0.6.0** Availability- und portable Guarantee-Semantik;
- **v0.6.1** Runtime-/Capability-Guarantee-Negotiation;
- **v0.6.2** ehrliche Compose-Realisierung und Verifikation;
- **v0.6.3** Topology- und Portability-Acceptance.

Environment, Runtime und Availability bleiben unabhaengig. Compose darf nur Garantien melden, die es tatsaechlich beweisen kann; es gibt kein stilles Downgrade und keine erfundene HA.

Human OIDC/RBAC/JIT, Remote-Management-WebUI und andere Platform-Access-Themen bleiben separate spaetere Tracks und definieren v0.6 nicht.

## v0.7 - Kubernetes Runtime

v0.7 implementiert Kubernetes als BaseHarbor Runtime Provider. Namespace-only mit einem vorprovisionierten Target-Namespace ist das primaere enterprise-kompatible Zielmodell.

- **v0.7.0** Runtime Foundation und Namespace-only Access Model;
- **v0.7.1** namespaced Security, Identity und Networking;
- **v0.7.2** platform-owned HTTP Exposure mit Gateway API;
- **v0.7.3** namespace-only Persistent Storage;
- **v0.7.4** Stateful Runtime Support ohne Capability-Provider-Leakage;
- **v0.7.5** Availability-Realisierung mit permission-aware Verification;
- **v0.7.6** Runtime Conformance ueber Restricted-Access-Profile;
- **v0.7.7** Compose-to-Kubernetes-Portability-Proof unter Namespace-only-Bedingungen;
- **v0.7.8** delegierte Delivery- und GitOps-Provider-Architektur mit Argo CD als erster Referenzimplementierung ohne Argo-spezifischen portablen Application Contract.

Cluster-Admin, Namespace-Erstellung und cluster-weite Discovery sind keine normalen Application-Lifecycle-Anforderungen. Platform-owned Ressourcen wie Namespaces, Gateway/GatewayClass, StorageClass, CRDs und Admission Policy bleiben nutzbar, ohne dass BaseHarbor sie besitzen muss.

v0.7.0 startet mit direkter Kubernetes-API-Delivery als Referenzpfad. Das ist nicht das einzige dauerhafte Reconciliation-Modell: v0.7.8 ergänzt delegierte/GitOps-Delivery über einen providerneutralen Delivery-Provider-Contract. kubectl, Helm, CRDs und ein BaseHarbor Operator sind keine notwendigen anwendungsseitigen Runtime-Engines.

## v0.8 - Kubernetes Complete

v0.8 schliesst die Produktparitaet und macht Kubernetes zu einer vollwertigen first-class BaseHarbor Production Runtime.

- **v0.8.0** vollstaendige Application-Lifecycle-Paritaet;
- **v0.8.1** Capability- und Provider-Placement-Paritaet;
- **v0.8.2** Backup-/Restore-/Disaster-Recovery-Paritaet;
- **v0.8.3** Observability-, Diagnostics- und Evidence-Paritaet;
- **v0.8.4** Update-, Migration- und Recovery-Paritaet;
- **v0.8.5** Agent-native- und Developer-Experience-Paritaet;
- **v0.8.6** Production Hardening und vollstaendige Conformance-Matrix;
- **v0.8.7** Kubernetes Complete First-Class-Runtime-Acceptance.

Abschlusskriterium ist, dass jede auf Kubernetes anwendbare BaseHarbor-Core-Funktion ueber BaseHarbor-Semantik funktioniert, inklusive Namespace-only, ohne Kubernetes-spezifischen portablen Application Contract und ohne normalen Raw-Kubernetes-Fallback.

Erst nach Kubernetes Complete folgen OpenShift-/Enterprise-spezifische Themen. Operator/OLM, SCC-/Route-spezifische Integration, Enterprise Proxy/Registry/Disconnected sowie Human OIDC/JIT bleiben separate spaetere Tracks, solange kein echter Prerequisite-Use-Case entsteht.

## Langfristiges Erfolgskriterium

BaseHarbor soll eine Anwendung von:

```text
"Ich programmiere mal eben was."
```

bis zu:

```text
"Das muss jetzt fuer einen Enterprise-Kunden auf Kubernetes/OpenShift laufen."
```

begleiten, ohne dass die Anwendung operational neu gebaut werden muss.

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

Ein Shared Provider ist niemals automatisch fuer alle Applications erreichbar. Zugriff bleibt explizit, least-privilege und deny-by-default. Eine Sharing Boundary erlaubt es dem Operator, genau eine Provider-Instanz bewusst fuer eine ausgewaehlte Gruppe von Applications gemeinsam zu nutzen, waehrend andere Applications ausserhalb dieser Trust Boundary bleiben.

Provider-Implementierungen deklarieren, welche Placements sie unterstuetzen. Wenn die Policy ein Placement aufloest, das der ausgewaehlte Provider nicht erfuellen kann, bricht BaseHarbor vor jeder Mutation fail-closed ab, statt still auf ein anderes Placement auszuweichen.

Der portable Application Contract enthaelt weder Provider-Placement noch Sharing Boundary, Lifecycle Ownership oder Runtime-Realisierungsmechanik. Der Entwickler beschreibt weiterhin nur die benoetigten Capabilities. BaseHarbor und Deployment Policy loesen die Infrastrukturdetails auf.

Die Placement-Semantik steht vor der Runtime-Realisierung fest. Eine Runtime darf plattformnative Mechanismen zur Umsetzung waehlen, die Bedeutung aber nicht neu interpretieren: `application` bleibt genau eine dedizierte Provider-Instanz fuer eine Application/Environment, `shared` bleibt BaseHarbor Platform-/Core-Runtime-Infrastruktur und `external` bleibt extern lifecycle-owned. Compose realisiert diese Garantien aktuell ueber dedizierte/geteilte Projects, Netze und Volumes. Spaetere Kubernetes-/OpenShift-Runtimes koennen Namespaces/Projects, Operators, NetworkPolicies oder andere plattformnative Mechanismen verwenden, ohne die Placement-Bedeutung oder den Application Intent zu veraendern.

Der Installations-Scope eines spaeteren Operators ist nicht dasselbe wie Provider-Placement oder Resource-Scope. Ein clusterweit installierter Operator kann application-scoped oder sharing-boundary-scoped Ressourcen verwalten.

Mehrere BaseHarbor-Installationen sind daher nicht notwendig, nur weil Gruppen von Applications bestimmte Provider gemeinsam nutzen. Getrennte BaseHarbor-Control-Planes bleiben echten administrativen, Trust-Domain-, Infrastruktur- oder Compliance-Grenzen vorbehalten.

Der aktuelle Implementierungsumfang bleibt Docker/Podman Compose. Kubernetes-/OpenShift-Abbildungen sind hier nur Architektur-Kompatibilitaetsanforderungen und noch keine implementierte Runtime-Funktionalitaet.

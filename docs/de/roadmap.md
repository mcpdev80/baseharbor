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

## Aktueller Stand v0.4.8

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
- breitere Environment-/Policy-Profile;
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

## Geplante Phasen

### v0.5 - Capability Provider

- austauschbare Capability-Provider hinter stabilen logischen Anforderungen;
- SQL, Key/Value und Secrets weiter providerisieren sowie zusaetzliche S3-Implementierungen hinter `object-storage.s3/v1` anbinden;
- Provider-Conformance und klare Unsupported-Fehler.

### v0.6 - Environment, Policy und Topologie

Diese Phase ist zugleich der natuerliche Einstieg fuer Remote-Management und eine schlanke WebGUI ueber denselben Core.

- explizite Environment-/Plattform-Policy;
- OIDC/RBAC/JIT;
- Standard- und HA-Topologieprofile;
- Security-/Durability-/Availability-Anforderungen als Policy;
- stabile maschinenlesbare BaseHarbor-API;
- schlanke WebGUI fuer Plan/Apply/Status/Doctor/Logs/Inputs/Backup/Restore/Update;
- gemeinsame Authorization-/Policy-Grenzen fuer CLI, API und WebGUI.

### v0.7 - Kubernetes

- Kubernetes als Runtime Provider;
- BaseHarbor Operator als cluster-native Control Surface ueber denselben Core;
- native Deployments/StatefulSets/Services/PVCs/Gateway/Ingress/Secrets/NetworkPolicies;
- gleicher logischer Application Contract;
- CRDs/Reconciliation und Status/Conditions aus derselben BaseHarbor-Runtime-Wahrheit;
- kein Wrapping von imperativen `baha`-Kommandos im Operator.

### v0.8 - OpenShift / Enterprise

- OpenShift-spezifische Routes, SCCs, Registry-/Proxy-/Offline-Integration;
- Enterprise-Policy und Operator-Integrationen;
- weiterhin derselbe portable Anwendungs-Intent.

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

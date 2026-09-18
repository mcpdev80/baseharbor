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

Siehe ADR [0009](../decisions/0009-shared-core-multiple-control-surfaces.md).

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

## Bewusste Grenzen von v0.4.0

Der aktuelle Standard bleibt Single-Node/Compose. HA, Kubernetes, OpenShift, providerneutrale Ingress-/TLS-Capabilities, Object Storage und Managed-Production-Identity/Policy werden separat entwickelt und versioniert. Diese zukuenftigen Faehigkeiten sind Architekturziele, keine impliziten Versprechen fuer v0.4.0.

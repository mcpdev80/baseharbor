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

Eine App soll beispielsweise SQL-Datenbank, Key-Value-Cache, S3-Object-Storage, Secrets oder TLS anfordern können, ohne an PostgreSQL, Valkey, SeaweedFS, OpenBao oder Caddy gekoppelt zu sein.

Ein lokales Environment kann diese Capabilities mit den BaseHarbor-Defaults erfüllen. Ein Enterprise-Environment kann stattdessen Kundendienste wie externes PostgreSQL, Managed Redis, Ceph RGW, Vault oder OpenShift Routes verwenden. Die Application-YAML soll dafür nicht neu geschrieben werden müssen.

Die vollständige Matrix steht unter [Capability- und Provider-Modell](capability-provider-model.md). Die verbindliche Architekturentscheidung ist ADR [0005](../decisions/0005-capabilities-not-products.md).

## Mehrere Instanzen und HA

Mehrere benannte PostgreSQL- oder Valkey-Instanzen sind mehrere unabhängige logische Dienste. Sie sind nicht automatisch Replikate. Zukünftiges HA wird als Topologie hinter einem stabilen logischen Dienst modelliert, damit der Anwendungsvertrag gleich bleibt.

## Provider-Grenze

Compose-Projektnamen, Netzwerke, Host-Ports, Volumes und generierte Overrides sind Implementierungsdetails des aktuellen Providers. Spätere Kubernetes-Ressourcennamen, Ingresses oder OpenShift-Routes wären ebenfalls Providerdetails und dürfen nicht zu fachlichen Abhängigkeiten der Anwendung werden.

Das aktuelle Manifest v1 enthält noch produktorientierte Felder für PostgreSQL/Redis/Valkey. Das ist der bestehende pre-v1-Compose-Contract von v0.2.0 und keine Vorgabe dafür, dass zukünftige providerneutrale Contracts Produktnamen verwenden müssen.

## Bewusste Grenzen von v0.2.0

Der aktuelle Standard ist Single-Node/Compose. HA, Kubernetes, OpenShift, öffentliche Exposition, Object Storage, Observability sowie erweiterte Identity-/Policy-Funktionen werden separat entwickelt und versioniert. Diese zukünftigen Fähigkeiten sind Architekturziele, keine impliziten Versprechen für v0.2.0.

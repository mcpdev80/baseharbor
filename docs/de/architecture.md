# Architektur

BaseHarbor ist kein monolithisches Backend für alle Anwendungen und kein generisches PaaS. Es ist ein wiederverwendbarer Infrastruktur-Core, den unabhängige Anwendungen über deklarative Verträge konsumieren.

## Grundprinzipien

- Anwendungen bleiben unabhängig und besitzen ihren eigenen Code sowie ihre fachliche Konfiguration.
- BaseHarbor besitzt nur die von ihm bereitgestellten Infrastrukturressourcen.
- Standardprotokolle und Standardvariablen bleiben die Anwendungsgrenze.
- Sicherheitsgrenzen werden fail-closed und mit Least Privilege umgesetzt.
- Der lokale Single-Node-Betrieb bleibt einfach; zusätzliche Deployment-Profile dürfen diese Einfachheit nicht zerstören.

## Ebenen

```text
Application Repository
  └── baseharbor.yaml
        ↓
baha CLI / Control Plane
        ↓
Application-scoped managed resources
  ├── PostgreSQL
  ├── Valkey
  ├── OpenBao secret scope
  ├── runtime identity / mTLS broker
  └── backend network + bindings
        ↓
Application workload via standard interfaces
```

Die Control Plane ist benutzer-/maschinenbezogen. Anwendungsressourcen sind nach Anwendung und Umgebung isoliert.

## Mehrere Instanzen und HA

Mehrere benannte PostgreSQL- oder Valkey-Instanzen sind mehrere unabhängige logische Dienste. Sie sind nicht automatisch Replikate. Zukünftiges HA wird als Topologie hinter einem stabilen logischen Dienst modelliert, damit der Anwendungsvertrag gleich bleibt.

## Bewusste Grenzen

Der aktuelle Standard ist single-node. HA, Kubernetes, öffentliche Exposition, Object Storage, Observability, AI/MCP/RAG und automatische KMS/HSM-Unseal-Profile werden separat entwickelt und versioniert.
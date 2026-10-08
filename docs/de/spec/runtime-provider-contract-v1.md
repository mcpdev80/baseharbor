# Laufzeitanbietervertrag v1

## Status

Pre-freeze v0.4.19 Architekturvertrag. Dies definiert die portable Runtime Provider Grenze und ist kein Anspruch von Kubernetes Produktionsunterstützung.

## Anwendungsbereich

Laufzeitanbieter ist eine eigenständige Anbieterachse.

```text
runtime != capability != delivery != workload source
```

Es ist absichtlich getrennt von:

- Lebensdauer von Fähigkeiten/Dienstleistungsanbietern
- Auswahl der Lieferer und Aussöhnung Eigentum
- Repository-Workload-Quellenerkennung und -Erstellung
- anwendungsorientierte Runtime Broker APIs
- lokale Docker/Podman-Projektdatei/Container-Mechanik

## Normen

BaseHarbor nimmt bestehende Standards an, wenn sie passen:

- OCI Image / Distribution für Laufzeit-Artefakte
- Komponieren Sie Spezifikation als eine Repository-Workload-Quelle
- Kubernetes/OpenShift APIs als provider-native Realisation APIs
- gRPC + Protokollpuffer für die externe Prozessgrenze des Runtime Providers
- OCI-Distribution für Anbieterverpackungen und unveränderliche Anbieteridentität

Kein vorhandener Standard definiert den kompletten tragbaren Workload-Lebenszyklus BaseHarbor-Anforderungen in Docker, Podman, Kubernetes und OpenShift, ohne das native Objektmodell einer Plattform in Core zu importieren.

## Grenzen

```text
Repository workload source
        |
        v
Development / source normalization
        |
        v
Build + artifact resolution
        |
        v
Resolved OCI workload plan
        |
        +-------------------------------+
        |                               |
        v                               v
bundled in-process adapter       external protocol adapter
        |                               |
        v                               v
semantic runtime lifecycle       baseharbor.runtime.provider/v1
                                         |
                                         v
                                   provider-native API
```

Harte Invarianten:

- Repository-Quelle/Build-Anweisungen enden vor der Runtime Provider-Grenze
- jeder lauffähige Dienst erreicht Runtime als bereits gelöstes OCI-Image
- `Target.scope ` undurchsichtig ist;`namespace`, Compose-Projekt und Systemd Unit-Namen sind Anbieter Realisierung Details
- runtime-native Objektnamen und Rollout-Bedingungen werden nie zur Anwendungsidentität
- Container-Endpunkte sind Workload-Semantik; öffentliche/host-Exposition bleibt eine Fähigkeit / Lieferung Bedenken
- Die Bereitstellung von Fähigkeiten ist keine Verantwortung des Runtime Providers.
- direkte vs delegierte/GitOps-Abgleich ist keine Runtime Provider-Verantwortung
- Klartext geheimes Material ist nicht portabel Runtime Provider Zustand

## Lebenszyklus

Tragbare semantische Operationen sind:

- Vorflug
- Anwenden
- Bereitschaft abwarten/beobachten
- Beobachten
- Protokolle
- Exec
- Zerstören

Mutierende Operationen müssen idempotent sein. Externe Anbieter verwenden Operations-IDs für asynchrone Mutationen.

## Lokale Umsetzung

Docker und Podman können reichere lokale Mechanik unterhalb der semantischen Grenze behalten, einschließlich:

- Projektdateien
- Zusammenstellung der Eingaben
- Quadlets
- Behälter
- Volumen
- lokale Netze
- Laufzeitspezifische Diagnose

Diese Mechanik sind Implementierungsdetails und MÜSSEN den Vertrag des portablen Runtime Providers NICHT erweitern.

## Kubernetes/OpenShift-Kompatibilität

Eine Kubernetes/OpenShift-Implementierung muss in der Lage sein, denselben tragbaren Laufzeitplan zu verwenden, ohne Kubernetes/OpenShift-Felder in portable Application Intent hinzuzufügen.

Namespace-scoped Betrieb ohne Cluster-Admin, Namespace-Erstellung, CRDs oder Operators müssen für den Adoptionspfad möglich bleiben. Provider-native Erweiterungen können später unterhalb dieser Grenze hinzugefügt werden.


## Verhandlungen über die Verfügbarkeit

Laufzeitanbieter erklären die Verfügbarkeitsgarantie, dass ihre Realisierung tatsächlich erfüllt und überprüft werden kann.

Die portable Applikationsanfrage bleibt globale/sparse Verfügbarkeitsabsicht. Runtime-native Nachbildungsobjekte, Terminierungsbeschränkungen, Ausfalldomänen und Plattformtopologie bleiben Anbieterrealisierungszustand.

Ein Runtime Provider, der eine effektive HA-Anfrage nicht erfüllen kann, gibt vor der Mutation ein typisiertes, nicht unterstütztes Ergebnis zurück.

Eine logische Workload-Komponente kann als 0..N Runtime-Instanzen beobachtet werden. Beobachtung behält alle Instanzen und Aggregate Bereitschaft gegen die angeforderte Garantie, ohne die Komponentenidentität zu ändern.

Die gebündelten Docker Compose- und Podman Quadlet-Anbieter sind explizit UNSUPPORTED für verifizierte HA-Workload-Orchestrierung in v0.4.21. Future Kubernetes/OpenShift-Anbieter können die gleiche Semantik implementieren, ohne den portablen Application Intent zu ändern.

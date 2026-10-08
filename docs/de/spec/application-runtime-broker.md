# Anwendung Laufzeit Broker

Der **Application Runtime Broker** ist BaseHarbors pro Applikations-Laufzeitkontrolloberfläche.

Verwaltete Geheimnisse durch OpenBao sind die ersten über den Broker implementierten Produktionslaufzeiten. Laufzeitressourcen und asynchrone Operationen verwenden die gleiche architektonische Grenze.

~~~text
application
    |
    | mTLS + app-scoped runtime identity
    v
Application Runtime Broker
    |
    +-- /runtime/v1/secrets
    +-- /runtime/v1/resources
    +-- /runtime/v1/operations/{id}
    +-- /runtime/v1/capabilities
    |
    v
capability/provider execution
~~~

## Ein Broker pro Anwendung

Der Broker ist an eine Anwendungs-/Umweltidentität gebunden:

~~~text
spiffe://baseharbor/apps/<app>/<environment>
~~~

Bestehende App-qualifizierte geheime Routen bleiben Kompatibilitäts-Aliasen. Canonische gebundene Routen wiederholen den Anwendungsnamen nicht, da mTLS bereits die Anwendungsidentität behebt.

## Geheimnisse sind ein Laufzeitmodul

Anwendungen, die dynamische verwaltete Geheimnisse verwenden, erfordern den Broker bereits heute.

Kanonische Geheimrouten:

~~~text
POST   /runtime/v1/secrets
POST   /runtime/v1/secrets/resolve
PUT    /runtime/v1/secrets/resolve
DELETE /runtime/v1/secrets/resolve
~~~

OpenBao bleibt hinter dem Broker und seine Anmeldeinformationen bleiben app-scoped.

## Laufzeitressourcen und -operationen

Der gleiche Brokernamespace wird für die Ressourcen der Applikationszeit verwendet:

~~~text
POST   /runtime/v1/resources
GET    /runtime/v1/resources/{resourceId}
DELETE /runtime/v1/resources/{resourceId}
GET    /runtime/v1/resources/{resourceId}/binding
GET    /runtime/v1/operations/{operationId}
GET    /runtime/v1/capabilities
~~~

Eine Fähigkeit wird nur dann als unterstützt beworben, wenn ein autorisierter Provider-Ausführungspfad existiert. Source-Code-Erkennung gewährt niemals Laufzeitberechtigung.

Für `object-storage.s3/v1`, der Broker delegiert Anbietermutation zu einem geteilten **Runtime Provider Executor**. Der Executor authentifiziert den Broker durch die bestehende BaseHarbor Workload SPIFFE Identität, besitzt die Provider-globale administrative Grenze und führt Bucket/IAM Mutationen gegen den ausgewählten S3-Anbieter durch.

## Entwicklungs-Swagger / OpenAPI

Wenn der Broker in einer Dev- oder Entwicklungsumgebung benötigt wird, ermöglicht BaseHarbor standardmäßig eine interaktive Laufzeit-API-Dokumentation.

Der docs listener ist von der mTLS Runtime API getrennt und wird nur auf Host-Loopback veröffentlicht:

~~~text
https://127.0.0.1:<allocated-port>/
~~~

Der Port wird einmal pro Anwendung zugewiesen und im owner-only BaseHarbor Zustand gespeichert. baha app app app app app up and baha app status report the URL.

Der kanonische Vertrag bleibt spec/runtime-api/v1/openapi.yaml. Swagger UI-Assets sind in das BaseHarbor Laufzeitbild eingebettet, so dass die Entwickler-Doks kein öffentliches CDN benötigen.

Umgebung Interaktive docs
| --- | --- |
dev / development enabled by default enabled
Test / Staging ist standardmäßig deaktiviert
Prod / Produktion ist standardmäßig deaktiviert

Operatoren können die Bereitstellungsrichtlinie mit BASEHARBOR_RUNTIME_DOCS_ENABLED=true oder false explizit überschreiben. Diese Einstellung ist keine portable Anwendungsabsicht und gehört nicht in baseharbor.yaml.

## Sicherheitsgrenze

Der Broker läuft weiterhin ohne Docker/Podman-Socket und ohne Provider-globale Administrator-Anmeldeinformationen. Der Runtime Provider Executor hat auch keinen Docker/Podman-Socket, hat keinen Host-veröffentlichten Port und ist nur über Broker-Container über die interne `baseharbor-runtime-control` Netz.

Für Laufzeit-only-Anwendungen besitzt der Pro-App-Broker das deterministische Backend-Netzwerk, das von autorisierten Workload-Diensten genutzt wird, um `baseharbor-runtime`. Wenn die App bereits PostgreSQL/Valkey Runtime Services verwaltet hat, bleibt das vorhandene Backend-Netzwerk maßgeblich. Runtime S3 Provider-Netzwerk-Anhänge ist servicespezifisch: Nur Dienste, die explizit in der Laufzeitberechtigung aufgeführt sind, erhalten sie.

Der Entwickler docs listener verwendet TLS auf Host-Loopback und wird durch eine echte HTTPS-Anfrage als Teil der Laufzeitbereitschaft überprüft. Es stellt nur Dokumentation aus. Es umgeht nicht die authentifizierte Laufzeit-API und zeigt keine Anwendungsanmeldeinformationen, OpenBao-Anmeldeinformationen, Laufzeitträger-Token oder sichere Bindungen auf.

# Application Runtime Broker

Der **Application Runtime Broker** ist die anwendungsgebundene Runtime-Control-Surface von BaseHarbor.

Managed Secrets ueber OpenBao sind die erste produktiv umgesetzte Runtime-Capability. Runtime Resources und asynchrone Operationen verwenden dieselbe Architekturgrenze.

~~~text
Anwendung
    |
    | mTLS + app-spezifische Runtime Identity
    v
Application Runtime Broker
    |
    +-- /runtime/v1/secrets
    +-- /runtime/v1/resources
    +-- /runtime/v1/operations/{id}
    +-- /runtime/v1/capabilities
    |
    v
Capability-/Provider-Ausfuehrung
~~~

## Ein Broker pro Anwendung

Der Broker ist an genau eine Application-/Environment-Identity gebunden:

~~~text
spiffe://baseharbor/apps/<app>/<environment>
~~~

Bestehende app-qualifizierte Secret-Routen bleiben als Compatibility-Alias erhalten. Neue gebundene Routen wiederholen den App-Namen nicht, weil mTLS die Application Identity bereits eindeutig festlegt.

## Secrets sind ein Runtime-Modul

Anwendungen mit dynamischen Managed Secrets brauchen den Broker bereits heute.

Kanonische Secret-Routen:

~~~text
POST   /runtime/v1/secrets
POST   /runtime/v1/secrets/resolve
PUT    /runtime/v1/secrets/resolve
DELETE /runtime/v1/secrets/resolve
~~~

OpenBao bleibt hinter dem Broker und seine Credentials bleiben app-spezifisch.

## Runtime Resources und Operations

Derselbe Broker-Namespace wird fuer Application-Time-Ressourcen verwendet:

~~~text
POST   /runtime/v1/resources
GET    /runtime/v1/resources/{resourceId}
DELETE /runtime/v1/resources/{resourceId}
GET    /runtime/v1/resources/{resourceId}/binding
GET    /runtime/v1/operations/{operationId}
GET    /runtime/v1/capabilities
~~~

Eine Capability darf erst dann als supported erscheinen, wenn ein autorisierter Provider-Ausfuehrungspfad existiert. Source-Code-Erkennung vergibt niemals Runtime-Berechtigungen.

## Development Swagger / OpenAPI

Wenn der Broker in einer dev- oder development-Umgebung benoetigt wird, aktiviert BaseHarbor die interaktive Runtime-API-Dokumentation standardmaessig.

Der Docs-Listener ist vom mTLS-Runtime-API-Listener getrennt und wird nur auf Host-Loopback veroeffentlicht:

~~~text
https://127.0.0.1:<automatisch-vergebener-port>/
~~~

Der Port wird einmal pro Anwendung vergeben und owner-only im BaseHarbor-State gespeichert. baha app apply, baha app up und baha app status zeigen die URL an.

Der kanonische Vertrag bleibt spec/runtime-api/v1/openapi.yaml. Die Swagger-UI-Assets sind im BaseHarbor-Runtime-Image eingebettet; die Dev-Doku braucht kein oeffentliches CDN.

| Environment | Interaktive Doku |
| --- | --- |
| dev / development | standardmaessig an |
| test / staging | standardmaessig aus |
| prod / production | standardmaessig aus |

Operatoren koennen die Deployment-Policy explizit mit BASEHARBOR_RUNTIME_DOCS_ENABLED=true oder false ueberschreiben. Diese Einstellung ist kein portabler Application Intent und gehoert nicht in baseharbor.yaml.

## Security Boundary

Der Broker laeuft weiterhin ohne Docker-/Podman-Socket und ohne providerweite Administrator-Credentials.

Der Development-Docs-Listener verwendet TLS auf Host-Loopback und wird als Teil der Runtime-Readiness wirklich per HTTPS geprueft. Er liefert nur Dokumentation. Er umgeht die authentifizierte Runtime API nicht und gibt weder Application Credentials noch OpenBao-Credentials, Runtime Bearer Tokens oder Secure Bindings aus.

# Runtime Resource API

Die BaseHarbor Runtime Resource API ist die Application-facing Control Surface, mit der eine bereits laufende Anwendung logische Ressourcen anfordern kann.

Fuer die eigentliche Ressource verwendet die Anwendung weiterhin Standardprotokolle. Die Runtime Resource API ist nur fuer Lifecycle-Anforderungen wie Erzeugen, Nachschlagen oder Loeschen einer logischen Ressource zustaendig.

## Kanonischer Vertrag

Der versionierte OpenAPI-3.1-Vertrag liegt unter:

`spec/runtime-api/v1/openapi.yaml`

OpenAPI ist der normative Application-facing API-Vertrag. Swagger UI, Scalar, Redoc oder ein anderer Renderer ist nur eine Darstellungsoberflaeche dieses Vertrags.

Anwendungen sprechen HTTP/JSON mit der Runtime Resource API. Provider-Integrationen bleiben davon getrennt und verwenden weiterhin den BaseHarbor Provider Contract und dessen gRPC-/Protocol-Buffers-Richtung.

## Environment-Grenze fuer interaktive API-Dokumentation

| Environment | Interaktive Swagger-/Scalar-artige UI |
| --- | --- |
| Development | standardmaessig verfuegbar |
| Test / Staging | standardmaessig deaktiviert; nur explizites Plattform-/Operator-Opt-in |
| Production | standardmaessig deaktiviert; nur explizites Plattform-/Operator-Opt-in |

Das ist Deployment-/Plattform-Policy und kein Application Intent. `baseharbor.yaml` bekommt daher kein portables Feld wie `swagger: true`.

Der OpenAPI-Vertrag selbst existiert in jeder Umgebung, auch wenn die interaktive Dokumentation deaktiviert ist.

Das Aktivieren einer interaktiven Dokumentationsoberflaeche schwaecht niemals Authentifizierung oder Autorisierung der API und darf keine Credentials, Tokens oder Secure Bindings offenlegen.

## Resource Lifecycle

Der initiale v1-Vertrag definiert:

```text
GET    /runtime/v1/capabilities
POST   /runtime/v1/resources
GET    /runtime/v1/resources/{resourceId}
DELETE /runtime/v1/resources/{resourceId}
GET    /runtime/v1/resources/{resourceId}/binding
GET    /runtime/v1/operations/{operationId}
```

Eine Create-Anforderung ist providerneutral:

```json
{
  "capability": "object-storage.s3",
  "name": "user-4711"
}
```

Die Anwendung fordert weder SeaweedFS noch AWS S3, Ceph RGW oder ein anderes konkretes Produkt an.

## Authorization

Repository Inspection und Runtime Authorization sind bewusst getrennt. Source Code wie ein S3-`CreateBucket`-Aufruf kann anzeigen, dass `runtime.create` vermutlich benoetigt wird, vergibt die Operation aber niemals.

Ein Runtime Request wird nur akzeptiert, wenn die authentifizierte Workload Identity fuer die angeforderte Capability und Operation explizit autorisiert ist. Die Kenntnis einer Resource-ID reicht niemals als Autorisierung.

## Idempotency und asynchrone Operationen

Jeder mutierende Runtime Request benoetigt einen `Idempotency-Key`.

Wenn die Anwendung nach Timeout oder verlorener Antwort erneut sendet, muss dieselbe logische Anforderung zum selben Operation-/Resource-Ergebnis fuehren und darf keine Duplikate erzeugen.

Die API unterstuetzt auch asynchrone Provider-Arbeit. Eine Anforderung kann eine Ressource im Zustand `provisioning` zusammen mit einer Operation-ID liefern, die ueber `GET /runtime/v1/operations/{operationId}` abgefragt wird.

## Bindings und Secrets

`GET /runtime/v1/resources/{resourceId}/binding` liefert providerneutrale Binding-Metadaten.

Secret-Material wird nicht ueber normale Metadatenfelder ausgegeben. Credentials bleiben hinter der BaseHarbor Secure-Binding-/Runtime-Identity-Grenze.

## Kompatibilitaet

Die Runtime Resource API wird unabhaengig von einzelnen Capability Specifications versioniert. Capability-spezifische Parameter gehoeren in die jeweilige Capability Specification; Provider-spezifische Felder sind keine portablen Runtime-Request-Parameter. Breaking Changes benoetigen eine neue API-Version.

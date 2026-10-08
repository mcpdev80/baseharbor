# Runtime Resource API

Die Runtime Resource API von BaseHarbor ist die anwendungsorientierte Kontrolloberfläche für die Abfrage logischer Ressourcen, während eine Anwendung bereits läuft.

Die Anwendung verwendet weiterhin Standardprotokolle für die Ressource selbst. Die Runtime Resource API ist nur für Lifecycle-Anfragen wie das Erstellen, Aufsuchen oder Löschen einer logischen Ressource.

## Kanonischer Vertrag

Der versionierte OpenAPI 3.1-Vertrag lautet:

`spec/runtime-api/v1/openapi.yaml`

OpenAPI ist der normative anwendungsorientierte API-Vertrag. Swagger UI, Scalar, Redoc oder ein anderer Renderer ist nur eine Präsentationsebene über diesen Vertrag.

Anwendungen verwenden HTTP/JSON gegen die Runtime Resource API. Provider-Integrationen bleiben getrennt und verwenden weiterhin den BaseHarbor-Providervertrag und seine gRPC/Protokoll Buffers-Richtung.

## Umgebungsgrenze für interaktive API-Dokumentation

Umgebung: Interaktiver Swagger/Scalar-Stil: Benutzeroberfläche:
| --- | --- |
Die Entwicklung ist standardmäßig verfügbar
Testen / Stativieren Standardmäßig deaktiviert; explizite Plattform/Opt-in-Opt-in-Opt-in-Opt-in-Opt-in-Opt-in-Opt-in-Opt-in-Opt
Voreingestellte Produktion deaktiviert; explizite Plattform/Opt-in-Opt-in-Opt-in-Opt-in-Opt-in-Opt-in

Dies ist Bereitstellungs-/Plattformrichtlinie, nicht Anwendungsabsicht.`baseharbor.yaml ` darf kein tragbares Feld wie z.B.`swagger: true`.

Der OpenAPI-Vertrag selbst existiert in jeder Umgebung, auch wenn die interaktive Dokumentation deaktiviert ist.

Die Aktivierung einer interaktiven Dokumentation UI schwächt niemals die API-Authentifizierung oder Autorisierung und darf keine Anmeldeinformationen, Tokens oder sichere Bindungen ausstellen.

## Lebenszyklus der Ressourcen

Der ursprüngliche v1-Vertrag definiert:

```text
GET    /runtime/v1/capabilities
POST   /runtime/v1/resources
GET    /runtime/v1/resources/{resourceId}
DELETE /runtime/v1/resources/{resourceId}
GET    /runtime/v1/resources/{resourceId}/binding
GET    /runtime/v1/operations/{operationId}
```

Eine Create-Anfrage ist anbieterneutral:

```json
{
  "capability": "object-storage.s3/v1",
  "name": "user-4711"
}
```

Die Anwendung beantragt keine SeaweedFS, AWS S3, Ceph RGW oder ein anderes Betonprodukt.`object-storage.s3/v1`, der aktuelle lokale Referenzpfad löst sich auf SeaweedFS hinter der Provider-Grenze. Docker verwendet Docker Compose; Podman gibt die gleiche Definition wie Quadlet.

## Genehmigung

Repository-Inspektion und Laufzeitgenehmigung sind bewusst getrennt. Quellennachweise wie ein S3 `CreateBucket` Anruf kann darauf hindeuten, dass `runtime.create` Es ist notwendig, aber es gewährt die Operation nie.

Eine Laufzeitanfrage wird nur akzeptiert, wenn die authentifizierte Workload-Identität explizit für die angeforderte Funktion und den gewünschten Betrieb autorisiert ist.

## Idempotenz und asynchrone Operationen

Jede mutierende Laufzeitanforderung erfordert eine `Idempotency-Key`.

Wenn eine Anwendung nach einem Timeout oder einer verlorenen Antwort zurückholt, muss die gleiche logische Anforderung auf das gleiche Betriebs-/Ressourcenergebnis aufgelöst werden, anstatt Duplikate zu erstellen.

Die API führt asynchrone Mutationen aus. Erzeugen und Löschen einer Operations-Identität mit `pending`, ` running `, ` succeeded ` or ` failed`Zustand. Operation Zustand wird durch die pro-Anwendung Broker fortbestehen und unvollendete Arbeit wird nach Broker-Neustart versöhnt.

## Bindungen und Geheimnisse

`GET /runtime/v1/resources/{resourceId}/binding ` ist der explizit authentifizierte verbindliche Endpunkt.`object-storage.s3/v1` Es liefert die von einem nativen S3-Client benötigten S3-Endpunkt-, Bucket-, Region- und Ressourcen-Scope-Anmeldeinformationen zurück.

Die Anmeldeinformationen werden absichtlich **nicht** im asynchronen Betriebszustand, gewöhnlichen Ressourcen-Metadaten, Protokollen, Metriken oder `baseharbor.yaml`. Sie verbleiben im geschützten Ausführungszustand und werden nur über die anwendungsskopierte mTLS + Laufzeit-Token geschützte verbindliche Anfrage offengelegt. Provider-globale Administrator-Anmeldeinformationen verlassen den Laufzeit-Provider-Executor nie.

## Vereinbarkeit

Die Runtime Resource API wird unabhängig von individuellen Leistungsspezifikationen versioniert. Capability-spezifische Parameter gehören zur entsprechenden Leistungsspezifikation; providerspezifische Felder sind keine portablen Runtime Request-Parameter. Durchbrechen von API-Änderungen ist eine neue API-Version erforderlich.


## Ausführungspfad des Laufzeitanbieters

Für die erste ausführbare Laufzeit ist der Request-Pfad:

```text
authorized workload service
        | mTLS + runtime token
        v
Application Runtime Broker
        | mTLS / SPIFFE application identity
        v
shared Runtime Provider Executor
        | provider-admin boundary
        v
object-storage.s3/v1 provider
```

Der Broker und der Executor mounten keinen Docker/Podman Socket. Der Shared Executor hat keinen host-veröffentlichten Port und kommuniziert mit Broker nur auf dem internen `baseharbor-runtime-control` network. Laufzeit-S3-Workloads erhalten Provider-Netzwerk-Zugriff nur, wenn dieser spezifische Workload-Dienst ausdrücklich für die S3-Laufzeitfähigkeit autorisiert ist.

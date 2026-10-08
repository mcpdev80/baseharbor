# Standards-first-Architektur

BaseHarbor verwendet etablierte Standards, bevor eigene Verträge entstehen.

## Regeln

1. Ein vorhandener offener Standard MUSS verwendet werden, wenn er die geforderte Semantik abdeckt.
2. Fehlt ein geeigneter formaler Standard, SOLLTE ein etablierter De-facto-Standard genutzt werden.
3. Anerkannte Architektur- und API-Muster SOLLTEN die Modelle prägen, wenn sie die Interoperabilität erhöhen, ohne plattformspezifische Konzepte zu importieren.
4. BaseHarbor-eigene Verträge definieren nur Semantik, die etablierte Standards nicht angemessen abdecken.
5. Erweiterungen sind ausdrücklich gekennzeichnet, versioniert und providerneutral.
6. Providerspezifische Verträge bleiben hinter der Provider-Grenze.

Jede neue Service-Art beziehungsweise jeder Provider dokumentiert vor Implementierungsbeginn bestehende und übernommene Standards, Abweichungen, BaseHarbor-Erweiterungen und die Kompatibilitätsfolgen.

## Übernommene Standards und Muster

| Bereich | Regel |
| --- | --- |
| Schema und Validierung | JSON Schema 2020-12 |
| Workload-Verbindungsdaten | Service Binding Specification 1.1, wo semantisch passend |
| Ressourcen und Provider | Crossplane-Muster für Ressourcen, Provider und Reconciliation als Architekturvorlage; keine Kubernetes-API-Objekte im portablen Intent |
| Katalog, Provisioning, Binding | Open Service Broker API-Konzepte, wo passend |
| Provider-Prozessgrenze | gRPC und Protocol Buffers |
| Provider-Artefakte | OCI Image/Distribution, Digest als Identität |
| Observability | OpenTelemetry und OTLP |
| Identity | OpenID Connect/OAuth, wo passend |
| Messaging-API | AsyncAPI |
| Ereignis-Umschläge | CloudEvents, wo Event-Semantik benötigt wird |
| Redis-/Valkey-kompatibler Cache | Provider deklariert RESP-Kompatibilität |
| Object Storage | S3-API als De-facto-Protokollfähigkeit, nicht als allgemeine Service-Identität |

## Service, Protokoll, Provider und Produkt bleiben getrennt

```text
Application Intent
  -> BaseHarbor Service-Vertrag
  -> Protokoll- / Semantikanforderungen
  -> Provider-Auflösung
  -> Provider-Implementierung
  -> Produkt- / Engine-Realisierung
```

Beispiel:

```text
service contract: sql/v1
provider:         baseharbor/postgresql@1.3.0
product:          PostgreSQL 18.x
```

Diese Versionen sind unabhängig.

## Kompatibilität mit v0.4

Kennungen wie `database.sql/v1`, `cache.key-value/v1`, `object-storage.s3/v1` und `telemetry.otlp/v1` bleiben zunächst bestehen. Vor dem Freeze wird jede als kanonischer Service-Vertrag, Protokoll-/Semantikfähigkeit innerhalb eines größeren Service-Vertrags oder migrationspflichtiger Kompatibilitätsalias eingeordnet.

Ein bestehender Application-Vertrag darf erst geändert werden, wenn sein Kompatibilitätspfad dokumentiert und getestet wurde.

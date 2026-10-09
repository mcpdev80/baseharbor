# Manifest-Referenz

Das Application-Manifest beschreibt portablen Application-Intent.

## Grundregel

In das Manifest gehören ausschließlich Application-Anforderungen. Runtime-/Provider-Implementierungsdetails, generierte Zugangsdaten, private Schlüssel, lokale Trust-Pfade und geschützter Deployment-Zustand gehören nicht hinein.

## Zentrale Felder

| Feld | Bedeutung |
| --- | --- |
| `version` | Version des Application-Vertrags |
| `app.id` | Stabile opake `application_id` im portablen Repository-Vertrag |
| `app.name` | Menschenlesbarer Application-Name |
| `app.environment` | Deklarierter beziehungsweise voreingestellter Umgebungskontext |
| `services` | Anforderungen an Application-Fähigkeiten |
| `secrets.required` | Erforderliche logische Secret-Namen |
| `workload.components` | Stabile logische Workload-Komponenten; Quelltyp und -pfad bleiben außerhalb des portablen Intent |
| `identity` | Portable OIDC-/Identity-Anforderungen |
| `telemetry` / `metrics` / `logs` | Portable Observability-Anforderungen |
| `runtime.permissions` | Explizite Application-Berechtigungen für die Runtime API |
| `exposures` | Providerneutrale HTTP-Veröffentlichungen |
| `ha` | Globale Hochverfügbarkeitsanforderung; false/Standard fordert keine HA-Garantie |
| `availability` | Gezielte HA-/Instanzzahlabweichungen für Komponenten und Capabilities |
| `consumes` | Logische Schnittstellennutzung von Applications/Komponenten ohne Runtime-Adressen |

## Service-Familien ab v0.4.19

SQL, Key-Value-Cache, dauerhafte Key-Value-Datenbank, Dokumentdatenbank, Queue-/Pub/Sub-/Stream-Messaging, Objektspeicher, Secrets, Identity und explizit modellierte Provider-Management-Oberflächen.

Die exakten Schemas und Typen bestimmen die Feldstruktur. PostgreSQL, Valkey, MongoDB und RabbitMQ sind Produkte, keine Ersatznamen für portable Service-Typen. Versionierte Service-/Capability-Spezifikationen legen die genaue Semantik fest.

## Hochverfügbarkeit ab v0.4.21

Ohne `ha` beziehungsweise mit `ha: false` wird PostgreSQL als Einzelserver realisiert. HA muss global ausdrücklich angefordert werden:

```yaml
ha: true
```

Optionale `availability`-Einträge dürfen `ha` und eine feste Zahl `instances` als gezielte Abweichungen definieren. Providerspezifische Clusterbegriffe gehören nicht hinein.

Jede HA-Anforderung wird mit Runtime- und Capability-Provider gesondert ausgehandelt. Nicht unterstützte Garantien werden vor Änderungen abgelehnt; kein stillschweigender Rückfall auf eine Instanz.

## Application Consumption ab v0.4.21

`consumes` identifiziert einen logischen Produzenten über stabile `application_id`, Komponente und Schnittstelle. Ohne `application_id` gilt die aktuelle Application.

Runtime-Adressen, Pods, Container, Namespaces, Nodes und Replicas sind keine portablen Consumption-Identitäten. Der konkrete Endpunkt ist Deployment-Zustand.

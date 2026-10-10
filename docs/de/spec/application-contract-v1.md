# Application Contract v1

## Gültigkeitsbereich

Diese Spezifikation definiert die stabile, portable Grenze für die Anforderungen einer Application.

## Anforderungen

- Portabler Intent MUSS die Anforderungen der Application beschreiben, nicht die Auswahl konkreter Infrastrukturprodukte.
- Runtime-spezifische Realisierungsdetails DÜRFEN im portablen Intent NICHT vorausgesetzt werden.
- Providerspezifische Konfiguration DARF nur dann Teil des portablen Intent sein, wenn sie nachweislich eine portable Application-Semantik beschreibt.
- Generierte Zugangsdaten und geschützter Deployment-Zustand DÜRFEN NICHT als gewöhnlicher portabler Intent abgelegt werden.
- Nicht unterstützte, aber benötigte Semantik MUSS vor jeder Änderung scheitern.

## Stabile Identität

Portable und persistierte Manifeste MÜSSEN eine gültige opake UUIDv4 als `app.id` enthalten. Sie ist die stabile `application_id`.

Menschenlesbarer Application-Name, Umgebung, Repository-Pfad, Compose-Projektname und Runtime-eigene Objektnamen DÜRFEN die dauerhafte Eigentümeridentität `application_id` NICHT ersetzen.

Das Ändern eines lesbaren Namens oder Repository-Pfads DARF NICHT implizit eine neue Application-Identität erzeugen.

Die Deployment-Identität verbleibt im geschützten Target-lokalen Zustand und DARF NICHT in portablen Application-Intent wandern.

## Capability-Anforderungen

Die Anforderungen der Application an Fähigkeiten sind produktneutral.

Die v1-Servicefamilie umfasst unter anderem:

- SQL;
- rekonstruierbaren Key-Value-Cache;
- dauerhaft gespeicherte Key-Value-Datenbank;
- Dokumentdatenbank;
- Queue-Messaging;
- Pub/Sub-Messaging;
- Stream-Messaging;
- Objektspeicher;
- verwaltete Secrets;
- OIDC-Identity;
- HTTP-Exposition;
- Metrik-, Log- und Telemetrie-Semantik.

Ein Referenzprodukt DARF mehrere logische Verträge implementieren. Die Wiederverwendung eines Produkts DARF aber keine unterschiedlichen Application-Semantiken zusammenlegen. Insbesondere bleibt dauerhaftes `database.key-value` unabhängig von `cache.key-value`.

## Identität von Workload-Komponenten

Portable Workload-Identität wird durch stabile logische Komponenten wie `api`, `worker` oder `web` ausgedrückt.

Die logische Identität MUSS von der ursprünglichen Repository-/Workload-Quellidentität unabhängig bleiben:

```text
logical component: api

Compose:      services.api
Quadlet:      api.container
Kubernetes:   Deployment/api
```

Workload-Quelltyp, Quellpfad, Kubernetes-Kind/-Name, Quadlet-Dateiname und Compose-Service-Syntax sind Angaben zur Herkunft, keine Pflichtfelder des portablen Application-Intent.

Bei der Repository-Adoption DARF eine gesonderte, sicher commitbare `baseharbor.repository.yaml` verwendet werden, wenn eine maßgebliche Quellauswahl gespeichert werden muss. Sie beschreibt Repository-Autorendaten, nicht Application-Intent.

## Trennung von Provider und Runtime

Portabler Application-Intent DARF NICHT enthalten:

- Docker-/Podman-/Kubernetes-/OpenShift-Realisierungsdetails;
- Provider-Produktnamen, wenn eine portable Fähigkeit existiert;
- Provider-Platzierung oder Runtime-Auswahl;
- Eigentümerschaft eines Delivery-Providers;
- private Schlüssel, generierte Zugangsdaten oder maschinenlokale Trust-Pfade.

## Kompatibilität

Nach dem Contract-Freeze verlangen inkompatible Änderungen des öffentlichen Vertrags eine explizite Vertragsversionierung.

Verhalten vor dem Freeze, das der akzeptierten v1-Architektur widerspricht, kann ersetzt werden, ohne alte Formen durch Kompatibilitäts-Aliase weiterzuführen.

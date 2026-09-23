# Provider

Provider setzen BaseHarbor-Semantik um, ohne den Application Contract zu verändern.

## Runtime Provider

Führen Workloads aus.

Aktuell: Compose.

Später: Kubernetes und OpenShift.

## Capability Provider

Realisieren Anforderungen wie SQL, Cache, Object Storage, Secrets, Identity oder Observability.

## Delivery Provider

Bestimmen, wie gewünschter Runtime-State ausgeliefert und reconciled wird.

## Mitgelieferte und externe Provider

Die aktuellen First-Party-Provider liegen noch im BaseHarbor-Repository, besitzen aber eine eigene Provider-ID und Implementierungsversion. BaseHarbor-Version, Provider-Version, Capability-Spezifikation und konkrete Produktversion sind getrennte Informationen.

Mitgelieferte Provider werden über dieselbe Provider-Contract-Grenze aufgelöst, die später externe Provider verwenden. Eine spätere Auslagerung in ein eigenes Repository ändert deshalb nicht den Application Intent.

## Placement

- `application`: BaseHarbor besitzt den Lifecycle.
- `shared`: mehrere Anwendungen teilen einen klar abgegrenzten Provider.
- `external`: BaseHarbor bindet an Infrastruktur, die es nicht besitzt.

Nicht unterstützte Anforderungen müssen vor der Mutation scheitern.

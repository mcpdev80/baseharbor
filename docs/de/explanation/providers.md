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

## Placement

- `application`: BaseHarbor besitzt den Lifecycle.
- `shared`: mehrere Anwendungen teilen einen klar abgegrenzten Provider.
- `external`: BaseHarbor bindet an Infrastruktur, die es nicht besitzt.

Nicht unterstützte Anforderungen müssen vor der Mutation scheitern.

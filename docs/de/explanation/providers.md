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

### PostgreSQL-Administration und Anwendungszugriff

Der gemeinsam genutzte PostgreSQL-Referenzprovider besitzt genau eine interne Administrationsidentität, `baseharbor_admin`. Sie gehört ausschließlich zur BaseHarbor-Control-Plane und dient nur dem Provider-Lifecycle.

Jede registrierte SQL-Ressource besitzt eine eigene Datenbank, eine eigene Least-Privilege-Rolle und eine geschützte Credential-Referenz. Application Bindings enthalten ausschließlich Host, Port, Datenbank, App-Rolle, App-Credential und Trust-Material dieser Ressource. Provider-weite Administrations-Credentials verlassen die Provider-Grenze niemals.

Backup, Restore und Destroy leiten ihren Ressourcensatz aus der geschützten Registrierung ab. Mehrdeutige Ownership führt zu Fail-Closed; rekonstruierte Namen allein autorisieren keine Löschung.


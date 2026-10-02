# Provider

Provider setzen die BaseHarbor-Semantik um, ohne den Anwendungsvertrag zu verändern.

## Runtime-Provider

Führen Workloads aus.

Aktuell: Compose.

Später: Kubernetes und OpenShift.

## Fähigkeits-Provider

Realisieren Anforderungen wie SQL, Cache, Objektspeicher, Geheimnisse, Identität oder Observability.

## Auslieferungs-Provider

Bestimmen, wie der gewünschte Runtime-Zustand ausgeliefert und abgeglichen wird.

## Mitgelieferte und externe Provider

Die aktuell mitgelieferten Provider liegen noch im BaseHarbor-Repository, besitzen aber eine eigene Provider-ID und Implementierungsversion. BaseHarbor-Version, Provider-Version, Capability-Spezifikation und konkrete Produktversion sind getrennte Informationen.

Mitgelieferte Provider werden über dieselbe Provider-Vertragsgrenze aufgelöst, die später externe Provider verwenden. Eine spätere Auslagerung in ein eigenes Repository ändert deshalb nicht die Anwendungsanforderung.

## Platzierung

- `application`: BaseHarbor besitzt den Lebenszyklus.
- `shared`: mehrere Anwendungen teilen einen klar abgegrenzten Provider.
- `external`: BaseHarbor bindet an Infrastruktur, die es nicht besitzt.

Nicht unterstützte Anforderungen müssen vor der Mutation scheitern.

### PostgreSQL-Administration und Anwendungszugriff

Der gemeinsam genutzte PostgreSQL-Referenzprovider besitzt genau eine interne Administrationsidentität, `baseharbor_admin`. Sie gehört ausschließlich zur BaseHarbor-Control-Plane und dient nur dem Provider-Lifecycle.

Jede registrierte SQL-Ressource besitzt eine eigene Datenbank, eine eigene Rolle mit minimalen Rechten und eine geschützte Zugangsdaten-Referenz. Anwendungsbindungen enthalten ausschließlich Host, Port, Datenbank, App-Rolle, App-Zugangsdaten und Vertrauensmaterial dieser Ressource. Provider-weite Administrationszugangsdaten verlassen die Provider-Grenze niemals.

Sicherung, Wiederherstellung und Löschen leiten ihren Ressourcensatz aus der geschützten Registrierung ab. Mehrdeutiger Besitz führt zu einem sicheren Abbruch; rekonstruierte Namen allein autorisieren keine Löschung.


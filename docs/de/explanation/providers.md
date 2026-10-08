# Provider

Provider setzen die BaseHarbor-Semantik um, ohne den Anwendungsvertrag zu verändern.

## Runtime-Provider

Führen Workloads aus.

Aktuell: Docker und rootless Podman. Compose ist ein Workload-Source-/Eingabemodell, keine portable Runtime-Identität. Docker verwendet Docker Compose; Podman erzeugt native Quadlets und verwaltet sie mit `systemd --user`.

Später: Kubernetes und OpenShift.

## Fähigkeits-Provider

Realisieren Anforderungen wie SQL, Cache, Objektspeicher, Geheimnisse, Identität oder Observability.

## Auslieferungs-Provider

Bestimmen, wer den gewünschten Runtime-Zustand abgleicht: bei `direct` besitzt BaseHarbor den Abgleich, bei `delegated` ein externer Reconciler. Runtime, Capability und Delivery bleiben getrennte Achsen.

## Mitgelieferte und externe Provider

Die aktuell mitgelieferten Provider liegen noch im BaseHarbor-Repository, besitzen aber eine eigene Provider-ID und Implementierungsversion. BaseHarbor-Version, Provider-Version, Capability-Spezifikation und konkrete Produktversion sind getrennte Informationen.

Mitgelieferte Provider werden über dieselbe Provider-Vertragsgrenze aufgelöst, die später externe Provider verwenden. Eine spätere Auslagerung in ein eigenes Repository ändert deshalb nicht die Anwendungsanforderung.

## Platzierung

- `application`: eigener Provider-Lebenszyklus für eine Application/Umgebung.
- `shared`: Target-/Provider-Infrastruktur mit isolierten Application-Ressourcen und Zugangsdaten.
- `external`: BaseHarbor bindet an Infrastruktur, die es nicht besitzt.

Nicht unterstützte Anforderungen müssen vor der Mutation scheitern.

SQL, Secrets und Identity bilden den verpflichtenden Core, aktuell mit PostgreSQL, OpenBao und Keycloak. Die Console ist optional. Core-Capabilities sind verpflichtend; Provider können shared oder pro Application isoliert platziert sein. Zusätzliche Isolation kann weitere Instanzen und Ressourcen benötigen.

Capabilities bleiben produktneutral: `database.document` schreibt kein MongoDB fest, `messaging.queue` kein RabbitMQ. Dauerhafter `database.key-value` bleibt von rekonstruierbarem `cache.key-value` getrennt, auch wenn beide Valkey verwenden.

Externe Provider-Registrierungen übertragen keinen Infrastruktur-Besitz. Entfernen löscht nur die Referenz/Bindung, niemals den fremden Dienst. `provider_instance_id`, `application_id` und `deployment_id` sind getrennte stabile Identitäten; Container-Namen oder Repository-Pfade ersetzen keine Besitzprüfung.

### PostgreSQL-Administration und Anwendungszugriff

Der gemeinsam genutzte PostgreSQL-Referenzprovider besitzt genau eine interne Administrationsidentität, `baseharbor_admin`. Sie gehört ausschließlich zur BaseHarbor-Control-Plane und dient nur dem Provider-Lifecycle.

Jede registrierte SQL-Ressource besitzt eine eigene Datenbank, eine eigene Rolle mit minimalen Rechten und eine geschützte Zugangsdaten-Referenz. Anwendungsbindungen enthalten ausschließlich Host, Port, Datenbank, App-Rolle, App-Zugangsdaten und Vertrauensmaterial dieser Ressource. Provider-weite Administrationszugangsdaten verlassen die Provider-Grenze niemals.

Sicherung, Wiederherstellung und Löschen leiten ihren Ressourcensatz aus der geschützten Registrierung ab. Mehrdeutiger Besitz führt zu einem sicheren Abbruch; rekonstruierte Namen allein autorisieren keine Löschung.

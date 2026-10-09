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


## Weitere unveränderte technische Beispiele

```text
runtime != capability != delivery
```

```text
one shared PostgreSQL provider
├── app-a database + least-privilege role
├── app-b database + least-privilege role
└── app-c database + least-privilege role
```

```text
provider_instance_id != application_id != deployment_id
```


Technische Kennungen: `database.sql`, `messaging.pubsub`, `messaging.stream`, `object-storage.s3`.


## Explizite HA und native Provider-Topologie

Bei einer frischen Installation wählen fehlendes `ha` und `ha: false` die Standardtopologie. `ha: true` wählt die unterstützte native Topologie; ein Komponenten-Override hat Vorrang vor der globalen Anforderung. Feste native Mitgliederzahlen werden vor Änderungen geprüft. Unabhängige logische Instanzen bleiben unabhängige Datenbestände, auch wenn ihre Namen mit einer Zahl enden.

Der folgende Audit umfasst Core und gebündelte Application-Provider. Die Zahlen beschreiben Daten- oder Dienstrollen, nicht die gesamte Containerzahl. Runtime-Status und Doctor vergleichen geschützte gespeicherte Compose-Definitionen mit den laufenden Diensten der ausgewählten Runtime. Sie melden angeforderte/aktive HA, Datenmitglieder, Dienstmitglieder, konfigurierte Replikation, Hilfsrollen und die tatsächliche Ausfalldomäne Runtime-Host. Nicht erhobene Replikations- oder Failover-Nachweise werden ausdrücklich benannt; mehrere Container allein beweisen keine Replikation.

| Provider | Standardtopologie | Explizite native HA | Datenreplikation | Hilfsdienste | Ressourcenbedarf / Audit-Ergebnis |
| --- | --- | --- | --- | --- | --- |
| Core PostgreSQL | Ein PostgreSQL-Datenmitglied | Drei Patroni-Datenmitglieder und drei etcd-Voter | Streaming-WAL an zwei Standbys | Stabiler SQL-Proxy, Administrationsclient, Initialisierungs-/TLS-Jobs | Ein statt drei Daten-Volumes, zusätzlich drei HA-DCS-Volumes; Proxy/Admin sind keine Datenbankreplikate |
| Core OpenBao | Ein OpenBao-Prozess mit Core-SQL-Datenbank | Drei OpenBao-Prozesse mit gemeinsamer Core-HA-SQL-Datenbank | SQL-Replikation gehört PostgreSQL; OpenBao-Prozesse sind keine getrennten Kopien des Datenbestands | Stabiler API-Proxy und Administrationsclient | Ein statt drei Anwendungsprozesse; SQL-Speicher wird einmal gezählt |
| Core / Application Keycloak | Ein Identity-Prozess und ein eigenes SQL-Datenmitglied | Drei Identity-Prozesse, drei Patroni-SQL-Mitglieder und drei etcd-Voter | SQL-WAL-Replikation; Identity-Prozesse teilen den Datenbestand | SQL-Proxy, Administrations-/TLS-/Initialisierungsdienste | Identity- und Datenbankmitglieder werden getrennt gezählt; feste Kardinalität drei |
| Shared PostgreSQL | Ein Datenmitglied pro Environment-Grenze | Ungerade Mitgliederzahl ab drei; Standard drei | Natives Streaming-WAL | SQL-Proxy, Administrationsclient, Initialisierung und optionale UI | Ein statt N Daten-Volumes und natives DCS; ein bestehendes Environment wechselt die Topologie nicht implizit |
| Application PostgreSQL | Ein isoliertes SQL-Mitglied pro logischer Instanz | Application-Platzierung lehnt HA ausdrücklich ab; für HA den nativen Shared-Provider verwenden | Keine im isolierten Single-Modus | Optionale Administration/UI | Bisher stillschweigende Single-Realisierung angeforderter HA scheitert jetzt vor Änderungen; diese Platzierungsgrenze bleibt ausdrücklich bestehen |
| SeaweedFS | Ein kombiniertes Master-/Volume-/Filer-/S3-Mitglied | Ungerade Mitgliederzahl ab drei; Standard drei | Native Volume-Replikation `100` und koordinierte Filer-Metadaten; Single verwendet `000` | Stabiler authentifizierter TLS-S3-Gateway; optionale Managementdienste | Ein statt N Daten-Volumes; der unbedingte Drei-Node-Standard ist entfernt |
| Prometheus | Ein Scraper/TSDB | Standardmäßig zwei unabhängige Scraper/TSDBs; explizite Anzahl ab zwei | Null Replikate eines gemeinsamen Datenbestands: jede TSDB speichert ihre eigene Scrape-Historie | Stabiler authentifizierter Zugangs-Gateway | Ein statt N TSDB-Volumes; der unbedingte Zwei-Instanzen-Standard ist entfernt |
| OpenTelemetry Collector | Ein Receiver | Standardmäßig zwei Receiver; explizite Anzahl ab zwei | Keine Datenreplikate; Receiver sind zustandslos | Stabiler authentifizierter OTLP-Gateway | Ein statt N Collector-Prozesse; der unbedingte Zwei-Receiver-Standard ist entfernt |
| Loki | Ein Datendienst mit lokalem Speicher | Drei native Lese-/Schreibmitglieder mit Replikationsfaktor zwei | Zwei Ingester-Kopien; langfristige Objekte verwenden die ausdrücklich angeforderte Object-Storage-Topologie | Alloy-Sammlung und authentifizierter Zugangs-Gateway | Ein statt drei lokale WAL-/Daten-Volumes; Loki-HA aktiviert nicht implizit S3-HA |
| Tempo | Ein Prozess mit lokalem Speicher | Verteilte native Rollen: zwei Distributoren, sechs Live Stores über drei Partitionen/zwei logische Zonen, drei Block Builder, zwei Query Frontends, zwei Querier, ein Backend Scheduler und zwei Backend Worker | Zwei Live-Store-Eigentümer pro Partition; drei Redpanda-Broker; Object Storage folgt seiner eigenen Anforderung | Kafka-Initialisierung und authentifizierter Query-Zugang | Achtzehn Tempo-Dienstrollen plus drei Redpanda-Datenbroker; `instances: 2` ist eine native Topologieangabe, nicht die gesamte Containerzahl zwei |
| Valkey Cache / dauerhafter Key-Value | Ein Standalone-Datenmitglied pro logischer Instanz | Ungerade Mitgliederzahl ab drei; Standard drei, zusätzlich drei Sentinel-Voter | Ein Primary und N−1 Replikate | Access-Proxy, Sentinel und optionale Management-/UI-Zugänge | Ein statt N Daten-Volumes pro Instanz; Sentinel/Proxy/UI sind keine Datenreplikate |
| RabbitMQ | Ein Broker pro logischer Instanz | Ungerade Mitgliederzahl ab drei; Standard drei | Provider-native Quorum Queues / replizierte Streams | Stabiler Access-Proxy und optionaler Managementzugang | Ein statt N Broker-Daten-Volumes; die explizite Mitgliederanforderung bestimmt die Replikationstopologie |
| MongoDB | Ein Standalone-Datenmitglied pro logischer Instanz | Ungerade Mitgliederzahl ab drei; Standard drei | Natives Replica Set mit Primary/Secondaries | Optionale Management-UI und deren Zugangs-Gateway | Ein statt N Daten-Volumes; Standalone wird nicht als Replica Set bezeichnet |
| Gateways, Administrationsclients, Init-Jobs, Alloy und UI-Dienste | Starten nur für ihre angeforderte technische/Management-Rolle | Kein unabhängiger Daten-HA-Standard | Keine Replikate des Provider-Datenbestands | Diese Dienste sind selbst Hilfsrollen | CPU/RAM und eigener UI-Zustand zählen weiterhin zum Gesamtbedarf; sie werden nicht allein zur Verringerung der Containerzahl entfernt |

Die Ressourcenangaben beschreiben Prozesse und persistente Speicherbereiche. CPU/RAM-Verbrauch hängt von Workload, Aufbewahrung und Image-Konfiguration ab; erfundene Messwerte oder allgemeine Mindestwerte werden nicht behauptet. Die Runtime-Abnahme zeichnet native Mitglieder-/Hilfsnamen auf und prüft authentifizierte Readiness, wiederholte Reconciliation und besitzgebundenes Löschen.

### Bestehende Installationen und Recovery

Bestehende Single- oder HA-Datentopologie bleibt erhalten. Eine widersprüchliche explizite oder standardmäßige Manifest-Anforderung scheitert vor Änderungen an gespeichertem Compose, Zugangsdaten oder nativen Mitgliedern; Wartung ohne neue Topologieanforderung verwendet die gespeicherte Mitgliederzahl. Es gibt keine automatische destruktive Cluster-zu-Single-Migration. Die native Abnahme versucht außerdem eine widersprüchliche Anforderung und prüft unveränderte laufende Containeridentitäten.

Provider-SQL-Recovery verwendet die geschützte Operator-Identität der Installation ausschließlich innerhalb der Wiederherstellung der Provider-eigenen Datenbank. Das verifizierte Archiv erhält ursprünglichen Besitz und ACLs; Application-Zugangsdaten behalten minimale Rechte. OpenBao wird vor der Readiness-Prüfung entsiegelt. Recovery eines ausgefallenen Providers akzeptiert einen entfernten oder gestoppten Container nur, wenn geschütztes gespeichertes Compose und freigegebene unveränderliche Original-/Ziel-Image-Digests die besitzgebundene Quelle beweisen; die normale Upgrade-Auswahl erfordert weiterhin lebendes natives Inventar.

Das Tracking-Issue ist [#851](https://github.com/mcpdev80/baseharbor/issues/851), direkt integriert in [PR #837](https://github.com/mcpdev80/baseharbor/pull/837). Runtime-Nachweise und finaler Kandidat werden dort festgehalten. Dieser Audit allein erklärt keine Release-Bereitschaft und behauptet keine grünen Ergebnisse ausstehender Runtime-Gates.

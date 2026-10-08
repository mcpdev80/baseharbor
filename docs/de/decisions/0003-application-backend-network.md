# ADR 0003: Stabiles Backend-Netzwerk für Anwendungen

- Status: angenommen
- Geburtsdatum: 2026-09-08

## Kontext

BaseHarbor bietet Backend-Dienste wie PostgreSQL und Valkey für unabhängige Anwendungen an. Die MVP-Referenzanwendungen (AWC, MailFlow und AI-Coding-System) bringen bereits eigene Containertopologie- und Compose-Netzwerke mit.

BaseHarbor muss die vorhandenen Workloads mit den Backend-Diensten verbinden, die es verwaltet, ohne anwendungseigene Netzwerke zu ersetzen, die Anwendungsarchitektur umzuschreiben oder eine generische Containerhosting-Plattform zu werden.

Die Anwendung muss auch ohne BaseHarbor nutzbar bleiben.

## Entscheidung

Jede BaseHarbor Anwendungsumgebung hat eine stabile logische **Application Backend Network**.

Für die aktuelle lokale Docker Compose / Podman Quadlet Laufzeit ist der Netzwerkname deterministisch:

```text
baseharbor-<application>-<environment>_default
```

BaseHarbor entlarvt diese Identität durch `ApplicationBackendNetworkName` anstatt den Code für die Workload-Integration zuzulassen, um Provider-Namensregeln unabhängig zu rekonstruieren.

BaseHarbor verwaltet PostgreSQL und Valkey Dienste laufen auf diesem Laufzeit-eigenen Backend-Netzwerk. Docker realisiert es durch Compose; Podman realisiert es über eine generierte Quadlet-Netzwerkeinheit. Zukünftige Workload-Integration heftet die Anwendungs-Container, die Backend-Zugang zu diesem Netzwerk als **additional** Netzwerk erfordern.

Die anwendungseigenen Netze bleiben unverändert.

Beispiel:

```text
MailFlow internal network
        |
   api / worker / web
        |
        +---------------- Application Backend Network ----------------+
                                  |                  |
                             PostgreSQL            Valkey
```

Für AWC erlaubt die gleiche Regel dem Koordinator oder anderen ausgewählten Workload-Containern, die bestehenden `awc-runtime` Beziehungen bei gleichzeitigem Erreichen von BaseHarbor-gemanagten Diensten.

## Isolierung

Das Application Backend Network ist nach Anwendung und Umgebung scoped.

Daher:

- `mailflow/prod ` und`mailflow/dev` kein Backend-Netzwerk teilen;
- `mailflow/prod ` und`awc/prod` kein Backend-Netzwerk teilen;
- Unabhängige Anwendungen sind nicht standardmäßig mit den BaseHarbor-Backend-Diensten des jeweils anderen verbunden.

Normal LAN/Internet egress ist durch diese Entscheidung nicht deaktiviert. Anwendungs-Workloads wie AWC, MailFlow und AI-Coding-System müssen weiterhin externe Dienste wie AgentGateway, Mailserver, Git-Forges und andere Anbieterendpunkte erreichen können.

## Host- und Container-Zugriff sind unterschiedliche Ansichten der gleichen logischen Dienste

Host-run-Entwicklung hält den bestehenden Loopback-Only-Vertrag, zum Beispiel:

```text
DATABASE_URL=postgresql://...@127.0.0.1:<allocated-port>/...
REDIS_URL=redis://...@127.0.0.1:<allocated-port>/0
```

Containerisierte Workloads erhalten Container-routable URLs mit Dienst-DNS-Namen im Application Backend Network.

Anwendungscode verbraucht immer noch gewöhnliche Variablen wie `DATABASE_URL` und `REDIS_URL`; BaseHarbor wählt die richtige Ansicht, wenn der Laufzeitvertrag materialisiert/injiziert wird.

## Eigentum und Lebenszyklus

Das aktuelle lokale Laufzeit-Backend besitzt die Erstellung und Entfernung des Netzwerks zusammen mit dem BaseHarbor-Backend-Projekt. Docker verwendet Compose-Ressourcen; Podman verwendet generierte Quadlet-Netzwerkeinheiten. Lifecycle-Inspektion behandelt das Netzwerk als eigene Laufzeitressource und validiert BaseHarbor-Eigentum vor zerstörerischen Operationen.

Sobald die Applikations-Workload-Anhänger implementiert sind, muss die Shutdown-Ordnung die Workloads von Anwendungen lösen/stoppen, bevor das BaseHarbor-Backend-Netzwerk entfernt wird. BaseHarbor darf nicht stillschweigend anwendungseigene Netzwerke entfernen.

## Folgen

positiv:

- bestehende Anwendungen behalten ihre eigene Compose-Topologie;
- Backend-Konnektivität hat einen stabilen Befestigungspunkt;
- Die App/Umwelt-Isolierung ist explizit;
- Der hostseitige Loopback-Zugang bleibt verfügbar;
- Es wird kein BaseHarbor SDK oder proprietäres Datenprotokoll eingeführt.

Kompromisse:

- der aktuelle Name des physischen Netzwerks enthält eine Benennungskonvention für Compose-Provider;
- Workload-Lebenszyklus muss die Netzanbindung vor dem Abriss des Backends koordinieren;
- künftige Anbieter wie Kubernetes können die gleiche logische Grenze zu einer anderen Implementierung abbilden.

Der logische Application Backend Network Vertrag ist stabil, auch wenn ein zukünftiger Anbieter die physische Netzwerkimplementierung verändert.

## MVP-Implikation

Diese Entscheidung ist Voraussetzung für die MVP Workload-Integration. Die MVP-Akzeptanztests für AWC, MailFlow und AI-Coding-System müssen beweisen, dass anwendungseigene Netzwerke weiterhin funktionieren, während ausgewählte Workload-Container ihre eigenen BaseHarbor-gemanagten Services erreichen und implizit nicht das Backend-Netzwerk einer anderen Anwendung erreichen können.

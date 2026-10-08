# ADR 0007: Auswahl der Laufzeitanbieter

## Status

Angenommen für die v0.4.17-Voreinfrieren-Laufzeitgrenze.

## Kontext

BaseHarbor muss einen tragbaren Anwendungsvertrag halten und gleichzeitig die gleiche gewünschte Arbeitsbelastung durch verschiedene Laufzeitmechanismen realisieren lassen.

Die Laufzeitauswahl ist daher ein Problem bei der Bereitstellung, nicht eine Anwendungsfähigkeit. Sie ist von den Leistungsanbietern wie PostgreSQL, Valkey, Objektspeicher, Geheimnisse oder Identität getrennt und von der durch ADR 0011 definierten Delivery Provider-Achse getrennt.

Repository-Workload-Quelle und Runtime-Provider-Identität sind auch separate Konzepte. Ein Repository kann die Compose-Spezifikation als Workload-Eingabe verwenden, ohne "Compose" zu einem Runtime-Provider zu machen.

## Entscheidung

BaseHarbor nutzt einen versionierten Provider-neutralen Laufzeitvertrag in `internal/runtime/contract`. First-Party-Laufzeit-Implementierungen leben hinter expliziten Paketgrenzen in ` internal/providers/runtime/docker`und ` internal/providers/runtime/podman`; ` internal/runtime/resolver`ist die einzige Auswahl-/Registrierungsgrenze für Erstanbieter.

Die Laufzeitgrenze besteht aus:

- eine normalisierte Anbieteridentität;
- eine versionierte `ProviderDescriptor`;
- explizite Anbieterfähigkeiten;
- deklarierte Kompatibilität zwischen Workload und Quelle;
- deklarierte Laufzeitrealisierung;
- ein Anbieterregister, das Deskriptoren an Fabriken kartiert;
- ein Anbieter-neutral `RuntimeProvider` Ausführungsvertrag.

Die aktuelle Vertragsversion lautet:

```text
baseharbor.runtime/v1
```

Provider-Auswahl kommt aus Target/Deployment-Status. Fehlende Provider-Metadaten für ein neues lokales Target löst `docker`.

Die beiden implementierten lokalen Anbieter sind:

```text
Portable application/workload semantics
                 |
        RuntimeProvider contract
          /                 \
         /                   \
DockerProvider           PodmanProvider
     |                        |
Docker Compose        Quadlet + systemd --user
```

Repository Compose bleibt ein Workload-Source-Standard:

```text
Repository workload source
        |
Compose Specification
        |
normalized semantics
        |
selected RuntimeProvider
```

Es ist keine Anbieteridentität.

## Anbieterregister

Provider-IDs sind normalisiert erweiterbare Identifikatoren anstatt ein geschlossenes Produkt Enum. First-Party Docker und Podman Anbieter sind in der Standard-Registrierung registriert, aber der Registrierungsvertrag kann einen zusätzlichen Anbieter Deskriptor und Fabrik ohne Änderung der tragbaren Anwendung Absicht akzeptieren.

Die Registrierung eines Anbieters scheitert, wenn:

- die Provider-ID ungültig oder nicht normalisiert ist;
- die Laufzeit-Vertragsversion ist unvereinbar;
- die Anbieterversion fehlt;
- Workload-Source-Kompatibilität ist nicht angemeldet;
- Laufzeiterfassung ist nicht angemeldet;
- das Anbieterwerk fehlt;
- eine Provider-ID zweimal registriert ist;
- der realisierte Anbieterdeskriptor passt nicht zu seiner Registrierung.

Kubernetes und OpenShift bleiben zukünftige Anbieterziele, sind aber nicht als ausführbare Anbieter in v0.4.17 registriert.

## Befähigungsregel

Ein Runtime-Anbieter werben Verhalten Orchestrierung kann abhängig von. Der ursprüngliche Vertrag umfasst:

- Workload-Lebenszyklus;
- Dienstleistung exec;
- Veröffentlichung der Hafeninspektion;
- Kontrolle des Ressourcenbesitzes im Besitz der Anbieter.

Capability Verhandlung ist versioniert und fail-closed. BaseHarbor reduziert nicht stillschweigend Lebenszyklus, Sicherheit oder Verifikation Garantien, wenn ein Anbieter fehlt erforderliches Verhalten.

Provider-Fähigkeiten sind Laufzeitmechanik, nicht Anwendungsanforderungen. Zum Beispiel,`database.sql` bleibt die gleiche tragbare Anwendungsfähigkeit unabhängig davon, ob ihre Implementierung anwendungsskopiert, geteilt, extern, Docker-backed, Podman-backed oder später Kubernetes-backed ist.

## Schwanzlutscher

`docker` ist der Standardanbieter für lokale Laufzeiten.

Seine derzeitige Workload-Source-Kompatibilität ist `compose-spec`, realisiert durch Docker Compose.

Docker-spezifische Ausführung bleibt innerhalb der Laufzeitimplementierung. Tragbare Orchestrierung darf auf Docker-Produktidentität nicht verzweigen.

## Podman

`podman ` wird nativ durch Quadlet und den Benutzer realisiert`systemd` Direktor.

Der Anbieter verbraucht Compose Specification Workload-Eingänge, macht die erforderlichen Quadlet-Einheiten und bedient sie durch `systemd --user` Und Podman.

Es gibt keine `podman compose` Fallback. Wenn Podman, Quadlet oder die erforderliche user-systemd-Umgebung nicht verfügbar ist, schlägt die Provider-Erkennung fehl.

## Kubernetes und OpenShift-Implikation

Kubernetes und OpenShift sind später Runtime Provider-Implementierungen, keine Änderungen an portablen Anwendungsabsichten.

Ein zukünftiger Anbieter kann den Lebenszyklus durch native Ressourcen wie Deployments, StatefulSets, Services, Jobs, PersistentVolumeClaims, NetworkPolicies, Gateway API-Ressourcen oder OpenShift-spezifische Ressourcen realisieren.

Kubernetes/OpenShift-Unterstützung muss einen kompatiblen Provider-Deskriptor/Fabrik registrieren und den gleichen tragbaren Laufzeitvertrag erfüllen. Es darf keine Docker/Podman-Zweigen in Core erfordern.

## Grenzüberschreitende Vollstreckung

Ein statischer Architekturtest scannt produktive Core-Pakete und lehnt ab:

- Vermächtnis `ProviderCompose` / ` DetectCompose`Identifikatoren;
- Beton `runtime.Compose`, ` DockerProvider ` or ` PodmanProvider`Verweise aus dem Kern;
- direkte Importe von Beton-Docker/Podman-Laufzeitpaketen;
- Erzeugnis `Engine()` Zugriff von portable Core.

Das Laufzeitpaket enthält auch einen wiederverwendbaren Konformitätsbaum für Erstanbieter.

## Vereinbarkeit

v0.4.17 behält absichtlich nicht das alte `compose` runtime-provider identity. Es gibt keine Produktion BaseHarbor Installationen, die diesen Migrationspfad erfordern.

Diese Entscheidung enthält folgende Bestimmungen:

- tragbare Anwendung manifestiert Semantik;
- Repository Workload-Eingang komponieren;
- anwendungsorientierte Umwelt- und Dienstleistungsbindungsverträge;
- Sicherung/Wiederherstellung der Identität;
- Docker Compose Realisierung;
- Podman Quadlet Realisierung.

## Folgen

positiv:

- Die Identität des Laufzeitanbieters wird nicht mehr mit den Compose-Eingängen zusammengeführt;
- Docker und Podman verwenden die gleiche portable Ausführungsgrenze;
- Podman hat keinen versteckten Compose-Fallback;
- die Entdeckung des Anbieters ist nicht ein Produktschalter, sondern ein Register/Deskriptor;
- spätere Anbieter können das Register ohne Änderung der portablen Absicht erweitern;
- architektonische Regression wird durch Tests bewacht.

Kompromisse:

- `RuntimeProvider` Nach wie vor ein erheblicher Lebenszyklusvertrag, da Docker und Podman bereits nachweisen, dass diese Operationen gemeinsam durchgeführt werden;
- Kubernetes und OpenShift müssen die Konformität nachweisen, bevor sie registriert werden können.
- Provider-spezifische Realisierung existiert weiterhin intern, wie beabsichtigt, darf aber nicht in Core wieder austreten.

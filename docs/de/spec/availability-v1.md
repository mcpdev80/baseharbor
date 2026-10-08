# Verfügbarkeit und hohe Verfügbarkeit v1

## Anwendungsbereich

BaseHarbor-Verfügbarkeit ist eine semantische Lösung der bestehenden Application-, Runtime-Provider- und Capability-Provider-Verträge.

Es ist kein zweiter Scheduler, Cluster Manager, Replication Protocol oder Provider Registry.

## Tragbare Absicht

Die normale Entscheidung ist ein Boolean:

```yaml
ha: true
```

or:

```yaml
ha: false
```

Wenn HA aktiviert ist, können spärliche Komponenten/Kapazitätsüberbrückungen nur die effektive HA-Anforderung und, wenn sinnvoll, die gewünschte Kardinalität ändern:

```yaml
ha: true

availability:
  sql:
    instances: 5
  identity:
    instances: 3
  logs:
    ha: false
```

Die Auflösungsanordnung lautet:

```text
component/capability override
    -> global ha
    -> explicit instances
    -> provider/runtime recommended topology
    -> generic 3 only when no stronger provider/runtime rule exists
```

Instanzenzahl ist topologische Eingabe, nie Anwendung, Komponente, Ressource, Bindung oder Konsumidentität.

## Verhandlungen

Die Verfügbarkeit von Laufzeiten und Fähigkeiten wird unabhängig voneinander ausgehandelt.

Ein ausgewählter Anbieter/Laufzeit erklärt einen von:

```text
SUPPORTED
PARTIALLY_SUPPORTED
UNSUPPORTED
```

Das Ergebnis zeichnet auch die konkreten Garantien auf, die BaseHarbor tatsächlich überprüft:

- Toleranz für das Versagen von Bauteilen/Prozessen;
- Fehlertoleranz des Hosts;
- rollende Wartungskontinuität;
- Kontinuität der kridentiellen Rotation;
- PKI-Rotationskontinuität;
- Kontinuität der Management-Oberflächen-Kontinuität;
- die realisierte Fehlerdomäne.

Eine erforderliche HA-Garantie, die nicht unterstützt wird MUSS vor der Mutation mit einem typisierten durchführbaren Ergebnis scheitern.

Für einen von BaseHarbor verwalteten/platzierten Anbieter `UNSUPPORTED` ist kein akzeptabler Vervollständigungszustand, nur weil der aktuelle Adapter immer noch Single-Instance ist. Wenn das vorgelagerte Produkt einen etablierten, produktionstauglichen HA/Replikation/Failover-Modus liefert, muss BaseHarbor diesen Modus implementieren und überprüfen für `ha: true`. ` UNSUPPORTED`ist einer echten Produkt-/Laufzeit-/Garantiegrenze vorbehalten.

## Beobachtung

Eine logische Komponente/Resource kann auf Null, eine oder viele Laufzeit-Instanzen auflösen.

Die Beobachtung behält jede beobachtete Instanz und Aggregate Bereitschaft gegen die geforderte Garantie. Sie MUSS nicht mehrere Instanzen in einen beliebigen Vertreter kollabieren.

Stabile anwendungsbezogene Identitäts-, Bindungs- und Verbrauchsreferenzen ändern sich nicht, wenn sich die Instanzen zählen, platzieren, umplanen oder ersetzen.

## Laufende Änderungen

Wenn ein Anbieter / Laufzeit Werbung HA-Unterstützung, Zertifikat, Geheimnis, Vertrauen, Konfiguration, Bild und Mitglied / Instanz Änderungen müssen überprüft gesunde Kapazität erhalten.

Die portable Regel lautet:

```text
prepare/reload replacement
    -> verify readiness and required semantics
    -> retire only safe old capacity
    -> stop safely on failure
```

Wo die gewählte Realisierung Kontinuität nicht erhalten kann, scheitert BaseHarbor vor disruptiver Mutation oder erfordert eine explizite Oberflächen-Störungsgenehmigung. Sie startet niemals stillschweigend alle gesunden Kapazitäten neu, während sie HA beansprucht.

## Aktuelle v0.4.21 Bezugsgrenze

Laufzeit HA und Leistungserbringer HA sind unabhängig.

Docker Compose und rootless Podman auf einem Host können nicht ehrlich behaupten Host-Failure Toleranz nur, weil mehrere Anbieter Mitglieder laufen. Sie können jedoch erfüllen Mitglied / Prozess Ausfall Toleranz, stabil-Endpunkt Kontinuität und Rolling Maintenance für Provider-native HA Topologien. Die gemeldete Garantie MUSS daher die realisierte Ausfall-Domäne identifizieren.

Gebündelte BaseHarbor-gemanagte Anbieter, die über einen etablierten vorgelagerten HA-Mechanismus verfügen, sind verpflichtet, diesen Mechanismus vor v0.4.21 umzusetzen und zu überprüfen. Externe/BYO-Anbieter können durch den Anbietervertrag stärkere Garantien unabhängig deklarieren.

Future Kubernetes, OpenShift und Managed/Cloud Laufzeiten können die Ausfall-Domain-Garantien hinter der gleichen portablen Absicht stärken, ohne die Anwendungsidentität oder das HA Vokabular zu ändern.

## Überprüfung der Portabilität

Der Vertrag wurde gegen diese Realisierung Formen überprüft:

- Ein-Host komponieren;
- Kubernetes Multiknoten;
- OpenShift-Unternehmenscluster;
- AWS Laufzeit mit verwaltetem PostgreSQL/Objektspeicher;
- Azure Laufzeit mit verwalteter PostgreSQL/Speicherung;
- GCP-Laufzeit mit verwalteter SQL/Speicherung.

Keines erfordert eine portable Absicht, Pod/Container-Namen, Namespaces, Node/Zonen-Namen, StatefulSet/PDB-Konzepte, Cloud-Produktmodi, Replik-Member-Rollen oder Provider-Cluster-Vokabular zu enthalten.

Laufzeit HA und Fähigkeitsanbieter HA bleiben unabhängig, so dass eine starke verwaltete Datenbank mit einer schwächeren Workload-Laufzeit koexistieren kann und umgekehrt.

Künftiges Autoskalieren kann feste Kardinalität durch dynamischen gewünschten Zustand ersetzen, ohne die logische Anwendung/Komponente/Ressourcen/Verbrauchsidentität zu ändern.

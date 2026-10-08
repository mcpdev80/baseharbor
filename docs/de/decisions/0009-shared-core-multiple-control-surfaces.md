# ADR 0009: Gemeinsamer Kern mit mehreren Kontrollflächen

- Status: angenommen
- Geburtsdatum: 2026-09-18

## Kontext

BaseHarbor stellt derzeit seinen Anwendungs-Lebenszyklus in erster Linie durch die `baha` CLI und realisiert Workloads durch Compose.

Die Produkt-Roadmap umfasst auch:

- eine leichte Web-UI für normale Anwendungen/Plattformen;
- eine stabile HTTP-API für Automatisierung und Fernbetrieb;
- Kubernetes und OpenShift Laufzeitanbieter;
- ein Kubernetes/OpenShift Operator, der den gewünschten Zustand der BaseHarbor-Anwendung kontinuierlich miteinander in Einklang bringt.

Diese Oberflächen dürfen nicht zu unabhängigen Umsetzungen desselben Lebenszyklus werden.

Wenn CLI-Befehle, HTTP-Handler und Kubernetes jeweils ihre eigenen Plan/Apply/Status/doctor/Backup-Regeln umsetzen, würde BaseHarbor divergierendes Verhalten, Sicherheitsrichtlinien und Bereitschaftssemantik ansammeln. Es würde auch den portablen Anwendungsauftrag in lokalen Compose- und Cluster-Umgebungen erheblich erschweren.

## Entscheidung

BaseHarbor verwendet **einen gemeinsamen Domänen-/Lebenszykluskern mit mehreren Kontrollflächen**.

Konzeptionell:

```text
                         BaseHarbor Core
              +--------------------------------+
              | application/domain models      |
              | PortableContract                |
              | input resolution                |
              | lifecycle / convergence         |
              | plan / preflight / apply        |
              | verify / status / diagnostics   |
              | recovery / update semantics     |
              | provider selection/capabilities |
              +---------------+----------------+
                              |
          +-------------------+-------------------+
          |                   |                   |
          v                   v                   v
       baha CLI            HTTP API        Operator controllers
                              |
                              v
                          lightweight
                            Web UI
```

Die Bedienoberflächen sind Adapter über die gleichen Applikationsdienste und Domänenmodelle.

### KLI

`baha` bleibt die primäre lokale Entwickler/Operator-Schnittstelle.

Der lokale Betrieb kann den gemeinsamen Kern im Prozess aufrufen. Ein zukünftiger Remote/Kontext-Modus kann die BaseHarbor HTTP API verwenden, aber der CLI darf nicht der Ort der einzigartigen Lebenszyklus-Geschäftslogik werden.

### HTTP API

Die API ist eine Transport- und Autorisierungsgrenze, keine zweite Lebenszyklusimplementierung.

Es stellt maschinenlesbare Pläne, Inputs, Status, Diagnosen und Lebenszyklus-Aktionen auf, die durch denselben gemeinsamen Kern unterstützt werden, der von `baha`.

Die API darf keine Lebenszyklus-Semantik implementieren, indem sie auf `baha`.

### Web UI

Die Web UI ist absichtlich leicht und verwendet die BaseHarbor API.

Sie kann Maßnahmen durchführen, wie z. B.:

- Anwendungsübersicht und -bereitschaft;
- plan/preflight/apply/up/down;
- Arzt und Sanierung;
- Protokolle;
- Angabe der Inputs für die Deklaration des Einsatzes;
- geheime Metadaten zur Anwesenheit/Verwendbarkeit;
- Sicherung/Wiederherstellung;
- Aktualisierung;
- Anbieter/Plattform Gesundheit.

Die Benutzeroberfläche darf keine normalen Richtlinien, Validierungs-, Vorflug-, Verifikations- oder Geheimabfertigungsregeln umgehen.

geheime Werte werden nicht standardmäßig angezeigt. Jede zukünftige enthüllen/erhöhte Operation erfordert explizite Richtlinien und Autorisierung.

Die Web UI darf keine Shell-Befehle aufrufen oder Lifecycle-Entscheidungen in Frontend-Code umimplementieren.

### Kubernetes/OpenShift Operator

Ein zukünftiger BaseHarbor Operator ist eine weitere Kontrolloberfläche über das gleiche Domain/Provider-Modell.

Der Betreiber versöhnt den gewünschten Zustand kontinuierlich:

```text
BaseHarbor Application CR
          |
          v
      Reconcile
          |
          v
shared contract/lifecycle logic
          |
          v
Kubernetes/OpenShift runtime provider
          |
          v
observe + verify
          |
          v
CR status / conditions
```

Der Betreiber muss idempotente Aussöhnung und Anbieter-native Beobachtung verwenden, anstatt Imperativ zu umhüllen.`baha` Befehlen.

Kubernetes/OpenShift CRDs sind clusterseitige Darstellungen des gewünschten BaseHarbor-Zustandes. Sie müssen die gleiche logische Anwendungsabsicht bewahren und dürfen Anwendungen nicht zwingen, sich auf Kubernetes/OpenShift-Implementationsdetails zu verlassen.

### Gemeinsame Ergebnismodelle

Lebenszyklus- und Diagnoseergebnisse sollten maschinenlesbar sein, bevor sie von einer Oberfläche gerendert werden.

Beispiele hierfür sind gemeinsame Konzepte wie:

- Anwendungszustand/Bereitstellung;
- Maßnahmen planen;
- Ausfälle vor dem Flug,
- Ausfälle der Anbieterfähigkeit;
- Diagnosecode/Schwerpunkt/Message/Sanierung;
- Ressourcenstatus;
- Bedingungen für den Lebenszyklus;
- Eingabedefinitionen und ungelöste Werte.

Die CLI macht diese für Terminals, die Web UI macht sie visuell, die API serialisiert sie, und der Operator Karten sie Kubernetes Status / Bedingungen, wo angemessen.

Keine Oberfläche darf ein optimistischeres Bereitschaftsmodell erfinden als das gemeinsame Prüfergebnis.

## Architektonische Regel

BaseHarbor Core darf nicht von CLI, HTTP, Browser oder Kubernetes/OpenShift-Präsentationen abhängen.

Insbesondere:

- domain/application-Pakete dürfen keine Cobra-Befehlspakete importieren;
- domain/application-Pakete dürfen nicht vom Frontend-Code abhängen;
- geteiltes Lebenszyklusverhalten darf Kubernetes-Typen nicht erfordern, es sei denn, das Verhalten befindet sich speziell in einem Kubernetes/OpenShift-Provider-Adapter;
- CLI-, API- und Operator-Adapter können vom Kern abhängen, nie umgekehrt;
- anbieterspezifische Objekte bleiben hinter den Anbietergrenzen zurück;
- Sicherheits- und Politikentscheidungen werden unter der Präsentationsebene geteilt oder durchgesetzt, anstatt pro Benutzeroberfläche zu duplizieren.

## Gegenwärtiger und künftiger Anwendungsbereich

### Implementiert in v0.4

- `baha` als primärer CLI;
- Wiederverwendbarer Anbieter-neutral `PortableContract`;
- Auswahl/Verhandlung von Laufzeitanbietern;
- deklarative Eingabeauflösung getrennt von CLI-Aufforderung;
- gemeinsame Laufzeit-Wahrheit, die von mehreren CLI-Befehlen verwendet wird.

### Zukunft

- stabile BaseHarbor HTTP API;
- leichtes Web UI;
- Kontexte/Entfernungsziele für `baha`;
- Kubernetes Operator und Laufzeitanbieter;
- OpenShift Spezialisierung;
- gemeinsam verwaltete Identität/RBAC/JIT-Politik für API-, UI-, CLI- und Operator-orientierte Aktionen.

Dieser ADR definiert nur Architekturrichtung. Er behauptet nicht, dass die Web UI, API oder Operator bereits in v0.4 existiert.

## Folgen

### Positiv

- CLI, GUI und Operator können das gleiche Verhalten ohne drei Implementierungen aufdecken.
- Tests auf Kernlebenszyklusverhalten gelten für jede zukünftige Oberfläche.
- Bewerbungsverträge bleiben unabhängig von Präsentation und Laufzeit.
- Eine leichte GUI bleibt möglich, da sie in erster Linie ein Renderer/Client strukturierter API-Operationen ist.
- Kubernetes/OpenShift-Unterstützung kann Domänen- und Providerlogik wiederverwenden, anstatt BaseHarbor um CRDs zu rekonstruieren.

### Handelshemmnisse

- Weiteres Verhalten muss aus den Kommandohandlern in wiederverwendbare Anwendungsdienste übergehen, während BaseHarbor sich entwickelt.
- Ergebnistypen benötigen stabile maschinenlesbare Strukturen statt terminal-only Strings.
- API-Autorisierung und Operator-Abgleich führen zusätzliche Grenzen ein, die explizite Tests erfordern.
- Einige lokale CLI Conveniences können oberflächenspezifisch bleiben, aber sie dürfen nicht erforderlich werden Anwendungslebenszyklus semantik.

# ADR 0011: Lieferer, delegierte Aussöhnung und Werkzeugneutralität

## Status

Akzeptiert für die post-v0.4 Architektur und erforderlich vor dem v0.5 Contract Freeze.

## Kontext

BaseHarbor trennt bereits portable Anwendungsabsichten von Runtime-Providern und Capability-Providern. Der gesamte aktuelle lokale Runtime-Pfad verwendet Docker Compose für Docker und native Quadlets für Podman unter Wahrung der Compose-basierten Repository-Kompatibilität; Kubernetes und OpenShift sind spätere Runtime-Implementierungen. Capability-Provider realisieren logische Anforderungen wie SQL, S3, Geheimnisse, Beobachtung und Belichtung.

Eine zweite Frage ist unabhängig von beiden Achsen: **Wer liefert und versöhnt die gewünschte Laufzeitrealisierung?**

Für die aktuelle lokale Laufzeit führt BaseHarbor die Laufzeitmutation direkt durch: Docker durch Docker Compose und Podman durch generierte Quadlet-Einheiten. Bei Kubernetes/OpenShift ist die direkte API-Mutation gültig, aber etablierte GitOps-Systeme wie Argo CD oder Flux können eine kontinuierliche Abstimmung besitzen und wertvolle native Betriebsansichten liefern.

Die Herstellung von Argo-CD, Flux, Helm, Git oder Kubernetes-Ressourcen als Teil der portablen Applikationsabsicht würde ein Produkt/Laufzeit-Lock-in schaffen. BaseHarbor-Implementierung jeder ausgereiften Betriebssteuerung selbst würde etabliertes OSS duplizieren und den BaseHarbor Core für produktspezifische Integrations-Curn verantwortlich machen.

Gleichzeitig soll BaseHarbor zu einem offenen Ökosystem werden. Community, Anbieter und Unternehmen müssen Integrationen ohne Patching BaseHarbor Core umsetzen können. Die bestehende Provider Integration Contract-Richtung basierend auf versionierter Semantik, gRPC/Protokoll Buffers, wo eine Prozessgrenze erforderlich ist, OCI-Distribution, Konformität und offenen Supply-Chain-Standards bleibt strategisch wichtig.

## Entscheidung

### 1. Laufzeit, Kapazität und Lieferung sind unabhängige Achsen

```text
                         BaseHarbor Core
                               |
             +-----------------+-----------------+
             |                 |                 |
             v                 v                 v
       Runtime Provider  Capability Provider  Delivery Provider
             |                 |                 |
 Docker Compose / Podman   SQL / S3         direct
            Quadlet         secrets          delegated/GitOps
        Kubernetes          telemetry
         OpenShift
```

Harte Regel:

```text
runtime != capability != delivery
```

- Laufzeitanbieter: Wo/wie Workload Runtime Primitive realisiert werden.
- Capability Provider: Wie eine logische Anwendungsfähigkeit realisiert wird.
- Lieferer: Wie die gewünschte Laufzeitrealisierung zur gewählten Laufzeit angewendet/versöhnt wird.

Keine dieser Auswahlen ist portable Anwendung Absicht.

### 2. Direkte und delegierte Lieferung sind erstklassige Semantik

Direktlieferung:

```text
BaseHarbor -> Runtime API
```

BaseHarbor besitzt Mutation/Versöhnung für den verwalteten Ressourcensatz.

Delegierte Lieferung:

```text
BaseHarbor
    -> desired runtime realization
    -> Delivery Provider
    -> external reconciler
    -> Runtime
```

BaseHarbor besitzt immer noch tragbare Semantik, Politik, Provider-Auswahl, Lebenszyklus-Intention, Beobachtung, semantische Verifizierung und Evidenz. Der delegierte Versöhner besitzt Laufzeitmutation für den delegierten Ressourcensatz.

### 3. Genau ein Aussöhnungsbesitzer

Für einen verwalteten Ressourcensatz gibt es genau einen aktiven Mutation/Versöhnungsbesitzer.

BaseHarbor darf keine direkt mutierten Felder/Ressourcen, die ein delegierter Aussöhner während des normalen Betriebs besitzt, mutieren.

### 4. Platzierung Semantik sind konsistent über Anbieter-Familien

Gegebenenfalls verwenden die Lieferer die gleiche kanonischen Platzierungssemantik wie die Leistungserbringer:

```text
application
shared
external
```

Die Semantik bleibt konsequent:

- Anwendung: eine App/Umgebung, BaseHarbor-eigener Lebenszyklus, sofern unterstützt;
- geteilt: explizit geteilter Anbieter mit einer expliziten Shared-Grenze;
- extern: Der Lebenszyklus des Anbieters befindet sich außerhalb von BaseHarbor.

Konsequente Platzierungssemantik verschmilzt nicht die Verantwortung des Anbieters.

### 5. Verträge, nicht Werkzeuge

BaseHarbor besitzt stabile Semantik und Verträge. Produkte und Werkzeuge sind Implementierungen.

Argo CD kann eine Referenzimplementierung eines GitOps Delivery Providers sein. Flux oder eine andere konforme Implementierung muss ohne portable Vertragsänderungen möglich bleiben.

Ebenso können Provider-Implementierungen intern ausgereifte OSS-, offene Standards, Standard-APIs/SDKs, Controller/Operatoren/CRDs oder Managed-Service-APIs verwenden. Diese Mechanismen bleiben hinter der Provider-Grenze.

### 6. Offene Anbieter-Ökosystem bleibt strategisch

BaseHarbor darf nicht zum Integrationsengpass werden.

Dritte müssen in der Lage sein, Anbieter zu implementieren, ohne BaseHarbor Core zu ändern.

Die strategische offene Ökosystem-Richtung bleibt:

```text
BaseHarbor versioned contract/specification
        ↓
Provider Integration Contract
        ↓
gRPC / Protocol Buffers where a process boundary is required
        ↓
OCI distribution
        ↓
independent community/vendor/company provider
```

OCI-Register, Digest Pinning, Signaturen/Beweis und Konformität werden einem proprietären BaseHarbor-Marktplatz-/Paketformat vorgezogen.

Ein Anbieter kann intern auf ein anderes OSS-Tool oder Plattform-API delegieren. Das ist eine Implementierungsoption, kein Grund, die BaseHarbor-Providergrenze zu umgehen.

### 7. BaseHarbor-erste Entwickler-Erfahrung

Die normalen Entwickler- und Agentenschnittstellen bleiben:

```text
baha
JSON
MCP
```

Benutzer sollten nicht `argocd`, Fluxspezifische CLIs,` kubectl`, Helm oder Provider-native Infrastruktur CLIs für den normalen BaseHarbor-Lebenszyklus.

Native Tools UIs und CLIs bleiben für Plattform-Ingenieure und Experten Drill-Down verfügbar. BaseHarbor verbirgt die betriebliche Komplexität, nicht die operative Fähigkeit.

### 8. Kompose Kompatibilität und Podman Quadlet bleiben erstklassige

Kompose-basierte Repository- / Runtime-Definitionen dürfen nicht von Kubernetes, GitOps, Argo-CD, Flux, CRDs, Cloud-APIs oder anderen späteren Runtime-Mechanismen abhängen.

Die gesamten aktuellen lokalen Pfade bleiben konzeptuell:

```text
runtime: docker  -> Docker Compose
runtime: podman  -> generated Quadlet + systemd --user
delivery: direct
```

Die Abstraktion des Lieferanbieters muss laufzeitneutral genug sein, um einen zukünftigen Nicht-Kubernetes-Deleged-Delivery-Use-Fall nicht zu untersagen, aber BaseHarbor implementiert keine spekulativen Wege ohne nachgewiesene Notwendigkeit.

## Folgen

positiv:

- BaseHarbor kann sich mit GitOps integrieren, ohne Argo CD/Flux Teil der Anwendungsabsicht zu machen.
- Argo CD UI und native Plattform-Tooling bleiben nützlich.
- Compose bleibt unabhängig und erstklassig.
- Hersteller/Gemeinschaft können Integrationen ohne Core Patches versenden.
- reifes OSS kann hinter Anbietern wiederverwendet werden, anstatt neu implementiert zu werden.
- Produkt/Version Churn bleibt außerhalb BaseHarbor Core.
- die gleichen Grundsätze der Platzierung, des Eigentums, des Lebenszyklus, der Überprüfung und der Nachweise für die Anbieterfamilien.

Kompromisse:

- BaseHarbor muss Lieferung Semantik sorgfältig genug definieren, um falsche Abstraktion zu vermeiden.
- Die delegierte Lieferung fügt Synchronisation/Revision/Eigentümer hinzu, dass eine direkte Lieferung nicht erforderlich ist.
- BaseHarbor muss tool-reported sync/health von BaseHarbor semantische Überprüfung unterscheiden.
- Provider-Konformität wird wichtiger, weil Integrationen außerhalb des Core leben können.

## Auswirkungen auf die Akzeptanz

Bevor v0.5 den Kernvertrag einfriert:

- Bereitstellungs-/Operatorstaat kann die Auswahl des Lieferers und die Aussöhnung des Eigentümers darstellen;
- direkte oder delegierte Lieferung ohne Produktnamen in portabler Absicht aussprechbar ist;
- Typisierte Ergebnismodelle können Bereitstellungssynchronisation/Konflikt/nicht verfügbare Zustände darstellen;
- Anwendung/gemeinsame/externe Platzierungssemantik bleibt wiederverwendbar;
- Die direkte Lieferung bleibt unverändert.

Die Real-Delegated/GitOps-Referenzimplementierung ist für v0.7.8 (#325) vorgesehen, wobei Argo-CD eher als Referenznachweis statt als Vertragsabhängigkeit dient.

# ADR 0011: Delivery Provider, delegierte Reconciliation und Tool-Neutralität

## Status

Akzeptiert für die Architektur nach v0.4 und vor dem v0.5-Contract-Freeze erforderlich.

## Kontext

BaseHarbor trennt bereits portables Application Intent von Runtime Providern und Capability Providern. Compose ist der aktuell vollständige Runtime Provider; Kubernetes und OpenShift folgen später. Capability Provider realisieren logische Anforderungen wie SQL, S3, Secrets, Observability und Exposure.

Davon unabhängig ist die Frage: **Wer liefert und reconciled die gewünschte Runtime-Realisierung?**

Bei Compose mutiert BaseHarbor die Runtime heute direkt. Bei Kubernetes/OpenShift ist direkte API-Mutation gültig, aber etablierte GitOps-Systeme wie Argo CD oder Flux können die kontinuierliche Reconciliation übernehmen und wertvolle native Betriebsansichten bereitstellen.

Argo CD, Flux, Helm, Git oder Kubernetes-Ressourcen dürfen nicht Teil des portablen Application Intent werden. Gleichzeitig soll BaseHarbor nicht jeden etablierten Infrastruktur-Controller selbst neu implementieren.

BaseHarbor soll außerdem ein offenes Ökosystem ermöglichen. Community, Hersteller und Unternehmen müssen Integrationen ohne Änderungen am BaseHarbor Core implementieren können. Die bestehende Richtung des Provider Integration Contract mit versionierter Semantik, gRPC/Protocol Buffers bei Prozessgrenzen, OCI-Verteilung, Conformance und offenen Supply-Chain-Standards bleibt strategisch wichtig.

## Entscheidung

### 1. Runtime, Capability und Delivery sind unabhängige Achsen

```text
                         BaseHarbor Core
                               |
             +-----------------+-----------------+
             |                 |                 |
             v                 v                 v
       Runtime Provider  Capability Provider  Delivery Provider
             |                 |                 |
          Compose           SQL / S3         direct
        Kubernetes          Secrets          delegated/GitOps
         OpenShift          Telemetry
```

Harte Regel:

```text
runtime != capability != delivery
```

Keine dieser Auswahlen ist portables Application Intent.

### 2. Direct und delegated delivery sind First-Class-Semantiken

Direct:

```text
BaseHarbor -> Runtime API
```

BaseHarbor besitzt Mutation/Reconciliation des verwalteten Ressourcensatzes.

Delegated:

```text
BaseHarbor
    -> gewünschte Runtime-Realisierung
    -> Delivery Provider
    -> externer Reconciler
    -> Runtime
```

BaseHarbor behält portable Semantik, Policy, Provider-Auswahl, Lifecycle-Intent, Observation, semantische Verifikation und Evidence. Der delegierte Reconciler besitzt die Runtime-Mutation des delegierten Ressourcensatzes.

### 3. Genau ein Reconciliation Owner

Für einen verwalteten Ressourcensatz existiert genau ein aktiver Mutation-/Reconciliation-Owner.

BaseHarbor darf im Normalbetrieb keine Felder/Ressourcen direkt mutieren, die ein delegierter Reconciler besitzt. Mehrdeutige oder konkurrierende Ownership schlägt fail-closed fehl.

### 4. Placement bleibt über Provider-Familien konsistent

Wo anwendbar nutzen Delivery Provider dieselben kanonischen Placement-Semantiken:

```text
application
shared
external
```

Konsistente Placement-Semantik bedeutet nicht identische Provider-Verantwortung.

### 5. Contracts, not tools

BaseHarbor besitzt stabile Semantik und Contracts. Produkte und Tools sind Implementierungen.

Argo CD kann Referenzimplementierung eines GitOps Delivery Providers sein. Flux oder andere konforme Implementierungen müssen ohne Änderung des portablen Contracts möglich bleiben.

Provider dürfen intern reifes OSS, offene Standards, Standard-APIs/SDKs, Controller/Operatoren/CRDs oder Managed-Service-APIs verwenden. Diese Mechanismen bleiben hinter der Provider-Grenze.

### 6. Offenes Provider-Ökosystem bleibt strategisch

BaseHarbor darf nicht zum Integrations-Flaschenhals werden.

Dritte müssen Provider ohne Änderung des BaseHarbor Core implementieren können.

Die strategische Richtung bleibt:

```text
versionierter BaseHarbor Contract
        ↓
Provider Integration Contract
        ↓
gRPC / Protocol Buffers bei Prozessgrenzen
        ↓
OCI Distribution
        ↓
unabhängiger Community-/Hersteller-/Unternehmens-Provider
```

OCI-Registries, Digest-Pinning, Signaturen/Provenance und Conformance werden einem proprietären BaseHarbor-Paket-/Marketplace-Format vorgezogen.

### 7. BaseHarbor-first Developer Experience

Die normalen Entwickler-/Agenten-Schnittstellen bleiben:

```text
baha
JSON
MCP
```

Für normale BaseHarbor-Lifecycle-Operationen sollen Nutzer keine `argocd`-, Flux-, `kubectl`-, Helm- oder provider-nativen Infrastruktur-CLIs benötigen.

Native Tool-UIs/CLIs bleiben für Platform Engineers und Experten verfügbar. BaseHarbor versteckt operative Komplexität, nicht operative Fähigkeiten.

### 8. Compose bleibt First-Class

Compose darf nicht von Kubernetes, GitOps, Argo CD, Flux, CRDs, Cloud-APIs oder späteren Runtime-Mechanismen abhängen.

Der vollständige aktuelle Pfad bleibt konzeptionell:

```text
runtime: compose
delivery: direct
```

## Konsequenzen

BaseHarbor kann GitOps unterstützen, ohne Argo CD/Flux in den Application Contract zu ziehen. Compose bleibt unabhängig. Community und Hersteller können Integrationen ohne Core-Patches liefern. Reifes OSS kann hinter Providern wiederverwendet werden. Gleichzeitig muss BaseHarbor Tool-Sync/Health weiterhin von echter semantischer BaseHarbor-Verifikation unterscheiden.

## Acceptance-Auswirkungen

Vor dem v0.5-Freeze muss Deployment-/Operator-State Delivery-Provider-Auswahl und Reconciliation-Ownership ausdrücken können. Direct vs. delegated bleibt produktneutral. Typisierte Resultate müssen Delivery-Sync/Conflict/Unavailable darstellen können. Die reale GitOps-Referenzimplementierung folgt in v0.7.8 (#325), mit Argo CD als Referenzbeweis und nicht als Contract-Abhängigkeit.

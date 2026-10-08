# Repository-Workload-Quellen

BaseHarbor interpretiert Repository-Syntax über einen Workload Source Adapter, normalisiert Evidenz und erzeugt daraus portablen Application Intent. Source, Runtime, Delivery und Development Adapter sind verschiedene Aufgaben.

| Source | Inspect/Adoption | Aktuelle Einschränkung |
| --- | --- | --- |
| Compose | Unterstützt | Bestehender Docker-/Podman-Workload-Pfad |
| Repository-Quadlet | Unterstützt | Adoption bedeutet noch keine Ausführung dieser Repository-Units |
| Raw Kubernetes YAML | Unterstützt | Kubernetes-Runtime folgt später |
| Helm/Kustomize | Später | Noch kein Source Adapter |

Erkennung garantiert keine Ausführung durch die gewählte Runtime. Plan/Preflight liefert bei fehlender Unterstützung einen typisierten Fehler.

## Auswahl

CLI, JSON und MCP verwenden `baseharbor.workload-source-resolution/v1`. Zustände sind `selected`, `ambiguous`, `not_detected`, `invalid` und `unsupported`. Ein fehlender ausgewählter Source allein ist kein Mehrdeutigkeitsnachweis.

Ein eindeutiger Source benötigt keine Metadatei. Bei mehreren tragfähigen Kandidaten fragt geführte Adoption einmal und speichert die Auswahl in `baseharbor.repository.yaml`:

```text
Repository
  -> Workload Source Adapter
  -> Normalized Workload Evidence
  -> repository inspection / adoption
  -> portable Application Intent
  -> Runtime Provider
```

Das ist commitfähige Repository-Metadaten, kein portabler Intent und kein geschützter Runtime-Zustand.

## Grenzen

Scans begrenzen Dateigröße, Gesamtbytes, Anzahl und Tiefe. Symlinks werden übersprungen; Secret-Werte gehören nicht in normale Evidenz. Compose unterstützt Merge-Keys, dateiinternes `extends` und deterministische Variablen-Defaults. `include` und externes `extends.file` scheitern ausdrücklich. Externe Variablen werden nicht aus der Operator-Shell gelesen.

Quadlet erkennt `.container`, `.pod`, `.network`, `.volume` und `.kube`; fehlende lokale Inputs scheitern sicher. Kubernetes-YAML wird statisch gelesen, ohne Cluster-Lookup oder aus Namen abgeleiteten Namespace-Besitz. Unbekannte Ressourcen bleiben opake Evidenz.

## Nachweise

Adapter-Tests, kontrollierte Konformitätsfälle, Cross-Source-Parität und 30 gepinnte reale Repositorys sind getrennte Prüfungen. Im Core-Checkout:

```text
Workload Source != Runtime Provider
Workload Source != Delivery Provider
Workload Source != Development Adapter
Workload Source != Portable Application Intent
```

Vollständige Modelle: [kanonische Erklärung (EN)](https://mcpdev80.github.io/baseharbor/explanation/workload-sources/).


## Weitere unveränderte technische Beispiele

```json
{
  "schema_version": "baseharbor.workload-source-resolution/v1",
  "state": "selected",
  "reason": "production_candidate_dominates",
  "candidate_count": 3,
  "selected": {
    "kind": "compose",
    "path": "docker/docker-compose.yml"
  }
}
```

```text
baseharbor.repository.yaml
```

```yaml
version: 1
workload-source:
  kind: kubernetes
  path: deploy/k8s
```

```text
source-adapter-unit
  -> focused positive and negative adapter tests

source-conformance
  -> controlled fixtures with known semantic ground truth
  -> broken/adversarial inputs
  -> fail-closed security bounds

source-cross-parity
  -> one reference application represented as Compose, Quadlet and Kubernetes YAML
  -> equivalent normalized semantics where representable

source-realworld
  -> 10 Compose + 10 Quadlet + 10 Kubernetes public repositories
  -> pinned revisions + documented quality/pattern tags
```

```text
testdata/adoption-conformance/
testdata/adoption-realworld/
```

```text
scripts/adoption-conformance.sh unit
scripts/adoption-conformance.sh conformance
scripts/adoption-conformance.sh parity
scripts/adoption-conformance.sh realworld
```


Technische Kennungen: `single_candidate`, `explicit_repository_selection`, `multiple_viable_candidates`, `cross_family_ambiguity`, `only_low_confidence_candidates`, `no_supported_source`, `invalid_repository_metadata`, `unsupported_repository_metadata`, `${VAR:-default}`, `${VAR-default}`, `${VAR}`, `scripts/realworld-workload-source-corpus.sh`.

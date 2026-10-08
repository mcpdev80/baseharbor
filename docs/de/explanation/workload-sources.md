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

```yaml
version: 1
workload-source:
  kind: kubernetes
  path: deploy/k8s
```

Das ist commitfähige Repository-Metadaten, kein portabler Intent und kein geschützter Runtime-Zustand.

## Grenzen

Scans begrenzen Dateigröße, Gesamtbytes, Anzahl und Tiefe. Symlinks werden übersprungen; Secret-Werte gehören nicht in normale Evidenz. Compose unterstützt Merge-Keys, dateiinternes `extends` und deterministische Variablen-Defaults. `include` und externes `extends.file` scheitern ausdrücklich. Externe Variablen werden nicht aus der Operator-Shell gelesen.

Quadlet erkennt `.container`, `.pod`, `.network`, `.volume` und `.kube`; fehlende lokale Inputs scheitern sicher. Kubernetes-YAML wird statisch gelesen, ohne Cluster-Lookup oder aus Namen abgeleiteten Namespace-Besitz. Unbekannte Ressourcen bleiben opake Evidenz.

## Nachweise

Adapter-Tests, kontrollierte Konformitätsfälle, Cross-Source-Parität und 30 gepinnte reale Repositorys sind getrennte Prüfungen. Im Core-Checkout:

```bash
scripts/adoption-conformance.sh unit
scripts/adoption-conformance.sh conformance
scripts/adoption-conformance.sh parity
scripts/adoption-conformance.sh realworld
```

Vollständige Modelle: [kanonische Erklärung (EN)](https://mcpdev80.github.io/baseharbor/explanation/workload-sources/).

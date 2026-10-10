# Application-Befehle

Application-Befehle arbeiten mit dem portablen Contract und seinem Deployment-Zustand.

| Befehl | Aufgabe |
| --- | --- |
| `baha new application` | Neue ecosystem-native Application erzeugen |
| `baha init` | Bestehendes Repository übernehmen |
| `baha inspect` | Repository-/Application-Evidenz lesen |
| `baha app show` | Aufgelösten Vertrag anzeigen |
| `baha list` | Bekannte Applications auflisten |
| `baha plan` | Plan anzeigen |
| `baha app preflight` | Vor Mutation validieren |
| `baha up` / `baha app apply` | Application konvergieren/abgleichen |
| `baha down` | Application-Runtime stoppen |
| `baha status` | Zustand beobachten |
| `baha doctor` | Diagnostizieren, optional reparieren |
| `baha app update` | Unterstützte Application-Source aktualisieren |
| `baha destroy` | Application-eigene Ressourcen entfernen |

## Erzeugen oder übernehmen

In einem übergeordneten Verzeichnis ohne vorhandenen Ordner `shop-api`:

```text
baha init my-app \
  --workload-source kubernetes:deploy/k8s \
  --workload-component api \
  --workload-component worker
```

Das Scaffold deklariert SQL/Cache, ergänzt Go-Clients und Runtime-Bindungsnamen. Fachliche Endpunkte und Tabellen ergänzt du selbst. Nach `baha up` zeigen `baha app logs app` die Logs und `baha app env --format json` maskierte Bindungen.

Bei vorhandenem Repository beginnst du dort mit `baha inspect .`, danach `baha init`; kein zweites Scaffold über vorhandene Dateien schreiben.

## Source-neutrale Adoption

Compose, Repository-Quadlet und Raw Kubernetes YAML werden über denselben Source-Vertrag erkannt. Das garantiert nicht ihre Runtime-Ausführung. Mehrdeutige Quellen werden nicht stillschweigend ausgewählt. `--quick` scheitert sicher, wenn keine deterministische Wahl möglich ist.

Explizite Source-/Komponentenwahl, wenn im Repository tatsächlich vorhanden:

```bash
baha new application shop-api --stack go --http --sql --cache
cd shop-api
baha inspect .
baha app show
baha plan
```

Helm und Kustomize sind noch keine Source Adapter. Eine benötigte explizite Auswahl wird in `baseharbor.repository.yaml` gespeichert.

Weitere Operationen umfassen [Backup/Restore](../how-to/backup-restore.md), Secrets, Logs, Shell/Exec, Umgebung, TLS, Evidence, Runtime-Identität und Datenzugriff. CLI und Maschinenpfade verwenden dasselbe Domänenergebnis.

Exakte Referenz: [Application-Befehle (EN)](https://mcpdev80.github.io/baseharbor/cli/applications/).


Technische Bezeichner: `baseharbor.yaml`, `baseharbor.workload-source-resolution/v1`, `baha status`, `baha plan`, `baha doctor`, `baha app ...`, `DATABASE_URL`, `DATABASE_CA_FILE`, `REDIS_URL`, `REDIS_CA_FILE`.

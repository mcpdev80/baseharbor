# Application-Befehle

Application-Befehle arbeiten mit dem portablen Contract und seinem Deployment-Zustand.

| Befehl | Aufgabe |
| --- | --- |
| `baha app new` | Neue ecosystem-native Application erzeugen |
| `baha app init` | Bestehendes Repository übernehmen |
| `baha app inspect` | Repository-/Application-Evidenz lesen |
| `baha app show` | Aufgelösten Vertrag anzeigen |
| `baha app list` | Bekannte Applications auflisten |
| `baha app plan` | Plan anzeigen |
| `baha app preflight` | Vor Mutation validieren |
| `baha app up` / `baha app apply` | Application konvergieren/abgleichen |
| `baha app down` | Application-Runtime stoppen |
| `baha app status` | Zustand beobachten |
| `baha app doctor` | Diagnostizieren, optional reparieren |
| `baha app update` | Unterstützten Update-Pfad verwenden |
| `baha app destroy` | Application-eigene Ressourcen entfernen |

## Erzeugen oder übernehmen

In einem übergeordneten Verzeichnis ohne vorhandenen Ordner `shop-api`:

```bash
baha app new shop-api --stack go --http --sql --cache
cd shop-api
baha app inspect .
baha app show
baha app plan
```

Das Scaffold deklariert SQL/Cache, ergänzt Go-Clients und Runtime-Bindungsnamen. Fachliche Endpunkte und Tabellen ergänzt du selbst. Nach `baha up` zeigen `baha app logs app` die Logs und `baha app env --format json` maskierte Bindungen.

Bei vorhandenem Repository beginnst du dort mit `baha app inspect .`, danach `baha app init`; kein zweites Scaffold über vorhandene Dateien schreiben.

## Source-neutrale Adoption

Compose, Repository-Quadlet und Raw Kubernetes YAML werden über denselben Source-Vertrag erkannt. Das garantiert nicht ihre Runtime-Ausführung. Mehrdeutige Quellen werden nicht stillschweigend ausgewählt. `--quick` scheitert sicher, wenn keine deterministische Wahl möglich ist.

Explizite Source-/Komponentenwahl, wenn im Repository tatsächlich vorhanden:

```bash
baha app init my-app --workload-source kubernetes:deploy/k8s --workload-component api --workload-component worker
```

Helm und Kustomize sind noch keine Source Adapter. Eine benötigte explizite Auswahl wird in `baseharbor.repository.yaml` gespeichert.

Weitere Operationen umfassen [Backup/Restore](../how-to/backup-restore.md), Secrets, Logs, Shell/Exec, Umgebung, TLS, Evidence, Runtime-Identität und Datenzugriff. CLI und Maschinenpfade verwenden dasselbe Domänenergebnis.

Exakte Referenz: [Application-Befehle (EN)](https://mcpdev80.github.io/baseharbor/cli/applications/).

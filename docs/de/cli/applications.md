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

## Lokale Workload ohne managed Capabilities (v0.4.25)

Ein selbst geschriebenes `baseharbor.yaml` darf keine `app.id` enthalten;
`services` ist optional:

```yaml
version: 1
app:
  name: my-app
  environment: dev
```

Lege eine unterstützte, eindeutig auswählbare Compose-Workload im Repository ab
und führe `baha app init --quick`, `baha app plan --json` und `baha app apply` aus.
Init vergibt über den geschützten Repository-App-Store eine stabile ID, ohne das
autorierte YAML umzuschreiben. Lesebefehle vergeben keine ID. Fehlende oder
mehrdeutige Source ist ein Fehler und kein READY-Deployment.

Eine echte lokale Workload ohne angeforderte managed Capabilities überspringt
im vorhandenen Lifecycle Core, managed Gateway und Broker. Es braucht weder
Profil-Schalter noch implizites SQL oder Core-Login. Status und Doctor verwenden
echte Workload-Beobachtungen; Stop/Up und wiederholtes Destroy behalten die
Ownership-Prüfungen. `baha serve` erhält dadurch keine Freigabe: Seine
Management-Sicherheitsvoraussetzungen und die explizite Core-Installation
bleiben erforderlich.

Plan-JSON sowie MCP/HTTP-Plan verwenden denselben `footprint`
(`baseharbor.footprint/v1`). Ein neu geplanter Binding ist `requested/unbound`
und kein Live-Nachweis. Angeforderte managed Capabilities werden mit ausgewählten
Providern und ihren echten Abhängigkeiten realisiert. Ein bedingungsloses
SQL-/Secrets-/Identity-Paket für jede App gibt es nicht. Ein unabhängig aktivierter
Management-Core behält seine Sicherheitsvoraussetzungen. Speicher-/Containerwerte
bleiben unbekannt, wenn ihre Zuordnung nicht nachgewiesen ist. Provider-Beobachtungen
bleiben bis zum geprüften nativen Inventar `unverifiable`. Nichtinteraktives
App-`--yes` autorisiert keine Einrichtung des Management-Core.

Kompatible native Placements werden wiederverwendet. Nicht unterstützte native
BYO- oder Ersatzadapter werden vor Provisionierung abgelehnt; externe OIDC-/OTLP-
Adapter bleiben unterstützt. Der Plan nennt die tatsächlichen Ausführungsgrenzen,
ohne einen funktionierenden Ersatz zu behaupten. Es gibt keine Migration alter
Manifest-Formate.

Capability-Entfernung löst das App-Binding und gleicht die verbleibenden
Target-weiten Abhängigkeiten ab. Eigene Runtime-Dienste ohne verbleibenden
Verbraucher werden entfernt. Anforderungen anderer Apps oder unabhängig aktivierter
Management-Verbraucher erhalten gemeinsam genutzte Dienste. Persistente Daten,
Credentials und Recovery-Material bleiben standardmäßig an ihren eigenen Orten.
Erhaltenes Material ermöglicht Wiederherstellung; es belegt keinen weiter
funktionierenden Dienstzugriff. Unterstütztes Wiederhinzufügen verwendet die
bestehende Systemidentität und den geschützten Provider-Zustand erneut.

Bewusste Provider-Änderungen zeigen betroffene Verbraucher, verlorene Funktionen,
gewählte Fortsetzung und erhaltenes/gelöschtes Material. Alternativlose,
destruktive oder unbekannte Folgen werden mit **Achtung** angezeigt und erfordern
zwei ausdrückliche Bestätigungen des aktuellen Plans. Gewöhnliches `--yes`
ersetzt weder diese Bestätigungen noch die Zustimmung zur Datenlöschung.
Tatsächliche Berechtigung, Ownership und Prüfung nach der Änderung bleiben
Voraussetzung. Fehlgeschlagene oder unterbrochene Ausführung bleibt teilweise
erledigt und wiederaufnehmbar. Der v0.4.25-Kandidat benötigt weiterhin die vollständige
Abnahme aller Provider, Mutationspfade und des nativen Ecosystems. Einzelne grüne
Fixtures belegen keine Pre-Release-Bereitschaft.

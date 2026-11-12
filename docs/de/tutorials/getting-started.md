# Erste Schritte

Mit BaseHarbor kannst du eine vorhandene Anwendung übernehmen oder eine neue erstellen. Du benötigst die `baha`-CLI und ein eingerichtetes Docker- oder Podman-Target. Falls noch keines vorhanden ist, siehe [Targets einrichten](../cli/targets.md).

## Vorhandene Anwendung übernehmen

Öffne das Repository deiner Anwendung und führe aus:

```bash
baha inspect .
baha init
baha plan
baha up
baha status
```

`inspect` erkennt die Workloads, ohne Dateien zu verändern. `init` erstellt die Anwendungsbeschreibung und fragt nur bei notwendigen Entscheidungen nach. `plan` zeigt die geplanten Änderungen, `up` führt sie aus. `status` zeigt den Zustand und die Adresse der Anwendung.

Bei Problemen hilft `baha doctor`. Mit `baha down` stoppst du die Anwendung, ohne persistente Daten zu löschen. `baha up` startet sie erneut.

Bei eindeutig erkannten Repositories kannst du für die Automatisierung `baha init --quick` verwenden. Bei Mehrdeutigkeiten ist eine ausdrückliche Auswahl notwendig.

## Neue Anwendung erstellen

Wechsle in einen Ordner, der noch kein Verzeichnis `orders-api` enthält:

```bash
baha new application orders-api --stack go --http --sql
cd orders-api
baha plan
baha up -e dev
baha status
```

Das Beispiel erstellt eine Go-HTTP-Anwendung mit PostgreSQL. Öffne die von `baha status` angezeigte HTTPS-Adresse und rufe `/healthz` auf. Bei Docker und rootless Podman können die Ports unterschiedlich sein.

## Was passiert beim ersten Start?

Eine lokale Anwendung ohne verwaltete Dienste benötigt keinen Core. Fordert sie beispielsweise SQL oder Secrets an, verwendet BaseHarbor eine passende Installation oder fragt vor deren Einrichtung um Zustimmung. Im nichtinteraktiven Modus wird Core nicht stillschweigend installiert.

Gegebenenfalls fragt BaseHarbor nach erforderlichen Secrets, dem Ablageort einer Wiederherstellungsdatei oder einem Port. Geheimnisse werden ohne sichtbare Eingabe erfasst. Die Erstellung der Projektdateien startet noch keine Anwendung.

Für einen getrennten Entwicklungsrechner und Core-Host siehe [Entwicklungsrechner verbinden](../how-to/remote-workstation.md). Die Einrichtung des ersten Operators und das Enrollment entfernter Hosts sind eigenständige Abläufe.

## Mehr erfahren

[Anwendungsanforderungen](../how-to/application-needs.md) · [Targets](../cli/targets.md) · [Umgebungen](../how-to/environments.md) · [PostgreSQL](../how-to/postgres.md) · [Backup und Restore](../how-to/backup-restore.md) · [CLI-Referenz](../cli/command-index.md)

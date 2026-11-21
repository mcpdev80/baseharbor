# Development und Workspaces

Development-Einstellungen bleiben außerhalb des portablen Application Intent.

## Stack Profiles

`baha stack list`, `baha stack show NAME` und `baha stack create` verwalten wiederverwendbare Entwicklungseinstellungen aus Built-in-, Benutzer-, Repository- oder Organisationskatalogen. Ein Stack entscheidet nicht über SQL-/Cache-Provider.

```text
baha stack list
baha stack show NAME
baha stack create
```

## Developer-Zugriff

`baha dev domain` verwaltet die Target-Domain. `baha dev credentials` zeigt lokale Management-Zugangsdaten ausdrücklich an; normale Status-/Doctor-Ausgaben tun das nicht. Siehe [Developer-Zugriff](../how-to/developer-access.md).

## Workspaces

Workspaces ordnen Application-Komponenten lokalen Source-Checkouts zu. Absolute Pfade bleiben lokaler Developer-Zustand, keine Application-Identität.

`baha app workspace status` liest Zustand. `update --check` holt Upstream-Informationen und zeigt sichere Änderungen, ohne Checkout-Revisionen zu bewegen. `update` verwendet Git und führt nur Fast-Forward-Updates sauberer Branches mit Upstream aus.

BaseHarbor macht kein automatisches Stash, Reset, Rebase, divergierendes Merge oder Branch-Wechsel. Dirty, detached, ahead-only oder divergierende Repositorys bleiben unangetastet und bekommen einen Git-Nächsten-Schritt. `init`, `map`, `show` und `resolve` bleiben für explizite Mappings verfügbar.

In einem Application-Repository mit vorhandenem Git-Remote `origin` und Component `app`:

```text
baha dev credentials
baha dev domain
```

Source-Identität gehört in `.baseharbor/sources.yaml`; der Checkout-Pfad bleibt lokal. Weitere Komponenten müssen tatsächlich im Application Contract vorhanden sein. Quellen sind Git-Repositorys, lokale Verzeichnisse oder OCI-Images; Komponenten sind optionale eindeutige Identitäten und können sich eine Quelle teilen.

Inkrementelle Pflege mit `baha app workspace source add`, `baha app workspace source remove`, `baha app workspace component add`, `baha app workspace component remove` und `baha app workspace path` (nur bestehende Verzeichnisse; letzte Quelle einer Anwendung bleibt erhalten; lokale Dateien werden nicht gelöscht). `baha app workspace source add` und `baha app workspace component add` ersetzen einen vorhandenen Eintrag derselben Identität, ohne Duplikate. `baha app workspace source remove` löst die Quelle, ihre Komponenten-Mappings und ihren lokalen Mapping-Eintrag; die letzte Quelle einer Anwendung bleibt erhalten. Keine dieser Befehle legt lokale Verzeichnisse an oder löscht sie. Eine lokale Verzeichnisquelle behält ihren absoluten Pfad in `.baseharbor/sources.yaml`; ist das Verzeichnis selbst ein Git-Checkout, prüfen `status` und `update` ihn nach denselben Sicherheitsregeln wie gemappte Repositorys:

```text
baha app workspace status
baha app workspace update --check
baha app workspace update
```

[Exakte Befehle (EN)](https://mcpdev80.github.io/baseharbor/cli/development/).


## Ergänzende Befehlsreferenz

Die folgenden unveränderten Beispiele und Bezeichner entsprechen der englischen Referenz.

```bash
baha app workspace init --local legacy=/srv/legacy-app --component api=legacy@services/api
baha app workspace source add --source extra=https://git.example/extra.git
baha app workspace component add worker extra@services/worker
baha app workspace component list
baha app workspace path legacy --show-current
baha app workspace path api-source ~/dev/api
baha app workspace component remove worker
baha app workspace source remove extra
```
```bash
repository_url="$(git remote get-url origin)"
baha app workspace init --source backend="$repository_url" --component app=backend
baha app workspace map backend "$PWD"
baha app workspace show
baha app workspace resolve -o json
baha app workspace status
baha app workspace update --check
```

```bash
baha stack list
baha stack show go
```

Technische Bezeichner: `orders-api`, `--stack go`, `app new`.

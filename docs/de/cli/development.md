# Development und Workspaces

Development-Einstellungen bleiben außerhalb des portablen Application Intent.

## Stack Profiles

`baha stack list`, `baha stack show NAME` und `baha stack create` verwalten wiederverwendbare Entwicklungseinstellungen aus Built-in-, Benutzer-, Repository- oder Organisationskatalogen. Ein Stack entscheidet nicht über SQL-/Cache-Provider.

```bash
baha stack list
baha stack show go
```

## Developer-Zugriff

`baha dev domain` verwaltet die Target-Domain. `baha dev credentials` zeigt lokale Management-Zugangsdaten ausdrücklich an; normale Status-/Doctor-Ausgaben tun das nicht. Siehe [Developer-Zugriff](../how-to/developer-access.md).

## Workspaces

Workspaces ordnen Application-Komponenten lokalen Source-Checkouts zu. Absolute Pfade bleiben lokaler Developer-Zustand, keine Application-Identität.

`baha app workspace status` liest Zustand. `update --check` holt Upstream-Informationen und zeigt sichere Änderungen, ohne Checkout-Revisionen zu bewegen. `update` verwendet Git und führt nur Fast-Forward-Updates sauberer Branches mit Upstream aus.

BaseHarbor macht kein automatisches Stash, Reset, Rebase, divergierendes Merge oder Branch-Wechsel. Dirty, detached, ahead-only oder divergierende Repositorys bleiben unangetastet und bekommen einen Git-Nächsten-Schritt. `init`, `map`, `show` und `resolve` bleiben für explizite Mappings verfügbar.

In einem Application-Repository mit vorhandenem Git-Remote `origin` und Component `app`:

```bash
repository_url="$(git remote get-url origin)"
baha app workspace init --source backend="$repository_url" --component app=backend
baha app workspace map backend "$PWD"
baha app workspace show
baha app workspace resolve -o json
baha app workspace status
baha app workspace update --check
```

Source-Identität gehört in `.baseharbor/sources.yaml`; der Checkout-Pfad bleibt lokal. Weitere Komponenten müssen tatsächlich im Application Contract vorhanden sein.

[Exakte Befehle (EN)](https://mcpdev80.github.io/baseharbor/cli/development/).

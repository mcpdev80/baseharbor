# Shell und Bedienung

`baha completion bash|zsh|fish` erzeugt lesend Completion. `baha shell-init bash|zsh|fish` liefert Helfer für Target-Aktivierung und Prompt. `baha prompt` zeigt Shell-lokalen Kontext; `baha tui` öffnet die interaktive Oberfläche, sofern verfügbar.

Prompt-Rendering startet oder verändert keine Runtime. Shell-Helfer sind optional; explizites `--target` funktioniert ohne sie.

## Bash

Generierte Helfer zunächst ansehen, dann bei Bedarf in dieser Shell laden:

```bash
baha shell-init bash > /tmp/baseharbor-shell.bash
```

Nach Prüfung der Datei, mit bereits vorhandenem Target `docker-dev`:

```bash
source /tmp/baseharbor-shell.bash
baha target activate docker-dev
baha prompt
baha target deactivate
```

Completion für diese Bash-Sitzung:

```bash
source <(baha completion bash)
```

Zsh/Fish verwenden ihre jeweiligen Skripte. Dauerhaftes Laden ist eine eigene Entscheidung für deine Shell-Konfiguration.

[Exakte Befehle (EN)](https://mcpdev80.github.io/baseharbor/cli/shell-ux/).

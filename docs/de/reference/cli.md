# CLI-Referenz

Die BaseHarbor CLI Referenz wird durch Befehlsverantwortung organisiert.

Verwenden Sie die kanonische[CLI documentation](../cli/index.md):

- [Core workflow](../cli/core.md)
- [Applications](../cli/applications.md)
- [Targets](../cli/targets.md)
- [Providers](../cli/providers.md)
- [Development](../cli/development.md)
- [Organization and policy](../cli/organization-policy.md)
- [Security and trust](../cli/security-trust.md)
- [Automation and agents](../cli/automation-agents.md)
- [Shell and UX](../cli/shell-ux.md)
- [Global options](../cli/global-options.md)

Befehlsspezifisch `--help` bleibt die maßgebliche Syntax, die durch die Implementierung erzeugt wird. Die Dokumentation erklärt Intent, Sicherheit, Kontextauflösung, Automatisierungsverhalten und gemeinsame Workflows rund um diese Syntax.

## Process exit codes

Der `baha`-Prozess beendet sich mit einem einzelnen ganzzahligen Status, der das Ergebnis für Shells, Pipelines und Automatisierung codiert. Die Menge der Exit-Codes ist ein stabiler Public-Contract: die dokumentierte Menge und das Verhalten in `internal/cli/cli.go` (`ExitCode`) und `cmd/baha/main.go` sind zusammengesperrt und durch einen Go-Contract-Test sowie eine Documentation-Audit-Zeile verifiziert.

| Code | Bedeutung | Herkunft |
| --- | --- | --- |
| `0` | Erfolg | `ExitCode(nil)` gibt `0` zurück (`internal/cli/cli.go:114-117`) |
| `1` | Laufzeitfehler | Unklassifizierter Befehlsfehler läuft auf `1` aus (`internal/cli/cli.go:126`) |
| `2` | Nutzungsfehler | `*UsageError` wird `2` zugeordnet (`internal/cli/cli.go:118-121`) |
| `130` | Unterbrochen (`SIGINT`) | `os.Interrupt`-Handler (`cmd/baha/main.go:61-63`) |
| `143` | Beendet (`SIGTERM`) | `syscall.SIGTERM`-Handler (`cmd/baha/main.go:83-86`) |

Child-Status-Passthrough: `baha app exec` gibt den Status eines Child-Prozesses unverändert weiter. `internal/cli.WithExitCode` trägt einen expliziten OS-Status im Bereich `1-255` durch den Befehlsbaum als `*ExitCodeError`, und `ExitCode` gibt diesen Status zurück (`internal/cli/cli.go:102-127`). Statuses außerhalb `1-255` (inklusive `0` und `>255`) werden von `WithExitCode` abgelehnt und fallen auf den Laufzeitfehler-Code `1` zurück.

Wie der Code gewählt wird: `main` führt den Befehlsbaum aus und ruft beim Fehler `cli.ExitCode(err)` auf. Ein `context.Canceled` aus einer Unterbrechung wird `130`; ein `SIGTERM`-Signal wird `143`; ein `*UsageError` wird `2`; ein mit `WithExitCode` markierter Fehler wird sein getragener Status; alles andere wird `1`. Siehe die [outcome classes](errors.md), wie das semantische Ergebnis auf diese Codes abbildet.

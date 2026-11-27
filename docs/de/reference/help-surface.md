# Goldene CLI-Help-Oberfläche

Karte 0428-306 (Bug-Hunt H-V36) bewertete die CLI-Help-Oberfläche als konsistent und ergänzte
stattdessen einen Regressionsschutz. `cmd/baha/testdata/cli-help-golden.txt` hält den exakt
gerenderten `--help`-Text jedes öffentlichen Befehls fest.

Abweichungen lassen die normalen CLI-Pakettests in der CI scheitern. Ein Golden-Diff wird wie jede
andere Produkt-Oberfläche reviewed; anschließend neu erzeugen:

```bash
BASEHARBOR_UPDATE_MACHINE_DOCS=1 go test ./cmd/baha -run TestCLIHelpGoldenSurface
```

Das Golden rendert den Befehlsbaum außerhalb eines Application-Repository bei `COLUMNS=100`.
Die Live-Hilfe bleibt maßgeblich: `baha COMMAND --help` für den jeweiligen Befehl. Siehe
[Befehlsindex](../cli/command-index.md) und [Globale Optionen](../cli/global-options.md).

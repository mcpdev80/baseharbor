# Deployment-Umgebung wählen

Die Umgebung steuert Policy, Placement und geschützten Deployment-Zustand. Sie macht aus SQL-Intent keine Produktwahl.

Im Application-Repository mit konfiguriertem Runtime-Target:

```bash
baha plan -e dev
baha up -e dev
baha status -e dev
baha doctor -e dev
```

Application, Umgebung und Target der Ausgabe prüfen, bevor URLs oder Daten verwendet werden.

Test-Policy ohne Deployment vergleichen:

```bash
baha plan -e test
baha policy check -e test
baha policy explain -e test
```

| Umgebung | Zweck |
| --- | --- |
| `dev` | Sichere lokale Entwicklung |
| `test` | Produktionsnahe Validierung |
| `prod` | Restriktiver, auditierbarer Betrieb |

Ein grünes Dev-Deployment beweist weder Freigabe noch Konfiguration von Test/Prod. Ein anderes Target wird ausdrücklich gewählt, etwa `baha --target docker-dev plan -e test`. Umgebung ersetzt keine Target-/Provider-Einrichtung.

[Kanonische Anleitung (EN)](https://mcpdev80.github.io/baseharbor/how-to/environments/).

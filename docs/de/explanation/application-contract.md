# Anwendungsvertrag

Der Anwendungsvertrag beschreibt, was eine Anwendung benötigt.

Er soll klein und portabel bleiben.

```yaml
version: 1

app:
  name: my-app

services:
  sql:
    enabled: true

  cache:
    enabled: true

secrets:
  required:
    - name: APP_SECRET
```

Die wichtigste Regel:

```text
Anwendungsanforderung != Provider-Konfiguration
```

Runtime-spezifische Namen, Provider-Auswahl, Platzierung und generierte Zugangsdaten gehören nicht in den portablen Vertrag.

Die normative Definition steht in der englischen [Spezifikation des Anwendungsvertrags v1](https://mcpdev80.github.io/baseharbor/spec/application-contract-v1/).

# Application Contract

Der Application Contract beschreibt, was eine Anwendung braucht.

Er soll klein und portabel bleiben.

```yaml
version: 1

app:
  name: my-app

services:
  postgres:
    enabled: true

  redis:
    enabled: true

secrets:
  required:
    - name: APP_SECRET
```

Die wichtigste Regel:

```text
Application Intent != Provider-Konfiguration
```

Runtime-spezifische Namen, Provider-Auswahl, Placement und generierte Credentials gehören nicht in den portablen Contract.

Die normative Definition steht in der englischen [Application Contract v1 Spec](https://mcpdev80.github.io/baseharbor/spec/application-contract-v1/).

# Anwendungsvertrag

Der Anwendungsvertrag beschreibt, was eine Anwendung benötigt.

Er soll klein und portabel bleiben.

```yaml
version: 1

app:
  id: 550e8400-e29b-41d4-a716-446655440000
  name: my-app
  environment: dev

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

`app.id` ist die stabile, opake `application_id`. Onboarding erzeugt sie automatisch. Umbenennen, Verschieben des Repositorys oder eine andere Runtime ändern diese Identität nicht. Name und Umgebung bleiben lesbare Selektoren, keine dauerhaften Deployment-Primärschlüssel.

Applications können SQL, Cache, dauerhaften Key-Value-Speicher, Dokumentdatenbanken, Messaging, S3, Secrets, Identity, Exposure und Telemetrie anfordern. `database.document` schreibt kein MongoDB fest; `messaging.queue` kein RabbitMQ. `database.key-value` bleibt von rekonstruierbarem `cache.key-value` getrennt.

Die normative Definition steht in der englischen [Spezifikation des Anwendungsvertrags v1](https://mcpdev80.github.io/baseharbor/spec/application-contract-v1/).

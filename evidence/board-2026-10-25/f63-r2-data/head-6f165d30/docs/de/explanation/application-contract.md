# Anwendungsvertrag

Der Anwendungsvertrag beschreibt, was eine Anwendung benötigt.

Er soll klein und portabel bleiben.

```yaml
version: 1

app:
  name: my-app
  environment: dev

services:
  sql:
    enabled: true

  key_value:
    enabled: true

  messaging_queue:
    enabled: true

  document_database:
    enabled: true

secrets:
  required:
    - name: APP_SECRET
```

Die wichtigste Regel:

```text
application intent != provider configuration
```

Runtime-spezifische Namen, Provider-Auswahl, Platzierung und generierte Zugangsdaten gehören nicht in den portablen Vertrag.

Entwickler setzen keine `app.id`; diese Eingabe wird vor Änderungen abgelehnt.
BaseHarbor erzeugt die stabile `application_id` einmal bei expliziter
Initialisierung und verwaltet sie im bestehenden systemeigenen State.
Lesen und Parsen erzeugen keine IDs oder State. Umbenennen, Verschieben des Repositorys oder eine andere Runtime ändern diese Identität nicht. Name und Umgebung bleiben lesbare Selektoren, keine dauerhaften Deployment-Primärschlüssel.

Applications können SQL, Cache, dauerhaften Key-Value-Speicher, Dokumentdatenbanken, Messaging, S3, Secrets, Identity, Exposure und Telemetrie anfordern. `database.document` schreibt kein MongoDB fest; `messaging.queue` kein RabbitMQ. `database.key-value` bleibt von rekonstruierbarem `cache.key-value` getrennt.

Die normative Definition steht in der englischen [Spezifikation des Anwendungsvertrags v1](https://mcpdev80.github.io/baseharbor/spec/application-contract-v1/).

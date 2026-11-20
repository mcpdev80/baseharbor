# BaseHarbor-Beispiel für GitHub Actions

Dieses Beispiel startet ausschließlich über die manuelle Aktion **Run
workflow**. Es gibt keinen `push`- oder Pull-Request-Trigger. Der gesamte Ablauf
läuft in einem Job auf einem kurzlebigen, von GitHub gehosteten Ubuntu-Runner,
damit lokaler Core und Anwendungszustand während des Beispiels erhalten bleiben.

Kopiere `consume.yml` und `scripts/consumption-example.sh` in ein
Anwendungsrepository mit gültiger `baseharbor.yaml` und Workload-Quelle. Der Job
verwendet die Docker-Engine des Runners und stoppt die Anwendung vor dem Ende.
Verwende eine Wegwerfanwendung und Wegwerfgeheimnisse; der Workflow ist nicht
für ein Kunden-Target gedacht.

Setze die Release-Version als manuelle Eingabe. Füge `APP_API_KEY` nur dann als
maskiertes GitHub-Actions-Geheimnis hinzu, wenn die Anwendung `API_KEY`
deklariert. Setze `APP_CHECK_POSTGRES_CREDS=true` nur bei einer Anwendung mit
PostgreSQL-Verbindung. Der [CI-Leitfaden](../../how-to/ci-consumption.md)
beschreibt Sicherheitsgrenzen und weitere Eingaben.

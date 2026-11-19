# BaseHarbor-Beispiel für GitLab CI

Diese Pipeline akzeptiert ausschließlich den manuellen Start **Run pipeline**.
Auch ihr Job muss manuell gestartet werden; Pushes und Merge Requests lösen
keinen Lauf aus. Der Job verwendet einen einzelnen kurzlebigen Runner, damit
Core und Anwendungszustand zusammenbleiben.

Kopiere `pipeline.yml` und `scripts/ci-consumption-example.sh` in ein
Anwendungsrepository mit gültiger `baseharbor.yaml` und Workload-Quelle.
Registriere einen kurzlebigen Runner mit dem Tag
`baseharbor-ephemeral-docker` und Docker-Socket. Verwende eine Wegwerfanwendung
und Wegwerfgeheimnisse; das Beispiel ist nicht für ein Kunden-Target gedacht.

Setze `BASEHARBOR_VERSION` auf eine veröffentlichte Version. Füge `APP_API_KEY`
nur dann als maskierte und geschützte CI/CD-Variable hinzu, wenn die Anwendung
`API_KEY` deklariert. Setze `APP_CHECK_POSTGRES_CREDS=true` nur bei einer
PostgreSQL-Verbindung. Der [CI-Leitfaden](../../how-to/ci-consumption.md)
beschreibt Sicherheitsgrenzen und weitere Eingaben.

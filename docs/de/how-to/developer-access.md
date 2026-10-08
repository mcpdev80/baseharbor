# Lokaler Developer-Zugriff

Verwende logische Application-/Ressourcennamen statt generierter Container-Namen und Ports. Lokales `dev` braucht im trusted-local-Modus keinen Operator-Login; test/prod verwenden ihre OIDC-/Policy-Grenze.

## Kanonische URLs

```bash
baha dev domain
```

Der Standard ist `baha.localhost`; `baha dev domain dev.example.internal` ändert ihn ausdrücklich. Application-Routen verwenden `<app>.<domain>`, Management-Routen semantische Hosts wie `pgadmin`, `auth`, `auth-admin`, `storage`, `secrets` und `metrics`.

Der Target-Gateway verwendet HTTPS mit verifiziertem Upstream-TLS. Docker nutzt normalerweise Port 443, rootless Podman den unprivilegierten Port 8443. Die gemeldete kanonische URL ist maßgeblich; Fallback-Ports können abweichen.

Für einen intern HTTPS-fähigen Repository-Workload ist das Service-Label `io.baseharbor.workload.protocol: "https"` explizit. Der Gateway prüft dann die projizierte Workload-CA und verwendet den Service-Namen als SNI. Die Workload-Protokollwahl verändert nicht das verpflichtende TLS der Core-/Provider-Dienste.

## Management-Zugangsdaten

```bash
baha dev credentials
```

Der Standardbenutzer `developer` ist änderbar mit `--username USER`; `--reset` rotiert ausdrücklich. Die Target-/Environment-lokale Management-Identität bleibt von Datenbank-, S3-, Runtime- und Operator-Zugangsdaten getrennt. Normale Status-/Doctor-Ausgaben zeigen das Passwort nicht.

## Datenzugriff

Im Application-Repository mit installierten nativen Clients:

```bash
baha app psql
baha app redis
```

Mehrere Instanzen werden explizit gewählt, etwa `baha app psql analytics` oder `baha app redis cache`. Außerhalb eines Repositorys ist eine Application mit `--app NAME` nötig. Passwörter gehen über die Child-Prozess-Umgebung, nicht über Befehlsargumente. Ein fehlender Client wird nicht durch eine fremde Container-Shell ersetzt.

`baha app creds postgres` zeigt standardmäßig maskierte Metadaten. `--reveal` ist eine separate bewusste Aktion.

## Workload-Zugriff

```text
baha app logs api
baha app logs api --follow
baha app shell api
baha app exec worker ./bin/worker --version
```

Die Services müssen im Application Contract ausgewählt sein. Zugriff benutzt dieselben Bindungen/Overlays wie der Lifecycle; Repository-Compose wird nicht umgeschrieben. Fehlende oder mehrdeutige Instanzen scheitern klar.

[Vollständige Erklärung (EN)](https://mcpdev80.github.io/baseharbor/how-to/developer-access/).

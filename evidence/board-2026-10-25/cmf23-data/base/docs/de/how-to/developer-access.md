# Lokaler Developer-Zugriff

Verwende logische Application-/Ressourcennamen statt generierter Container-Namen und Ports. Lokales `dev` braucht im trusted-local-Modus keinen Operator-Login; test/prod verwenden ihre OIDC-/Policy-Grenze.

## Kanonische URLs

```text
baha.localhost
```

Der Standard ist `baha.localhost`; `baha dev domain dev.example.internal` ändert ihn ausdrücklich. Application-Routen verwenden `<app>.<domain>`, Management-Routen semantische Hosts wie `pgadmin`, `auth`, `auth-admin`, `storage`, `secrets` und `metrics`.

Der Target-Gateway verwendet HTTPS mit verifiziertem Upstream-TLS. Docker nutzt normalerweise Port 443, rootless Podman den unprivilegierten Port 8443. Die gemeldete kanonische URL ist maßgeblich; Fallback-Ports können abweichen.

Für einen intern HTTPS-fähigen Repository-Workload ist das Service-Label `io.baseharbor.workload.protocol: "https"` explizit. Der Gateway prüft dann die projizierte Workload-CA und verwendet den Service-Namen als SNI. Die Workload-Protokollwahl verändert nicht das verpflichtende TLS der Core-/Provider-Dienste.

## Management-Zugangsdaten

```bash
baha dev domain
baha dev domain dev.example.internal
```

Der Standardbenutzer `developer` ist änderbar mit `--username USER`; `--reset` rotiert ausdrücklich. Die Target-/Environment-lokale Management-Identität bleibt von Datenbank-, S3-, Runtime- und Operator-Zugangsdaten getrennt. Normale Status-/Doctor-Ausgaben zeigen das Passwort nicht.

## Datenzugriff

Im Application-Repository mit installierten nativen Clients:

```text
<app>.<domain>
<app>-<service>.<domain>

pgadmin.<domain>
cache.<domain>
storage.<domain>
auth.<domain>
auth-admin.<domain>
secrets.<domain>
metrics.<domain>
```

Mehrere Instanzen werden explizit gewählt, etwa `baha app sql analytics` oder `baha app cache cache`. Außerhalb eines Repositorys ist eine Application mit `--app NAME` nötig. Passwörter gehen über die Child-Prozess-Umgebung, nicht über Befehlsargumente. Ein fehlender Client wird nicht durch eine fremde Container-Shell ersetzt.

`baha app creds postgres` zeigt standardmäßig maskierte Metadaten. `--reveal` ist eine separate bewusste Aktion.

## Workload-Zugriff

```yaml
services:
  api:
    labels:
      io.baseharbor.workload.protocol: "https"
```

Die Services müssen im Application Contract ausgewählt sein. Zugriff benutzt dieselben Bindungen/Overlays wie der Lifecycle; Repository-Compose wird nicht umgeschrieben. Fehlende oder mehrdeutige Instanzen scheitern klar.

[Vollständige Erklärung (EN)](https://mcpdev80.github.io/baseharbor/how-to/developer-access/).


## Ergänzende technische Beispiele

```bash
baha dev credentials
baha dev credentials --username USER
baha dev credentials --reset
```

```bash
baha app sql
baha app cache
```

```bash
baha app sql primary
baha app sql analytics
baha app cache cache
baha app cache sessions
```

```bash
baha app sql --app mailflow
baha app cache cache --app mailflow
```

```bash
baha app creds postgres
baha app creds valkey cache
```

```bash
baha app creds postgres --reveal
```

```bash
baha app logs
baha app logs api
baha app logs api --follow
```

```bash
baha app shell api
```

```bash
baha app exec api env
baha app exec worker ./bin/worker --version
```


Technische Bezeichner: `baseharbor.yaml`, `https://<host>:8443`, `baha status`, `baha doctor`, `valkey-cli`, `redis-cli`, `baha login`, `baha login -e test`, `baha whoami -e test`, `baha logout -e test`.


### Next.js-Upstream-Port stimmt nicht überein (P0 #844)

Eine generierte Next.js-Application deklariert `exposure.http.port: 8080`. Der Compose-Workload muss ebenso auf `PORT=8080` lauschen, `8080:8080` veröffentlichen, `EXPOSE 8080` deklarieren und `http://127.0.0.1:8080/healthz` prüfen. Ein fehlerfrei laufender Container auf Port 3000 beweist nicht, dass der auf 8080 deklarierte Upstream erreichbar ist. HTTP 502/503/504 am kanonischen Development-Gateway bedeutet einen nicht erreichbaren Application-Upstream, nicht zwangsläufig eine fehlende HTTPS-Domain oder Gateway-Route. Zuerst Listener, Exposure-Port und Target-Netzwerk prüfen. Interne Caddyfile- und Gateway-State-Pfade gehören nicht in Fehlermeldungen.

Der SQL-Zugriff benötigt den lokalen Client `psql`; für Cache-Zugriff wird ein passender lokaler Client verwendet.

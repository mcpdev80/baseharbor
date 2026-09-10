# Entwicklerzugriff

BaseHarbor v0.3 ergänzt eine lokale Entwicklerzugriffsschicht für den täglichen Compose-Betrieb. Entwickler arbeiten mit logischen Anwendungs-, Ressourcen- und Service-Namen statt mit generierten Ports, Container-Namen oder OpenBao-Interna.

## Datenbank- und Cache-Shells

Im Anwendungsrepository:

```bash
baha app psql
baha app redis
```

Bei mehreren logischen Instanzen wird die gewünschte Instanz explizit gewählt:

```bash
baha app psql primary
baha app psql analytics
baha app redis cache
baha app redis sessions
```

`psql` verwendet das bereits materialisierte, nur für den Besitzer lesbare PostgreSQL-Binding. Das Passwort wird über die Umgebung des Kindprozesses und nicht als Kommandozeilenargument übergeben. `redis` bevorzugt `valkey-cli` und fällt bei Bedarf auf `redis-cli` zurück; auch hier erfolgt die Authentifizierung über die Client-Umgebung.

Der jeweilige Client muss lokal installiert sein. BaseHarbor verschleiert einen fehlenden Client nicht durch einen unerwarteten Container-Shell-Workaround.

Eine gespeicherte Anwendung außerhalb ihres Repositories kann explizit gewählt werden:

```bash
baha app psql --app mailflow
baha app redis cache --app mailflow
```

## Verbindungsmetadaten

Verbindungsdaten bleiben standardmäßig maskiert:

```bash
baha app creds postgres
baha app creds valkey cache
```

Ausgegeben werden Ressource, Instanz, Host, Port sowie nicht geheime Datenbank-/Benutzerinformationen. Passwort und credential-tragende URI bleiben verborgen.

Explizites Anzeigen ist eine getrennte Aktion:

```bash
baha app creds postgres --reveal
```

Der lokale v0.3-Pfad verwendet noch keine verwaltete Production-Policy mit OIDC/RBAC/JIT. Die Action-Grenze ist aber so aufgebaut, dass spätere Environment-Policies dieselben Befehle kontrollieren können, ohne den Entwickler-Workflow umzubenennen.

## Workload-Logs

```bash
baha app logs
baha app logs api
baha app logs api --follow
```

Die Befehle arbeiten auf dem durch `baseharbor.yaml` ausgewählten anwendungseigenen Compose-Workload. Der Entwickler muss den generierten Compose-Projektnamen oder Container-Namen nicht kennen.

## Workload-Shell und Exec

Shell in einem ausgewählten Workload-Service:

```bash
baha app shell api
```

Nichtinteraktiven Befehl ausführen:

```bash
baha app exec api env
baha app exec worker ./bin/worker --version
```

Workload-Befehle laufen über BaseHarbors Compose-Runtime-Grenze und verwenden dieselben materialisierten Overlays/Bindings wie die Lifecycle-Operationen. BaseHarbor schreibt für Entwicklerzugriffe die Compose-Datei der Anwendung nicht um.

## Sicherheitsmodell

- Ressourcen werden zuerst über logische Anwendung-/Instanznamen aufgelöst.
- Generierte Host-Ports und Container-Namen bleiben Implementierungsdetails.
- Datenbank-/Cache-Passwörter landen nicht in Client-Kommandozeilenargumenten.
- Credential-Werte sind standardmäßig verborgen.
- Explizites Reveal ist von normalen Connect-/Use-Aktionen getrennt.
- Workload-Zugriff ist auf die im Application Contract ausgewählten Services beschränkt.
- Fehlende oder mehrdeutige Instanzen führen zu einem klaren Fehler statt zu einer Vermutung.
- Verwaltete Production-Identity, Freigaben und JIT-Elevation bleiben spätere Policy-Schichten und werden nicht zur Voraussetzung für lokale Entwicklung.

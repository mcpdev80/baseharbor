# `baha` CLI

`baha` ist die zentrale Operator- und Entwickler-Schnittstelle von BaseHarbor. Mutationen erfolgen fail-closed: planen/pruefen, aendern, verifizieren.

## Wichtige Befehle

```text
baha
├── up / down / status / doctor
├── serve
├── connect SOURCE TARGET
├── disconnect SOURCE TARGET
├── connections
├── update
├── app
│   ├── init / create / list / show
│   ├── plan / preflight / apply
│   ├── env / status / doctor
│   ├── backup / restore / update
│   ├── psql / redis / valkey / creds
│   ├── logs / shell / exec
│   ├── tls update
│   ├── down / up / destroy
│   ├── runtime-identity rotate|revoke
│   └── secret set|list|delete|tls-set
├── openbao status|bootstrap|unseal
└── version
```

Die exakte Syntax liefert immer die ausfuehrbare Hilfe:

```bash
baha --help
baha app --help
baha app update --help
baha app tls update --help
baha update --help
baha openbao --help
```

## Read-only Repository Inspection und Reconciliation

```bash
baha app inspect .
baha app inspect . --json
```

`app inspect` bleibt strikt read-only. Bei vorhandenem `baseharbor.yaml` vergleicht der gemeinsame Inspection-Core Repository-Evidenz mit dem deklarierten Contract und unterscheidet `satisfied`, `new`, `ambiguous` und `stale`.

Aktuelle semantische Detectoren erkennen neben PostgreSQL/Redis auch S3-kompatible Nutzung, moegliche S3-Runtime-Bucket-Erzeugung, OpenMetrics-`/metrics`-Endpoints und OTLP-Export. Findings tragen eine Capability-Richtung und koennen Runtime-Hinweise wie `runtime.create` enthalten.

`stale` entfernt niemals Contract-State. Eine erkannte Runtime-Operation vergibt keine Berechtigung und provisioniert keine Ressource.

Repository-first `baha up` verwendet denselben Reconciliation-Core vor der Convergence. Neu erkannte oder unklare Capabilities und Runtime-Operations-Hinweise werden sichtbar gemeldet, aber `baseharbor.yaml` wird nicht automatisch umgeschrieben und es werden keine Runtime-Berechtigungen vergeben.

## Metrics-Collection-Policy

Ein Repository kann eine oder mehrere providerneutrale `metrics/v1`-Sources deklarieren:

```yaml
metrics:
  sources:
    - name: application
      service: api
      port: 8080
      path: /metrics
```

Damit wird **nicht** Prometheus angefordert. Deklariert wird ein von der Anwendung bereitgestellter OpenMetrics-kompatibler HTTP-Endpunkt.

Die Compose-Collection-Policy gehoert zum Deployment:

- `dev` / `development`: standardmaessig aktiv;
- Test/Staging/Produktion: standardmaessig deaktiviert;
- `BASEHARBOR_METRICS_ENABLED=true|false`: expliziter Operator-Override.

Bei aktiver Collection loesen `baha app apply` und `baha app up` Prometheus ueber die generische Provider-Placement-Schicht auf, zeigen das aufgeloeste Placement vor der Mutation an, registrieren Targets automatisch, starten den Workload und verlangen danach einen echten erfolgreichen Scrape, bevor der Metrics-Pfad als bereit gilt. Der sichere Default ist shared Prometheus mit einem isolierten Metrics-Netz je Application. Fortgeschrittene Operatoren koennen application-scoped Prometheus oder eine benannte Shared Boundary ueber die generische Provider-Policy waehlen: `BASEHARBOR_PROVIDER_PROMETHEUS_SCOPE=shared|application` sowie fuer gruppiertes Shared Placement `BASEHARBOR_PROVIDER_PROMETHEUS_SHARING_BOUNDARY=<name>`. Nicht unterstuetztes Placement bricht vor Mutation fail-closed ab. Ein externer Prometheus-Adapter ist in v0.4.10 bewusst noch nicht implementiert. Grafana, Loki und Tempo werden nicht allein durch Application-Metrics gestartet.

v0.4.10 sammelt zusaetzlich sichere OpenMetrics-Endpunkte laufender BaseHarbor-Provider, wenn die Metrics-Policy die Source-Klassen `application-provider` bzw. `platform-provider` erlaubt. Die Targets kommen aus der generischen Observability-Registry, werden anhand Placement/Sharing Boundary gefiltert und mit einem echten Prometheus-`up=1` verifiziert. Prometheus enthaelt keine Loki-/Tempo-/Collector-spezifischen Scrape-Sonderfaelle.

## Cross-Application-Connectivity

Cross-Application-Zugriff ist deny-by-default und wird genau einmal auf BaseHarbor-Platform-Ebene konfiguriert statt in beiden Application Contracts doppelt eingetragen.

```bash
baha connect app-a/api app-b/sql
baha connections
baha disconnect app-a/api app-b/sql
```

Die kurze Form ist der Normalfall. BaseHarbor loest aktives Environment, konkreten Runtime-Service, Target-Netz und Target-TCP-Port aus dem Runtime-State auf. Nur bei Mehrdeutigkeit verlangt die CLI eine Qualifizierung wie `app-b@prod/api:8080`.

Die Regel ist gerichtet. Compose haengt Source und Target nicht in dasselbe gemeinsame Bridge-Netz. BaseHarbor erzeugt ein verbindungsspezifisches Source-Link-Netz und startet einen gehaerteten Relay aus dem versionsgleichen BaseHarbor-Runtime-Image. Nur Source-Service und Relay haengen am Link; der Relay haengt zusaetzlich in genau einem vorhandenen Target-Netz und leitet ausschliesslich zum aufgeloesten Target-Service/-Port weiter. Das Target kommt niemals in das Source-Link-Netz.

`baha app down` pausiert betroffene Relay-Runtimes und behaelt die Policy. `baha app up`/`apply` reconciled sie erneut, sobald beide Endpunkte laufen. `baha app destroy` bricht fail-closed ab, solange eine Connectivity-Regel auf die Application zeigt; die Regel muss vorher explizit entfernt werden.

Provider-Sharing ist davon unabhaengig. Ein `shared` Provider vergibt niemals automatisch Application-zu-Application-Netzwerkzugriff.
## Control Plane

```bash
baha up
baha up --yes
baha up --postgres-port 15432 --openbao-port 18200
```

Beim ersten Start werden Ports geprueft. Belegte Standardports werden nicht blind verwendet.

Der globale Control Plane kann nach dem Entfernen aller Application-Bindings explizit geloescht werden:

```bash
baha destroy
baha destroy --yes
```

Der Befehl entfernt nur das BaseHarbor-eigene Control-Plane-Compose-Projekt, dessen Volumes, Runtime-State und Provider-Registry-Metadaten. Solange Application-Bindings existieren, bricht er fail-closed ab.

## Anwendung

Der normale Entwicklerweg beginnt im bestehenden Projektverzeichnis:

```bash
baha app init
```

`baha` analysiert das Repository zuerst read-only und erkennt, soweit eindeutig:

- gaengige Compose-Dateien;
- PostgreSQL- und Redis/Valkey-Abhaengigkeiten;
- wahrscheinliche Workload-Services;
- Infrastrukturvariablen aus gaengigen Env-Beispieldateien;
- wahrscheinliche Namen benoetigter Secrets.

Secret-Werte werden dabei weder angezeigt noch in `baseharbor.yaml` uebernommen. Die Regel lautet: **zuerst erkennen, nur Unklares nachfragen**.

Anschliessend zeigt der Wizard eine kompakte Auswahl der erkannten Capabilities. Vorausgewaehlte Werte koennen geaendert werden. Bei mehreren moeglichen Compose-Dateien raet `baha` nicht, sondern fragt explizit nach.

Vor dem Schreiben wird das erzeugte `baseharbor.yaml` als Vorschau angezeigt. Eine vorhandene Datei wird niemals still ueberschrieben.

Nicht-interaktiv und erkennungsbasiert:

```bash
baha app init --quick
```

`--quick` akzeptiert nur eindeutige Erkennungen und sichere Defaults. Bei Mehrdeutigkeit bricht der Befehl fail-closed ab. Credential-aehnliche Namen aus Env-/Beispieldateien bleiben heuristische Hinweise und werden ohne explizite Entwicklerbestaetigung niemals zu `secrets.required`.

Der deterministische Flag-Pfad bleibt fuer CI/Skripte erhalten:

```bash
baha app init mailflow \
  --postgres \
  --redis \
  --require-secret SECRET_KEY
```

S3-kompatibler Object Storage:

```bash
baha app init mailflow --s3-bucket attachments
```

Mehrere unabhaengige logische Buckets werden mit wiederholtem `--s3-bucket` deklariert; `--s3` fordert einen Default-Bucket an. Der Application Contract bleibt produktneutral: SeaweedFS ist nur der aktuelle Compose-Referenzprovider hinter `object-storage.s3/v1`.

`apply`, `up`, `status` und `doctor` verifizieren Managed Buckets mit einem authentifizierten S3-Put/Get. S3-Credentials sind in `baha app env` standardmaessig maskiert.

Backup/Restore bricht fuer Managed Object Storage aktuell fail-closed ab, da Bucket-Inhalte noch nicht Bestandteil der Recovery-Einheit sind.

Benannte Instanzen:

```bash
baha app init mailflow \
  --postgres-instance primary \
  --postgres-instance analytics \
  --redis-instance cache \
  --redis-instance sessions
```

Mehrere Instanzen sind mehrere logische Services und keine HA-Replikate.

Repository-Deployments initialisieren in v0.4 geschuetzten Deployment-State ueber den deklarativen Input-Resolver. Interaktiv koennen **Public FQDN** und TLS-Modus abgefragt werden. Existing/BYOC-TLS akzeptiert ein Zertifikatsverzeichnis, validiert Zertifikat/Key/FQDN und normalisiert die Dateien in owner-only BaseHarbor-State. Diese Deployment-Details gehoeren nicht in den providerneutralen `PortableContract`; Manifest v1 bleibt der oeffentliche Kompatibilitaetsvertrag.

Danach:

```bash
baha app plan
baha app preflight
baha app apply
baha app show
baha app status
baha app doctor
```

Fehlende Pflicht-Secrets blockieren `apply`/`up`. Secret-Werte selbst werden nie ausgegeben.

## Runtime-Truth

Ein laufender Container ist nicht automatisch READY. Fuer ausgewaehlte Compose-Services unterscheidet BaseHarbor Running/Healthy, Starting, Unhealthy, Exited und Missing. Konventionelle app-eigene HTTP/HTTPS-Publisher werden lokal aktiv geprueft.

Redirects gelten als erreichbare Exposition. 5xx oder nicht erreichbare Endpunkte sind NOT READY. Bei hostname-gebundenem HTTPS wird lokal verbunden, aber der konfigurierte Public FQDN als HTTP Host/TLS ServerName verwendet.

`show`, `status` und `doctor` nutzen dieselbe Workload-Wahrheit.

## Environment und Bindings

```bash
baha app env
baha app env --format json
baha app env --path
```

Credential-haltige Werte sind standardmaessig maskiert.

## Trusted-local Developer Access

```bash
baha app psql [INSTANCE]
baha app redis [INSTANCE]
baha app valkey [INSTANCE]
baha app creds postgres [INSTANCE]
baha app creds valkey [INSTANCE]
baha app logs [SERVICE]
baha app shell SERVICE
baha app exec SERVICE COMMAND [ARG...]
```

DB-/Cache-Passwoerter landen nicht als normale Argumente im Prozessaufruf. Credentials bleiben standardmaessig maskiert. Generierte Container-Namen bleiben Provider-Detail.

## Secrets

```bash
printf '%s' "$API_TOKEN" | baha app secret set API_TOKEN --stdin
baha app secret list
baha app secret delete API_TOKEN --yes
```

## Deployment-TLS

Fuer Repository-Deployments mit `tls: existing`:

```bash
baha app tls update --check
baha app tls update
```

`--check` ist read-only. Mutation validiert Quelle, Key-Pair und FQDN, verweigert Downgrades, installiert owner-only Dateien, startet den Workload bei Bedarf neu und verifiziert Readiness. Bei Fehlern wird der vorherige geschuetzte Zertifikatsstand wiederhergestellt.

ACME-Automation, OpenBao-PKI-Issuance und ein providerneutraler TLS-Contract bleiben Future Work.

## Stoppen, fortsetzen und zerstoeren

```bash
baha app down
baha app up
baha app destroy
baha app destroy --yes
baha app destroy --yes --full-reset
```

Der normale `app destroy` behaelt Repository-Deployment-State (`.baseharbor/init.env`) und normalisierte lokale TLS-Dateien fuer eine spaetere Neuerstellung. Der Destroy-Plan zeigt diesen erhaltenen State explizit. `--full-reset` entfernt zusaetzlich nur diese BaseHarbor-eigenen Repository-Deployment-Dateien; `baseharbor.yaml`, app-eigene Compose-Daten/Volumes und externe Zertifikatsquellen bleiben erhalten.

## Backup und Restore

Interaktiv:

```bash
baha app backup
baha app restore ./backup.bhbackup
```

Fuer Automation:

```bash
baha app backup --password-file ./backup-password.txt
baha app restore ./backup.bhbackup --password-file ./backup-password.txt
```

Guided Password-Eingabe deaktiviert Terminal-Echo und legt das Passwort nicht in argv. Restore bleibt fail-closed und meldet READY erst nach erfolgreicher Backend-, Runtime-Identity-, Workload- und HTTP/TLS-Verifikation.

## Application Update

Read-only pruefen:

```bash
baha app update --check
```

Mutation ist strict fast-forward only. Dirty/Ahead/Diverged schlagen fail-closed fehl. Anwendungen mit dauerhaftem BaseHarbor-State benoetigen entweder ein verschluesseltes Pre-Update-Recovery oder eine explizite `--no-backup`-Bestaetigung. Nach dem Source-Update wird der normale Apply-/Readiness-Pfad wiederverwendet.

## BaseHarbor Self-Update

```bash
baha update --check
```

Stable ist der Default-Channel. Mutation erfordert explizite Bestaetigung, prueft Release-Artefakte/Checksums, ersetzt die CLI atomar und behaelt eine Recovery-Binary. Schlaegt die Post-Verifikation fehl, wird zurueckgerollt. `sudo` wird nicht automatisch aufgerufen.

## Version

```bash
baha version
```

Offizielle Releases enthalten Semantic Version, Commit und Build-Zeit. Development-Builds sind als solche erkennbar.


## Verwaltete HTTP-Exposition

Managed Exposure wird in `baseharbor.yaml` deklariert; es gibt dafuer keine Caddy-spezifische Parallel-CLI. Die normalen Lifecycle-Befehle konvergieren und beobachten den Provider:

```bash
baha app apply
baha app status
baha app doctor
baha app down
baha app up
baha app destroy --yes
```

`app status` und `app doctor` zeigen die Ende-zu-Ende-Readiness aus dem gemeinsamen Endpoint-/Exposure-State. Redirects gelten weiter als erreichbar; HTTP 5xx und nicht erreichbare Routen sind NOT READY.

Bestehende app-eigene HTTP/HTTPS-Publisher bleiben im bisherigen Discovery-/Observation-Pfad und werden nicht in den Managed-Provider-Lifecycle uebernommen.

## OTLP-Telemetrie

OTLP-Transport wird in `baseharbor.yaml` deklariert und nicht als produktspezifischer CLI-Service ausgewaehlt:

```yaml
workload:
  services:
    - api

telemetry:
  otlp:
    signals:
      - traces
```

Bei `app apply` und `app up` loest BaseHarbor den OTLP-Provider auf, konvergiert bei Bedarf den shared Managed Collector, materialisiert normale `OTEL_*`-Workload-Einstellungen und verifiziert einen echten OTLP-HTTP/Protobuf-Export.

Ein bestehender externer OTLP-Endpunkt wird als Deployment-State gesetzt:

```bash
export BASEHARBOR_OTLP_ENDPOINT=https://otel.example.com
```

Optionale Authorization-Header verwenden `BASEHARBOR_OTLP_HEADERS`. Sie sind Runtime-/Deployment-Secrets und duerfen nicht in `baseharbor.yaml` committed werden.

Nur OTLP anzufordern startet weder Prometheus noch Loki, Tempo oder Grafana.

## v0.4.10 Managed Trace Storage

Trace-Transport und Trace-Storage bleiben getrennt. Eine Application mit OTLP-Signal `traces` verwendet Managed Trace-Retention nur, wenn die Deployment-Policy sie aktiviert:

```bash
export BASEHARBOR_TRACES_ENABLED=true
baha app up
```

BaseHarbor provisioniert bzw. verwendet dann den shared Tempo-Referenzprovider, verbindet den Managed OpenTelemetry Collector ueber ein BaseHarbor-eigenes Provider-Netz mit Tempo, exportiert den normalen Verification Trace ueber den Collector und verlangt, dass derselbe Trace in Tempo abfragbar ist, bevor der Trace-Pfad READY wird.

Der Compose-Referenzadapter unterstuetzt aktuell nur das Default-shared-Tempo-Placement. Application-scoped, external und Named-Sharing-Boundary Placement werden vor Mutation abgelehnt, weil v0.4.10 die dafuer notwendige Isolation bzw. Adapter-Semantik noch nicht sicher implementiert. Tempo bleibt Provider-State; `baseharbor.yaml` erhaelt kein Tempo-Feld und Grafana bleibt optional/Post-v0.5.

## v0.4.9 Log-Collection und Workload-Security-Policy

Zentrale Workload-Logs sind Deployment-Policy und kein Loki-Feld in `baseharbor.yaml`.

- Development: Application-Log-Collection standardmaessig aktiv;
- Test/Staging/Produktion: standardmaessig deaktiviert;
- `BASEHARBOR_LOGS_ENABLED=true|false`: expliziter Operator-Override;
- `BASEHARBOR_LOGS_COLLECT=application,application-provider,platform-provider`: Auswahl der Source-Klassen.

Der aktuelle Compose-Adapter sammelt die ausgewaehlten Repository-Workload-Services. Provider Placement verwendet die generischen Controls `BASEHARBOR_PROVIDER_LOKI_SCOPE=shared|application` und optional `BASEHARBOR_PROVIDER_LOKI_SHARING_BOUNDARY=<name>`. External Loki Placement wird bewusst abgelehnt, weil der v0.4.9-Compose-Adapter es nicht implementiert.

`app apply` und `app up` provisionieren bzw. verwenden den ausgewaehlten Loki/Alloy-Provider vor dem Workload-Start, erzeugen einen BaseHarbor-eigenen Logging-Override, starten den Workload und verlangen vor READY eine erfolgreiche Loki-Query. `status` und `doctor` pruefen Loki erneut.

Repository-Workload-Mutationen durchlaufen ausserdem den v0.4.9-Security-Preflight. `privileged: true`, Container-Runtime-Sockets, Host-Network/PID/IPC, gefaehrliche Capabilities und kritische Host-Mounts werden in Managed Environments abgelehnt. Explizite Development-Ausnahmen verwenden `BASEHARBOR_WORKLOAD_SECURITY_ALLOW=<comma-separated-codes>`; `BASEHARBOR_WORKLOAD_SECURITY_MODE=development|managed` ist ein Operator-Policy-Override. Managed Mode akzeptiert keine Development-Acknowledgement-Bypaesse.

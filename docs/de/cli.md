# `baha` CLI

`baha` ist die zentrale Operator- und Entwickler-Schnittstelle von BaseHarbor. Mutationen erfolgen fail-closed: planen/pruefen, aendern, verifizieren.

## Wichtige Befehle

```text
baha
├── up / down / status / doctor
├── serve
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

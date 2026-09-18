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

## Control Plane

```bash
baha up
baha up --yes
baha up --postgres-port 15432 --openbao-port 18200
```

Beim ersten Start werden Ports geprueft. Belegte Standardports werden nicht blind verwendet.

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

`--quick` akzeptiert nur eindeutige Erkennungen und sichere Defaults. Bei Mehrdeutigkeit bricht der Befehl fail-closed ab.

Der deterministische Flag-Pfad bleibt fuer CI/Skripte erhalten:

```bash
baha app init mailflow \
  --postgres \
  --redis \
  --require-secret SECRET_KEY
```

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

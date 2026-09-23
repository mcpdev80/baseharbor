# Repository-first Workflow

Der bevorzugte BaseHarbor-Vertrag liegt direkt im Repository der Anwendung als `baseharbor.yaml`. Manifest v1 bleibt der oeffentliche Kompatibilitaetsvertrag; v0.4 uebersetzt daraus portablen Intent in `PortableContract`. Generierter Deployment-/Runtime-State bleibt ausserhalb von Git. Secret-Werte, generierte Zugangsdaten, Deployment-TLS-Material und Laufzeitstatus bleiben ausserhalb von Git.

## Repository vor dem Manifest inspizieren

Repository-Verstaendnis ist in v0.4.3 eine gemeinsame, strikt read-only Core-Funktion:

```bash
baha app inspect .
baha app inspect . --json
```

Die Inspection Engine sammelt nachvollziehbare Evidence aus Compose-/Dockerfile-Dateien, Dependency-Metadaten, Environment-Variablennamen, Source-Imports, Konfiguration, Ports und Healthchecks. Findings werden als **Detected**, **Suggested** oder **Possible** klassifiziert.

Nur **Detected** darf von `app init --quick` automatisch uebernommen werden. Suggested/Possible bleiben Hinweise. Gibt es weder eine sicher erkannte Capability noch einen explizit erkannten Workload, bricht Quick-Init fail-closed ab, statt PostgreSQL oder einen anderen Backend-Service zu erfinden.

Inspection schreibt weder `baseharbor.yaml` noch Runtime-State oder andere Repository-Dateien. Environment-Werte werden vor Analyse/Ausgabe verworfen und Symlink-Dateien werden nicht verfolgt.

## Manifest erzeugen

Der normale interaktive Weg ist:

```bash
baha app init
```

BaseHarbor analysiert das Repository zuerst read-only, erkennt soweit moeglich Compose-Dateien, PostgreSQL/Redis/Valkey, Workload-Services und moegliche Secret-Namen und fragt nur nach fehlenden oder mehrdeutigen Informationen. Vor dem Schreiben wird das erzeugte Manifest angezeigt; eine vorhandene Datei wird niemals still ueberschrieben.

Nicht-interaktiv und detect-first:

```bash
baha app init --quick
```

Deterministisch fuer Skripte/CI:

```bash
baha app init mailflow --environment production --postgres --redis
```

Mehrere logische Instanzen werden explizit benannt:

```bash
baha app init mailflow \
  --postgres-instance primary \
  --postgres-instance analytics \
  --redis-instance cache \
  --redis-instance sessions
```

Ohne Namen verwendet `baha app init` den aktuellen Verzeichnisnamen, sofern er als Anwendungsslug gueltig ist.

## Deployment-Init in v0.4

Repository-Deployments koennen zusaetzlich geschuetzten Compose-spezifischen Deployment-State erhalten. Interaktiv koennen **Public FQDN** und TLS-Modus abgefragt werden.

Existing/BYOC-TLS akzeptiert ein Zertifikatsverzeichnis, validiert Zertifikat/Key/FQDN und schreibt normalisierte owner-only Dateien in BaseHarbor-State. Automatisch gewaehlt Host-Port-Fallbacks fuer konfigurierbare Compose-Publisher werden ebenfalls geschuetzt persistiert.

Diese Werte sind Deployment-/Provider-Details und gehoeren nicht in den providerneutralen `PortableContract`.

Seit v0.4.2 werden Provider-Platzierung und Lifecycle-Ownership zusaetzlich in der geschuetzten Provider-Registry gespeichert. Shared, application-scoped oder externe Provider bleiben Operator-State und veraendern den Repository-Contract nicht.

## Danach aus dem Repository arbeiten

BaseHarbor sucht im aktuellen Verzeichnis und seinen Eltern nach der naechsten `baseharbor.yaml`:

```bash
baha app plan
baha app preflight
baha app apply
baha app show
baha app status
baha app doctor
```

Der Anwendungsname muss dabei normalerweise nicht wiederholt werden.

## Bestehendes Compose bleibt Anwendungseigentum

BaseHarbor ersetzt die Compose-Topologie der Anwendung nicht. Bei einem vorhandenen Compose-Workload werden nur die tatsaechlich benoetigten BaseHarbor-Verbindungen ueber geschuetzte generierte Overrides ergaenzt. Bestehende Anwendungsnetzwerke und anwendungseigene Volumes bleiben erhalten.

Ein expliziter Compose-Workload kann auch als Workload-only-Anwendung ohne kuenstliche PostgreSQL-/Valkey-Abhaengigkeit betrieben werden. BaseHarbor erfindet dafuer keine Backend-Services, Credentials oder Netzwerke.

Host-Prozesse erhalten Loopback-Endpunkte, Container erhalten containerfaehige DNS-Endpunkte. Die Anwendung konsumiert weiterhin normale Variablen wie `DATABASE_URL` oder `REDIS_URL`.

```bash
baha app env --path
```

zeigt den geschuetzten dotenv-Pfad fuer normale IDE-, Prozessmanager- oder Framework-Nutzung.

## Readiness

Ein laufender Container ist nicht automatisch READY. BaseHarbor bewertet ausgewaehlte Services health-aware und prueft konventionelle app-eigene HTTP/HTTPS-Publisher auf den lokal veroeffentlichten Ports.

Redirects gelten als erreichbar. 5xx oder nicht erreichbare Endpunkte sind NOT READY. Bei hostname-gebundenem HTTPS wird lokal verbunden, aber der konfigurierte Public FQDN als HTTP Host/TLS ServerName verwendet.

## Lebenszyklus

```text
apply/up: BaseHarbor Backend -> Anwendungs-Workload -> Readiness pruefen
down:     Anwendungs-Workload -> BaseHarbor Backend
destroy:  Workload stoppen -> BaseHarbor-eigene Ressourcen loeschen
restore:  validieren -> stoppen -> wiederherstellen -> Identity neu -> starten -> verifizieren
update:   preflight -> strict fast-forward -> reconcile -> verifizieren
```

`baha app destroy --yes` entfernt BaseHarbor-eigene Laufzeitressourcen, laesst aber die committed `baseharbor.yaml` und anwendungseigene Compose-Volumes bestehen.

## Developer Access

Trusted-local Komfortbefehle verwenden logische Ressourcen/Services statt generierter Container-Namen:

```bash
baha app psql
baha app valkey
baha app logs
baha app shell SERVICE
baha app exec SERVICE COMMAND
```

## Backup, Restore und Update

Interaktiv:

```bash
baha app backup
baha app restore ./mailflow-production.bhbackup
```

Automation kann weiterhin explizit `--password-file` verwenden. Restore bleibt fail-closed und meldet READY erst nach erfolgreicher Backend-, Runtime-Identity-, Workload- und HTTP/TLS-Verifikation.

Git-basierte Updates lassen sich vor Mutation pruefen:

```bash
baha app update --check
```

Mutation ist strict fast-forward only. Dirty/Ahead/Diverged schlagen fehl. Fuer dauerhaften BaseHarbor-State muss vor Mutation entweder ein verschluesseltes Recovery erstellt oder `--no-backup` explizit bestaetigt werden.

## Existing/BYOC TLS aktualisieren

```bash
baha app tls update --check
baha app tls update
```

`--check` ist read-only. Mutation prueft Quelle/Key-Pair/FQDN, verweigert Downgrades, installiert geschuetzte Dateien, startet bei Bedarf neu und verifiziert Readiness. ACME-Automation und providerneutraler TLS-Contract bleiben Future Work.

## Standards-First Repository Adoption

Repository-Evidence und portabler Application Intent bleiben getrennt.

BaseHarbor kann konkrete Produkt-Evidence erkennen, normalisiert sie fuer den gefuehrten Adoption-Flow aber auf generische Service-Bedeutung:

```text
PostgreSQL evidence  -> SQL Database
Redis/Valkey evidence -> Cache
MinIO/SeaweedFS/S3 evidence -> Object Storage + S3 compatibility
OpenTelemetry evidence -> Observability + OTLP
```

Das bestehende v0.4-Manifest bleibt bis zur expliziten Contract-Migration kompatibel. Produktnamen in bestehenden v0.4-Feldern sind dabei eine Compatibility Surface und keine neue Architekturgrenze.

Repository-Compose ist immer read-only. Bei gemischten Compose-Dateien trennt die Inspection bekannte ersetzbare Infrastruktur von Application Workload. PostgreSQL-, Redis/Valkey- und unterstuetzte S3-kompatible Services bleiben im Original-Compose unveraendert vorhanden, werden aber fuer den BaseHarbor-managed Pfad nicht in `workload.services` aufgenommen. Infrastrukturartig benannte Services, deren Rolle nicht sicher erkannt werden kann, werden als mehrdeutig markiert: `--quick` bricht fail-closed ab, der interaktive Wizard verlangt eine explizite Zuordnung. `docker compose up` ausserhalb BaseHarbor bleibt dadurch unveraendert moeglich.

`baha app init --quick` uebernimmt nur eindeutige Detected-Evidence. Metrics werden nur automatisch geschrieben, wenn Workload-Service und Container-Port eindeutig ableitbar sind. OTLP wird nur automatisch geschrieben, wenn der Signaltyp eindeutig erkannt wurde. Konkrete Runtime-Operationen erzeugen nur dann Runtime-Permissions, wenn auch deren Workload-Scope eindeutig ist; sonst bricht Quick-Init fail-closed ab.

Application-Logs werden bei erkanntem Workload als opt-in vorgeschlagen und nicht still aktiviert.

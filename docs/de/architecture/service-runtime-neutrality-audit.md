# Prüfung der Dienst- und Laufzeitneutralität

Ausgabe: #618
Elternfreigabe: #516 (v0.4.19)
Kubernetes Architekturnachweis: #616

## Zweck

Dieses Dokument verfolgt die Voreinfrieren-Audit der ausgelieferten BaseHarbor Service/Provider-Grenzen.

Ziel ist es nicht, Kubernetes in v0.4.19 umzusetzen.Ziel ist es, sicherzustellen, dass die vor v0.5 eingefrorenen Verträge später von Kubernetes/OpenShift realisiert werden können, ohne portable Anwendungen, Fähigkeiten, Anbieter, Bindungen, Vertrauen oder Lebenszyklussemantik neu zu entwerfen.

## Prüfungsmatrix

Benötigte gefrierfertige GrenzeBewerten Sie die aktuelle Laufzeitkupplung zum reviewBenötigte Freeze-ready-Grenze
| --- | --- | --- | --- |
.sql .sql . PostgreSQL . RuntimeProvider, ExecProject, Compose Runtime Generation, Netzwerke, Hostpfade, Volumes . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . .
• cache.key-value • Valkey • Compose topology • ExecProject • TLS Gateway • Netzwerkannahmen • Volumen-/Laufzeitzustand • Cache Semantik • Anmeldeinformationen • Isolation und Verifikation unabhängig von der Runtime Topology •
. . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . .
Objekt-Speicherung.s3. SeaweedFS.Compose Provider-Lebenszyklus, Endpunkt/Netzwerk-Adressierung, lokale Zustandspfade, Bucket/credential/Bindung/Verifikation Semantik unabhängig von der Laufzeit.
Belichtung.http-: Caddy-Host-Ports, Docker-Netzwerke/Aliases, Compose-Service-Lebenszyklus. portable route/exposure contract consumable later by Gateway API/platform ingress.
Telemetry.otlp-OTel Collector / externe OTLP-Container-/Netzwerkendpunkte und verwalteter Provider-Lebenszyklus Transportendpunkt/trust/bindende Semantik unabhängig von der Laufzeit
Metriken: Prometheus® scrape/discovery identity und Compose Provider-Lebenszyklus: logische Quellidentität und Anbieterplatzierung unabhängig von Container/Netzwerk-Identität:
Protokolle Lokie Docker/Container Protokollsammlung Annahmen runtime-neutrale Protokollquelle Identität und Sammlung Kontrakt; siehe #480
OTLP-Transport und -Trace-Speicher bleiben deutlich und Laufzeit-neutral
• identity.oidc-Keycloak / extern OIDC-KeycloakRuntime setzt ConfigProject/UpProject/DestroyProject; Endpoint/TLS-Realisierung; • OIDC-Reich/Client/Callback/MFA-Semantik unabhängig vom Compose-Lebenszyklus.
BaseHarbor-Laufzeitdienst , Laufzeit-Broker , Netzwerk-/Service-Entdeckung, host-projiziertes Identitätsmaterial , anwendungsskopierte Brokersemantik, die innerhalb einer Ziel-/Plattform-Grenze realisierbar ist ,
BaseHarbor Runtime service-Runtime-Executor-Runtime-Executor-Runtime-Executor-Runtime-Executor-Runtime-Executor-Runtime-Executor-Runtime-Runtime-Runtime-Service-Runtime-Runtime-Runtime-Service-Runtime-Runtime-Runtime-Runtime-Executor-Runtime-Runtime-Runtime-Runtime-Runtime-Runtime-Runtime-Runtime-Runtime-Runtime-Runtime-Runtime-Runtime-Runtime-Runtime-Runtime-Runtime-Runtime-Runtime-Runtime-Runtime-Runtime-Runtime-Runtime-Runtime-Runtime-Runtime-Runtime-Runtime-Runtime-Runtime-Runtime-Runtime-Runtime-Runtime-Runtime-Runtime-Runtime-Runtime-Runtime-Runssssssssssssssssssssure-Run-Run-Run-Run-Run-Run-Run-Run-Run-Run-Run-Run-Usssssssss-Run-Usssssssssss-Run-Run-Run-Run-Run-Run-Uss-Us-Us-Us-Us-Us-Us-Us-Run-Run-Run-Run-Run-Us-Us-Us-Us-Us-Us-Us-Us-Us-Us-Us-Us-Us-Us-Us-Us-Us-Usssss-Us-Uss-Us-Us-Us-Us-Us-Us-Us-Us-Us-Us

Management-UI-Oberflächen werden separat als Provider/Operator-Oberflächen geprüft. Sie dürfen nicht zu tragbaren Anwendungsfunktionen oder zu laufzeitspezifischen Anwendungsabsichten werden.

## Grenzüberschreitende Kontrollen

Für jeden ausgelieferten Anbieter/Dienstleister:

- tragbare Semantik bleibt Produkt/Laufzeit-neutral;
- die Platzierung ist gegebenenfalls beantragt/geteilt/extern;
- Der Lebenszyklus stellt den Vorflug, die Bereitstellung/Verweis, die Bindung, die Beobachtung, die Verifizierung und die Vernichtung/Freisetzung frei, ohne dass ein Vokabular an der eingefrorenen Grenze zusammengesetzt werden muss;
- verbindliche Identität hängt nicht von Containernamen, Docker-Netzwerken, Hostpfaden oder zukünftigen Kubernetes-Ressourcennamen ab;
- TLS/PKI/trust nimmt nicht an, dass auf der Maschine, die baha betreibt, Zertifikatsmaterial vorhanden ist;
- persistenter Datenbesitz ist provider/capability semantic anstatt Docker-Volume semantic;
- Status/Doktor/Evidenz melden semantische Bereitschaft anstelle von Behälterlebendigkeit;
- die Vernichtung von Anteilen/externen/fremden Eigentümern;
- CLI, JSON und MCP verwenden den gleichen Domain-Lebenszyklus.

## Validierungsregel

Grenzkorrekturen müssen das Verhalten von Docker und Podman beibehalten.

Verwenden Sie fokussierte Tests zuerst, dann gezielte Docker/Podman-Validierung. Die Kubernetes proof Filiale kann temporäre/Adaptor-Realisierungen nur weit genug implementieren, um diese Verträge zu beweisen oder zu verfälschen.

Produktion Kubernetes Unterstützung bleibt v0.7/v0.8 Umfang.


## v0.4.19 Schlussfolgerungen zur Prüfung

Die gefrierrelevante Überprüfung unterscheidet nun portable Semantik explizit von Runtime/Delivery Realisation.

Oberflächenbeschaffenheit Freeze-ready Schlussfolgerungen Beweise
| --- | --- | --- |
PostgreSQL /`database.sql ` SQL Resource Identität, Platzierung und authentifiziert`SELECT 1 ` Die Verifikation ist runtime-neutral; Docker/Podman Projektausführung ist eine Adapterimplementierung.`BackendProbeExecutor` plus reale k3s semantische Sonde
Valkey / . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . .`cache.key-value `• Cache-Identität, Service-Bindungsvertrag und authentifizierte PING/PONG-Semantik sind laufzeitneutral; RESP-Transportrealisierung und lokales TLS-Gateway bleiben Adapterzustand. •` BackendProbeExecutor`plus reale k3s PING/PONG-Proof-
Öffnen Sie Bao /`secrets` Bootstrap, AppRolle, KV, Anwendungs-Scopes und PKI verwenden die fokussierte Executor/Provider-Grenze.
SeetangFS /`object-storage.s3` Der Lifecycle ist hinter der Objekt-Speicher-Erkennung; Laufzeit-Adressierung ist keine tragbare Absicht....Real k3s S3 Lifecycle proof
Caddy /`exposure.http` Die Route-Intention ist unabhängig von Caddy, Host-Ports und Runtime-Netzwerk-Aliasen.
OpenTelemetrie /`telemetry.otlp` Endpoint, Trustmaterial, Client-Identität und HTTP-Client bilden die Realisierungsgrenze.... real mTLS k3s OTLP proof .
Prometheus /`metrics` Logische Metriken Die Quellidentität ist unabhängig von Docker-Aliasen; die Zielentdeckung ist im Besitz der Realisation....Real k3s Prometheus scrape proof
Loki /`logs ` Das portable Format ist`runtime-stream`; syslog/journald/CRI collection ist ein laufzeit-realisierungs-detail.---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------
Tempo /`traces ` Der Lebenszyklus der Spurenspeicherung und die Spurenverifizierung liegen hinter`TempoRealization`; OTLP-Transport bleibt eine separate Fähigkeit. . . . real k3s OTLP protobuf ingest + Tempo Query proof .
Keycloak /`identity.oidc` OIDC-Reich/Client/Callback/Policy-Semantik werden von Provider-Deployment getrennt.
Das Verhalten von HTTP/mTLS/workload-identity ist runtime-neutral. Komponieren Sie Materialisierung, Netzwerke, Mounts und Healthcheck-Bereitstellung bleiben Liefer-Adapter-Zustand.
• runtime-executor • request/resource semantics und SPIFFE-Workload-Identität sind runtime-neutral; die aktuelle Compose-Deployment ist implementation-privat. • runtime-control contract architecture gate • runtime-executor • runtime-executor • runtime-executor • runtime-executor •
Management UIs-Administrationsendpunkte sind Bedienoberflächen und verbrauchen kanonische Workloads Servicebindungen. Kompose/Gateway-Lieferung ist keine Anwendungsabsicht.

### Wichtige Feststellung des Eigentums

Der Drucktest Prometheus Kubernetes stellte in der ersten Beweiserfassung einen Eigentumsfehler auf: Provider-Abgleich nutzte einen anwendungsweiten Zerstörweg und entfernte damit die Applikationsmetrikenquelle zusammen mit Provider-Ressourcen.

Der korrigierte Invariant ist nun explizit:

```text
provider reconcile/destroy
  -> may remove only resources owned by that provider realization
  -> must never remove application workload resources
  -> shared/external resources follow their declared provider ownership
```

Diese Invariante gilt für jede Laufzeit, nicht nur für Kubernetes.

### Klassifizierung der Laufzeitsteuerung

Der Laufzeit-Broker und der Laufzeit-Executor behalten bewusst zwei Ebenen:

```text
frozen semantic/control contract
  HTTP + mTLS + SPIFFE/workload identity + capability operations
                    |
                    v
deployment realization
  Compose/Quadlet today
  Kubernetes/OpenShift later
```

Die Architektur-Konformitäts-Suite scannt die eingefrorene Laufzeit API und Executor Client/Handler und scheitert, wenn Docker, Podman, Compose oder Kubernetes Topologie in diesen Vertrag austritt.

### Klassifizierung Management-UI

Management UIs bleiben Lieferoberflächen für Anbieter/Betreiber. Sie verbrauchen kanonische Service-Bindungsfelder wie Host, Port, Anmeldeinformationen und Trust-Material. Sie sind keine Fähigkeiten, die durch Anwendungscode angefordert werden, und kein Kubernetes/OpenShift-spezifisches UI-Feld wird in portable Anwendungsabsichten eingeführt.

### Validierungsnachweise

Konzentrierte Validierung vor breiter Laufzeitregression abgeschlossen:

```text
Tempo Hosted neutrality R2                 36762950707  PASS
Tempo k3s proof                            36764079128  PASS
Valkey k3s semantic proof                  36764508145  PASS
Runtime-neutrality architecture/conformance 36775413937  PASS
Core identity/runtime-neutrality after rebase 36775419257 PASS
OTLP Hosted neutrality R2                  36752933918  PASS
OTLP k3s proof                             36757633570  PASS
Prometheus Hosted neutrality R2            36759083558  PASS
Prometheus k3s proof R4                    36760436728  PASS
Loki Hosted neutrality R2                  36761005744  PASS
Loki k3s proof                             36761362966  PASS
Docker Core regression (pre-rebase)        36767491690  PASS
Podman destroy adapter focused gate        36777110907  PASS
```

Die Kubernetes-Läufe sind ausschließlich Architekturdruckprüfungen. Sie stellen nicht den für v0.7/v0.8 geplanten Produktionslieferanten Kubernetes dar.

### Ausgangssynchronisation

Der Implementierungszweig v0.4.19 wurde auf den aktuellen `develop` Die Stabilisierung der Ausgangswerte nach v0,4,18 setzte sich parallel fort.

Zur Synchronisationszeit:

```text
develop  75d1a14c256ddaa06b7317536060489d8afba4f9
#619     393d9fd929afafd70dbd5ed97312993b0bbe8378
behind   0
```

Der Keycloak-Konflikt wurde semantisch gelöst:

- Laufzeitneutral `KeycloakRealization` bleibt die Kerngrenze;
- die neuere PostgreSQL Bereitschaft Abhängigkeit von `develop` bleibt erhalten;
- Der Docker/Quadlet-sichere Keycloak HTTPS-Gesundheitscheck wird beibehalten;
- Die Bereitschaftsdiagnostik wird durch die Realisierungsgrenze angebracht, anstatt den Lebenszyklus in den Identitätskern wieder einzuführen.
- aktuelles Targeted-Gate-Routing und die aktuelle v0.4.18 unveränderliche Demo-Revision werden beibehalten.

Die Post-Rebase-Core-Identität/Laufzeitneutralität und Architektur/Konformitäts-Gate sind grün. Docker/Podman Exact-Head Runtime Regressionsnachweise werden separat erfasst, wenn diese runnergebundenen Gates abgeschlossen sind.


### Endgültige Referenz-Laufzeit-Regression

```text
Docker Core regression (post-rebase)        36777265097  PASS
Podman Core regression (post-rebase)        36777185021  PASS
```

Beide Referenzlaufzeiten bestanden den gleichen semantischen Lebenszyklus nach der Grenzkorrektur, einschließlich Erholung, echter Log-Ingestion, echter Spurenverschluckung, Aufräumung und Evidenzgenerierung.

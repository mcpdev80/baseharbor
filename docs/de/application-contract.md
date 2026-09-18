# Anwendungsvertrag

Eine Anwendung beschreibt, welche Backend-Faehigkeiten und Secret-Namen sie benoetigt. BaseHarbor uebernimmt Bereitstellung, Isolation, Readiness und Lebenszyklus. Die Anwendung soll keine OpenBao-AppRole-Namen, internen KV-Pfade, Compose-Netzwerknamen oder proprietaere BaseHarbor-IDs kennen muessen.

## Anwendungsidentitaet und Deployment-Kontext

`app.name` ist die stabile logische Identitaet der Anwendung. `app.environment` beschreibt den Deployment-Kontext und ist **kein** Bestandteil der fachlichen Anwendungsidentitaet.

Dieselbe Anwendung kann deshalb als `dev`, `test`, `staging`, `production` oder kundenspezifische Instanz betrieben werden, ohne als neue Anwendung definiert zu werden. In der aktuellen Compose-Implementierung wird der Environment-Wert fuer Isolation und Runtime-Namen verwendet. Spaetere Provider wie Kubernetes oder OpenShift duerfen dieselben logischen Anforderungen anders realisieren.

Provider-spezifische Details wie Compose-Projektnamen, Netzwerke, Host-Ports, Volumes oder OpenBao-Pfade gehoeren nicht zum portablen Anwendungsvertrag.

## Portabler Contract und Deployment-State

`baseharbor.yaml` ist der repository-eigene portable Desired-State-Vertrag. Compose-spezifische Operator-/Runtime-Inputs werden davon getrennt in geschuetztem BaseHarbor-State gehalten.

Dazu gehoeren in v0.4 beispielsweise:

- Public FQDN des aktuellen Deployments;
- Deployment-TLS-Modus;
- Existing/BYOC-Zertifikatsquelle und normalisierte geschuetzte Zertifikat-/Key-Dateien;
- automatisch ausgewaehlte Host-Port-Fallbacks;
- generierte Compose-Overrides und Runtime-Identity-Material.

Diese Werte duerfen nicht nur deshalb in das portable Manifest wandern, weil Compose sie aktuell benoetigt.

```yaml
version: 1

app:
  name: mailflow
  environment: production

services:
  postgres:
    enabled: true
  redis:
    enabled: true
  secrets:
    enabled: true

secrets:
  required:
    - name: OPENAI_API_KEY
    - name: SMTP_PASSWORD
```

Secret-Werte gehoeren niemals in das Manifest.

## Mehrere Instanzen

Der einfache Fall bleibt eine Instanz namens `default`. Mehrere unabhaengige logische Dienste werden stabil benannt:

```yaml
services:
  postgres:
    instances:
      primary: {}
      analytics: {}
  redis:
    instances:
      cache: {}
      sessions: {}
```

Jede Instanz erhaelt eigene Zugangsdaten, persistenten Zustand und Bindings. Mehrere Instanzen sind **kein HA**. Zukuenftiges HA liegt hinter einem stabilen logischen Dienst-Endpunkt.

## Workload-only Anwendungen

Ein explizit deklarierter repository-eigener Compose-Workload kann auch ohne kuenstliche PostgreSQL- oder Valkey-Abhaengigkeit eine gueltige Anwendung sein. BaseHarbor erfindet fuer diesen Fall keine Backend-Services, Credentials oder Netzwerke.

Ein Manifest ohne verwaltete Capability und ohne expliziten Workload bleibt ungueltig. Managed-Secrets-only bleibt dort ungestuetzt, wo der aktuelle Runtime Broker ein materialisiertes Managed Backend benoetigt.

## Standard-Schnittstellen

BaseHarbor erzeugt normale Verbindungsinformationen wie:

```text
DATABASE_URL=postgresql://...
DATABASE_PRIMARY_URL=postgresql://...
REDIS_URL=redis://...
REDIS_SESSIONS_URL=redis://...
VALKEY_URL=redis://...
```

sowie geschuetzte Datei-Bindings. Ein Anwendungsprozess braucht zur Laufzeit weder `baha`, einen BaseHarbor-Login noch ein BaseHarbor-SDK.

Trusted-local CLI-Komfort wie `baha app psql`, `baha app valkey`, `logs`, `shell` oder `exec` ist optional und keine Runtime-Abhaengigkeit der Anwendung.

## Required Secrets

`baha app apply` und `baha app up` starten den Workload nicht, solange ein deklariertes Required Secret fehlt oder unbrauchbar ist. `show`, `status` und `doctor` zeigen nur Readiness-/Presence-/Usability-Metadaten und niemals Secret-Werte.

Die konkrete Secret-Auslieferung ist Provider-Sache. Compose kann heute Environment-/Datei-Bindings oder Runtime Identity verwenden; spaetere Kubernetes-/OpenShift-Provider koennen native Secret-Projektion oder Workload Identity verwenden, ohne den logischen Secret-Vertrag der Anwendung zu aendern.

## Workload-Readiness

Ein laufender Container ist nicht automatisch READY. BaseHarbor bewertet ausgewaehlte Compose-Services health-aware und prueft konventionelle app-eigene HTTP/HTTPS-Publisher aktiv.

Redirects gelten als erreichbare Exposition, 5xx oder nicht erreichbare Endpunkte als NOT READY. Bei hostname-gebundenem HTTPS wird der lokale Port geprueft, aber Public FQDN als HTTP Host/TLS ServerName verwendet.

## Deployment-TLS ist kein App-Produktvertrag

Die aktuelle Compose-Implementierung unterstuetzt den Existing/BYOC-Zertifikats-Lifecycle fuer das aktuelle Repository-Compose-Deployment. `baha app tls update --check` ist read-only. `baha app tls update` validiert Quelle, Key-Pair und FQDN, verhindert Downgrades, installiert owner-only Dateien, startet bei Bedarf den Workload neu und verifiziert Readiness.

Das macht Zertifikatsverzeichnisse, Caddy-Details oder Compose-TLS-Dateien nicht zu portablen App-Anforderungen. BaseHarbor-gesteuertes ACME, OpenBao-PKI-Issuance, automatische Rotation und providerneutrale TLS-Capabilities bleiben Future Work.

BaseHarbor trennt Infrastruktur-/Secret-Eingaben von normaler Anwendungskonfiguration. Fachliche Einstellungen wie Sprache, Batch-Groesse oder Schwellenwerte bleiben Eigentum der Anwendung.

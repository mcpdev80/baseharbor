# Anwendungsvertrag

Eine Anwendung beschreibt, welche Backend-Fähigkeiten und Secret-Namen sie benötigt. BaseHarbor übernimmt Bereitstellung, Isolation, Readiness und Lebenszyklus. Die Anwendung soll keine OpenBao-AppRole-Namen, internen KV-Pfade, Compose-Netzwerknamen oder proprietäre BaseHarbor-IDs kennen müssen.

## Anwendungsidentität und Deployment-Kontext

`app.name` ist die stabile logische Identität der Anwendung. `app.environment` beschreibt den Deployment-Kontext und ist **kein** Bestandteil der fachlichen Anwendungsidentität.

Dieselbe Anwendung kann deshalb als `dev`, `test`, `staging`, `production` oder kundenspezifische Instanz betrieben werden, ohne als neue Anwendung definiert zu werden. In v0.2.0 nutzt der Compose-Provider den Environment-Wert für Isolation und Runtime-Namen. Spätere Provider wie Kubernetes oder OpenShift dürfen dieselben logischen Anforderungen anders realisieren.

Provider-spezifische Details wie Compose-Projektnamen, Netzwerke, Host-Ports, Volumes oder OpenBao-Pfade gehören nicht zum portablen Anwendungsvertrag.

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

Secret-Werte gehören niemals in das Manifest.

## Mehrere Instanzen

Der einfache Fall bleibt eine Instanz namens `default`. Mehrere unabhängige logische Dienste werden stabil benannt:

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

Jede Instanz erhält eigene Zugangsdaten, persistenten Zustand und Bindings. Mehrere Instanzen sind **kein HA**. Zukünftiges HA liegt hinter einem stabilen logischen Dienst-Endpunkt.

## Standard-Schnittstellen

BaseHarbor erzeugt normale Verbindungsinformationen wie:

```text
DATABASE_URL=postgresql://...
DATABASE_PRIMARY_URL=postgresql://...
REDIS_URL=redis://...
REDIS_SESSIONS_URL=redis://...
VALKEY_URL=redis://...
```

sowie geschützte Datei-Bindings. Ein Anwendungsprozess braucht zur Laufzeit weder `baha`, einen BaseHarbor-Login noch ein BaseHarbor-SDK.

## Required Secrets

`baha app apply` und `baha app up` starten den Workload nicht, solange ein deklariertes Required Secret fehlt oder unbrauchbar ist. `status` und `doctor` zeigen nur Presence-/Usability-Metadaten und niemals Secret-Werte.

Die konkrete Secret-Auslieferung ist Provider-Sache. Compose kann heute Environment-/Datei-Bindings verwenden; spätere Kubernetes-/OpenShift-Provider können native Secret-Projektion oder Workload Identity verwenden, ohne den logischen Secret-Vertrag der Anwendung zu ändern.

BaseHarbor trennt Infrastruktur-/Secret-Eingaben von normaler Anwendungskonfiguration. Fachliche Einstellungen wie Sprache, Batch-Größe oder Schwellenwerte bleiben Eigentum der Anwendung.

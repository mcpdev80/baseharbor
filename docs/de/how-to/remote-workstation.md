# Entwicklungsrechner mit BaseHarbor Core verbinden

Diese Anleitung gilt, wenn Core auf einem anderen Rechner läuft. Der Entwicklungsrechner verbindet sich direkt mit Core. Den Node Connector brauchst du nur für zusätzliche entfernte Runtime-Hosts.

## 1. Core einrichten

Installiere BaseHarbor auf dem Core-Host und führe aus:

```bash
baha up --control-plane-only
baha onboard --init-tenant
```

Schließe die Anmeldung als Operator ab und richte einen erreichbaren, TLS-geschützten Management-Endpunkt ein.

## 2. Entwicklungsrechner verbinden

Installiere `baha` auf dem Entwicklungsrechner und führe aus:

```bash
baha onboard
baha target show
```

Folge dem Dialog, um dich mit dem vorhandenen Core zu verbinden. Gegebenenfalls benötigst du ein vertrauenswürdiges Enrollment-Bundle. Eine bereits geprüfte Verbindung wird wiederverwendet.

## 3. Anwendung starten

Öffne das Anwendungs-Repository und führe aus:

```bash
baha inspect .
baha init
baha plan
baha up
baha status
```

Weitere Optionen und Hinweise zur Wiederherstellung findest du unter [Target-Befehle](../cli/targets.md) und [Sicherheit](../cli/security-trust.md).

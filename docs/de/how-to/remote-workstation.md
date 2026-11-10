# Entwicklungsrechner mit BaseHarbor Core verbinden

BaseHarbor trennt den Management-Core vom Rechner, auf dem Anwendungen entwickelt werden. Core bleibt für Zustand, Sicherheit und Deployments verantwortlich.

## Auf dem Core-Host

BaseHarbor installieren und die Control Plane einrichten:

```bash
baha up --control-plane-only
baha onboard --init-tenant
```

Der erste Operator wird über die geschützte Anmeldung initialisiert. Der Core benötigt einen erreichbaren Management-Endpunkt mit korrekter TLS-Konfiguration.

## Auf dem Entwicklungsrechner

BaseHarbor CLI installieren und die geführte Verbindung beginnen:

```bash
baha onboard
baha target list
baha target show
```

Der Wizard erkennt bestehende Verbindungen, bietet Targets zur Auswahl und verwendet einen bereits geprüften Zugang wieder. Beim Verbinden mit einem vorhandenen Core werden dessen Dienste nicht erneut installiert.

Für ein bereits bekanntes Target kann `baha onboard --target NAME` verwendet werden. Ein Enrollment-Bundle muss aus dem vertrauenswürdigen Core stammen. Die private Geräteidentität bleibt auf dem Entwicklungsrechner.

## Anwendung starten

Sobald das Target verifiziert ist, im Anwendungs-Repository:

```bash
baha inspect .
baha init
baha plan
baha up
baha status
```

Der optionale Node Connector bindet entfernte Docker-/Podman-Runtime-Hosts an. Er wird für die Verbindung vom Entwicklungsrechner zum Core nicht benötigt.

Siehe [Targets](../cli/targets.md), [Sicherheit und Vertrauen](../cli/security-trust.md) und [Erste Schritte](../tutorials/getting-started.md).

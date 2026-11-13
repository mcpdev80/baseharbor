# Remote-Workstation-Enrollment

Den vollständigen lokalen, Remote- und Team-Weg findest du unter [Lokal, remote und gemeinsam entwickeln](local-remote-team-workflow.md). Diese Seite hält die Core-spezifischen Einladungsdetails fest. Workstation-Enrollment ist getrennt von Node-Connector-Enrollment und menschlichem Operator-Login.

## Einladung auf dem Core-Host erstellen

Führe auf dem Rechner aus, der den Core besitzt:

```bash
baha up
baha onboard --invite
```

Der Befehl erstellt eine private Geräte-Einladung zur einmaligen Nutzung. Übertrage sie geschützt und behandle ihren Inhalt vertraulich. Für ein weiteres Gerät einer bestehenden Person verwende `baha workstation new` auf dem Core-Host.

## Auf der Entwickler-Workstation einlösen

```bash
baha onboard --import /path/to/invitation.json
baha use NAME
```

BaseHarbor prüft Core und Einladung, erstellt den privaten Geräteschlüssel lokal und registriert die Verbindung. Die Einladung verleiht keine menschliche Operator-Rolle. Der Verbindungsalias ist vom Core-Target getrennt. Wähle und prüfe das Deployment-Target mit `baha target list` und `baha target show`. [Security- und Trust-Befehle](../cli/security-trust.md) erklären CA-Trust, Widerruf und Offboarding.

Im Application-Repository setzt du den [kanonischen lokalen, Remote- und Team-Workflow](local-remote-team-workflow.md) fort.

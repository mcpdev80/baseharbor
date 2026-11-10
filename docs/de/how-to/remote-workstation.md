# Entwicklungsrechner mit BaseHarbor Core verbinden

Core läuft auf einem Rechner, die Entwicklung auf einem anderen. Der Entwickler-PC verbindet sich direkt mit Core. Dafür ist kein Node Connector nötig.

## Auf der Core-VM

Installiere BaseHarbor und führe aus:

```bash
baha up
baha onboard
```

`baha onboard` erstellt eine kurzzeitig gültige Einladungsdatei, die nur einmal verwendet werden kann. Übertrage sie sicher auf den Entwickler-PC und behandle ihren Inhalt vertraulich.

## Auf dem Entwickler-PC

Installiere die `baha`-CLI und importiere die Einladung:

```bash
baha onboard --import /pfad/zur/einladung.json
```

BaseHarbor prüft die Identität des Core, erstellt den privaten Geräteschlüssel auf dem Entwickler-PC und richtet das Remote-Target ein.

Prüfe anschließend das ausgewählte Target:

```bash
baha target show
```

## Anwendung starten

Öffne das Anwendungs-Repository und führe aus:

```bash
baha inspect .
baha init
baha plan
baha up
baha status
```

Weitere Optionen und Fehlerbehebung findest du unter [Target-Befehle](../cli/targets.md) und [Sicherheit](../cli/security-trust.md).

# Einstieg

Dieser Einstieg bringt eine bestehende Anwendung unter BaseHarbor zum Laufen, ohne dass du Provider-Interna verstehen oder das Manifest manuell bearbeiten musst.

## Empfohlener Entwicklerpfad

```text
optionale Inspektion
      |
      v
baha app init
      |
      v
menschenlesbare Übernahme-Zusammenfassung
      |
      v
baha up
      |
      v
READY
```

Der normale Ablauf erfordert **keine** manuellen YAML-Ergänzungen, keine Compose-Umschreibung, keinen separaten OpenBao-Bootstrap, keine verpflichtende Vorprüfungs-/Anwendungskette und kein Weiterreichen von Geheimnissen über Shell-Pipes.

## 1. Optional: Repository prüfen

```bash
baha app inspect .
```

Die Inspektion ist schreibgeschützt. BaseHarbor zeigt den erkannten Workload, ersetzbare Infrastruktur und die erkannten Fähigkeiten, ohne das Repository zu verändern.

Detaillierte Nachweise:

```bash
baha app inspect . --verbose
```

## 2. Portablen Anwendungsvertrag erstellen

```bash
baha app init
```

BaseHarbor erkennt so viel wie sicher möglich und fragt nur bei Mehrdeutigkeit oder echten Benutzerentscheidungen nach.

Der Wizard kann unter anderem fragen nach:

- der maßgeblichen Workload-Quelle bei mehreren Compose-, Quadlet- oder Kubernetes-Kandidaten;
- Anwendungs-Workload gegenüber ersetzbarer Infrastruktur;
- provider-neutralen Anforderungen an SQL, Cache, Objektspeicher und Observability;
- anwendungseigenen Geheimnissen einschließlich Pflicht-/Optional-Status;
- automatischer Erzeugung, Eingabe beim ersten Anwenden oder späterer Konfiguration;
- Berechtigungen der Runtime-API aus konkreten Quellnachweisen.

Vor dem Schreiben von `baseharbor.yaml` zeigt BaseHarbor eine menschenlesbare Zusammenfassung. Rohes YAML erscheint nur als Zusatzdetail unter `--verbose`.

Deterministische Automatisierung bei eindeutiger Erkennung:

```bash
baha app init --quick
```

`--quick` bricht bei Mehrdeutigkeit der Quelle sicher ab und übernimmt heuristisch erkannte Geheimnis-Kandidaten niemals stillschweigend. Eindeutige Compose-, im Repository gepflegte Quadlet- und rohe Kubernetes-YAML-Quellen können geprüft und übernommen werden, ohne quellenspezifische Namen in den portablen Anwendungsvertrag zu übernehmen.

## 3. Anwendung starten

```bash
baha up
```

Beim ersten Lauf kann BaseHarbor nur Informationen abfragen, die es nicht sicher selbst bestimmen darf, zum Beispiel:

- Annahme oder Anpassung des vorgeschlagenen, Target-bezogenen Pfads für die vom Betreiber verwahrte OpenBao-Wiederherstellungsdatei;
- Wert eines fehlenden verpflichtenden Anwendungsgeheimnisses;
- Bestätigung eines sicheren Ausweichports.

Interaktive Eingabe von Geheimnissen erfolgt ohne Terminal-Echo. Zugangsdaten für Provider und Runtime werden von BaseHarbor verwaltet und nicht vom Entwickler abgefragt.

Derselbe `baha up`-Lauf konvergiert danach weiter bis READY. Nach erfolgreichem OpenBao-Bootstrap speichert BaseHarbor am effektiven Target nur die Pfadreferenz auf die Wiederherstellungsdatei. Spätere `baha up`-Läufe verwenden diese Referenz automatisch, um den gemeinsam genutzten OpenBao-Provider zu entsperren, wenn die Datei vorhanden ist.

## 4. Ergebnis pruefen

```bash
baha status
```

```bash
baha doctor
```

Erfolg bedeutet verifizierte Capability-Bereitschaft, nicht nur einen gestarteten Container.

## Fortgeschrittene und Automation-Befehle

Diese Befehle bleiben verfügbar, gehören aber nicht zum notwendigen Standardablauf:

```bash
baha plan
baha app preflight
baha app apply
baha app secret set APP_SECRET
```

Automatisierung kann weiterhin ausdrücklich `--stdin` verwenden:

```bash
printf '%s' "$APP_SECRET" | baha app secret set APP_SECRET --stdin
```

## Durchgängige Referenzdemo

Das externe Repository `mcpdev80/baseharbor-demo` ist der Release-seitige Nachweis dieses Ablaufs. Seine README beschreibt den vollständigen Test vom unveränderten Repository über `baha app init` und `baha up` bis READY, Neustart und Aufräumen.

Die Vorabprüfung validiert sowohl den geführten Benutzerablauf als auch deterministische Komponentenpfade. Der finale Release verwendet diese unveränderlichen Nachweise wieder, statt dieselbe aufwendige Matrix erneut auszuführen.

Weitere Themen:

- [Architektur](../explanation/architecture.md)
- [Anwendungsvertrag](../explanation/application-contract.md)
- [Provider](../explanation/providers.md)
- [Sicherheit](../explanation/security.md)
- [CLI-Referenz](https://mcpdev80.github.io/baseharbor/reference/cli/)

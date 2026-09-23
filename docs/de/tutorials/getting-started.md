# Einstieg

Dieser Einstieg bringt eine bestehende Anwendung unter BaseHarbor zum Laufen, ohne dass du Provider-Interna verstehen oder das Manifest manuell bearbeiten musst.

## Kanonischer Entwicklerpfad

```text
optionale Inspektion
      |
      v
baha app init
      |
      v
menschenlesbare Adoption-Zusammenfassung
      |
      v
baha up
      |
      v
READY
```

Der normale Happy Path erfordert **keine** manuellen YAML-Ergaenzungen, keine Compose-Umschreibung, keinen separaten OpenBao-Bootstrap, keine verpflichtende Preflight/Apply-Befehlskette und kein Shell-Piping fuer Secrets.

## 1. Optional: Repository pruefen

```bash
baha app inspect .
```

Die Inspektion ist read-only. BaseHarbor zeigt erkannten Workload, ersetzbare Infrastruktur und Capability-Evidence, ohne das Repository zu veraendern.

Detaillierte Evidence:

```bash
baha app inspect . --verbose
```

## 2. Portablen Application Contract erstellen

```bash
baha app init
```

BaseHarbor erkennt so viel wie sicher moeglich und fragt nur bei Mehrdeutigkeit oder echten User-Entscheidungen nach.

Der Wizard kann unter anderem fragen nach:

- der Application-Compose-Datei bei mehreren Kandidaten;
- Application Workload versus ersetzbarer Infrastruktur;
- provider-neutralem SQL-, Cache-, Object-Storage- und Observability-Intent;
- Application-eigenen Secrets inklusive required/optional;
- Generate, Eingabe beim ersten Apply oder spaeterer Konfiguration;
- Runtime-API-Permissions aus konkreter Source-Evidence.

Vor dem Schreiben von `baseharbor.yaml` zeigt BaseHarbor eine menschenlesbare Zusammenfassung. Rohes YAML ist nur Zusatzdetail unter `--verbose`.

Deterministische Automation bei eindeutiger Detection:

```bash
baha app init --quick
```

`--quick` bricht bei Mehrdeutigkeit fail-closed ab und uebernimmt heuristische Secret-Kandidaten niemals stillschweigend.

## 3. Anwendung starten

```bash
baha up
```

Beim ersten Lauf kann BaseHarbor nur Informationen abfragen, die es nicht sicher selbst bestimmen darf, zum Beispiel:

- Pfad fuer die operator-gehaltene OpenBao-Recovery-Datei;
- Wert eines fehlenden required Application Secrets;
- Bestaetigung eines sicheren Port-Fallbacks.

Interaktive Secret-Eingabe erfolgt ohne Terminal-Echo. Provider-/Runtime-Credentials werden von BaseHarbor verwaltet und nicht vom Entwickler abgefragt.

Derselbe `baha up`-Lauf konvergiert danach weiter bis READY.

## 4. Ergebnis pruefen

```bash
baha status
```

```bash
baha doctor
```

Erfolg bedeutet verifizierte Capability-Bereitschaft, nicht nur einen gestarteten Container.

## Fortgeschrittene und Automation-Befehle

Diese Befehle bleiben verfuegbar, gehoeren aber nicht zum notwendigen Basis-Happy-Path:

```bash
baha plan
baha app preflight
baha app apply
baha app secret set APP_SECRET
```

Automation kann weiterhin explizit `--stdin` verwenden:

```bash
printf '%s' "$APP_SECRET" | baha app secret set APP_SECRET --stdin
```

## End-to-End-Referenzdemo

Das externe Repository `mcpdev80/baseharbor-demo` ist der Release-seitige Nachweis dieses Journeys. Seine README beschreibt den kompletten Test vom pristine Repository ueber `baha app init` und `baha up` bis READY, Restart und Cleanup.

Pre-Release validiert sowohl den Guided-Human-Flow als auch deterministische CI-/Komponentenpfade. Der finale Release verwendet diese unveraenderliche Pre-Release-Evidence wieder, statt dieselbe teure Matrix erneut auszufuehren.

Weitere Themen:

- [Architektur](../explanation/architecture.md)
- [Application Contract](../explanation/application-contract.md)
- [Provider](../explanation/providers.md)
- [Security](../explanation/security.md)
- [CLI-Referenz](../cli.md)

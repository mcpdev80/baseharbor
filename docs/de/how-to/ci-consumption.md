# CI-Konsumption einer BaseHarbor-Release

BaseHarbor Core ist eine einzelne `baha`-Binäreplus ein version gekoppeltes
Laufzeitbild. Eine CI-Pipeline konsumiert die **bereids produzierte** Release:
sie pinnt das Bildoby immutable Digest, startet Core, treibt eine Anwendung
durch ihren Lebenszyklus mit ausdrücklichen Eingaben und liestrukturierte,
fail-closed-Ergebnisse zurück. Die Pipeline baut BaseHarbor nicht, baut die
Workload nicht neu und greift keinen Runner, kein Secret und keine Datenbank
an, die BaseHarbor gehört.

Diese Seite ist eine Developer-Integrationsanleitung. Sie ist kein
BaseHarbor-Release- oder Akzeptanzgate. Die Beispiele üben nur die öffentliche
`baha`-Oberfläche auf `develop` und das gepinnte Laufzeitbild aus. Halte
Secrets außerhalb der Pipeline und verknüpfen diese Schritte nicht mit einer
automatischen Auslösung.

## Was „Konsumption" hier bedeutet

Konsumption ist die kopflose Steuerung einer gepinnten Release aus einer
Pipeline:

- Die veröffentlichte Releaseoby immutable Digest pinnt, damit dasselbe
  Artifact in jeder Stufe (Commit → Test → Akzeptanz) läuft. Kein rebuild, kein
  `develop`-edge-Bild.
- Coremit einem ausdrücklichen Target und Umgebungexplizit starten, dabei den
  Operatorpfad `--control-plane-only` nutzen, damit die Pipeline den
  Core-Lebenszyklus OWNt.
- Die Anwendungdurch ihren Lebenszyklusmit ausdrücklichen, nicht-interaktiven
  Eingaben (`--no-input`, `--yes` wo unterstützt) treiben.
- Strukturierte JSON und Exit-Codes vor der Nutzung zurückslesen.

Dasselbe Bild und dieselben Befehle laufen auf einer Entwickler-Workstation.
Konsumption ist nur der nicht-interaktive, maschinenlesbare Gebrauch dieser
Oberfläche.

## Die gepinnte Release-Oberfläche

BaseHarbor veröffentlicht für jede Release `X.Y.Z` (und `0.x`-Hotfix
`X.Y.Z.H`):

- Core-Bild `ghcr.io/mcpdev80/baseharbor-runtime:X.Y.Z` (immutable-Digest-
  Kopplung);
- Laufzeitbild `ghcr.io/mcpdev80/baseharbor-runtime:X.Y.Z`;
- `baha`-Binäre, deren Version zur Release passt.

Siehe [Releases und Versionierung](releases.md) für die vollständige
Kanal-Tabelle und die Kompatibilitätsrichtlinie. Während der `0.x`-Reihe
bleibn Patch-Releases innerhalb der aktiven Minor-Linie rückwärtskompatibel;
pinne einen ausdrücklichen Bereich statt `develop` zu verfolgen.

```text
released Core image  ghcr.io/mcpdev80/baseharbor-runtime:X.Y.Z  (immutable digest)
matching runtime     ghcr.io/mcpdev80/baseharbor-runtime:X.Y.Z
CLI build            baha <X.Y.Z>  (version matches the pinned release)
```

## Nicht-interaktive globale Oberfläche

Jeder Befehl teilt diese globalen Optionen. Automation nutzt sie, nie
decorative menschliche Ausgabe.

| Flag | Bedeutung |
| --- | --- |
| `--no-input` | nie prompted; mit einer handlungsfähigen Fehlermeldung fehl schlagen, wenn eine benötigte Entscheidung fehlt. |
| `--plain` | stabiles, stilfreies, zeilenorientiertes menschliche Ausgabe. |
| `--no-color` | ANSI-Farben deaktivieren. |
| `-q, --quiet` / `--silent` | Fortschritt und nicht-essentielle menschliche Ausgabe unterdrücken. |
| `-o json` / `--output json` / `--json` | das vollständige maschinenlesbare Ergebnis ausgeben. |
| `-v, --verbose` | diagnostische Laufzeit-Details anzeigen. |
| `--version` | die `baha`-Version ausgeben. |

`--no-input` ist die Sicherheitsschwelle für Pipelines: es schlägt fail-closed
mit einer handlungsfähigen Fehlermeldung fehl, wenn eine benötigte
Entscheidung fehlt, statt eine Mutation still zu genehmigen.

## Nur-Lese-Inspektion

Eine Repositoryohne Änderung inspizieren. Dies ist der erste Pipeline-Schritt:
es beweist, dass die Repositoryeine übernebmbare Anwendung ist, und zeigt die
erkannte Intentz vor jeder Änderung an.

```bash
# Human-readable, stable lines
baha --no-input app inspect . --plain

# Complete machine-readable result
baha --no-input app inspect . -o json > inspection.json
```

Prüfe den Exit-Code bevor du die Ausgabe nutzt. Ein nicht-Null-Exit ist eine
Fehlermeldung; behandle sie als Fehler, nicht als Warnung.

## Lebenszyklus mit ausdrücklichen, nicht-interaktiven Eingaben

Der normale Core-Lebenszyklus ist `plan → preflight → apply → status`. In
einer Pipeline jeder Ausdruckliche Eingaben und schlägt fail-closed fehl.

```bash
# Show the plan without changing anything
baha --no-input app plan -o json > plan.json

# Validate before mutation
baha --no-input app preflight

# Converge the application and verify readiness (explicit approval where supported)
baha --no-input app apply --yes

# Read back materialized state and protocol-level readiness
baha --no-input app status -o json > status.json
```

Nur-Lese-Befehle, die Pipelines üblich nutzen:

| Befehl | Zweck | Sicherheit |
| --- | --- | --- |
| `baha status` | Anwendungszustand in einer Repository, sonst Control-Plane-Zustand | Nur-Lese |
| `baha plan` | Den Plan anzeigen | Nur-Lese |
| `baha app inspect` | Repositoryohne Änderung inspizieren | Nur-Lese |
| `baha app preflight` | Validieren vor einer Änderung | Nur-Lese |
| `baha app status` | materialisierter Zustand + Readiness | Nur-Lese |
| `baha app env` | Den Standard-Umgebungs-Contract einer Anwendung zeigen | Nur-Lese |
| `baha app creds` | Verbindungsmetadaten, standardmäßig ohne Credentials | Nur-Lese |
| `baha app evidence` | Secret-safe Lifecycle-/Verifizierungs-Evidenz exportieren | Nur-Lese |
| `baha app logs` | Anwendungslogs anzeigen | Nur-Lese |
| `baha version` | Build-Version | Nur-Lese |

Verbindungs-Bindings und der Laufzeit-Umgebungs-Contract sind stabile,
tragbare Eingaben für nachgelagerte Jobs:

```bash
# dotenv / shell / json / yaml, optionally with revealed secrets (pin exposure)
baha --no-input app env dev --format json

# Connection metadata (no plaintext by default; --reveal only when required)
baha --no-input app creds postgres
```

## Coreexplizit starten (Operatorpfad)

Wenn die Pipeline den Core OWNt (zum Beispiel eine separaten `deploy`-Stufe
oder einen isolierten Runner), starte ihnmit dem ausdrücklichen
Control-Plane-Pfad statt des Repository-Befehls. Dies hält den Core-Lebenszyklus
explizit und getrennt von jeder Anwendungs-Repository.

```bash
# Single-server control plane on the current target
baha --no-input up --control-plane-only --yes

# Explicit HA on a separate fresh target
baha --no-input up --control-plane-only --ha --yes
```

`baha up` ist fail-closed: Validierung und preflight laufen vor jeder
Laufzeit-Mutation. Der Standard-Startup-Plan enthält sieben Services (ein
native PostgreSQL-Server und ein OpenBao-Server auf frischen Targets), keine
versteckte HA-Kluster. Siehe [Core-Workflow-Befehle](core.md) für die
vollständige Befehls-Tabelle und Modi.

## Anwendungs-Secrets einsäen

Anwendungs-Manifeste deklarieren benötigte Secret-Namen, nie Klarworte.
Sähe Secrets aus Dateien oder stdin, dass Werte nie im Pipeline-Log erscheinen.

```bash
# From a file referenced by a pipeline secret (no value in the log)
baha --no-input app secret set API_KEY --file "$BASEHARBOR_API_KEY"

# From stdin (use a masked variable)
printf '%s' "$BASEHARBOR_API_KEY" | baha --no-input app secret set API_KEY --stdin
```

Halte Secrets außerhalb von Beispielen und jeder nachgelagerten Ausgabe. Gib
ein Secret nie als Argument inline aus.

## Ergebnisse lesen: JSON, Exit-Codes, Timeout

Automation muss auf strukturierte Ausgabe und Exit-Codes zählen, nie auf das
Parcen menschlichen Texts.

- **Strukturierte Ausgabe:** parse `-o json`-Dokumente; sie sind die einzige
  Wahrheitsquelle. Die menschliche Ausgabe ändert sich, ohne Machine-Clients zu
  brechen.
- **Exit-Codes:** ein nicht-Null-Exit ist eine Fehlermeldung. Prüfe sie bevor
  du die nutzt. `--no-input` schlägt fail-closed fehl, wenn eine Entscheidung
  fehlt.
- **Timeouts:** umgib lang laufende Befehle mit der Pipeline-Timeout, damit ein
  hanged Readiness-Check schlägt statt die Run hängen zu lassen.

```bash
# Guard a readiness check with a timeout and an explicit exit-code check
if timeout 120 baha --no-input app status -o json > status.json; then
  # parse status.json for readiness
  :
else
  code=$?
  echo "baha status failed (exit $code); run 'baha app doctor'" >&2
  exit "$code"
fi
```

## Secrets, Scope und erhaltene Grenzen

- Halte Secrets außerhalb von Beispielen und jeder committeden Pipeline.
  Referenzie sie als maskierte Pipeline-Variablen oder Dateien, nie inline.
- Löse keine automatischen Releases, Pushes oder Service-Installationen aus
  einem Beispiel aus. Diese Schritte treiben eine reale Umgebung; halte sie
  manuell oder gegated.
- Baue BaseHarbor oder die Workload im Beispiel nicht nach. Konsumiere das
  gepinnte Release-Artifact; dasselbe Artifact läuft über alle Stufen.
- Erhalte die native Delivery-/Stage-Standards-Grenze und die unterstützten
  Provider, die in [Provider-Modell](../explanation/providers.md) und
  [Provider-Befehle](../cli/providers.md) dokumentiert sind. Provider-Beispiele
  sind illustrativ; erfinde keine unterstützten Provider.

## Beispiele und Fixtures

Konkrete kopierfertige Workflows liegen unter:

- [GitHub Actions Beispiel](../example/github-actions/README.md) — Workflow + Fixture.
- [GitLab CI Beispiel](../example/gitlab-ci/README.md) — Pipeline + Fixture.

Befolge folgen demselben Konsumptions-Muster und sind reine Dokumentation: sie
illustrieren die akzeptierte `baha`-Oberfläche, ändern nichts an BaseHarbor und
keine Secrets und keine automatische Auslösung.

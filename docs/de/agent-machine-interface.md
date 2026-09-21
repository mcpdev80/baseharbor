# Agent-native Machine Interface

BaseHarbor v0.4.12 stellt eine kleine, versionierte Machine-Schnittstelle fuer Coding Agents und Automation bereit, ohne einen zweiten Lifecycle- oder Policy-Pfad einzufuehren.

Human CLI, strukturiertes JSON, TUI und MCP verwenden dieselben semantischen Operationen.

## Machine Contract

Der erste oeffentliche Machine Contract ist:

```text
baseharbor.machine/v1
```

Machine-Ergebnisse enthalten:

```json
{
  "contract_version": "v1"
}
```

Diese Version beschreibt den semantischen BaseHarbor-Result-Contract und ist unabhaengig von der MCP-Wire-Protokollversion.

Strukturierte Fehler verwenden stabile Kategorien fuer Validation, Policy-Denial, Konflikte, unklare Ownership, Provider-/Runtime-Verfuegbarkeit, Timeout, Verification und nicht unterstuetzte Operationen. Automation muss keine Terminal-Prosa parsen.

## Operationen entdecken

```bash
baha agent describe
baha agent describe -o json
```

Die Discovery zeigt BaseHarbor-Version, Machine-Contract-Version, semantische Operationen, Safety-Klasse, Confirmation-/Policy-Metadaten, Capability Specifications sowie MCP-Transport, Protokoll und Tool-Namen.

v0.4.12 startet bewusst klein:

| Operation | Safety | Zweck |
| --- | --- | --- |
| `inspect` | read-only | Repository-Evidence und Capability-Findings |
| `plan` | read-only | deterministischer Desired-State-Plan |
| `status` | read-only | Runtime-/Readiness-Zustand |
| `doctor` | read-only | diagnostische Verifikation |

## Strukturiertes JSON

```bash
baha app inspect . -o json
baha plan -o json
baha status -o json
baha doctor -o json
```

Human- und Machine-Ausgabe verwenden dieselben typisierten Result-Pfade. JSON ist ein stabiler Machine Contract und keine Serialisierung von Terminaltabellen oder ANSI-Ausgabe.

Schlaegt ein JSON-Aufruf vor einem normalen Result fehl, wird auf stderr ein versionierter strukturierter Fehler-Envelope ausgegeben.

## MCP

```bash
baha mcp serve
```

v0.4.12 verwendet das offizielle Model Context Protocol Go SDK v1.8.0 und zielt auf MCP `2026-07-28`. Die vom SDK ausgehandelte Kompatibilitaet mit `2025-11-25` bleibt fuer aeltere Clients erhalten.

Der erste Transport ist ausschliesslich **stdio**. BaseHarbor oeffnet keinen Netzwerk-Port, startet keinen Hintergrund-Daemon und fuehrt in v0.4.12 keine Remote-MCP-Authentifizierung ein.

Exponiert werden exakt vier semantische Tools:

```text
baseharbor.inspect
baseharbor.plan
baseharbor.status
baseharbor.doctor
```

Alle vier sind explizit read-only annotiert. `inspect` darf eine explizit uebergebene Remote-Git-Quelle lesen und ist deshalb als open-world markiert; die anderen Tools arbeiten im lokalen BaseHarbor-Kontext.

### Keine generische Ausfuehrung

Nicht exponiert werden generische Primitiven wie:

```text
exec(command)
shell(command)
docker(command)
compose(command)
```

Agents verwenden BaseHarbor-Semantik und erhalten keinen Bypass um Ownership, Isolation, Bindings, Validation oder Verification.

## Security-Grenzen

- keine Plaintext-Secrets oder Credentials in Machine Responses;
- keine provider-globalen Credentials;
- kein Runtime-Socket-Zugriff durch MCP;
- keine beliebige Command-Ausfuehrung;
- kein versteckter Confirmation-/Policy-Bypass;
- kein Remote-Listener in v0.4.12;
- Runtime-/Provider-Details werden nicht zu portablem Application Intent.

MCP Tool Annotations sind Discovery-Hinweise, nicht die Security-Grenze. Sicherheit entsteht durch die kleine explizite Tool-Oberflaeche und denselben BaseHarbor-Core wie bei den anderen Control Surfaces.

## Repository Guidance

`baha app init --agents` pflegt weiterhin nur den begrenzten BaseHarbor-Abschnitt in `AGENTS.md`. v0.4.12 weist Coding Agents zusaetzlich auf strukturierte Interfaces, `baha agent describe -o json` und `baha mcp serve` hin und verbietet Bypaesse ueber Shell/Docker/Compose.

Fremde Repository-Instruktionen bleiben erhalten; kaputte oder mehrdeutige Managed Marker brechen weiterhin fail-closed ab.

## Bewusst spaeter

Nicht Teil von v0.4.12 sind Remote/HTTP-MCP, MCP-OAuth/OIDC/RBAC, breite mutierende/destruktive MCP-Tools, eingebettete LLM-Logik, vendor-spezifische Agent-Integrationen, Application-MCP als portable Capability sowie Kubernetes/OpenShift-Runtime-Verhalten.

# Machine Interface v1

## Gültigkeitsbereich

CLI-JSON und MCP machen denselben zugrunde liegenden semantischen BaseHarbor-Zustand zugänglich.

Die Maschinenschnittstelle ist ein Adapter über den gemeinsamen BaseHarbor-Lifecycle. Sie stellt keinen zweiten Orchestrierungspfad dar.

## Lifecycle-Oberfläche

Der semantische Operationssatz von v1 umfasst den derzeit unterstützten Application-Lifecycle:

```text
inspect
plan
apply
status
doctor
observe
evidence
update
repair
backup
restore
destroy
policy.check
policy.explain
```

`apply` steht für die semantische Konvergenz. MCP bildet nicht jeden menschenlesbaren CLI-Alias oder Darstellungsbefehl ab.

## Sicherheitsklassen

Jede Maschinenoperation deklariert genau eine Klasse:

- `read_only`: ausschließlich lesend;
- `mutating`: zustandsverändernd;
- `destructive`: destruktiv.

Destruktive Operationen deklarieren, ob eine ausdrückliche Freigabe erforderlich ist. `destroy` verlangt vor jeder Änderung eine explizite Zustimmung.

Unbekannte oder nicht aufgelöste Sicherheitsanforderungen werden abgelehnt (*fail closed*).

## Autorisierung von Maschinenoperatoren

Jede Maschinenoperation wird unterhalb der Darstellungsschicht durch ein transportneutrales Entscheidungsmodell autorisiert.

Die Entscheidung verknüpft:

```text
wirksamer Akteur / Principal
        +
Operation und Sicherheitsklasse
        +
Application-/Environment-/Target-/Workspace-Kontext
        ↓
erlauben / ablehnen
```

In Entwicklungsumgebungen gelten ausdrücklich `trusted-local`-Akteursemantiken. Verwaltete Umgebungen verweigern den Zugriff, wenn die erforderliche authentifizierte Operator-Identität fehlt oder ungültig ist. Die Entscheidung ist secret-sicher und gibt nur stabile Identitäts- und Herkunftsfelder aus, niemals Tokens oder private Zugangsdaten.

MCP, CLI/JSON und zukünftige HTTP-/Console-Adapter müssen dieselbe Autorisierungsgrenze verwenden. Kein Adapter darf ein unabhängiges RBAC-Modell definieren.

## Nicht interaktives Verhalten

Maschinenoperationen hängen niemals von einem interaktiven Terminal ab.

Wenn eine Lifecycle-Aktion ungeklärte Eingaben eines Entwicklers oder Operators benötigt, wird ein strukturierter, handlungsfähiger Fehler zurückgegeben. Es wird weder eine Rückfrage gestellt noch ein Wert erraten. Beispiele:

- fehlende erforderliche Application-Secrets;
- ausdrückliche Recovery-Entscheidung vor der Änderung dauerhafter Daten;
- Freigabe einer destruktiven Operation;
- nicht unterstützte oder mehrdeutige Eigentümerschaft;
- Richtlinienverweigerung.

## Secret-Verarbeitung

Passwörter, Tokens, private Schlüssel und Secret-Werte DÜRFEN NICHT über gewöhnliche Maschinen-Ergebnisfelder angenommen oder ausgegeben werden.

Backup und Restore verwenden eine lokale, nur für den Eigentümer lesbare Passwortdatei statt eines Klartextpassworts.

URLs mit eingebetteten Zugangsdaten DÜRFEN NICHT als normale Maschinen-Ausgabe erscheinen.

## Anforderungen

- Maschinen-Ergebnisse MÜSSEN versioniert sein.
- Fehler MÜSSEN strukturiert und secret-sicher sein.
- CLI, JSON und MCP MÜSSEN dieselbe Lifecycle-Semantik verwenden.
- Schnittstellen DÜRFEN Autorisierung, Policy, Ownership, Preflight, Reconciliation oder Verifikation NICHT umgehen.
- In verwalteten Umgebungen MÜSSEN Operationen ohne die erforderliche authentifizierte Operator-Identität abgelehnt werden.
- Mutationen MÜSSEN den Ablauf `plan -> preflight -> apply -> verify` einhalten.
- Destruktive Operationen MÜSSEN ausdrückliche Sicherheits- und Freigaberegeln bewahren.
- MCP DARF KEINE generische Shell oder direkte Docker-/Compose-/Podman-/Provider-Ausführung als Ersatz für BaseHarbor-Lifecycle-Operationen anbieten.
- Details der Runtime-Provider-Implementierung DÜRFEN NICHT zur MCP-Semantik werden.
- Nicht unterstützte Lifecycle-Operationen MÜSSEN ein explizites typisiertes Ergebnis liefern.
- Docker/Compose und Podman/Quadlet MÜSSEN dort dieselben agentenfähigen semantischen Operationen anbieten, wo die Lifecycle-Operation unterstützt wird.

Die typisierte Registry unter `internal/machine` ist die maßgebliche Definition von Maschinenoperationen und ihren Sicherheitsklassen.

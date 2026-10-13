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
effective actor/principal
        +
operation + safety class
        +
application/environment/target/workspace context
        ↓
allow / deny
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

## Zustimmung zur Core- und Installationszerstörung

`control-plane.destroy` und `installation.destroy` akzeptieren
`provider_change_consent` zusätzlich zum üblichen `approval`. Die Ausführung
erfordert beide Stufen für den aktuellen gemeinsamen Scope und unabhängig davon
`delete_data=true`. `--yes` oder `approval=true` allein autorisieren keine
Zerstörung. CLI-Vorschau und typisierte Ablehnung zeigen den aktuellen konkreten
Scope; JSON-Berichte enthalten `provider_change_plan`. Geänderte Konfigurationsbytes,
Verbraucherregistrierungen, geschützte Core-Owner-Bytes, Target-Zugehörigkeit oder
native Ressourceninventare machen frühere Zustimmung vor Beginn des Abbaus
ungültig. Ungebundene App-Provider und geschützte Management-Verbraucher benötigen
eine eigene Autorisierung.

Diese Vorbereitung implementiert noch keine bewusst gewählte Core-Zerstörung bei
aktiven Verbrauchern oder vollständige Providerübergaben. Die aktive-Verbraucher-
Verweigerung, universelle Ausführung der Folgen, Wiederaufnahme und native destruktive
Abnahme bleiben notwendige v0.4.25-Integrationsarbeit. Unbekannte Folgen bleiben
teilweise verifiziert.

## Application-Stop über HTTP

Die Core-Discovery enthält jetzt die vorhandene Operation `app.stop`. Der
HTTP-Adapter nutzt dieselbe ausgewählte Deployment-Bindung, denselben
Provider-Folgenscope, unabhängige planbezogene Bestätigungen,
Eigentümerrevalidierung und tatsächliches Stop-Ergebnis wie CLI/JSON/MCP.
Die Eingabe akzeptiert `provider_change_consent`; gewöhnliche Zustimmung
ersetzt keine der erforderlichen Stufen. Eine Console bestätigt den äußeren
Plan `baseharbor.provider-impact-scope/v1` für dieses ausgewählte Deployment
statt eines Blattplans. Fremde Selektoren und mehrdeutige Bestätigungsobjekte
werden vor der Runtime-Operation abgewiesen. Erhaltene Eigentümer beweisen
keine weiterlaufenden Verbraucherfunktionen; ungeklärte Folgen bleiben teilweise.

Transport-Quellprüfungen ersetzen weder authentifizierte Browser- noch native
Runtime-Prüfungen am passenden Quellstand; die finale v0.4.25-Ökosystem-Abnahme
ist weiter offen.

### Ausgewählter Core-Stop

`control-plane.stop` akzeptiert `target` und optional `provider_change_consent`
mit derselben aktuellen Bereichsplan-ID und unabhängigen Bestätigungsstufen wie
andere Provider-Änderungen. Allgemeine Freigabe und `--yes` ersetzen diese nicht.
Das lokale `baha down --json` im Core-Modus und die MCP-Operation nutzen dieselbe
PostgreSQL-/OpenBao-Stop-Autorität und das typisierte Ergebnis: `target`, `state`,
`runtime` und `provider_change_result`. Andere Provider, Workloads und Verbindungen
bleiben angefordert und laufend. Die native Prüfung kontrolliert ausgewählte
Prozesszustände, Erhalt dauerhafter Ressourcen und Nachbardienste im gemeinsamen
Projekt. Der bestehende native Adapter kann ausgewählte gestoppte Container
entfernen; das Ergebnis inventarisiert den tatsächlichen Abbau und Erhalt, ohne
eine Datenlöschung zu behaupten. Der aktuelle Plan bindet beobachtete native
Prozessidentitäten/-zustände und interne Image-Identitätsrevisionen sowie
geschützte Dateiinhalte; Neustart oder Image-Änderung macht frühere Zustimmung ungültig.
Ein gestoppter Runtime-Dienst belegt keine vollständige Verbraucher-/Secrets-/
PKI-/Session-Kontinuität; der Folgennachweis bleibt bei ungeklärten Folgen teilweise.
Dieser Pfad behauptet keinen HTTP-Core-Stop-Adapter oder authentifizierten Browser-Test.

# Geschützte Maschinen-HTTP-Schnittstelle v1

## Anwendungsbereich

Diese Spezifikation definiert die authentifizierte HTTPS-Projektion der BaseHarbor Machine Interface, die von Console und anderen webfähigen Maschinenclients verwendet wird.

Das Schema des öffentlichen Umschlags ist
`contracts/machine/v1/control.schema.json `. seine` discovery `, ` execute_request`,
`execution `, ` event `und` error_result`Definitionen beschreiben den serialisierten Kern
Aufzeichnungen.`control.golden.json` enthält explizit synthetische Beispiele generiert
von Core-Typen und -Registern von
`go run ./scripts/tools/machine-control-fixtures`. Verbraucher pin beide Dateien zu einem
unveränderliche öffentliche Core Commit und behalten ihre SHA-256 Digests.
Ersetzt nicht die operationsspezifische Validierung, Authentifizierung oder Richtlinie.

Die HTTP-Schnittstelle ist ein Transport über die gleichen BaseHarbor semantischen Operationen, die von CLI, JSON und MCP verwendet werden. Es MUSS NICHT einen zweiten Lebenszyklus, gewünschten Zustand Store, Autorisierung Modell oder Laufzeit Abstraktion einführen.

## Sicherheitsgrenze

Alle Maschinen-HTTP-Endpunkte:

- MUSS nur vom BaseHarbor TLS-Verwaltungshörer bedient werden;
- MÜSSEN Klartext-HTTP ohne Downgrade-Fallback ablehnen;
- MUSS den bestehenden authentifizierten Operator HTTP Middleware passieren;
- MUSS den verifizierten Transport-Principal in die geteilte Maschinenbediener-Genehmigungsgrenze binden;
- MUSS die Betriebssicherheit, die Politik und die Semantik der Bestätigung wahren;
- MUSS normale Antworten, Ausführung Metadaten und Ereignis Metadaten geheim-sicher halten.

Browser-Clients verwenden Speicher-nur Träger-Authentifizierung gegen die konfigurierte
gleich-Ursprung HTTPS Core-Endpunkt. Origin-tragende Anfragen aus einem anderen
Autorität wird abgelehnt; Core authentifiziert keine Browser-Cookies. Konsole
Transporte müssen Fremde entdeckt Destinationen und Umleitungen vor ablehnen
SSE verwendet daher authentifiziertes Get-Streaming,
nicht eine Cookie-only EventSource Annahme.

Jeder zugelassene Event/Log/Exec-Stream endet am früheren Tag des verifizierten Token-Auslaufs
und fünf Minuten. Provider Streams schließen auf Anfrage Stornierung / Deadline; Socket
Schreiben sind auf 30 Sekunden begrenzt, an die Sitzungsfrist geklammert.
erfordert erneut Authentifizierung und Autorisierung. Dies verspricht nicht sofort
Widerruf von Änderungen der Identitätsanbieter-Gruppe innerhalb der Lebenszeit eines ausgestellten Tokens.

Transport-Authentifizierung und Betreiber-Zulassung sind separate Anforderungen. Eine gültige TLS-Verbindung genehmigt von sich aus keine Operation.

## Topologie der Konsoleninstallation

Die Standardeinstellung ist eine optionale Konsole, die direkt mit einem ausgewählten Core in verbunden ist.
eine BaseHarbor Installation und Sicherheitsgrenze. Serve Konsole und geschützt
Core HTTP auf dem gleichen HTTPS-Ursprung. Core bleibt die Autorität für den Lebenszyklus,
Politik, Identitäten und Geheimnisse. Die Console besitzt keine parallele Autorität oder
Installationszustand. Core überprüft Browser Origin mit dem Ziel HTTPS
Behörde, einschließlich ihres Ports; Speditions-Header können keine ausländische
Origin und doppelte Origin Header werden abgelehnt.

Die v0.4.23 Konsole weist einen konfigurierten fremden Core vor Authentifizierung und
Pins alle entdeckten Destinationen zum ausgewählten Core. Es liefert keine
zentrales Multi-Core-Backend, Dev-to-Prod-Weiterleitungs- oder Installations-Verband.
Für den grenzüberschreitenden Ursprungsverkehr ist eine gesonderte, ausdrückliche vertrauenswürdige Ursprungsbezeichnung erforderlich.
ist nicht durch CORS-Header oder Weiterleitungen in dieser Version aktiviert.
Installationsauswahl muss direkt zu jedem neu ausgewählten Core authentifizieren
und die Anmeldeinformationen und Streams der vorherigen Installation wegwerfen.

## Entdeckung

`GET /api/v1/machine/discovery`

Gibt den aktuellen Maschinenvertrag, Ausführungsvertrag, semantische Operationen und ausgehandelte HTTP-Funktionen zurück.

Clients MUSS Operationen entdecken, anstatt die Unterstützung aus der CLI-Buchstabe zu entnehmen.

HTTP Discovery listet nur die Operationen des ausgewählten HTTP auf
executor. Es nimmt ihre Deskriptoren und Sicherheit Metadaten aus dem gemeinsamen Core
registry. Die gleiche Support-Registry wählt Ausführung Handler und lehnt
unimplementierte Operationen vor dem Erstellen einer Ausführung. Eine semantische Operation
über eine andere Projektion zur Verfügung steht nicht impliziert HTTP-Unterstützung.

HTTP discovery gibt auch eine `http` Karte der benannten Endpunktbindungen, jeweils mit
`href `, ` method `und optional` protocol`. Die gleiche Core Registry installiert die
führt und produziert diese Deskriptoren. Browser bootstrap nur die dokumentierte
discovery endpoint, dann verwenden Sie diese Bindungen für die Ausführung und Streams.
`{execution_id}` und `{stream_id}` Platzhalter akzeptieren validierte Ressourcen-IDs.
Jedes aufgelöste Ziel bleibt an den konfigurierten HTTPS Core-Ursprung gebunden;
Die Entdeckung erlaubt niemals eine Umleitung, eine beglaubigte URL oder eine fremde Herkunft.

## Ausführung

`POST /api/v1/machine/executions`

Antrag:

```json
{
  "operation_id": "status",
  "context": {
    "application": "demo",
    "environment": "prod",
    "target": "prod-eu"
  },
  "input": {}
}
```

`context.environment` ist obligatorisch. Kontext-Selektoren sind der Autorisierungsbereich. Betriebseingabe MÜSSEN eine explizit autorisierte Anwendung, Umgebung, Ziel, Workspace oder Laufzeitressource NICHT überschreiben.

Akzeptierte Operationen geben HTTP zurück `202` mit einem `Execution v1` Ressourcen und a `Location` header.

Das Ausführungsmodell zeigt:

- stabil `execution_id`;
- semantisch `operation_id`;
- eine stabile Bezugsgröße für die Akteure;
- Rahmen für den begrenzten Betrieb;
- `pending | running | succeeded | failed | cancelled`;
- strukturierte Fortschritte;
- strukturiertes Ergebnis oder eingegebener Fehler;
- Start-/Endzeitstempel.

Die Eingabe der Operation wird nicht in normalen Metadaten der Ausführung beibehalten.
Executor weist unbekannte Felder zurück, dupliziert Schlüssel in jeder Tiefe, nicht Objekteingabe,
ungültige UTF-8, Verschachtelung über 64 Levels und hintere JSON-Werte ohne
Einschließlich eingereichter Feldnamen oder Werte in seinem Validierungsfehler.

## Ausführungsstatus und Ereignisse

`GET /api/v1/machine/executions/{execution_id}`

Gibt die strukturierte Ausführungsressource zurück.

`GET /api/v1/machine/executions/{execution_id}/events`

Gibt semantische Server-Sent-Events mit dem Machine Event v1 Hüllkurve zurück.

Die Aufnahmeköpfe werden sofort gespült. Alle 15 Sekunden kann der Stream emittieren
eine SSE-Stellungnahme (`: keepalive`), um den untätigen HTTPS-Transport während eines langen
Anbieter Betrieb. Kommentare sind nicht Maschinenereignisse: sie haben keine Reihenfolge,
Fortschritt oder Ergebnis und MUSS NICHT die Fortsetzung der Anwendung auslösen.
Verlängerung der Beobachtungsfrist für fünf Minuten/Zeugnen oder Wiederholung.

Das ursprüngliche Ereignisvokabular umfasst:

- `operation.started`;
- `operation.progress`;
- `operation.succeeded`;
- `operation.failed`;
- `operation.cancelled`;
- Anwendung/Provider/Ziel-/Laufzeit-Ressourcenzustandsereignisse für kompatible zukünftige Produzenten.

Ausführende Metadaten und Ereignisströme sind an den authentifizierten Ausführungsakteur gebunden. Ein authentifizierter Operator MÜSSEN den Ausführungsstream eines anderen Operators NICHT nur durch Kenntnis der Ausführungskennung lesen.

## Navigationsoperationen

Die semantische Maschinenregistrierung umfasst strukturierte Navigationsvorgänge, die von Console benötigt werden.

`app.list` es handelt sich um target-scoped und liefert geheim-sichere Bereitstellungszusammenfassungen, die stabile Anwendungs-/Deployment-Identitäten, Ziel/Umgebung, Laufzeitanbieter, beobachteten Zustand/Zustand und Quellverfügbarkeit enthalten.

`target.list` gibt geheime Target-Zusammenfassungen zurück, die Laufzeitanbieter, Zugriffsreferenz, Umfang und Selektorstatus enthalten.

Anbieter Listing/Inspektion und Organisation Inspektion nutzen ihre bestehenden Maschinenoperationen.`workspace.list `, ` workspace.resolve `und` workspace.status`Bereitstellung der strukturierten Arbeitsraum-Navigationsfläche.

Dies sind semantische Maschinenoperationen und bleiben daher konsequent für MCP und HTTP verfügbar.

## Laufzeitprotokolle

`POST /api/v1/machine/streams/logs`

Die Anfrage verwendet Stream Contract v1 und MUSS identifizieren:

- gegebenenfalls Anwendung/Umwelt/Zielkontext;
- Laufzeit `resource_kind`;
- stabile Laufzeit `resource_id`;
- begrenzte historische Auswahl wie z.B.`since ` or ` tail`sofern unterstützt.

Die Anfrage wird über die Freigabegrenze für die Autorisierung von Maschinenbedienern autorisiert, bevor ein Runtime Explorer-Adapter aufgerufen wird.

Wenn die aktive Laufzeit-Implementierung kein Protokoll-Streaming annonciert, gibt BaseHarbor getippt zurück `unsupported_operation`; es MUSS NICHT auf Runtime-native nicht authentifizierte Ports oder Shell-Befehle zurückfallen.

Stream-Antwort-Metadaten identifizieren den Stream, Schauspieler, Target und Laufzeit-Ressource ohne Einbettung von Anmeldeinformationen.

Erfolgreiche Zulassung spült die Response-Header auch dann, wenn der Produzent im Leerlauf ist.
Jeder Ausgabeblock wird gespült, ohne auf EOF zu warten. Kundenstornierung und
credential Ablauf beenden Sie die Beobachtung; ein folgender Client MUSS NICHT still
Replay der Anfrage oder behandeln Transport EOF als erfolgreiche Anwendungsausführung.

## Laufzeit exec / Terminal-Grenze

`POST /api/v1/machine/streams/exec`

Exec verwendet dieselbe authentifizierte TLS- und Autorisierungsgrenze und erfordert:

- explizite Laufzeit-Ressourcenart/-id;
- ein explizit gebundener Befehl;
- eine Runtime Explorer-Fähigkeit, die exec für diese Ressource erlaubt.

Container/pod exec bedeutet keinen Host-Shell-Zugriff.

Die v1-Grenze wird absichtlich aktiviert, bis eine Runtime Explorer-Implementierung einen explizit kompatiblen Session-Transport liefert. BaseHarbor MUSS zurückgetippt `unsupported_operation` anstatt eine generische Schale oder CLI-Passthrough zu erfinden.

## Lebenszyklusparität

Der HTTP Semantic Executor verwendet direkt die bestehenden BaseHarbor Core-Funktionen und Ergebnismodelle für Anwendungs-, Workspace-, Provider-, Organisations- und Lebenszyklusoperationen.

Die Umsetzung MUSS NICHT:

- aufrufen `baha` als Teilprozess;
- Parse Human CLI-Ausgang;
- Umgehung der gemeinsamen Genehmigung/Politik/Eigentümer/Vorflug/Reconciliation/Überprüfung;
- Console-spezifischer Wunschzustand einführen;
- die Eingabe des Betriebs zu ermöglichen, um seinem autorisierten Kontext zu entfliehen.

## Verhältnis zu Runtime Explorer und Target Access

Runtime-Resource Discovery und konkrete Log/Exec-Implementierungen werden durch den providerneutralen Runtime Explorer-Vertrag geliefert.

Remote-Ausführung oder -Beobachtung erreicht ein Target nur über die von Core ausgewählte Target Access Provider-Grenze. Konsolen- und HTTP-Clients MÜSSEN sich NICHT direkt mit einer Target Access-Implementierung verbinden.

## Interaktive Container-Terminal

`POST /api/v1/machine/terminals` gibt ein begrenzter Terminal und liefert ein
`StreamDescriptor `. Die Anfrage verwendet` StreamRequest `mit` tty: true`, ausdrücklich
Containerart/stabile ID, Umgebung, Target, argv und `rows`/` columns` in 1–512.
Der ausgewählte Runtime Explorer überprüft Ressourcenbesitz und -umgebung vor
Aufruf der eingegebenen Laufzeit Terminal primitiv. Eine Plattform oder verwaltete Ressource ist
benötigt. Lokale Linux Docker/Podman-Backends liefern ein PTY; Remote-Terminal
Qualifikation bleibt Teil der Anforderungen an die Fernintegration.

Der verifizierte Emittent/Subjekt des Erstellers besitzt die Sitzung, einschließlich `dev`.

`terminal.ready` bestätigt die Aufnahme des Transports, nicht die Bereitschaft der angeforderten
Programm. Clients fügen Eingabe-und Terminal-Protokoll Antworten vor der Rendering
Ausgangsausgabe. Der Transport bewahrt Eingabebytes; die Containerklemme
besitzt Line-Editing und Signal-Interpretation.
Jede Kontrollanfrage erfordert die Authentifizierung des Inhabers und die Weiterverwendung des freigegebenen
Operator-Grenze. Es gibt kein Terminal-Cookie, Abfrage-Token, Host-Command API oder
Browser-zu-Runtime-Verbindung.

Endpunkt, Endpunkt, Semantik,
| --- | --- |
| `GET /api/v1/machine/terminals/{stream_id}/events ` Eine SSE-Ausgangseinrichtung;`terminal.ready `, Basis64` terminal.output `, ` terminal.exit`mit Ausstiegscode. . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . .
| `POST /api/v1/machine/terminals/{stream_id}/input ` | A ` TerminalInput`Rahmen mit Basis64 Bytes oder Größenänderung Abmessungen.
| `DELETE /api/v1/machine/terminals/{stream_id}` Schließen Sie den Transport und seine Laufzeit CLI Prozess.

Die Eingabesequenz beginnt bei einem und muss genau inkrementiert werden.
vor dem Nebeneffekt; ein mehrdeutiges Schreiben schließt die Sitzung und ist nie
replayed. Ausgabesequenz ist unabhängig.`Last-Event-ID`, eine zweite Anlage
und ausländische Schauspieler werden abgelehnt. SSE EOF ohne `terminal.exit` ist ein Transport
Fehler, nicht erfolgreiche Ausführung des Befehls.

Die Sitzungen laufen am frühen Tag des zugelassenen Verfalls und fünf Minuten ab.
Unattached Sessions schließen nach zehn Sekunden. Kapazität ist auf sechzehn begrenzt
Sitzungen weltweit und vier pro Schauspieler. Ausgabe liest ein 16 KiB Stück auf einmal;
socket schreibt sind termingebunden, Eingabe schreibt haben eine fünf-Sekunden-Grenze. Ausgabe
Trennen oder Annullieren schließt den Laufzeittransport. Runtime-side-Prozess
Beendigung und Aufräumung erfordern immer noch die eigentliche Docker/Podman-Qualifikation;
Diese Quelltests allein stellen nicht fest, dass ein Container-Befehl beendet wird
durch das Exec-Abschaltverhalten jeder Laufzeit.

Drahtschemata sind verpackt in `contracts/machine/v1/terminal-*.schema.json` und
lösen Sie durch die offline öffentliche Registrierung. Terminal Bytes und argv sind nie
als Audit/Log-Felder verwendet; der Deskriptor liefert einen sicheren Akteur/Ressourcen-Kontext.

## Behobene Pächter-Berechtigungen

Wenn die geschützte API-Middleware eine Mietermitgliedschaft löst, Maschine
Autorisierung nutzt den vorhandenen RBAC-Dienst von Core: Viewer kann nur lesen
Operationen, während Editor kann auch mutieren und löschen. Unbekannte Rollen, fehlt
Mitgliedschaftskennungen und unbekannte Sicherheitsklassen leugnen. Eine authentifizierte
Antrag auf `dev` behält sich diese Prüfung vor; sie wird nicht zu einem vertrauenswürdigen lokalen Betreiber.
Diese Rolle überprüfen ergänzt effektive Politik und Ressourcenbesitz. Es tut
keine Mieterisolation für das Inventar oder die Laufzeitressourcen eines Adapters festlegen.

### Verbrauch von Navigationsergebnissen

`app.list `, ` target.list `, ` workspace.list `und` runtime.list`die erzeugten
[read-model schema](https://github.com/mcpdev80/baseharbor/blob/HEAD/contracts/machine/v1/read-models.schema.json)und
[synthetic examples](https://github.com/mcpdev80/baseharbor/blob/HEAD/contracts/machine/v1/read-models.golden.json). Das
Quelle ist Cores semantische Go-Typen, geteilt über CLI JSON, MCP und HTTP.
Verbraucher bestätigt das Ergebnis erst nach einer korrelierten erfolgreichen Ausführung.
Leeres Laufzeit-Explorer-Inventar kann sein `null`. Konfigurierte Ziele nicht
Bericht Verbindung Gesundheit, und Bereitstellung Beobachtungen bedeuten keine Live-Laufzeit
Gesundheit. Unbekannte Ergebnisfelder oder inkompatible Versionen erfordern eine unterstützte
Verbraucher-Update; ein Live-Fehler darf keine Fixture-Daten auswählen.

Terminal Verbraucher Beispiele werden von tatsächlichen emittiert `machine.StreamDescriptor`,
`machine.TerminalEvent ` und`machine.TerminalInput` Datensätze in der erzeugten
Lese-Modell-Artefakt. Runtime-Fähigkeit Beispiele verwenden Cores tatsächliche
`runtimeexplorer.CapabilitySet`. Diese Beispiele sind explizit synthetisch und
nur Dekodierungs-/Konformitätsprüfungen durchführen; sie qualifizieren keine authentifizierte
Browser oder eine echte PTY/Runtime Reise.

## Verwaltete Vertrauensrotation

`openbao.rotate` wird nur beworben, wenn der HTTP Semantic Executor implementiert
es. Wählen Sie ein explizites Installationsziel und Umgebung und senden
`{"approval":true}`. Fehlende Genehmigung, falsch aufeinander abgestimmte Eingabeziel und Anwendung
oder Workspace Selektoren werden vor dem Laufzeitzugriff abgelehnt. Core wählt die
geschützte Recovery-Datei aus der eigenen Installationskonfiguration; HTTP-Eingabe
kann keine Wiederherstellungspfade oder Wiederherstellungsschlüssel liefern.

Der Executor nennt die gleiche Credential- und Service-CA-Rotation, die von CLI/MCP verwendet wird,
gibt dann nur initialisierte/unversiegelte/manager-ready Flags zurück.
Verifikation und Pensionierung bleiben Kernaufgaben. Geschützt regelmäßig
Verwertungsmaterial wird vor der Mutation validiert. Ausfall oder Unterbrechung
Beobachtung darf nicht als Abschluss behandelt oder automatisch wiederholt werden.
Die Ausführungsbeobachtung bleibt durch die bestehende beglaubigte Fünf-Minuten-Beobachtung begrenzt
HTTP/Session-Lebensdauer; Prüfen Sie die vorhandene Ausführung, wenn die Beobachtung endet.

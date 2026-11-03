# Vertrag über den Target Access Provider

Status: normativ für das vor-v0.5 Target-Modell.

## Feste Grenze

```text
Runtime Provider != Target Access Provider
```

Ein Runtime Provider definiert laufzeitspezifische Realisations- und Beobachtungssemantik.
Ein Target Access Provider definiert, wie BaseHarbor Core ein Ziel erreicht und
Transporte, die bereits ausgewählte, bereits genehmigte, gebundene Beförderungen sind.

Keine Achse ersetzt die andere.

## Zielmodell

Das bestehende Target-Modell bleibt maßgeblich:

```text
Target
├── Runtime
│   └── provider
└── Access
    └── reference -> AccessDefinition
        ├── provider
        ├── reference
        └── native-context (optional)
```

Die effektive Projektion zeigt:

```text
name
runtime_provider
access_provider
access_reference
scope
```

Laufzeit- und Zugriffsanbieter sind unabhängig.

Gültige Beispiele sind:

```text
runtime=docker      access.provider=local
runtime=podman      access.provider=local
runtime=docker      access.provider=baseharbor-node-connector
runtime=podman      access.provider=baseharbor-node-connector
runtime=kubernetes  access.provider=native-api
runtime=openshift   access.provider=native-api
```

## Zuständigkeiten

Ein Zielzugangsanbieter kann Folgendes besitzen:

- Verbindungsaufbau;
- authentifizierter verschlüsselter Transport;
- Endpunkt und Peer-Identität;
- strukturierte Fähigkeitsentdeckung;
- Beantragung/Antwortung des Transports;
- log/event stream transport;
- Begrenzung des exec-Verkehrs mit Ressourcen;
- Verweise auf native Plattform-Kontexte;
- Verbindung Gesundheit.

Ein Zielzugangsanbieter darf nicht besitzen:

- transportabler Anwendungs-Intent;
- Anwendung oder Einsatz gewünschter Zustand;
- Laufzeitanbieter-Realisierungs-Semantik;
- Vermittlung von Anbietern;
- Umweltpolitik;
- Richtlinie über die Zulassung des Betreibers;
- HA-Semantik;
- geheime Quelle der Wahrheitssemantik;
- Reconciliationsentscheidungen von BaseHarbor.

## Zentrale Rufrichtung

```text
CLI / Console / MCP / HTTP
          |
          v
     BaseHarbor Core
          |
          v
 Runtime semantics
          |
          v
 Target Access Provider
          |
          v
 concrete target
```

Clients rufen nie direkt einen Connector oder einen anderen Access Provider an.

## Örtliche Zugänge

`access.provider=local` bedeutet BaseHarbor führt die bereits gewählte Laufzeit aus
Betreiberbetrieb auf der lokalen Maschine.

Der lokale Zugriff ist unabhängig von der Laufzeit:

```text
docker + local
podman + local
```

Das Vermächtnis CLI `--provider` Flagge ist nur ein Alias für
`--runtime-provider`. Es impliziert nicht den Zugangsanbieter.

## BaseHarbor Knotenverbindung

`access.provider=baseharbor-node-connector` ist ein optionaler entfernter Target Access
Implementierung für Docker/Podman/VM/Bare-Metal-Szenarien.

Der Stecker:

- die vom Core/Runtime-Anbieter ausgewählten begrenzten typisierten Operationen erhält;
- erhält keinen portablen Application Intent;
- keine eigene Politik, Platzierung oder Reconciliation;
- keine generische Host-shell/runtime-command/workspace-command-API besitzt;
- Stages übertragene Einsatz-Artefakte unter einer explizit geschützten Inszenierung
  Wurzel;
- verwendet gegenseitig authentifizierte TLS mit überprüfter Peer-Identität;
- verhandelt strukturierte Fähigkeiten nach der Authentifizierung.

Der Connector kann sich entwickeln und unabhängig von BaseHarbor Core lösen, während
entsprechend dem versionierten Target Access-Vertrag.

## Native API-Zugriff

`access.provider=native-api` stellt plattform-native authentifizierten Zugriff dar
wie Kubernetes/OpenShift API Kontexte.

Kubernetes/OpenShift benötigen den BaseHarbor Node Connector nicht nur für
erfüllen das Target Access-Modell.

## Sicherheit

Jeder nicht-lokale Zugriffsanbieter versagt geschlossen.

Erforderlicher Ausgangswert:

- authentifizierter verschlüsselter Transport;
- kein Klartext oder opportunistischer Rückfall;
- Verifizierter Endpunkt/Peer-Identität;
- Referenzen, die durch bestehende Anmelde- bzw. Vertrauensverträge referenziert werden;
- keine in portable Application Intent eingebetteten Anmeldeinformationen;
- Rotation/Erneuerung ohne Änderung der Target/Application-Identität;
- abgelaufene, widerrufene und noch nicht gültige Identitäten abgelehnt;
- Fähigkeitsverhandlungen nur nach der Authentifizierung;
- sicherheitsrelevante Verbindungsausfälle bleiben ohne Nachweis auditierbar
  Leckage.

## Entdeckung von Fähigkeiten

Zugangsmöglichkeiten sind strukturiert und anbieterneutral. Verbraucher dürfen nicht
Zweig auf Anbieter-Namen, wo Fähigkeit Entdeckung beantwortet die Frage.

Beispiele:

```text
connect
stream
exec-transport
peer-identity
native-context
```

Nur nachgewiesene BaseHarbor-Anforderungen fallen in den öffentlichen Auftrag.

## Vereinbarkeit

Diese Entkoppelung ist eine vorgefrorene Architektur.

```text
access.provider == runtime.provider
```

ist kein Kompatibilitätsversprechen und wird eher entfernt als emuliert.

Portable Application Intent bleibt unverändert.

## Canonical Connector Draht-Artefakt

Die sprachneutralen Transportdaten werden in
[wire.schema.json](https://github.com/mcpdev80/baseharbor/blob/HEAD/contracts/targetaccess/v1/wire.schema.json), mit
[synthetic golden records](https://github.com/mcpdev80/baseharbor/blob/HEAD/contracts/targetaccess/v1/wire.golden.json)und
[session semantics](https://github.com/mcpdev80/baseharbor/blob/HEAD/contracts/targetaccess/README.md).
Core-Pakete und löst sie offline. Verbraucher erwerben einen unveränderlichen Core
commit, reject duplicate keys und unbekannte fields, und überprüfen Sie die gleichen fixtures.

Anfragen tragen die Kernausführungskorrelation und eine absolute UTC-Deadline.
Getippte Nutzlasten geben einen generischen Host-Befehl nicht zu. Terminal-Zulassung kann nicht
wiederaufgenommen; binäre Stücke, argv und inszenierte Artefakt-Payloads haben explizite Grenzen.
Ein Schema-Match stellt nur Struktur her. Exakte authentifizierte Peer/scope,
Dauerhafte Nebenwirkungsaufnahme, Absage und Filesystem-Einschränkung müssen
auch durch die Live-Session und Laufzeitimplementierung durchgesetzt werden.

## Status der Steckverbinder-Implementierung

Die scoperd enrollment limite und PostgreSQL one-use store sind implementiert;
siehe[ADR 0020](../decisions/0020-scoped-connector-enrollment.md). Verwendung von Signaturen
die bestehende geschützte OpenBao-Behörde und kundeneigene CSR-Schlüssel. Bootstrap
Autorisierung bindet Mieter, Ziel, Knoten, Laufzeit und Nonce und läuft innerhalb
10 Minuten. Zertifikat TTL ist auf 24 Stunden begrenzt. Verbrauch verpflichtet sich vor
Unterzeichnen, so dass eine fehlgeschlagene oder unterbrochene Ausgabe erfordert eine neue Autorisierung.

Der Operator und Bootstrap HTTPS Handler verwenden die kanonischen Draht-Datensätze und
one-use Authority. Die Erstellung von Finanzhilfen erfordert eine authentifizierte Editor-Mitgliedschaft,
Kerneigene Target-Konfiguration und effektive Target-Richtlinie. Das Bootstrap
Der Austausch akzeptiert nur die sachdienliche Inhabergenehmigung und die CSR in Kundenbesitz;
Sie benötigt weder den Betreiber OIDC noch einen privaten Schlüssel.

Die Registrierung ist auf dem Operator Core explizit aktiviert mit
`BASEHARBOR_CONNECTOR_ENROLLMENT_ENABLED=true` und
`BASEHARBOR_CONNECTOR_AUTHORITY_TARGET=<local-core-target>`. Start erfordert
Betreiber OIDC, PostgreSQL und der initialisierte/unversiegelte geschützte OpenBao
autority. Das Signing Target ist beim Start fixiert und muss kanonischen lokalen
Zugang mit Docker oder Podman. Eine Anfrage kann keine andere Signierautorität wählen.

HTTPS-Endpunkt Autorisierung und Ergebnis
| --- | --- |
| `POST /api/v1/connectors/authorizations ` Betreiber OIDC und aufgelöster Mieter`create ` Erlaubnis; gibt nur die geschützte`token `, ` nonce `, ` expires_at`Projektion. . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . .
| `POST /api/v1/connectors/enroll ` Ein Scoped`Authorization: Bearer` credential plus kanonische Einschreibungsanfrage; gibt das signierte Client-Zertifikat und öffentliches Trust-Bundle zurück.
| `POST /api/v1/connectors/renewal-authorizations` Geschützte Operator-Identität, Mieter-Mitgliedschaft und Update-Zulassung; erstellt eine One-Use-Zulassung, die an das aktuelle aktive Zertifikat des Knotens gebunden ist.

Zuschuß-Eingabe ist `target_id`, ` node_id `, ` environment `, ` lifetime_seconds`
(1–600) und `certificate_ttl_seconds`(1–86400). Das vertrauenswürdige Kernziel
Konfigurationsbedarf `tenant-id` und Laufzeit; die Anfrage liefert weder.
Seine Zugriffsdefinition verwendet `baseharbor-node-connector` und eine stabile Node-id
`reference` passend zum gewünschten Knoten. Ungebundene Ziele, ausländische Mieter,
Nicht unterstützte Laufzeiten und Richtlinienverweigerungen scheitern geschlossen. Diese Endpunkte erfordern
HTTPS, gebundene Ganzkörper-JSON und gleich Ursprungs-Browser-Anfragen, und Verwendung
`Cache-Control: no-store`. Token gehören in die owner-only Bootstrap
Autorisierungsdatei, nie normale Ausführung Metadaten oder Shell-Argumente.

Die Zertifikatserneuerung nutzt die gleichen Berechtigungseingaben und den kanonischen CSR-Austausch.
Die Ersteinschreibung ersetzt niemals eine ausgestellte Identität.
der vorhandene Knoten und bindet seine Zuerkennung an die aktuelle Zertifikatsserie. A
die Erteilung wird widerrufen, abgelaufen oder gleichzeitig ersetzt;
Der Widerruf während der Unterzeichnung verhindert auch die Zulassung neuer Zertifikate.
Material wird erst nach dem Austausch zurückgegeben und ein Vorgänger verpflichtet
atomar. Der Vorgänger bleibt für höchstens fünf Minuten zulässig und
Eine spätere Verlängerung tritt in den Ruhestand jeder früheren Überlappung.
Die Wiederherstellung eines Vorgängers bewahrt den aktuellen Ersatz; Widerruf der aktuellen
Identität leugnet beides. Die Erneuerung Tische und Wachen erfordern Migration `0007`;
Das Zurückrollen bewahrt die frühere Einschreibung/Einnahme und versagt die Schemabereitschaft.
Die Source-Implementierung erfordert noch echte PostgreSQL/OpenBao und live
Konnektordrehungsnachweis vor Freigabequalifikation.

## Aufnahme von Outbound-Sitzungen

Der Betreiber Core kann seinen separaten Outbound-Session-Hörer mit
`BASEHARBOR_CONNECTOR_LISTEN_ADDR `, ` BASEHARBOR_CONNECTOR_TLS_CERT_FILE`,
`BASEHARBOR_CONNECTOR_TLS_KEY_FILE ` und`BASEHARBOR_CONNECTOR_TLS_CA_FILE`.
Anmeldung, Operator OIDC und PostgreSQL sind weiterhin erforderlich.
hat eine Core SPIFFE URI und Server-Auth-Nutzung; Connector-Zertifikate haben
Nutzung der Client-Auth und die exakte persistente Mieter/Target/Node/Runtime-Identität.
TLS 1.3 überprüft die Kette vor kanonischen Hallo und aktive Zertifikatszulassung.

Der Pool erlaubt maximal 64 Verbindungen und vier Sitzungen pro eingeschriebenen Umfang.
Live-Fähigkeiten werden vom authentifizierten Peer angefordert und an seine
genaue Hallo-Identität. Statische Zugriffsdeskriptoren beweisen keine Live-Unterstützung.
Laufzeit-Explorer Projekte Inventar und gebundene Container-Betriebe durch
der Transport; die bestehenden Entscheidungen über die Grundeigentums- und Mieterentscheidungen bleiben maßgeblich.
Der Core Projekt Realisation Adapter bindet jedes inszenierte Projekt an die exakte
Lebendmieter/Target/Node/Runtime-Identität. Eine Staging-Belegschaft muss die
Bundle-ID, jede Quelle verdauen und jeder relative Dateiname unter einem beengten,
unwandelbares Objektverzeichnis. Staged Bytes werden vor dem Versand kopiert; Anrufer
Änderungen können eine spätere Quadlet-Umsetzung nicht ändern.
akzeptieren nur Dateien aus diesem validierten Projekt; Podman verwendet seine eigenen nativen
Quadlet-Lebenszyklus. Jede Operation überprüft verhandelte Live-Fähigkeiten,
behält die Kernkorrelation und eine begrenzte Frist und führt einen Versand durch.
Ein getrennter, ersetzter Quittungsstatus oder fehlender Runtime Exit Status wird nicht geschlossen;
Es gibt keine automatische Mutationswiederholung oder lokale Ausführungs-Fallback.

Projekt-Service-Beobachtungen inspizieren unabhängig jeden ausgewählten Container und
seine unveränderliche ID und exakte native Projekt-/Service-Etiketten zu überprüfen.
Running State allein nie die Einsatzbereitschaft zu beweisen. Backend-Verifikation
verwendet einen gebundenen Befehl in einem eindeutig geführten Dienst, Erhaltung
die vorhandenen SQL/Cache TLS-Checks und Container-lokale Referenzen.
Fehlender Status des Austritts, geändertes Eigentum oder mehrdeutige Nachbildungen verweigern die Ausführung;
Ferndiagnosen werden nicht als Fehlerdetails zurückgegeben. Bundle-Identifikatoren folgen
der gebundene unveränderliche Publikationsvertrag des Knotens.

Generated Provider Bind-Dateien behalten ihre Core-genehmigten Berechtigungen so native
nichtprivilegierte Dienstleistungen können TLS-Material lesen. Umgebungen bleiben Eigentümer-nur;
lesbare Bind-Dateien bleiben unterhalb von owner-only staging/object/parent Verzeichnissen.
Die versionierten unveränderlichen Veröffentlichungen des Knotens zeichnen sowohl Digests als auch Dateimodi auf,
und revalidiert die geschützten Vorfahren und exakte Modi vor der Ausführung.
Erschienenes Quadlet kann zusätzlich auswählen `project_directory`, gebunden an
eine unveränderliche `bundles/.object-<32 lowercase hex>` Veröffentlichung. Die ausgewählten
Der Inhalt der Einheit muss mit ihren gebundenen Bytes übereinstimmen.
`@BASEHARBOR_BUNDLE@/` Referenzen sind nur für schreibgeschützte Datei-Volumes zulässig
und owner-only Container-Umgebungsdateien; der Knoten löst sie unter seiner
eigene Bundle root. Core Host Pfade und Bauanleitungen sind keine tragbaren Eingänge.
Die Konfiguration des node-native Storage/registry-Prozesses gehört zur Node.

Der Core Graph Adapter validiert alle ausgewählten Einheitennamen vor der Mutation,
veröffentlicht Netzwerk/Volumen-Definitionen vor dem Start von Containern und entfernt
Container vor Ressourcendefinitionen. Native Resource Name Kollisionen erfordern
ein eigener Realisierungsempfang und passende Projekt-/Service-Etiketten vor der Reparatur;
ausländische Ressourcen sind erhalten. Gewöhnliche Entfernung hält Provider-Datenmengen.
Diese Graphen/Provider-Primitive qualifizieren nicht die komplette Application-Engine.

Beschreibbare, privilegierte, ersetzte oder geänderte Modi scheitern geschlossen. Geschützter Kern
Quittungen binden die Wiederherstellung an die gleichen Quellmodi. Gewöhnliche Kompose zerstören
speichert Daten; Volumenentfernung erfordert eine ausdrückliche Core-owned Reset-Entscheidung.

Operator API Start bindet eine lokale Installation, auch wenn Connector Registrierung
Es ist deaktiviert. Anmeldung und Outbound-Sitzungen verwenden die gleiche Start-Auswahl;
pro-Anwendung Laufzeit Broker nicht zu Installationsbehörden werden.
Remote-Anwendung Voraussetzungen wählen Sie, dass gebundene lokale Installationsbehörde,
unabhängig vom Ausführungsknoten.
Anfrage muss seine überprüfte Mieter/Identität und Eigentum Target behalten. Es kann nicht
Wählen Sie einen anderen Core durch Anwendungseinstellungen oder Bootstrap einen zweiten Core
auf dem Knoten. Live SQL/Secrets/Identity Bereitschaft der gebundenen Installation ist
noch erforderlich; ein zwischengespeicherter Zustand stellt keine Bereitschaft her.
Die Start-gebundene API lehnt auch eine Anwendungspräferenz für eine andere lokale
Installation vor dem Lesen des Kernzustandes oder Versuch Bootstrap. Standalone
lokale CLI-Auswahl bleibt unabhängig; API-Anfragen können nicht die gebundenen ersetzen
Installation durch Auswahl eines anderen lokalen Targets.
Explizite Core Bootstrap verwendet die gleiche Startbindung und verweigert eine Ausführung
Node vor dem Erstellen des Installationszustandes. Ein registrierter Node kann nicht ein anderer werden
Kern durch den Maschinenaufbau.

Geschützter Core-Deployment-Zustand kann eine versionierte Projektquittung mit behalten
nur der exakte Knoten Scope, unveränderliches Verzeichnis, Bundle-ID und Quellverpflichtungen.
Die Wiederherstellung erfordert die ursprünglichen geschützten Quell-Bytes und Live-Scope-Bindung;
ausländischen, veränderten oder unvollständigen Zustand fehlschlägt vor dem Versand. Restauration selbst
nicht ein weiteres Bündel inszenieren oder eine Mutation erneut abspielen. Es ist kein Operator-Eingang
und stellt keine vollständige Anwendungsabstimmung oder Backend-Zustimmung fest.

Die verwaltete Backend-Dateiprojektion überprüft die exakte Core-generierte Definition
vor dem Kompilieren eines nativen Projektnamens. Es kopiert die Laufzeitumgebung und
referenzierte Dateimounts/geheime Dateien aus einem geschützten Verzeichnis; unabhängig
Installationsdateien sind ausgeschlossen. Modifizierte Definitionen, symbolische Links und
unconfined paths fail closed. Diese Vorbereitung bewahrt SQL/Cache TLS-Material;
Workload-Lieferung, Remote-Platzierung und die komplette Application-Engine erfordern
ihren eigenen Integrationsnachweis.

Dieser Projekt-Adapter trägt bereits genehmigte Laufzeit-Entscheidungen.
Ersetzen Core Application Planung, Anbieter Platzierung, geheime Behörde oder
fortdauernde Reconciliation. Komplette Remote-Anwendungsintegration und
Exact-Source native Qualifikation bleibt vor Freigabe Genehmigung erforderlich.

Folgen Sie Protokollen und interaktiven Terminals verbrauchen ausschließlich eine zugelassene Sitzung.
Der kanonische Stream öffnet sich und jedes Ereignis trägt die genaue Stream-ID und Core
Korrelation. Output und Terminal-Eingang haben unabhängige zusammenhängende Sequenzen;
Fremde Reichweite, Lücken und fehlgebildete Frames ziehen die Verbindung zurück.
sind auf 16 KiB begrenzt und verwenden Leser-Rückdruck. Terminal argv, Größe und Eingang
werden eingegeben; Eigentum und Umgebung werden von Core geprüft, bevor Stream geöffnet wird.
Exit-Codes werden propagiert. Langsame Leser, Token/Session-Auslauf und Trennen
den Transport zu schließen; ein interaktiver Prozess wird nie transparent wieder verbunden.
Diese Quellen-Level-Garantien erfordern immer noch echte Docker / rootless Podman und
authentifizierte Browser-Qualifikation beim Endkandidaten.

Eine Steuerungsoperation führt pro Verbindung aus. Jede Invokation überprüft
Zertifikat Zulassung und bewahrt Core Execution Korrelation. Keine Operation ist
Nach dem Annullieren, Trennen oder Vervollständigen automatisch wiederholt.
Der unterbrochene Transport wird eingestellt; Anrufer versöhnen beobachteten Zustand vor einem
Neue Mutation. Eine fehlende oder fremde Sitzung wählt niemals eine lokale Laufzeit aus.

Die Sitzungen laufen am Anfang von fünf Minuten, Peer Certificate Ablauf und
Kernlebensdauer. Persistierte Aufnahme und der aktuelle Knoten CA Bundle werden erneut überprüft
alle fünf Sekunden mit einer Zwei-Sekunden-Check-up-Deadline, einschließlich Leerlauf und aktiv
Sitzungen, und vor jedem Versand/Stream offen. Die Peer-Kette ist verifiziert
gegen frisch geladenes Vertrauen; ein zwischengespeicherter erfolgreicher Handshake bewahrt nicht
eine Behörde, die aus dem Überlappungsbündel entfernt wurde. Widerruf, Vertrauen-Reload-Ausfall,
pensionierte CA oder Register-Nichtverfügbarkeit schließt den Transport innerhalb dieser Grenze. Neu
handshakes reload server identity material und node CA trust, unterstützt eine
kontrolliertes Überlappungspaket; das Ändern der ausgewählten Core-Identität erfordert einen Neustart.
Reale OpenBao/Rotation/Laufzeit und authentifizierte private Beweise sind weiterhin erforderlich;
Diese Quelle Primitive sind keine v0.4.23 Freigabegenehmigung.

### Gespeicherte Core-Server-Signierung

Die verwaltete OpenBao-Behörde konfiguriert eine separate `baseharbor-core` CSR-Rolle für
`spiffe://baseharbor/platform/core/*`. Es ermöglicht nur die Server-Authentifizierung;
`baseharbor-nodes` bleibt nur die Client-Authentifizierung.
`ServiceIssuer.SignCoreCSR` bound überprüft das signierte Blatt gegen das eingereichte
Kernschlüssel, exakte URI-Identität, Lebensdauer und alleinige Server-Auth-Nutzung.
öffentliches Zertifikatsmaterial und nie ein privater Schlüssel. Node-Einschreibung kann nicht anrufen
Diese Grenze, um eine Kernidentität zu erhalten.

Core Server CSRs können zusätzlich bis zu acht ausdrücklich autorisierte,
kanonischen DNS-Namen, die vom internen Core-Aufrufer ausgewählt wurden.
die Namen müssen mit der unterzeichneten CSR übereinstimmen, und das zurückgegebene Zertifikat muss erhalten bleiben
genau diesen Satz. Dies ermöglicht eine normale TLS Server-Namen-Verifikation neben der
Kern-URI-Identität. Wildcards, IP-SANs, Duplikate und zusätzliche Namen werden verweigert;
Node Signing verweigert immer noch alle DNS SANs und kann keine Server-Namen-Behörde gewähren.

Dies bietet eine Emissionsgrenze für Core-owned Schlüssel. Es tut nicht von selbst
die konfigurierten Dateien des Hörers zu installieren oder zu drehen, Vertrauensüberlappung zu koordinieren oder
qualifizieren eine Produktion OpenBao Rotation. Diese Bereitstellung und End-to-End-Kontrollen
vor der Genehmigung vor der Freigabe erforderlich bleiben.

Managed Zertifikat Widerruf akzeptiert die positive hexadezimale Serie von einem
X.509 Blatt sowie die Doppelpunkt-getrennte Form des Emittenten. Kern normalisiert es zu
OpenBao Zertifikat Speicherschlüssel vor dem Widerruf; Fehlformung, Null oder
Übergroße Serien scheitern vor der Authentifizierung an den Emittenten. Ein fortbestehender Knoten
Widerruf verweigert selbständig den bestehenden Versand und die Aufnahme neuer Sitzungen.

## Beobachtung der quellgebundenen Quadlet-Vervollständigung

`runtime.quadlet.verify-completion` ist eine authentifizierte, schreibgeschützte Fähigkeit für
Linux Podman mit einem Live-Benutzer-systemd-Manager. Seine Nutzlast erfordert `name`
(a `.container ` Einheit), die exakte ungelöste`content `, und` project_directory`
(eine unveränderliche `bundles/.object-<hex>` Veröffentlichung). Es kann Host-Pfade nicht auswählen,
Start-Einheiten oder liefern eine `enable` Flagge. Normale Peer-, Target- und Fähigkeitsprüfungen
Anwendung; die Beobachtung gibt keine Mutation zu oder wiederholt sie nicht.

Der Knoten prüft das veröffentlichte Bundle, aktuelle Einheit und native Ressourcenbesitz,
Aufgelöster Quellverdauung, geschützter Aktivierungsempfang und aktueller Linux-Boot.
Native erfolgreichen Prozess Exit muss folgen, dass Quelle aufgezeichnet Aktivierung
und haben den Einheitszustand festgelegt. Fehlgeschlagene, ungestartete, geänderte, entfernte und veraltete Einheiten
Ein fehlender Behälter allein ist niemals ein erfolgreicher Abschluss.

Erfolgsgewinne `name`, ` project_directory `, ` content_sha256`(SHA-256 der
ungelöster Anforderungsinhalt) und `completed: true`. Core bestätigt alle vier
vor der Annahme der Beobachtung gegen sein scoped unveränderliches Projekt.
Dieses Primitive qualifiziert selbst nicht den kompletten Remote Application-Lebenszyklus
oder die Ermöglichung einer unqualifizierten Verwirklichung der Erfüllungsabhängigkeit zu genehmigen.

## Abschluss-in Kenntnis der Fernsequenzierung von Graphen

Cores expliziter Complete-aware-Projektor markiert die generierten Init-Einheiten und
hält Abhängigkeit Reihenfolge während Entfernen native implizite Aktivierung von denen
Einheiten. Es entfernt auch die erzeugten `ExecStartPre` shell state heuristic.
Der gewöhnliche Projektor lehnt weiterhin die Vervollständigung von Abhängigkeiten ab.

Die Complete-aware Applier validiert den gesamten Graphen und Fähigkeiten vor gesetzt
Veröffentlichung, veröffentlicht alle Einheiten ohne Container starten, und aktiviert sie
in Abhängigkeitsreihenfolge. Jede init-Einheit muss die quellgebundene Beobachtung vorübergehen
seine abhängigen beginnt. Warten wird durch den Anrufer und eine zwei-Minuten-Grafik begrenzt
Limit; nur Beobachtungen wiederholen sich. Fehlgeschlagene oder unterbrochene Mutationen sind nie
replayed. Diese Projektmechaniken stellen nicht die vollständige Bewerbungsqualifikation fest.

Für ein fertigungsabhängiges Quadlet-Diagramm sendet Core veröffentlicht `runtime.quadlet.apply`
Anträge mit `enable: true` und `autostart: false`. Der Knoten startet den exakten Besitz
Einheit explizit aber löscht seine automatische Zielaktivierung. Nach einem Node Boot,
Core muss den Graphen erneut anwenden und die erfolgreiche Init-Vervollständigung auf diesem Boot überprüfen
vor dem Start der abhängigen. Kein automatischer Node Start ersetzt dies
Überprüfung.`autostart` ist fakultativ für die gewöhnliche Anwendung und erfordert eine unveränderliche
`project_directory` wenn vorhanden. Native Beweise überprüft Zielmitgliedschaft; es
behauptet nicht, dass eine Maschine neu gestartet wurde.
Die Qualifikation bleibt getrennt.

Der interne Managed-Provider-Ausführungsadapter von Core bereitet jetzt einen geschützten
Momentaufnahme der erzeugten Compose-Definition, Umgebung und referenzierten TLS
Dateien. Podman-Compilation verbraucht nur diesen Snapshot in einem owner-only temporäre
tree; es kann geänderte ursprüngliche Core-Pfade nicht mehr lesen. Der temporäre Baum wird entfernt
vor der Veröffentlichung. Der Knoten erhält unveränderliche native Einheiten und deren referenzierte
Dateien, mit genauen Quellverpflichtungen und Modi.

Der gleiche Scoped-Adapter veröffentlicht einmal und erfordert Core, um die genaue Commit
Erhalt dauerhaft vor der Genehmigung der Aktivierung. Fehlende Persistenz wird abgelehnt
vor der Veröffentlichung; ein fehlgeschlagenes oder unterbrochenes Commit hinterlässt keinen ausführbaren Handle
und kann die Veröffentlichung nicht automatisch wieder abspielen.
der Eingang in das produktionsgeschützte Einsatzregister, bevor er zuerst gilt;
stellt es dann wieder ohne Inszenierung wieder her. Der Adapter stellt eine geschützte Bereitstellungsquittung wieder her
ohne zu republizieren, und gilt, beobachtet, Sonden, Reparaturen und zerstört, dass
projekt. Wiederherstellung muss mit seiner genauen Projektidentität, Knotenbereich und Quelle übereinstimmen;
eine fehlgeschlagene Wiederherstellung löscht seinen ausführbaren Griff. Gewöhnliche Zerstörung behält
Provider-Daten. Explizite im Besitz Daten-Reset bleibt eine separate Entscheidung für beide Laufzeit. Dieser Provider-Adapter nicht
Öffnen Sie den Remote CLI Guard oder qualifizieren Sie den vollständigen Application HTTP-Lebenszyklus.

## Explizit veröffentlicht Podman Volumen Zurücksetzen

`runtime.quadlet.reset-volume` ist eine mutierende, dauerhaft zugelassene Fähigkeit für
Linux Podman mit einem Live-Benutzer-systemd-Manager. Es erfordert eine `.volume` Name,
genaue ungelöste Quelle und unveränderlich `project_directory`. Hostpfade, generisch
Volume Selektoren und Kraftflaggen sind ausgeschlossen. Der Node überprüft die veröffentlichten
Quelle, fehlende aktive Einheit, exakt beibehaltene Realisierung und aktuelles natives Projekt
Etiketten vor dem Entfernen. Es hält nie einen Verbraucher oder zwingt die Entfernung eines In-use
Volumen, und überprüft native Abwesenheit danach. Ein bereits fehlendes Volumen wird akzeptiert
nur mit dem gleichen geschützten Erkenntnisnachweis.

Der explizite Graphen-Reset von Core überprüft zunächst alle erforderlichen Funktionen und validiert
das ganze Diagramm. Es versöhnt exakte Einheiten, ohne Container zu starten, zerreißt
owned Units, setzt dann nur veröffentlichte Volumenmitglieder zurück. Dies unterstützt eine explizite
retry nach normalem Abriss oder unterbrochenem Zurücksetzen unter Beibehaltung von Fremdressourcen
checks. Normal Graph zerstören bleibt Daten-erhalten. Native Qualifikation bei
die letzten Quellstifte sind erforderlich, bevor ein Reset als Release-Gate behandelt wird.

Generierte Remote-Init-Einheiten behalten `RemainAfterExit=yes` und `Restart=no`.
Erfolgreiche native Exit-Evidenz bleibt also ohne Ziel beobachtbar
Aktivierung oder automatische Wiederholungen. Core benötigt noch den abgeschlossenen Prozessausgang,
aktueller Boot- und Exact-Source-Aktivierungsempfang; ein bloßer aktiver/exitierter Zustand ist
nicht ausreichend. Diese Service-Einstellungen behalten die Beobachtung, nicht die Startautorität.

## Core-selektierte Aktivierungsphasen komponieren

`runtime.compose.apply ` kann eine unleere, einzigartige, begrenzt tragen`services` Liste.
Core überprüft jeden Namen mit den immutierbaren ausgewählten Compose-Dateien. Der Knoten
löst selbstständig effektive Namen mit nur Lese-only `compose config --services`
vor jeder Aktivierung. Ausgewählte Phasen verwenden `up --no-deps --no-build` und kann nicht
request Builds oder Waisenentfernung. Core muss explizit konvergieren und überprüfen
Voraussetzungen vor Aktivierung einer Workload-Phase. Reparieren nur ausgewählt
Dienstleistungen; eine verlorene Reaktion wiederholt die Mutation nicht.
`services` behält sein bestehendes Verhalten. Dieses Primitive nicht qualifiziert die
komplette Remote-Anwendungs-Engine.

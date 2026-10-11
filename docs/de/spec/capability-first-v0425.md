# Capability-first Application-Verträge (v0.4.25)

Verbindlicher Scope: [#872](https://github.com/mcpdev80/baseharbor/issues/872),
korrigiert am 2026-10-11. Er ersetzt das feste SQL/Secrets/Identity-Paket,
Entwickler-IDs und die vertagte Entfernung früherer Planung und Abnahme.
Bestehende Sicherheit, Ownership, HA und Recovery bleiben verbindlich.

## Autorenformat und systemeigene Identität

```yaml
version: 1
app:
  name: myapp
  environment: dev
```

Services sind optional und niemals implizit. SQL ist `services.sql: true` unter
`services`; ausführliche Anforderungen und benannte Instanzen nutzen die vorhandene
Grammatik. Keine `provider:`-Sektion, kein weiterer Parser, keine Pflicht-Zusatzdatei.

Entwickler setzen NIEMALS Application-IDs. `ParseYAML` lehnt jedes `app.id`, auch
leere Werte, vor Änderungen ab. Das Feld entfernen und die Anwendung explizit
initialisieren. Keine ID-Frage, kein ID-Flag, keine Konvertierung alter Autoren-IDs.
`ParseCanonicalYAML` nutzt denselben Parser mit Freigabe für interne Datensätze;
nur bestehende systemeigene Store-/Backup-Adapter verwenden diesen Zugang.
`YAML` serialisiert interne Records, `AuthoredYAML` lässt die ID für Entwickler-
Ausgaben weg. Interne IDs und Machine-Ausgaben bleiben gültig und werden beim
Lesen niemals neu erzeugt.

| Operation | Identität und Schreibverhalten |
| --- | --- |
| Parse / inspect / plan | Keine Vergabe oder Speicherung; nur bestehende autoritative Claims auflösen |
| Explizite Initialisierung / Registrierung | Einmal erzeugen, wenn wirklich neu; im bestehenden kanonischen Store speichern |
| Wiederholung / Neustart / Bearbeitung / anderes Target | Dieselbe systemeigene Identität verwenden |
| Widersprüchliche oder fehlende registrierte Identität | Vor Änderungen abbrechen; autoritativen State wiederherstellen, keine Ersatz-ID |
| Gleichzeitige Initialisierung | Store.Create-Gewinner übernehmen oder unvollständigen State zum sicheren Wiederholen melden |
| Entwickler-`app.id` | Verständlich ablehnen; Autoren-Datei und vorhandene Benutzerdaten erhalten |

`ResolveIdentity` ist rein. `Store.InitializeIdentity` nutzt den bestehenden
kanonischen Owner `.baseharbor/apps/<name>/baseharbor.yaml`. Deployment-Records
projizieren diese Identität. Adapter gleichen Repository-/Source-Ownership ab,
nicht App-Namen fremder Repositories. Lesende Adapter rufen Create,
InitializeIdentity oder Sync niemals auf. Keine parallele Identity-Registry,
automatische Migration oder Löschung vorhandener Daten.

## Provider-Auflösung und Footprint

Application Contract beschreibt Bedarf. Bestehende Target-/Deployment-Bindings
beschreiben Provider und Platzierung. `ProviderPreference` wählt optional
`InstanceID` oder `ProviderKind`; beide müssen zusammenpassen. Leer bedeutet
Automatic. Die Auswahl verwendet tatsächlich registrierte Capability-Angebote,
keine fest codierte Provider-Liste. Kompatible Bindings bleiben stabil; expliziter
Widerspruch ist ein typisierter Konflikt. Unterschiedliche Targets können denselben
portablen Bedarf unterschiedlich realisieren.

`ResolveFootprint` liest `capability.Registry` v2 und Facts vorhandener Pläne/
Observer eines Targets. Facts enthalten tatsächliche Dependency-Instanz-IDs:
null bedeutet unbekannt, [] verifiziert keine. Keine schreibende Discovery,
neue Registry, Scheduler oder zweiter Resolver. Fremde Targets/Ressourcen,
Zyklen, fehlende Dependencies und inkompatible/mehrdeutige Auswahl scheitern klar.
Automatic ohne verfügbare Deklaration bleibt unaufgelöst und autorisiert keine
Provisionierung. Eine verifiziert ausgewählte Instanz ist strukturell vollständig,
auch wenn das Binding erst in der bestehenden Apply-Transaktion gespeichert wird.

Adapter für Runtime-Berechtigungen/Consumption liefern verifizierte
`DependencyRoots` vorhandener Pläne an denselben Resolver. Nil bedeutet bei
dieser Absicht unvollständig; [] beweist keine zusätzliche Provider-Realisierung,
nicht den Wegfall von Sicherheitsanforderungen. Jeder ConsumerIntent liefert
eigene Roots; Verifikation wird nicht zwischen Verbrauchern übernommen.
Unbekannte Roots autorisieren keinen Core-losen Eingriff.

Keine Capability impliziert das alte Core-Paket. SQL, Cache, Secrets, Identity
und HTTPS wählen nur ihre Realisierung und echte erklärte Dependencies. Identity
kann SQL benötigen. HTTPS benötigt nicht automatisch Identity oder OpenBao.
Separat aktivierte Management-/Console-Dienste liefern ihre tatsächlichen
Sicherheitsanforderungen als eigene Verbraucher; TLS/OIDC/mTLS/Autorisierung/HA/
Recovery werden nicht abgeschwächt. Das bestehende `core`-Ausgabefeld klassifiziert
eine echte Deklaration und fordert kein Paket an.

Der gemeinsame Read-only-Befund heißt `baseharbor.footprint/v1`:

| Feld | Bedeutung |
| --- | --- |
| requirements | Bedarf, vorhandenes Binding oder requested/unbound, ausgewählte Instanz/State |
| existing / additional | Bestehende/shared/gestoppte Ressourcen gegen deklarierte neu benötigte Ressourcen |
| dependencies | Gewählte Dependency-IDs; null ist unverifizierbar, [] verifiziert keine |
| unused | Informations-IDs außerhalb dieses Bedarfs ohne Bindings; niemals Entfernungsfreigabe |
| scope / ownership / shared / consumers | Bestehende Ownership und Binding-Anzahl; keine erfundenen App-Zahlen |
| memory_bytes / containers | measured/estimated/unknown; Werte benötigen nichtnegative Menge und Herkunft |
| core / complete | Tatsächlich erklärter Bedarf und strukturelle Sicherheit; unvollständig verbietet unsichere Änderungen |

Unbekannte Messwerte sind bei strukturell vollständiger Auflösung zulässig.
Schätzungen kommen aus Provider-Nachweisen, niemals erfundenen RAM-/Container-
Versprechen oder Summen.

## Target-Verbraucherunion und Hinzufügen/Entfernen/Reaktivieren

`ResolveProviderPlan(snapshot, consumers, inventoryComplete)` delegiert jeden
aktiven Verbraucher an `ResolveFootprint` und vereinigt seine gesamte Dependency-
Closure. `ConsumerIntent` projiziert einen bestehenden App- oder unabhängigen
Management-Owner: Key, Active, Manifest, Preferences, DependencyRoots. Inaktive Einträge behalten
die Identität und geben Bindings frei. Der Aufrufer beweist die vollständige
Target-Inventarisierung aus bestehenden autoritativen Ownern. Nicht zugeordnete
Bindings oder unverifizierbare Facts machen den Plan unvollständig. Automatische
Konvergenz erhält dann Ressourcen; absichtliche Änderungen nutzen die konkrete
Folgenanalyse und Bestätigung unten.

`baseharbor.provider-plan/v1` enthält Verbraucher-Footprints, benötigte Provider-
IDs, freizugebende Bindings und Änderungen mit Ownership-/Evidenznachweis:

| Aktion | Bedeutung |
| --- | --- |
| add | Ausgewählte neu benötigte Realisierung materialisieren |
| reuse | Von App, Management-Verbraucher oder Dependency benötigte Realisierung erhalten |
| restart | Benötigte eigene gestoppte Runtime reaktivieren |
| stop | Eigene laufende Runtime nach Freigabe durch den letzten Verbraucher stoppen |
| remove-runtime | Unnötige eigene gestoppte Runtime entfernen; persistente Daten erhalten |
| preserve | External/BYO, fremd/unverifizierbar oder unvollständiges Inventar; niemals implizit löschen |

Alle Änderungen tragen `PreserveData=true`. Pläne erlauben keinen direkten
Runtime-Eingriff. Session 2 verwendet sie im bestehenden
plan -> preflight -> confirm -> apply -> verify. Daten-/Recovery-Elemente einzeln
mit Grund erhalten; Löschung benötigt explizite destruktive Zustimmung und
Ownership-Nachweis. Automatische Konvergenz erhält weiterhin verwendete Shared-
Ressourcen; absichtliche eigene Entfernung nennt sämtliche betroffenen Verbraucher.
Fremde/externe Infrastruktur wird niemals gelöscht. Wiederholtes Apply, unterbrochener Retry und Deaktivieren/
Reaktivieren konvergieren idempotent. Capability-Entfernung gehört zu v0.4.25.

Human-CLI, JSON und MCP verwenden dieselben Domain-Ergebnisse und vorhandene
Machine-Fehler-/Exit-/Autorisierungszuordnung. ResolutionError bleibt typisiert.
Fixtures: `internal/application/testdata/v0425-resolution.json`; Union-Invarianten:
`internal/application/provider_union_test.go`. Vollständiger Provider-/Capability-
Wizard: v0.4.26. #777-Profile-/Host-Verträge unten bleiben reine Spezifikation.

## Provider-Wechsel: Folgen und Entwicklerentscheidung

Entwickler dürfen Funktionen hinzufügen, ändern, ersetzen, deaktivieren und
entfernen, auch ohne Fortführung oder mit vollständiger Löschung. Fehlende
Alternativen sind offengelegte Folgen, kein dauerhaftes Verbot. Alternativlose,
unbekannte oder destruktive Ergebnisse zeigen **Achtung** und benötigen **zwei
unabhängige ausdrückliche Bestätigungen des konkreten aktuellen Plans**.
Datenlöschung benötigt zusätzlich die bestehende getrennte destruktive Freigabe.
Normale Apply-Zustimmung bestätigt weder beide Stufen noch implizit Datenlöschung.

`DescribeProviderChanges(snapshot, union, requests, observation)` ergänzt dieselbe
aufgelöste Verbraucherunion. Keine zweite Provider-Auflösung und keine Änderungen
am State. Adapter direkter Provider-Operationen und App-/Target-Lifecycles
projizieren ihre Aufträge in diese API. `ProviderPlan.Impact` trägt das optionale
gemeinsame Ergebnis `baseharbor.provider-impact/v1`; CLI, JSON/MCP und späterer
Wizard verwenden es unverändert. Ausführung erfolgt durch bestehende
plan/preflight/confirmation/apply/verify-Autoritäten.

| Domain-Ergebnis | Verbindliche Bedeutung |
| --- | --- |
| before / after | Tatsächlicher Provider-Typ/Instanz, Target, Scope/Owner, Orte und sichere Zugriffs-/Security-Referenzen; vorgeschlagenes After ist kein Ausführungsnachweis |
| dependencies / dependents / functions | Betroffene Apps und unabhängige Management-/Console-Verbraucher; transitiver Dependency-Pfad, notwendiger/optionaler Bedarf, verlorene/geänderte/erhaltene Funktion und beobachtetes Ablauf-/Validierungsverhalten |
| scenario | Capability nicht mehr benötigt; Provider trotz verbleibendem Bedarf entfernt; Ersatz; Provider weiterhin von anderem Verbraucher verwendet; ungenutzter Provider |
| materials | Einzelnes aktuelles Material und Ort, Recovery-Referenzen, geplantes retained/transferred/deleted/revoked/expired/invalidated/nonrecoverable/unknown-Ergebnis und Ziel/Zugriff |
| prerequisites | Konkrete Übergabe, Rebind oder adapterspezifische Verifikation, Unterstützung/Nachweis und Folge fehlender Verifikation; kein erfundener Fallback |
| effects | Tatsächlich relevante Vorher-/Nachher-Änderungen bei Verfügbarkeit, Konsistenz, Dauerhaftigkeit, Aufbewahrung, Protokoll, Policy/TLS/HA, Zustellung, Routing, Wiederverbindung oder Recovery aus dem Adapter |
| complete | Vollständigkeit des Wissens; false benötigt Achtung/zwei Bestätigungen und verbietet die informierte Entscheidung nicht |
| plan_id / revision | Deterministischer Hash aus Folgen, relevantem Binding-/Ownership-/Dependency-State und Revision bestehender Owner; wesentliche Änderungen entwerten alte Zustimmung |
| confirmations_required / destructive | Eine normale Stufe oder zwei für alternativlose/unbekannte/destruktive Folgen; explizite Datenlöschung bleibt getrennt |

`ProviderPlan.ConsumerActivity` enthält auch explizit inaktive Verbraucher aus
denselben Intents; vergangene Runtime-Nutzung wird nicht als aktueller Bedarf
interpretiert. Nur Mengen-Messrauschen entwertet unveränderte Folgen nicht.

`ProviderImpactObservation` projiziert bestehende Installation-, Deployment-,
Runtime-, Registry-, Credential-, Zertifikats- und Recovery-Owner nur vorübergehend.
`IntegrationDescriptor.ChangeContract()` beschreibt Funktionen, Material- und
Betriebsinventar des ausgelieferten Providers. `ReferenceIntegrations()` iterieren;
keine zweite Provider-Registry oder feste Produkt-Warnliste. Unbekannte Extension-
Verträge und fehlende Beobachtungen bleiben sichtbar unbekannt. Sie werden niemals
pauschal als sicher oder erfolgreich fortgeführt dargestellt.

Verbraucherspezifische `Continuations` beweisen unterstützten Ersatz und
verifiziertes Rebind. Ein laufender kompatibler Ersatz allein genügt nicht.
Sichere Übergabe/Zugriff verifizieren, bevor Fortführung behauptet oder die alte
Realisierung entfernt wird. Ohne Übergabe darf der Entwickler stattdessen den
Funktionsverlust zweimal bestätigen und die Entfernung ausführen. Erhaltener,
nicht zugänglicher Speicher ist Recovery, keine Secret-Auslieferung, Token-Vergabe
oder Zertifikatserneuerung. Keine Secrets in Klartext-Manifeste, Repositories,
Logs oder erfundene Ersatzspeicher schreiben. Absichtliches Entfernen benötigter
Authentifizierung zeigt betroffenen Dienst/Zugriff als nicht verfügbar; es
aktiviert niemals stillschweigend unauthentifiziertes Management.

Nur tatsächliche OpenBao-Nutzung erzeugt Folgen für Secret-Policies, dynamische
Credentials oder PKI. Lease-Ablauf/Widerruf und Downstream-Gültigkeit benötigen
Nachweise; exportierte Credentials beweisen keine Fortführung. Aktuelle Token-/
Zertifikatsgültigkeit unterscheidet sich von künftiger Vergabe, Erneuerung,
JWKS-Refresh, Sessions und Widerruf. Beobachtete Offline-Validierung mit Ablauf
kann geänderte statt sofort nicht verfügbare Validierung bedeuten. Keine pauschal
sofort ungültigen Tokens/Zertifikate erfinden. Caches, Jobs, Sessions, Queues und
Telemetriepuffer sind nicht automatisch entbehrlich. Unbekannte Aufbewahrung/Orte
bleiben unbekannt. Erhaltenes Material bleibt am tatsächlichen Ort; gelöschtes
Material bedeutet **kein Ziel; Löschung**. Bestehende Owner beweisen Ownership und
autorisierte Referenzen. Zwei Bestätigungen geben keine Rechte auf fremde externe
Ressourcen.

Mehrere Änderungen bilden einen Gesamtplan. Entfernung einer Dependency des
Ersatzproviders entwertet seine Fortführung; SQL-Stop nennt abhängige Identity-/
Broker-Funktionen. Verbraucherbezogenes Unbind erhält die Runtime anderer
Verbraucher und nennt den verlorenen Zugriff. Automatische Konvergenz nach dem
letzten Verbraucher erhält Daten; explizite vollständige Löschung inventarisiert
vor Zustimmung jedes betroffene Element.

`ValidateProviderChangeConsent` prüft beide Stufen, aktuellen Plan-Hash und getrennte
Löschfreigabe. Bestehende interaktive/nichtinteraktive Autoritätsadapter erheben die
Bestätigungen; keine zusätzlichen CLI-Flags oder unabhängige Frontend-Policy.
Unmittelbar vor Änderungen aus aktuellen autoritativen Beobachtungen neu planen.
Veraltete Pläne erneut anzeigen und bestätigen. Technisch nicht unterstützte
Übergabe ehrlich melden, niemals als erfolgreiche Verlagerung darstellen.

`RecordProviderChangeResult` übernimmt tatsächliche verifizierte Ausführungs-
ergebnisse, meldet complete/partial/failed, echte Endorte und ausstehende Schritte.
Vorgeschlagenes After niemals als erledigt übernehmen. Fehlendes Material,
abweichende Behandlung oder fehlender Recovery-Nachweis bleibt Teilfehler.
`PendingProviderChanges` behält den ursprünglichen Verbraucherscope und liefert
nur unerledigte Operationen/Materialaktionen an den bestehenden idempotenten
Replan-/Retry-Lifecycle. Kein Scheduler, kein neuer State-Owner. Unterbrochene
Übergabe, Rollback/Wiederverbindung und irreversible Folgen bleiben sichtbar.

### Bestehende Ancillary-Realisierungen

`ResolutionSnapshot.Ancillary` beobachtet bestehende Gateway-/Sidecar-Realisierungen
aus Runtime-/Routenownern. `ProviderFact.Ancillary` und `OwnerReference` kennzeichnen
sie; `ProviderFootprint` und Impact-Realisierungen erhalten diese Unterscheidung.
Das ist ein vorübergehendes Inventar neben der bestehenden Registry, keine zweite
Registrierung oder State-Autorität. Ancillary-`Provider.Capabilities` ist leer:
Automatic bietet sie nicht als App-Provider an; explizite Capability-Auswahl ist
inkompatibel. Dieselbe Dependency-Closure übernimmt ihre tatsächlichen IDs nur aus
verifizierten Provider-Dependencies oder Verbraucherroots. Dieselbe Union erfasst
Shared-Nutzer, eigene Entfernung, unbekannten State und Folgen. Kein verpflichtendes
Sidecar-/Core-Paket.

Ein Target-eigener Shared-Gateway verwendet seinen tatsächlichen bestehenden
Installation-/Routenowner ohne erfundene Application-UUID. Er behauptet keine
allgemeine Shared-Platzierung der auswählbaren Caddy-Integration. App-Sidecars
behalten ihre tatsächliche systemeigene App-ID. Broker-/Executor-Metadaten
beschreiben tatsächliche Autorisierungs-/Credential-, Ausführungs-/Steuerungs- und
In-flight-Folgen; Nutzung benötigt weiterhin Beobachtungen.

### Abdeckung ausgelieferter Provider

Die Tabelle inventarisiert die deklarative Abdeckung. Adapter liefern
**tatsächliche** Daten-/Zugriffsorte und Nutzung; Materialarten sind mögliche
Inventarkategorien, keine Behauptung, dass jede Instanz alles enthält.
Runtime-Einstiegspunkte und native Abnahme gehören zu den konsumierenden
Lifecycle-Tests. Bestehende runtime-broker/runtime-executor-Sidecars besitzen ausschließlich
Ancillary-Folgenmetadaten, keine auswählbare Capability-Integration. Unbekannte
Extensions bleiben unbekannt.

| Registrierter Provider | Relevante Funktionen/Materialinventar | Autorität für Orte/Zugriff und konkrete Folgen |
| --- | --- | --- |
| PostgreSQL | Transaktionen/Abfragen; Datenbank, Schemas, WAL, Credentials, Backups | Bestehende Datenbank-/Runtime-/Recovery-Owner; Transaktionen, Konsistenz, Dauerhaftigkeit, Reconnect und Recovery |
| Valkey | Cache/durable KV, tatsächliche Sessions/Jobs; Snapshots/AOF | Bestehende Runtime-/Daten-Owner; Ablauf/Eviction, Dauerhaftigkeit und relevanter flüchtiger Verlust |
| OpenBao | Tatsächliche Secret-/Policy-, Lease-/PKI-Nutzung; Secrets, Schlüssel/Zertifikate, Recovery | Bestehende Credential-/OpenBao-/PKI-Owner und gewählter Backing Store; sichere Auslieferung, Erneuerung, Widerruf und Ablauf |
| Caddy | HTTP-Routing und tatsächliche TLS-/Erneuerungsnutzung; Routen/Konfiguration, Schlüssel/Zertifikate | Bestehende Exposure-/PKI-/Runtime-Owner; DNS/Routen, HTTPS-Verfügbarkeit und künftige Erneuerung |
| SeaweedFS | Objektlesen/-schreiben; Objekte/Dateien, Metadaten, Credentials, Backups | Bestehende S3-/Runtime-/Recovery-Owner; Dauerhaftigkeit, S3-Kompatibilität, Rechte und Aufbewahrung |
| OpenTelemetry Collector | Empfang/Weiterleitung; tatsächliche Puffer/Konfiguration | Bestehende Telemetrie-/Runtime-Owner; Zustellungslücken, Pufferverlust, Routing und Aufbewahrung |
| External OTLP | Tatsächlicher Empfang/Weiterleitung und registrierte Referenzen | Externer Registrierungs-/Zugriffsowner; nur Unbind-/Rebind-Folgen, keine Löschung fremder Infrastruktur |
| Prometheus | Ingest/Query und tatsächliche Regeln; Metriken, WAL, Konfiguration | Bestehende Metrik-/Runtime-/Daten-Owner; Scrape-Lücken, Aufbewahrung, Abfragezugriff und Recovery |
| Loki | Ingest/Query; Logs, WAL/Index, Konfiguration | Bestehende Log-/Runtime-/Storage-Owner; Zustellungslücken, Aufbewahrung und Abfragezugriff |
| Tempo | Ingest/Query; Traces, WAL/Index, Konfiguration | Bestehende Trace-/Runtime-/Storage-Owner; Zustellungslücken, Aufbewahrung und Abfragezugriff |
| Keycloak | Tatsächliches Login/SSO, Vergabe/Validierung/JWKS, Sessions/Refresh; Benutzer/Realms/Clients/Signierschlüssel/Tokens | Bestehende Identity-/Datenbank-/Zugriffsowner; Login/Refresh, gecachte Gültigkeit und authentifizierte Management-Verfügbarkeit |
| External OIDC | Tatsächliche OIDC-Verbraucher und registrierte Referenzen | Externer Registrierungs-/Zugriffsowner; lokale Binding-Entfernung, keine Löschung fremder Benutzer/Realms |
| RabbitMQ | Tatsächliche Queue-/PubSub-/Stream-Nutzung; Nachrichten/In-flight, Offsets/Topologie/Credentials | Bestehende Messaging-/Runtime-/Storage-Owner; Reihenfolge, Acknowledgements, Zustellung, Dauerhaftigkeit und Reconnect |
| Ancillary Runtime-Broker | Tatsächliche Autorisierung/Credentials/Policy; Konfiguration, Zertifikate/Recovery | Bestehende Broker-/Runtime-/Credential-Owner; Autorisierung, Erneuerung, TLS und Verfügbarkeit |
| Ancillary Runtime-Executor | Tatsächliches Execute/Observe/Control; In-flight/Konfiguration, Zertifikate/Credentials | Bestehende Executor-/Runtime-Owner; In-flight-Arbeit, Autorisierung/TLS und Reconnect |
| MongoDB | Lesen/Schreiben/Transaktionen; Dokumente/Indizes/Oplog, Credentials/Backups | Bestehende Document-/Runtime-/Storage-/Recovery-Owner; Konsistenz, Dauerhaftigkeit, Protokoll und Reconnect |

Fixtures: `internal/application/testdata/v0425-provider-impact.json` und
`v0425-provider-impact-consent.json`. Domain-Tests umfassen jeden registrierten
Provider, negative Ownership/unbekannten State, Provider-Kaskaden und zwei Apps/
Console, Ersatzverifikation, gemeinsame Dependency-Entfernung, begrenzten Retry,
tatsächliche finale und teilweise Ergebnisse. Offline-Schemas prüfen identische
Machine-Ergebnisse und lehnen Secret-Inhalte sowie einstufige unbekannte/
destruktive Bestätigung ab. Vollständiger Capability-/Provider-Wizard: v0.4.26.

## #777 portabler Connection-Profile-Transport (nur Vertrag)

Dies ist eine Import-/Export-Transporthülle für die spätere #777-Implementierung,
**kein** Ersatz für `development.StackProfile` (development/v1), Target-Config,
Access-Store oder Provider-Registry. Diese Spezifikation liefert weder
Implementierung noch Dateierzeugung oder Onboarding-Oberfläche.

| Feld der Hülle | Vertrag |
| --- | --- |
| version | Exakt `baseharbor.connection-profile/v1`; unbekannte Version ablehnen |
| name | Anzeigename; niemals Identität oder Berechtigungsnachweis |
| core.installation_id / core.url | Bestehende Core-UUID und HTTPS-Management-Endpunkt; Serveridentität prüfen |
| trust.ca_pem / trust.sha256 | Ein öffentliches PEM-CA-Zertifikat und sein DER-SHA-256-Fingerprint; Übereinstimmung prüfen und unabhängige autorisierte Trust-Entscheidung verlangen |
| targets | Portable Deskriptoren: name, tenant_id, runtime_provider, scope; bestehende Target-Semantik erhalten |
| access | OIDC issuer, client_id und scopes; kein importierter Berechtigungsnachweis |
| expires_at | Optionale Transportablaufzeit; abgelaufenes Enrollment-Material ablehnen |
| extensions | Namespaced optionale spätere Felder; keine Änderung bestehender Pflichtsemantik |

Der Transport beschreibt Core, Trust und gewählte Targets. Er enthält weder
App-Absicht noch Provider-Credentials oder einen neuen autoritativen State.
Targets referenzieren gegebenenfalls einen Core; Profile sind keine Targets und
führen dev/prod-Installationen nicht zusammen. Core-loser Lokalbetrieb benötigt
keinen Profile-Import.

Passwörter, private Schlüssel, Refresh-/Access-Tokens, AppRole-Secrets,
Recovery-Material, lokale Sockets/Engine-Kontexte, Checkout-Pfade und State-Roots
ausschließen. Auth-/Enrollment-Artefakte werden nach explizitem Login/Enrollment
in vorhandenen geschützten Access-Ownern gespeichert. Deskriptor-Import darf keine
Rolle erteilen, fremde Deployments übernehmen, beliebige Endpunkte aktivieren
oder Ressourcen starten. Importierter öffentlicher Trust autorisiert einen
Server nicht selbst. Vor Schreiben bestehender Config-/Access-Owner Core-UUID,
HTTPS, Issuer, Tenant und gewähltes Target prüfen.

Import: parse -> validate -> Referenzen/Konflikte auflösen -> Diff zeigen ->
explizite Autorisierung -> bestehende Owner-Transaktion -> verifizieren. Gleiche
Identität/Inhalte sind idempotent; gleicher Alias mit anderer Identität ist ein
Konflikt. Teiländerungen zurückrollen oder als unvollständig mit Recovery-Aktion
anzeigen, niemals Erfolg melden. Export ist lesend und erhält portable Identität
ohne Secrets/lokale Felder. Host-lokale Werte nach Import in bestehender lokaler
Config auflösen. Die spätere Implementierung benötigt Schema-/Transaktions-
Konformität gegen diese Semantik.

## Host-Platform-Grenze (nur Vertrag)

Die semantische Grenze `baseharbor.host-platform/v1` ist Adapter-Capability-
Verhandlung, keine neue Runtime, Registry, Scheduler oder Multi-OS-Implementierung.

| Verantwortung | Grenze |
| --- | --- |
| Plattform-/Feature-Erkennung | Host-Facts und supported/unsupported/unverifiable mit Nachweis |
| Engine-Auswahl | Tatsächlicher Endpoint, Kontext und rootless/rootful; kein stiller privilegierter Fallback |
| Lokale Pfade / Source-Zugriff | Host-lokale Zuordnung außerhalb Manifest/Profile; deterministischer Fehler bei fehlender Source |
| Credential-/Trust-Zugriff | Bestehender geschützter Owner und explizite Trust-Änderungen; keine automatische Sicherheitsabschwächung |
| Browser / PTY / Shell | Tatsächliche Unterstützung oder typisierte Nichtverfügbarkeit; Non-TTY/EOF/Cancel erhalten |
| Runtime-Ausführung | Bestehender RuntimeProvider verantwortet Lifecycle und Provider-Verifikation |
| Enrollment / Remote-Autorität | Bestehende Core-/Node-Autorität und mTLS; lokaler Host-Adapter erzeugt keine Remote-Autorität |

Linux-, WSL2- und Darwin-Implementierungen folgen später. Adapter melden echte
Unterstützung, statt Erfolg nicht unterstützter Operationen vorzutäuschen.
Host-Pfade, Sockets und Plattform-Defaults verändern weder ApplicationID,
Capability-Absicht, Target-Ownership, Footprint-Evidenz noch Machine-Fehler.
Kein Host-Probing mit dauerhaften Effekten aus Inspektion/Footprint.


Normative Offline-Schemas: `contracts/footprint/v1/footprint.schema.json` und `contracts/connection-profile/v1/profile.schema.json`. Profile-Shape-Validierung beweist weder gültiges CA-Material, Fingerprint-Übereinstimmung, Ablaufzeit, eindeutige Tenants noch Autorisierung; spätere Importer müssen diese Semantik prüfen und nur geprüfte nicht geheime Extensions zulassen.

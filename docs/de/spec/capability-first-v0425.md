# Capability-first Application-Verträge (v0.4.25)

Scope: [#872](https://github.com/mcpdev80/baseharbor/issues/872).
Diese Verträge ersetzen den bedingungslosen Core-Bootstrap der ersten Anwendung
für Application-Deployments. Die bereits gelieferten Management-Core-Pflichten
aus #810 bleiben bestehen. Korrekturen vor v0.5 benötigen eine ausdrückliche
Vertragsprüfung; spätere Releases konsumieren diese Semantik.

## Autorenformat und Identität

Das minimale Entwickler-Manifest verwendet den bestehenden Parser und Version:

```yaml
version: 1
app:
  name: myapp
  environment: dev
```

SQL bleibt `services.sql: true` (unterhalb von `services`). Die ausführliche Form
`services.sql.enabled: true`, benannte Instanzen und bestehende Felder bleiben
gültig. Services sind optional. Top-Level-SQL und ein zweiter Kurzformat-Parser
werden nicht unterstützt. Ein leeres Dokument oder alleinstehendes `{}` liefert
Version/Name/Environment nicht; Erkennung muss echte Absicht liefern und darf
keinen ausführbaren Workload erfinden.

`New` erhält explizite Flags und erzeugt eine ID für neue kanonische Objekte.
Sind alle Flags false, wird kein SQL hinzugefügt. `ParseYAML` erlaubt fehlende
IDs, erzeugt niemals eine und prüft Autorenabsicht mit `ValidateIntent`.
`Validate` erhält die Voraussetzungen ausführbarer Manifeste. Gültige
Autorenabsicht beweist weder vorhandene Quellen noch laufende Container oder
Bereitschaft.

| Operation | ID und Schreibverhalten |
| --- | --- |
| Parse / inspect / plan | Bestehende Owner abgleichen; neue unaufgelöste ID bleibt leer; keine Dateisystem-/Runtime-Erzeugung |
| Autorisiertes init / Registrierung | Unterstützten Workload erkennen, Owner abgleichen, nur bei wirklich neuer Anwendung einmal erzeugen und im bestehenden Store speichern |
| Wiederholung / anderes Target | Kanonische ID wiederverwenden; gestoppte Engine oder anderes Target erzeugt keine neue ID |
| Bestehende Autoren-UUID | Erhalten; Widerspruch zu Store-/Deployment-Identität ist ein Fehler |
| Früher registriert, ID fehlt | Owner wiederherstellen/abgleichen; keine stille Ersatz-ID |
| Mehrere UUIDs für einen Owner | Klar ablehnen; ausdrücklicher Ownership-Abgleich nötig |
| Umbenennung mit bestehendem Store-Owner | Owner-Abgleich verlangen; keine unbemerkte doppelte Identität |

`ResolveIdentity` ist rein und konsumiert zugeordnete Claims bestehender Owner.
`Store.InitializeIdentity` verwendet die vorhandene kanonische Repository-
Store-Kopie `.baseharbor/apps/<name>/baseharbor.yaml`, wenn Autoren-YAML keine
ID enthält. Diese Kopie ist für diese Eingabe der einzige dauerhafte ID-Owner.
Autoren-UUIDs bleiben autoritativ und müssen mit der Kopie übereinstimmen. Keine
neue ID-Datei oder Registry. Der Target-Deployment-Store ist eine Projektion und
kann portable Identität nicht initialisieren: sein Pfad benötigt bereits die UUID.

Adapter müssen vor Initialisierung bestehende kanonische Repository-/Source-
Ownership und Deployment-Records abgleichen. Gleicher App-Name in fremden Repos
ist kein hinreichender Nachweis. Repository-YAML wird niemals implizit geändert.
Konkurrierende Initialisierungen übernehmen den bestehenden Store.Create-
Gewinner oder melden unvollständige Initialisierung zum Wiederholen; sie ersetzen
den Gewinner nicht. Lesende Adapter dürfen Create, InitializeIdentity und Sync
nicht aufrufen.

## Provider-Auflösung und Footprint

`PortableContractFromManifest` bleibt portable Capability-Absicht.
`ProviderPreference` enthält eine optionale konkrete Instanz-ID im vorhandenen
Deployment-/Operator-Binding-State, außerhalb des Entwickler-YAML; leer bedeutet
automatic. Provider-Produkt, Distribution-ID und Instanz-ID sind unterschiedliche
Identitäten. Auswahl prüft das tatsächliche Capability-Angebot über die bestehende
`capability.Registry` Version 2.

`ResolveFootprint(manifest, snapshot, preferences)` konsumiert Registry und
Facts vorhandener Pläne/Observer eines ausgewählten Targets. Kein Discovery-,
Dateisystem-, Registry-, Lifecycle-, Enrollment- oder Scheduling-Effekt. Facts
müssen das Target sowie verifizierten Provider-State, erklärten Core-Bedarf und
tatsächlich gewählte Dependency-Instanz-IDs enthalten. Nil-Abhängigkeiten bedeuten
unbekannt; leere Liste bedeutet verifiziert keine. Unbekannte Deklarationen nicht
durch Vermutungen ersetzen. Fremde Targets, Zyklen, fehlende Dependencies und
fremde Ressourcen werden klar abgelehnt.

| Fall | Ergebnis / Verhalten |
| --- | --- |
| Keine Capabilities | Core nicht erforderlich; keine gewählten Provider-Ressourcen; ungenutzte bestehende Ressourcen erhalten |
| Automatic, kompatible Instanz | Genau eine zulässige kompatible Instanz auswählen; Frage nur bei Mehrdeutigkeit |
| Bestehendes Binding | UUID-/Environment-/Resource-Binding wiederverwenden; expliziter Widerspruch scheitert |
| Explizit missing / incompatible | Typisierter Fehler vor Änderungen |
| Mehrere zulässige Instanzen | Typisierter ambiguous-Fehler; keine beliebige erste Auswahl |
| Fremder Owner / Target | Typisierter foreign-/foreign-target-Fehler; keine Adoption oder Änderung |
| Shared / reused | Physische Instanz einmal aufnehmen; registrierte Verbraucher erhalten |
| External / BYO | Externe Ownership, kein eigener Provisionierungs-/Restart-Schritt; SQL impliziert keinen verwalteten Core |
| Automatic ohne Instanz | Requested/unbound, Core unknown; vorhandener Planner muss vor Provisionierung eine unterstützte Deklaration auflösen |
| Keine Runtime-Beobachtung | Unverifiable, unvollständig; Registry-Eintrag beweist weder Betrieb noch Abwesenheit |
| Gestoppte eigene Instanz | Bestehende Ressource, stopped-but-owned; bestehender Lifecycle startet sie erneut, keine Duplikate |
| Deklarierte neu benötigte Instanz | Additional; tatsächlichen Provider-Plan verwenden, keine Topologie ableiten |
| HTTPS | Fügt keine Identity hinzu; Dependencies kommen aus gewählten Provider-Facts |
| Expliziter Management-Core | SQL/Secrets/Identity und Sicherheit unabhängig vom App-Bedarf Pflicht |

Die gemeinsame JSON-Projektion heißt `baseharbor.footprint/v1`:

| Feld | Bedeutung |
| --- | --- |
| version / target / application_id | Version, gewähltes Target und bekannte kanonische UUID |
| requirements | Angeforderte Capability, bound oder requested/unbound, gewählte Instanz und beobachteter State |
| existing / additional | Bestehende/shared/gestoppte/unverifizierbare Instanzen gegen deklarierte zusätzliche Anforderungen |
| unused | Target-weite Registry-IDs ohne verbleibende Bindings und außerhalb der gewählten Dependency-Union; **keine** Löschfreigabe |
| core | required / not-required / unknown; unknown autorisiert weder Core-losen noch verwalteten Eingriff |
| complete | Strukturelle Auflösung vollständig; false bei ungebundenen Anforderungen, unbekanntem Core/Dependencies oder unverifizierbarem State |
| scope / ownership / shared / consumers | Bestehende Registry-Scope/-Ownership; consumers zählt Bindings, keine erfundene App-Anzahl |
| memory_bytes / containers | Pro Instanz measured / estimated / unknown mit Herkunft; unknown ohne Wert |

Measured/estimated benötigt nichtnegative tatsächliche Werte und Herkunft.
Schätzungen kommen aus geprüften Provider-Adaptern und werden nicht vom Resolver
erfunden. Unbekannte Messwerte sind bei strukturell vollständiger Auflösung
zulässig. Keine festen RAM-/Container-Versprechen oder unbelegten Summen.
Die bestehende vollständige Management-Core-Topologie bleibt erhalten. Entfernen
einer Capability verändert App-Absicht/Binding-Pläne; ungenutzte Provider bleiben
sichtbar und zugeordnet. Freigabe ist eine gesondert autorisierte spätere Operation.

`ManagementCoreRequirements` erhält #810. Lifecycle-Verbraucher verwenden
`Footprint.Core` als gemeinsame frühe Entscheidung: not-required erlaubt
unterstützte lokale Workloads ohne Core/Login; required nutzt autorisierte
bestehende Wiederverwendung/Bootstrap; unknown blockiert voreilige Änderungen
bis zur Auflösung. Kein ungesichertes `baha serve`, keine Abschwächung von
test/prod oder Remote-Sicherheit, kein Entfernen bereits installierter Ressourcen.

Human-CLI, JSON und MCP verwenden denselben Domain-Befund und vorhandene
Machine-Fehler-/Exit-Zuordnung. Keine CLI-eigene Provider-Auswahl oder zweiter
Identity-State. Resolver-Fehler sind über `ResolutionError.Code` typisiert;
Adapter nutzen bestehende Ownership-, Conflict-, Validation- und Not-Found-
Verträge. Diese reine API führt keinen neuen Befehl/Transport ein.
Ausführbare Fixtures: `internal/application/testdata/v0425-resolution.json`.

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

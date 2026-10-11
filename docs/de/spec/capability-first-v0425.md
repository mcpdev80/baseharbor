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
Bindings oder unverifizierbare Facts machen den Plan unvollständig und blockieren
sämtliche Stop-/Entfernungsentscheidungen.

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
Ownership-Nachweis. Weiterhin verwendete Shared- und fremde/externe Ressourcen
niemals löschen. Wiederholtes Apply, unterbrochener Retry und Deaktivieren/
Reaktivieren konvergieren idempotent. Capability-Entfernung gehört zu v0.4.25.

Human-CLI, JSON und MCP verwenden dieselben Domain-Ergebnisse und vorhandene
Machine-Fehler-/Exit-/Autorisierungszuordnung. ResolutionError bleibt typisiert.
Fixtures: `internal/application/testdata/v0425-resolution.json`; Union-Invarianten:
`internal/application/provider_union_test.go`. Vollständiger Provider-/Capability-
Wizard: v0.4.26. #777-Profile-/Host-Verträge unten bleiben reine Spezifikation.

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

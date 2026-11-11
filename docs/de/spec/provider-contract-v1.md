# Anbietervertrag v1

## Anwendungsbereich

Diese Spezifikation legt die gemeinsamen Regeln für Anbieter fest.

## Versorgerachsen

Laufzeit, Kapazität und Lieferer sind unabhängige Achsen.

```text
runtime != capability != delivery
```

Implementierungen MÜSSEN KEINE Achse zu einer versteckten Anforderung einer anderen machen.

## Standards – erste Interoperabilität

Anbieter MÜSSEN einen etablierten offenen Standard verwenden, wenn er bereits die erforderliche interoperable Semantik definiert. De-facto-Standards und etablierte Ökosystemkonventionen SOLLTEN dort verwendet werden, wo kein geeigneter formaler Standard existiert.

Anbieterspezifische Verträge MÜSSEN hinter der Provider-Grenze bleiben. Ein Anbieter MÜSSEN BaseHarbor Core oder portable Applikationsabsichten nicht verlangen, Produktspezifische Konfigurationen nur zu übernehmen, weil dieser Anbieter sie braucht.

Bevor eine neue Serviceart oder ein neuer Anbieter implementiert wird, werden folgende Konstruktionsprotokolle erstellt:

- Bestehende Normen
- Angenommene Normen
- Abweichungen
- BaseHarbor-Erweiterungen
- Auswirkungen auf die Kompatibilität

## Identität und Versionierung des Anbieters

Eine Providerimplementierung MUSS eine stabile Provider-ID und Implementierungsversion unabhängig von BaseHarbor und unabhängig von Capability Specification-Versionen ausstellen.

Zum Beispiel:

```text
provider: baseharbor/postgresql
provider version: 0.1.0
protocol: baseharbor.provider/v1
capability: database.sql/v1
```

Eine konkrete Produktversion oder Bildverdauung ist Realisierung Metadaten. Es MUSS NICHT ersetzen die Provider-Implementierung Version oder die Fähigkeit Spezifikation Version.

## Fähigkeiten

Ein Anbieter MUSS die Semantik, die er unterstützt, deklarieren.

BaseHarbor MUSS einen Anbieter vor der Mutation zurückweisen, wenn eine erforderliche Semantik nicht unterstützt wird.

Nominale Fähigkeits-Namen-Gleichstellung allein MÜSSEN NICHT als Kompatibilität behandelt werden.

## Service-Anschluss-Ausgänge

Provider-Verbindungsausgänge richten sich nach Service Binding Specification 1.1 bekannten Eintragsnamen, wann immer die Semantik zusammenpasst:

```text
type
provider
host
port
uri
username
password
certificates
private-key
```

Alternative Aliasnamen wie z.B.`hostname `, ` connectionHost `, ` user `, ` pass ` or ` connectionString`MÜSSEN NICHT eingeführt werden, wenn der Standardname gilt.

Geheime Einträge können intern durch geschützte Referenzen dargestellt und nur an der vertrauenswürdigen Arbeitsbelastungs-Projektionsgrenze aufgelöst werden. Standardbenennungen erlauben keine Klartext-Geheimnisse in der Diagnostik, im Registry-Status oder in der portablen Absicht.

BaseHarbor-spezifische Binding-Metadaten verwenden einen explizit versionierten Erweiterungsnamensraum und bleiben von Service Binding-Feldern getrennt.

## Platzierung und Eigentum

Capability Provider verwenden das gemeinsame Platzierungsvokabular, bei dem die zugrunde liegende Produkt- und Providerimplementierung die erforderliche Semantik erfüllen kann:

```text
shared
application
external
```

Ein Anbieter Deskriptor MUSS jeden Platzierungsbereich, den er tatsächlich implementiert, deklarieren. BaseHarbor MUSS einen nicht unterstützten angeforderten Umfang vor der Mutation ablehnen und MUSS NICHT stillschweigend die Isolation oder das Eigentum herabsetzen.

Für `shared` Vermittlung:

- Der Lebenszyklus der Anbieterinfrastruktur MUSS eher zur Ziel-/Anbietergrenze als zu einer Anwendung gehören;
- Anwendungsdaten/logische Ressourcen, Anmeldeinformationen, Identitäten und Servicebindungen MÜSSEN anwendungsskopiert bleiben;
- eine Anwendung MÜSSEN die logischen Ressourcen einer anderen Anwendung NICHT lesen, mutieren oder zerstören können, indem die Standardanwendung verbindlich ist;
- die Vernichtung der Anwendung MUSS nur anwendungseigene Ressourcen entfernen und den gemeinsamen Anbieter und Geschwister-Anwendungen bewahren;
- Anbieter/Ziel zerstören MAY den gemeinsam genutzten Anbieter nach Freigabe des Anwendungseigentums zu entfernen;
- ein Anbieter MUSS NICHT geltend machen `shared` Unterstützung, wenn diese Isolations- und Lebenszyklusgarantien nicht eingehalten werden können.

Die Freigabe der Provider-Infrastruktur ist explizit ein Ressourceneffizienz-Mechanismus. Sie MUSS NICHT durch bloße Wiederverwendung eines globalen Application Credentials oder eines unpartitionierten Application Data Namespaces implementiert werden.

`application` Die Platzierung stellt einen speziellen Lebenszyklus eines Anbieters für eine Anwendung/Umgebung dar und bleibt gültig, wenn eine stärkere physische Isolierung oder Produktbeschränkungen dies erfordern.

`external` Platzierung hält Anbieter Lebenszyklus Eigentum außerhalb BaseHarbor. BaseHarbor MUSS NICHT destruktiv mutieren ausländische / externe Ressourcen, die es nicht besitzt.

Eine benannte Sharing-Grenze MAI Unterteilung `shared` Vermittlung ohne Einführung eines vierten Anwendungsbereichs.

## Beglaubigte Besitzsteuer

Provider-Implementierungen MUSS die normative[Credential and access ownership v1](credential-access-v1.md)Taxonomie.

Anbieter MÜSSEN die Unterscheidung zwischen Human/Management-Identität, Application-Service-Anmeldeinformationen und BaseHarbor-interne Maschinenidentität beibehalten.

Ein Anbieter MÜSSEN interne Maschinenanmeldeinformationen NICHT durch freigegebene Human-/Developer-Anmeldeinformationen ersetzen, und die gemeinsame Provider-Infrastruktur MÜSSEN keine gemeinsamen Anmeldeinformationen für den Anwendungsservice implizieren.

Anwendung Business-Benutzer, Gruppen, Rollen und Berechtigungen bleiben Anwendung / IdP-Eigenschaft und außerhalb des Anbieter Credential-Modell.

## Überprüfung

Ein Anbieter Berichtsprozess Gesundheit ist nicht ausreichend, wenn die Fähigkeit erfordert Protokoll / Datenfluss Überprüfung.

## Übergabe des Container-Dateisystems

Provider, die Containerdateien erzeugen, MÜSSEN den vorgesehenen Verbraucher
deklarieren und den Datei- und Verzeichniszugriff unabhängig von der Host-umask
für dessen effektive UID/GID sicherstellen. Ein erfolgreicher Schreibvorgang
auf dem Host beweist nicht, dass der Container die Datei lesen oder den Pfad
durchlaufen kann. Nicht geheime Projektionen SOLLTEN mit den engsten passenden
Modi (Dateien `0644`, Verzeichnisse `0755`) lesbar sein und MÜSSEN
schreibgeschützt eingebunden werden, sofern der Verbraucher nicht schreiben
muss. Geheime Quelldateien bleiben auf dem Host eigentümergeschützt. Eine für
nicht-root benötigte Secret-Kopie MUSS verbraucherbezogen unter einem
geschützten Elternverzeichnis liegen. Provider DÜRFEN Zugriffsfehler weder mit
weltbeschreibbaren Modi wie `0777` noch durch Ausführen als root oder durch
Aufweiten der ursprünglichen Secret-Rechte beheben.

Provider-Lifecycle und Konformitätsprüfungen MÜSSEN den Verbraucherzugriff
unter restriktiver umask und den tatsächlichen Lese- oder Protokollpfad prüfen.
Eine Container-/Provider-Bereitschaft allein reicht nicht.

### Dateisystemanforderungen ausgelieferter Provider

Diese Tabelle ist ein verbindlicher Teil des Vertrags. Provider-Deskriptoren
MÜSSEN sie für die konkrete Topologie um Image/Version, effektive UID/GID,
Mount-Ziel und Zugriff ergänzen. Provider DÜRFEN keine numerische UID
festschreiben und UID `0` nicht voraussetzen. Bei Docker/Compose erhält das
Weglassen von `user` die Image-Standardidentität; OpenShift kann eine beliebige
nicht-root UID zuweisen. Images und Pfade MÜSSEN diese UIDs unterstützen.
Beobachtete Docker-UIDs sind Diagnosewerte, keine Provider-Anforderungen.

| Provider / Verbraucher | Identität und Dateisystemanforderung |
|---|---|
| PostgreSQL-Mitglied | Beliebige Plattform-UID; Datenvolume über Laufzeit-Ownership/Gruppe schreibbar. Key, Zertifikat, CA und `pg_hba.conf` schreibgeschützt; Key-Quelle eigentümergeschützt. |
| PostgreSQL-HAProxy | Beliebige Plattform-UID; `haproxy.cfg` und Serverzertifikat lesbare, schreibgeschützte Projektionen; kein Datenbankvolume. |
| pgAdmin | Beliebige Plattform-UID; `servers.json`, `pgpass`, CA, UI-Kennwort, TLS-Zertifikat/Key nur als geschützte, schreibgeschützte, auf pgAdmin beschränkte Projektionen. |
| Valkey | Gepinntes BaseHarbor-Ableitungsimage mit lokalem Upstream-Konto `valkey`, Compose-UID offen; `/data` gruppenbeschreibbar/-ausführbar für GID `0`. AOF-Dateien mit restriktiver gruppenkompatibler umask. Bestehende Valkey-eigene AOF-Dateien beim Start zu GID `0` und Gruppenrechten migrieren; bei nicht sicher migrierbaren Daten mit verständlichem Fehler abbrechen. ACL und TLS schreibgeschützt, ACL nur für Valkey projiziert. |
| Valkey-HAProxy | Beliebige Plattform-UID; `haproxy.cfg` und Zertifikat lesbar, schreibgeschützt; keine Zugangsdaten oder Providerdaten. |
| Redis Commander | Beliebige Plattform-UID; `local.json`, `local-production.json`, HTTP-Kennwort und TLS als geschützte, schreibgeschützte UI-Projektionen. |
| MongoDB | Beliebige Plattform-UID; Datenvolume laufzeitgestützt beschreibbar. Init-Skripte, Zertifikat und CA schreibgeschützt; Key-Quelle geschützt. |
| MongoDB-UI | Beliebige Plattform-UID; MongoDB-CA/Trust schreibgeschützt; UI-Proxy isoliert, Datenbankzugang nur über authentisierten Upstream. |
| MongoDB-UI-Proxy | Beliebige Plattform-UID; Caddyfile, Zertifikat und Key schreibgeschützt; Key nur als geschützte Verbraucherprojektion. |
| RabbitMQ | Beliebige Plattform-UID; Brokerdaten laufzeitgestützt beschreibbar; Konfiguration und TLS schreibgeschützt, Zugangsdaten/Keys geschützt und beschränkt eingebunden. |
| RabbitMQ-UI-Proxy | Beliebige Plattform-UID; Caddyfile, Zertifikat und Key schreibgeschützt; Authentifizierungsprüfsumme nur für diesen Proxy. |
| SeaweedFS-Knoten/Admin | Docker-Image-Entrypoint nutzt nur `CHOWN`, `SETGID` und `SETUID`, um `/data`-Eigentümerrechte zu korrigieren und auf das Dienstkonto zu wechseln; alle übrigen Capabilities bleiben entfernt und `no-new-privileges` bleibt aktiv. Auf Plattformen mit beliebiger UID muss diese `/data` über die Volume-Eigentümer-/Gruppenrechte beschreiben können. S3/Admin-Zugangsdaten und TLS sind an der Quelle geschützt und je Verbraucher auf schreibgeschützte Projektionen begrenzt. |
| SeaweedFS-Proxy | Beliebige Plattform-UID; Caddy-Konfiguration und TLS schreibgeschützt; privater Key verbraucherbezogen. |
| OpenBao-Server | Beliebige Plattform-UID; Konfiguration, CA, Server-Key/-Zertifikat und PostgreSQL-CA schreibgeschützt; OpenBao-Speicher laufzeitgestützt beschreibbar. |
| OpenBao-HAProxy | Beliebige Plattform-UID; HAProxy-Konfiguration lesbar/schreibgeschützt; keine OpenBao-Zugangsdaten oder Speichermounts. |
| OpenBao-Adminoberfläche | Beliebige Plattform-UID; Admin-CA/Trust und Konfiguration schreibgeschützt; Zugangsdaten nur über Secret-Kanäle, nie breit lesbar. |
| Keycloak | Beliebige Plattform-UID; Daten laufzeitgestützt beschreibbar; TLS und DB-CA schreibgeschützt, private Daten geschützt. |
| Keycloak-Proxy | Beliebige Plattform-UID; Caddy-Konfiguration, Upstream-Trust, Zertifikat und Key schreibgeschützt und verbraucherbezogen. |
| Prometheus | Beliebige Plattform-UID; Konfiguration/Targets lesbar und durchquerbar, Datenvolume laufzeitgestützt beschreibbar; Scrape-Keys/-Zertifikate verbraucherbezogen schreibgeschützt. |
| Prometheus-Proxy | Beliebige Plattform-UID; Caddy-Konfiguration, Upstream-Trust und TLS schreibgeschützt; private Keys nur für diesen Proxy. |
| OpenTelemetry Collector | Beliebige Plattform-UID; Konfiguration, CA, Clientzertifikat und Key schreibgeschützt; nur deklarierte Queue-/Speicherpfade beschreibbar. |
| Loki | Beliebige Plattform-UID; Konfiguration/Trust schreibgeschützt; nur Datenpfade unter `/loki` beschreibbar; Object-Storage-Zugangsdaten nicht in Konfiguration. |
| Grafana Alloy | Beliebige Plattform-UID; Alloy-Konfiguration schreibgeschützt. Agent-Zustand liegt auf einem temporären Dateisystem mit Modus `1777`, damit jede zugewiesene Nicht-root-UID ihn anlegen kann; dauerhaft gespeicherte Logs gehören zu Loki. Host-Journal ausschließlich über plattformgestützte schreibgeschützte Gruppe/Mount, nicht per UID `0`. |
| Tempo | Beliebige Plattform-UID; `tempo.yaml` und öffentliche Object-Storage-CA schreibgeschützt, `/var/tempo` beschreibbarer Datenpfad; S3-Zugangsdaten nicht in `tempo.yaml`. |
| Runtime-Broker/Executor | Beliebige Plattform-UID; Runtime-Konfiguration, Servicetokens, Workload-Identität und Operationspfade schreibgeschützt, außer deklariertem Status; Keys/Tokens einzelne Secret-Mounts. |
| Anwendungsworkload | Beliebige Plattform-UID; Bindings, CA-Bundles und Runtime-Identität schreibgeschützt; nur deklarierte Anwendungsdatenpfade beschreibbar. |
| Caddy-Exposure und Providerzugriffsproxy | Beliebige Plattform-UID; Caddy-Konfiguration/Zertifikate schreibgeschützt; nur deklarierte Status-/Cachepfade beschreibbar; private Keys verbraucherbezogen. |

Zusätzlich gelten für jede Zeile folgende Regeln:

- Nach Schreiben oder atomarem Ersetzen MUSS der Provider den effektiven
  Dateimodus ausdrücklich setzen und mit `stat` prüfen. Die angeforderte
  Erstellungsberechtigung allein genügt bei restriktiver umask nicht.
- Provider MÜSSEN beliebige nicht-root UIDs unterstützen und DÜRFEN
  `runAsUser`, Compose `user` oder ein Äquivalent nicht auf eine feste
  numerische UID setzen. Dateizugriff erfolgt über schreibgeschützte Mounts,
  Laufzeit-Ownership oder kompatible Gruppenrechte, nie mit UID `0` als
  Voraussetzung.
- Nicht geheime Konfiguration und öffentliche Zertifikate MÜSSEN jeder
  zugewiesenen UID lesbar sein, ohne Schreibzugriff zu erlauben. Secret-Material
  MUSS über verbraucherbezogene schreibgeschützte Mounts und Laufzeit-Ownership
  oder Gruppenmechanismen bereitgestellt werden, nie über weltweit lesbare
  Quelldateien. Bei OpenShift-kompatiblen Images MÜSSEN beschreibbare Imagepfade
  GID `0` gehören und, soweit erforderlich, gruppenbeschreibbar/-ausführbar sein
  (`g=u`); das ist keine UID-`0`-Voraussetzung.
- Private Keys, Zugangsdaten, Tokens und Secret-Speicher MÜSSEN auf dem Host
  geschützt sein (üblicherweise Dateien `0600`, Elternverzeichnisse `0700`).
  Ihre Quellrechte DÜRFEN NICHT aufgeweicht werden.
- Bind-Mounts MÜSSEN schreibgeschützt sein, außer genau dieser Pfad muss
  beschreibbar sein. Datenpfade sind von Konfiguration/Secrets getrennt und
  verwenden deklarierte Laufzeit-Ownership oder Gruppenrechte.
- Datei-Ersatz aktualisiert keinen laufenden Bind-Mount. Provider MÜSSEN den
  Verbraucher passend neu laden/erstellen und vor Bereitschaft oder
  Rotationsabschluss den aktiven Inode/die aktive Konfiguration prüfen.
- Bereitschaft MUSS Verbraucher-Lesezugriff und Provider-Protokoll-/Datenpfad
  prüfen, nicht nur Host-`stat`, Dateierstellung oder Containerstatus.

Implementierungen MÜSSEN jede Tabellenzeile auf konkrete Mount-Quellen,
Dateimodi, Verzeichnis-Durchquerungsrechte, Arbitrary-UID-Verhalten und
Reload-/Neuerstellungsmaßnahmen abbilden und testen. Neue Provider und
Verwaltungsoberflächen MÜSSEN ihre Anforderungen hier ergänzen, bevor sie als
unterstützt gelten.

## Registergrenzen

BaseHarbor unterscheidet zwei Register:

- **Runtime Provider Registry** — bereitgestellte Provider-Instanzen, Platzierung, Eigentümerschaft und Anwendungs-/Ressourcenbindungen.
- **Provider Catalog** — installierbare Provider-Distributionen, Versionen, unterstützte Service-Verträge/Fähigkeiten, Produktkompatibilität, Plattformen, OCI-Artefakt-Identität, Digest, Schema und Provenienz.

Die Runtime Provider Registry MUSS NICHT zu einem Paket-/Distributionskatalog werden.

Provider-Artefakte SHOULD verwenden OCI-kompatible Distribution. Artefakt-Verdauung ist unveränderliche Identität; Tags sind Entdeckungs-Aliasen.

## Erweiterbarkeit

Provider-Implementierungen MAY verwenden ausgereifte OSS-, Standard-APIs/SDKs, Controller/Operatoren oder Managed-Service-APIs hinter dem BaseHarbor-Vertrag.


## Standards-erste Anbieter Metadaten

Provider-Metadaten MÜSSEN diese Versionsachsen unabhängig halten:

```text
BaseHarbor service contract version
Provider protocol version
Provider implementation version
Product/engine version
Artifact digest
```

Ein Anbieter-Deskriptor SHOULD identifizieren:

- stabile Anbieter-ID und Provider-Version;
- unterstützte Service-Arten und BaseHarbor Service/Kapazität Vertragsversionen;
- Protokoll-/Semantische Fähigkeiten;
- gegebenenfalls unterstützte Produkt-Motor-Versionen;
- unterstützte Plattformen;
- OCI-Artefaktreferenz und unveränderliche Verdaulichkeit, wenn sie als Artefakt verteilt werden;
- JSON-Schema 2020-12-Anbieterkonfigurationsschema;
- Standard-Signatur/SBOM/Beweisvermerke, soweit vorhanden.

Das kanonische Zielschema ist `contracts/provider/v1/provider-descriptor.schema.json`.

## Dienstleistungsbindungen

Provider-Verbindungsausgänge MUSS Service Binding Specification 1.1 bekannte Eintragsnamen verwenden, wo ihre Semantik gelten.

BaseHarbor-spezifische Bindungserweiterungen MÜSSEN in einem separaten versionierten Erweiterungsnamensraum verbleiben und MÜSSEN keine Standardnamen neu definieren.

geheime Werte können undurchsichtige Verweise bis zur vertrauenswürdigen Projektionsgrenze bleiben; dies rechtfertigt keine alternativen Feldnamen.

## Dienst versus Protokoll

Ein Anbieter implementiert einen BaseHarbor Servicevertrag und MAY erklärt Protokollkompatibilität.

Beispiele:

- `cache` ist der Dienst; RESP ist eine Protokoll-Kompatibilitäts-Eigenschaft.
- `object-storage` ist der Dienst; S3 API-Kompatibilität ist eine Protokoll/API-Eigenschaft.
- `observability` ist die Service-Familie; OpenTelemetry/OTLP ist der Standard-Telemetrieprotokoll/Datenpfad.

Anbieter-Produktnamen werden nie zu tragbaren Anwendungsservice-Arten.


## Zugang zum Management-Oberflächen-Zugang

Mensch-zugewandte Anbieter-Management-Oberflächen folgen[Management surface access v1](management-access-v1.md). Anbieter deklarieren die effektive Authentifizierungsklasse und das Rollenmapping; BaseHarbor MUSS KEINE stärkere Autorisierung ableiten, als der Anbieter durchsetzen kann.

Die normative Versand-Provider/Oberflächenklassifikation ist[Provider and management-surface acceptance v1](provider-management-acceptance-v1.md).

## Verfügbarkeit

Kapazitätsanbieter verhandeln unabhängig vom Runtime Provider die gleichen Anforderungen an die tragbare Verfügbarkeit.

Anbieter erklären SUPPORTED, PARTIALLY_SUPPORTED oder UNSUPPORTED mit expliziten Grenzen. Eine erforderliche nicht unterstützte Garantie scheitert vor der Provider-Mutation. Provider-native Clustering, Quorum, Replik-Rollen und Managed-Service-Produktmodi bleiben Realisationszustand und nie in portable Application Intent eingeben.

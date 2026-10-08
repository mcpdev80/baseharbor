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

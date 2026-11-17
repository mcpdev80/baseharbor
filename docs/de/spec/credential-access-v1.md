# Beglaubigungs- und Zugangsrecht v1

## Anwendungsbereich

Diese Spezifikation definiert die normative BaseHarbor Credential Taxonomie und Eigentumsgrenze.

Es klassifiziert das von BaseHarbor verwaltete Material und Identität. Es erstellt kein neues IAM-, RBAC-, Secret-Store- oder Provider-Protokoll.

Die Taxonomie gilt konsequent für Anbieterimplementierungen, Managementflächen, sichere Bindungen, CLI, Maschine JSON, MCP, Status, Doctor und Nachweis.

## Beglaubigungsklassen

### A. Identität des Menschen / des Managements

Klasse A stellt nur den Zugang zum Menschen dar, der auf die Infrastruktur ausgerichtet ist.

Beispiele sind Managed Identity Login, Provider/Management GUIs und BaseHarbor-managed Infrastrukturrollen.

Das Rollenprofil des portablen Infrastrukturmanagements ist:

```text
admin
edit
view
audit
```

Diese Rollen MÜSSEN NICHT zu Anwendungsgeschäftsrollen werden.

### B. Anmeldeinformationen für den Bewerbungsservice

Klasse B stellt technische Referenzen dar, die von einer Anwendung verwendet werden, um eine Fähigkeit zu verbrauchen.

Beispiele dafür sind PostgreSQL, Valkey/cache, RabbitMQ/messaging, MongoDB/documente database, object storage/S3 und technische Anwendungs-Client-Geheimnisse.

Klasse-B-Anmeldeinformationen bleiben für die Anwendung, Umgebung und logische Ressource, wie durch den Fähigkeitsvertrag erforderlich. Sie MÜSSEN NICHT als menschliche Identität behandelt werden.

### C. BaseHarbor-interne Maschinenidentität

Klasse C stellt interne Maschinenidentitäts- und Kontrollanmeldeinformationen dar.

Beispiele sind mTLS private Schlüssel, SPIFFE/Workload-Identitäten, Laufzeit-Broker-Token, OpenBao AppRolles, Provider-Control/Provider-Admin-Anmeldeinformationen und interne Service-to-Service-Anmeldeinformationen.

Klassen-C-Anmeldeinformationen MÜSSEN isoliert bleiben und MUSS NICHT gemeinsam genutzte Human-/Entwickler-Anmeldeinformationen oder Anmeldeinformationen für den Application-Service erben oder wiederverwenden.

## Die geschäftliche Identität der Bewerbung ist nicht anwendbar.

Anwender, Gruppen, Rollen und Berechtigungen von Anwendungen sind Eigentum der Anwendung und ihres ausgewählten Identitätsanbieters.

BaseHarbor MUSS keine parallele Anwendungs-User-Datenbank oder ein proprietäres Business-RBAC-Modell unterhalten.

Anwendungen können normale OIDC, OAuth2, Ansprüche, Gruppen, Kundenrollen oder ein anderes ausgewähltes externes Identitätssystem verwenden.

## Umweltpolitik

### Entwicklung

- Klasse A MAY verwenden eine Ziel-Scope-Entwickler-zugewandte Credential auf kompatiblen BaseHarbor-managed nicht-OIDC-Oberflächen, wo technisch sicher.
- Native OIDC/OAuth2 bleibt bevorzugt, wenn eine Oberfläche sie unterstützt.
- Klasse B MAI verwenden einfache oder gemeinsame Voreinstellungen nur, wenn Anwendung/Umwelt/Ressourcen-Scope und erforderliche Isolation korrekt bleiben.
- Klasse C MUSS isoliert bleiben.

### Prüfung und Produktion

- Klasse A SHOULD verwendet zentral verwaltete Identität/OIDC, wo unterstützt wird.
- Klasse A MUSS Standard für isolierte pro-Oberflächen-Anmeldeinformationen sein, wenn OIDC nicht verfügbar ist.
- Klasse B MUSS Standardeinstellungen für isolierte, am wenigsten privilegierte Anmeldeinformationen sein.
- Explizites unterstütztes Teilen MAY nur dann ausgewählt werden, wenn der Anbieter die erforderliche Autorisierung, Eigentümerschaft und Isolation Semantik bewahren kann.
- Klasse C MUSS isoliert bleiben.

## Eigentumsregeln und Regeln für die gemeinsame Nutzung

Beglaubigungsfreigabe ist eine politische Entscheidung nur innerhalb der gleichen Beglaubigungsklasse.

```text
A Human / management
    != B Application-service
    != C BaseHarbor-internal machine
```

Teilen MUSS KEINE Klassengrenzen kollabieren.

Ein gemeinsames Entwicklungs-Credential MÜSSEN Provider-Kontrolle, Workload-Identität, Laufzeit-Broker, AppRolle oder andere Maschinenanmeldeinformationen NICHT ersetzen.

Gemeinsame Provider-Infrastruktur MÜSSEN KEINE geteilten Klassen-B-Anmeldeinformationen zwischen Anwendungen implizieren.

## Anforderungen des Anbieters

Provider-Implementierungen MUSS diese Taxonomie konsumieren, anstatt Provider-lokale Anmeldeklassen zu erfinden.

Das Verhalten des Anbieters MUSS die Anmeldeklasse, den Eigentümer, den Anwendungsbereich/die Umwelt/den Ressourcenumfang, die Isolationsanforderungen, die maßgebliche Secret-Source-Semantik, die Projektions-/Liefer-Semantik und die Rotations-/Revolutionssemantik beibehalten.

Ein Anbieter MUSS ein nicht unterstütztes oder eingeschränktes Mapping explizit melden, anstatt die angeforderte Sicherheitssemantik stillschweigend zu schwächen.

## Grenzfläche des Managements

Menschliche Authentifizierung für von BaseHarbor verwaltete Managementflächen gehört zur Klasse A.

Provider-Administration-Anmeldeinformationen, die intern von BaseHarbor verwendet werden, gehören zur Klasse C.

Ein provider-natives Username/Passwort, das direkt von einem Menschen als effektives Management-Login verwendet wird, gehört zur Klasse A, auch wenn der Anbieter es in seinem nativen Anmeldesystem speichert.

## Sichere Bindungsgrenze

`secure-binding/v1` Nach wie vor der Arbeitsaufwand/Kapazitätssicherheit und der Vertrag über die Lebenszyklusbindung.

Es enthält anbieterneutrale Referenzen und Sicherheitsmetadaten für Material der Klassen B und Class C, wenn dieses Material an Workload/Capability-Bindung teilnimmt.

Es wird nicht zu einem Humanlogin- oder Business-RBAC-Vertrag.

Plaintext geheime Werte bleiben in portabler Absicht, Anbieter-Register, Plan, Status, Doctor, Beweise und normale Protokolle verboten.

## Maschinenschnittstellenparität

CLI, Machine JSON und MCP MUSS die gleiche Anerkennungs-Klassifikation aufdecken und KEINE Klassen zusammenfügen.

Status, Doctor und Beweismittel MÜSSEN KEINE stärkeren Eigentums-, Isolations- oder Authentifizierungsgarantien geltend machen, als die Implementierung bestätigt hat.

## Versionierung

Dieses Dokument definiert credential/access ownership semantics Version 1.

Additive Evolution MAY hinzufügen Metadaten oder zusätzliche explizite Klassifikationen, ohne bestehende Klassengrenzen zu schwächen.

Eine künftige inkompatible Neuzuteilung von Eigentumssemantik erfordert eine explizite neue Vertragsversion.

## Management-Zugriffs-Profile

Dieser Abschnitt ist der normative Faltungspunkt für den portablen Management-Zugriffs-Vertrag, der zuvor in `docs/spec/management-access-profiles-v1.md` dokumentiert war. Er deklariert, was ein nicht-geheimes Management-Profil, eine sensible Einmal-Geräte-Registrierung und ein explizit verschlüsseltes Zugangsdaten-Export-Payload mitführt, und was das Importieren dieser Objekte nicht darf. Er deklariert keine Anbieterrealisation und führt keine Geheimnisse mit.

`ManagementAccessKind` ist der portable Management-Zugriffs-Typ-Marker:

```go
type ManagementAccessKind string

const (
    ManagementProfile          ManagementAccessKind = "management-profile"
    DeviceEnrollment           ManagementAccessKind = "device-enrollment"
    CredentialExport           ManagementAccessKind = "credential-export"
)
```

`ManagementProfile` ist der nicht-geheimes Deskriptor des Management-Zugriffs einer Installation (Identität / Trust / Empfänger / Umfang / Gültigkeitsfenster). `DeviceEnrollment` ist die sensible einmalige Registrierung eines Knotens. `CredentialExport` ist das explizit verschlüsselte lokale Zugangsdaten-Payload.

Die Importfähigkeit ist fail-closed. Ein Record ist nur dann vertragsgültig für den Import, wenn jedes Feld `false` ist:

```go
type ImportCapability struct {
    RightsGranting   bool `json:"rights_granting"`
    PluginExecution  bool `json:"plugin_execution"`
    CredentialRedirect bool `json:"credential_redirect"`
}
```

Ein Record ist nur dann vertragsgültig für den Import, wenn jedes Feld `false`
ist. Jeder Record, der ein wahres Flag ankündigt, wird abgelehnt, bevor er einen
Anbieter erreicht.

Der Speicher wird nach Sensibilität getrennt:

```go
type StorageClass string

const (
    SessionOnly    StorageClass = "session-only"
    ProtectedDurable StorageClass = "protected-durable"
)
```

Session-only-Speicher lebt in Speicher für die Lebensdauer des Prozesses und wird nie persistiert. Protected-durable-Speicher wird hinter Host-Schutz (Keyring, OS-geschützter Speicher) persistiert.

`ManagementAccessProfile` bindet Installations-Identität, Trust, Empfänger, Umfang, Gültigkeitsfenster, Importfähigkeit, Speicher und optionale Export-Verschlüsselung:

```go
type ManagementAccessProfile struct {
    Kind           ManagementAccessKind  `json:"kind"`
    Version        string                `json:"version"`
    InstallationID string                `json:"installation_id"`
    Trust          Trust                 `json:"trust"`
    Recipient      string                `json:"recipient"`
    Scope          string                `json:"scope"`
    NotBefore      time.Time             `json:"not_before"`
    NotAfter       time.Time             `json:"not_after"`
    Import         ImportCapability      `json:"import"`
    Storage        Storage               `json:"storage"`
    Export         *EncryptionDescriptor `json:"export,omitempty"`
}
```

Die Export-Verschlüsselung führt den Algorithmus, das verschlüsselte Flag und den Nonce mit:

```go
type EncryptionDescriptor struct {
    Algorithm string `json:"algorithm"` // aes-256-gcm only in v1
    Encrypted bool   `json:"encrypted"`
    Nonce     string `json:"nonce"`
}
```

Diese Profile sind ein fail-closed-Vertrag, noch keine in einen Produktions-Consumer eingebundene Oberfläche. `contracts/managementaccess/v1` bindet den Wire-Vertrag an die bestehende autoritative Oberflächen auf develop: `ManagementAccessProfile.ValidateKindEnforcement` schlägt pro Typ fail-closed fehl: Ein Management-Profil hat vorhandene, sichere Storage; eine Enrollment ist sensibel; eine Zugangsdaten-Ausgabe hat einen verschlüsselten Descriptor, und die Import-Grenze lehnt zusätzlich ein abgelaufenes/zukünftiges/verkehrtes Fenster (`ValidityWindow`) und einen Fingerprint ab, der nicht zu seinem eigenen Zertifikat hashet (`ValidateTrust` / `internal/hosttrust.ParseCA`). `ValidateRecipientBind` bindet den Empfänger an den Einmal-Enrollment-Scope (`internal/targetenrollment`) und lehnt einen fremden Pin ab. `EnrollDevice` registriert das validierte Enrollment-Profil (pinned CA, Empfänger, Gültigkeitsfenster; sein Feld `installation_id` wird im Profil mitgeführt und schema-geprüft, aber `EnrollDevice` liest und bindet es nicht) an der Einmal-Enrollment und führt eine Validate-Then-Enroll-Sequenz aus: Sie verlangt `Kind=device-enrollment` auf der Einmal-Enrollment-Grenze (ein anderer Typ wird an der Grenze abgelehnt, bevor die Einmal-Speicher verbraucht wird), bindet die pinned CA selbst über `Enforcer.ResolveFingerprint` (Vergleich des im Profil gepinten CA-Fingerprints mit dem Fingerprint des von ihm bereitgestellten Zertifikats — ein selbstkonsistentes fremdes Zertifikat besteht diese Selbstbindung), erzwingt den vollen Typ-Vertrag (`ValidateKindEnforcement`) **vor** der Enrollment-Aktion (`enforcer.Enroll`, die Einmal-Speicher verbraucht), lehnt ein abgelaufenes/zukünftiges/verkehrtes Fenster und einen fremden Empfänger ab und erst danach wird das signierte Zertifikat gegen die vertrauenswürdige CA geprüft (`serviceaccess.ValidateSignedCSR`), damit ein abgelaufenes Fenster, ein fremder Empfänger oder ein ungültiges Profil fail-closed scheitern, ohne den Einmal-Pin zu verbrauchen. Das Wire-Format serialisiert den kind-Wert in den JSON-Schlüssel `kind` und den Versionsmarker in `version`, kongruent zum Schema (kein Feldtausch). Das enrollierte Profil wird **nicht** gegen eine unabhängige CA-Autorität geprüft, und die Enrollment ist an keine unabhängige Installations-Identität gebunden: EnrollDevice prüft (a) die Selbstkonsistenz des Profil-Zertifikats gegen seinen eigenen Fingerprint, (b) den Empfänger der Anfrage und (c) das Zeitfenster. Eine unabhängige CA-/Installations-Bindung und das gleiche-CA-Verhältnis zu `connection-profile/v1` sind verschobene Zukunftsthemen mit [t-20261018-r27-authority-contract](https://github.com/mcpdev80/baseharbor-plan/blob/main/tasks/t-20261018-r27-authority-contract.md) und [t-20261018-r27-state-owner](https://github.com/mcpdev80/baseharbor-plan/blob/main/tasks/t-20261018-r27-state-owner.md); keine Aussage hier behauptet, dass EnrollDevice sie jetzt prüft. Innerhalb dieses Vertrags ist die Trust-Prüfung Selbstkonsistenz: Das von einem Profil getragene Zertifikatsmaterial wird gegen den Fingerabdruck desselben Profils geprüft, zusammen mit dem Empfänger der Anfrage und dem Gültigkeitsfenster; ein Abgleich mit einem unabhängigen Anker (beispielsweise über `internal/hosttrust`) sowie die Bindung an eine Installation werden von diesem Vertrag ERFORDERLICH gemacht, aber erst mit den in *Deferred interfaces* genannten Folgearbeiten durchgesetzt. Das gleiche-CA-Verhältnis zu `contracts/connection-profile/v1` ist von diesem Vertrag erforderlich; seine Durchsetzung ist, wie oben benannt, verschoben. Einmal-Enrollment wiederverwendet `internal/targetenrollment` (Core-Issuer + einmaliger Store). Zugriffs-Records führen keinen neuen Transport mit; sie wiederverwendet die nativen Transportmittel (Git, SSH, Kubernetes, Cloud), die für das Ziel bereits existieren. Disaster-Recovery-Speicher steht außerhalb des sensiblen Einmal-Pfads (`disaster_recovery` beim Storage, `encrypted` beim Export).

Beziehung zu `contracts/connection-profile/v1`: `management-access/v1` und `connection-profile/v1` überschneiden sich bei Identitäts- und Trust-Feldern, erfüllen aber eine andere Aufgabe. `connection-profile/v1` ist **nur Transport**: es wählt den Core-Endpunkt und den Transport-Trust einer Verbindung. `management-access/v1` ist **nur Zugriff**: es deklariert, was ein nicht-geheimes Management-Profil, eine sensible Einmal-Enrollment oder eine verschlüsselte Zugangsdaten-Ausgabe mitführt, und was das Importieren dieser Objekte nicht darf. Beides vorhanden: der `connection-profile/v1`-Trust verankert den Transport und der `management-access/v1`-Trust muss zur selben CA hashen (`sha256`) und am Empfänger/Scope des Zugriffs-Records gebunden bleiben; der Transport-Binding erweitert den Zugriffs-Record nicht. Der `management-profile`-Typ darf keine Transport-`connection-profile/v1`-Rollenfelder mitführen (Umfang für den Zugriff, nicht für den Transport).

Deferred interfaces:

`management-access/v1` bindet an bestehende, akzeptierte Oberflächen auf develop (den `internal/hosttrust`-Anker, `serviceaccess.ValidateSignedCSR` zur Prüfung des signierten Zertifikats und `internal/targetenrollment` als Einmal-Autorität). Die restlichen Behörden liegen außerhalb der begrenzten Write-Reservierung dieser Karte und landen mit ihrer eigenen Karte:

- **Produktions-Consumer / Entry-Point.** Kein Produktions-Consumer importiert `contracts/managementaccess/v1`; der rufbare Pfad wird als Interface angeboten, noch nicht als eingebundener Endpunkt.
- **Anker + Installations-/Empfänger/Scope-Bindung + vertrauenswürdige Zeit.** Die vollständige Produktionsbindung von Installations-Identität, Empfänger/Scope, vertrauenswürdiger Uhrzeit und Eigentum an Credentials wird später erzwungen, mit [t-20261018-r27-authority-contract](https://github.com/mcpdev80/baseharbor-plan/blob/main/tasks/t-20261018-r27-authority-contract.md) (unabhängiger Anker + Installations-/Scope-Bindung) und [t-20261018-r27-state-owner](https://github.com/mcpdev80/baseharbor-plan/blob/main/tasks/t-20261018-r27-state-owner.md) (nicht akzeptiert, gleiche Dateien). Keine Aussage hier behauptet, dass dieser Profilpfad sie jetzt prüft.
- **Disaster-Recovery-Trennung im Produktionsablauf.** Die hier deklarierte DR-Trennung ist noch nicht in einen Produktions-Recovery-Pfad eingebunden.

### Codec- und Store-Serialisierung

Der v1-Codec akzeptiert einen Profil-Record nur, wenn seine serialisierte `version` exakt `baseharbor.management-access/v1` lautet. Export und Import erzwingen diesen Marker unabhängig von einer vom Aufrufer gewählten erwarteten Version. Der Import verlangt außerdem ein nicht-leeres erwartetes Empfängerpräfix und lehnt jeden Record ab, dessen `recipient` nicht an dieses Präfix gebunden ist; ein fehlendes Präfix bedeutet nie „jeden Empfänger akzeptieren“. Diese Codec-Bindung ist eine erforderliche Store-Eingabe und behauptet nicht, dass eine unabhängige Produktionsautorität bereits angebunden ist.

Die Reihenfolge der kanonischen JSON-Member ist festgelegt. Der Container verwendet `version`, `dest_path`, `records`, `timestamp`. Jeder Record verwendet `version`, `kind`, `installation_id`, `trust`, `recipient`, `scope`, `not_before`, `not_after`, `import`, `storage` und danach optional `export`. Die Reihenfolge verschachtelter Objekte ist `trust: ca_pem, sha256`; `import: rights_granting, plugin_execution, credential_redirect`; `storage: class, durable, encrypted, disaster_recovery`; `export: algorithm, encrypted, nonce`. Der kanonische Golden-Container fixiert Bytes und Reihenfolge.

Der Store-Import kodiert und validiert vor dem Persistieren, erstellt eine Staging-Datei im selben Verzeichnis und unternimmt genau einen Schreibversuch. Die zurückgegebene Bytezahl muss vor Sync oder Rename der angeforderten Länge entsprechen. Ein Kurzschreiben (`n < len(data)`) schlägt mit `ErrPartialWrite` fehl; die Staging-Datei wird entfernt und es wird kein finaler Record festgeschrieben. Der Writer wird nie ein zweites Mal aufgerufen, um ein Kurzschreiben nachträglich „zu vervollständigen“. Beim Ersetzen wird der neue Staging-Record festgeschrieben, bevor der veraltete Record entfernt wird. Scheitert dessen Entfernung, liefert der Import `ErrNoWrite`, entfernt den neuen Record und erhält den alten Dateiinhalt als eindeutigen Zustand auf der Platte.

Vertragsinvarianten:

- Genau eines der drei `kind`-Werte ist vorhanden.
- `import` ist immer `{rights_granting:false, plugin_execution:false, credential_redirect:false}`.
- Ein `device-enrollment`-Record muss sensibel sein (protected-durable oder session-only, nie ein Plaintext-Durable-Ausgabe).
- Ein `export`-Objekt ist nur für `credential-export` zulässig: Ein `management-profile`- oder `device-enrollment`-Record mit einem Export-Objekt wird abgelehnt, und ein `credential-export`-Record muss ein Export-Objekt führen.
- Ein `credential-export`-Record muss mit dem einzigen unterstützten AEAD-Chipher (`aes-256-gcm`) und einem kanonischen AEAD-Nonce aus 12 Zufallsbytes, kodiert als 16 Base64url-Zeichen ohne Padding (`4fIDBAUGBwgJCgsM` für die Golden-Fixtures; volle Oktettanzahl, nicht-kanonische Nachbits und jede andere Länge scheitern) verschlüsselt sein.
- Der gespeachte Trust-Fingerprint passt zum eigentlichen Zertifikat (`ValidateTrust`).
- Das Gültigkeitsfenster ist geordnet, aktuell und nicht abgelaufen (`ValidityWindow`).
- Der Empfänger ist am Einmal-Enrollment-Scope gebunden; ein fremder Pin wird abgelehnt (`ValidateRecipientBind` / `EnrollDevice`).
- Nicht schema-gesteuert (Doku-Verhalten, kein Gate): Der `management-profile`-Typ darf keine Transport-`connection-profile/v1`-Rollenfelder `access.role` mitführen (Umfang für den Zugriff, nicht für den Transport).
- Same-CA-/DR-Durchsetzung gegen einen externen `connection-profile/v1`-Transport-Anker und einen autoritativen Core-Scope-Resolver ist eine schnittstellenübergreifende Bindung; sie wird hier dokumentiert, aber nicht von dieser Karte verdrahtet, weil diese Autoritäten (connection-profile/Core-Scope-Resolver, host-trust-Anker) außerhalb dieses Write-Sets liegen und ihren eigenen_slices zugeordnet sind.

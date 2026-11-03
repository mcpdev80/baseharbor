# Management-Zugriffsschnittstellen (v1)

Minimal, anbieterneutrale Schnittstellen für den portablen Management-Zugriffs-
vertrag. Dies sind die aufrufbaren Produktionsweg-Oberflächen; sie deklarieren,
*was* ein Profil, eine Enrollment-Registrierung oder eine Zugangsdatenausgabe
mitführt und *was* das Importieren dieser Objekte nicht darf. Sie deklarieren
keine Anbieterrealisation und führen keine Geheimnisse mit.

## `ManagementAccessKind`

```go
type ManagementAccessKind string

const (
    ManagementProfile          ManagementAccessKind = "management-profile"
    DeviceEnrollment           ManagementAccessKind = "device-enrollment"
    CredentialExport           ManagementAccessKind = "credential-export"
)
```

`ManagementProfile` ist der nicht-geheime Deskriptor des Management-Zugriffs
einer Installation (Identität / Trust / Empfänger / Umfang / Gültigkeitsfenster).
`DeviceEnrollment` ist die sensible einmalige Registrierung eines Knotens.
`CredentialExport` ist das explizit verschlüsselte lokale Zugangsdaten-Payload.

## `ImportCapability`

```go
type ImportCapability struct {
    // RightsGranting reports whether importing this record grants any access
    // right. The contract requires it to be false.
    RightsGranting bool `json:"rights_granting"`
    // PluginExecution reports whether importing this record can run a plugin.
    // The contract requires it to be false.
    PluginExecution bool `json:"plugin_execution"`
    // CredentialRedirect reports whether importing this record routes existing
    // credentials to a different recipient. The contract requires it to be false.
    CredentialRedirect bool `json:"credential_redirect"`
}
```

Ein Record ist nur dann vertragsgültig für den Import, wenn jedes Feld `false`
ist. Jeder Record, der ein wahres Flag ankündigt, wird abgelehnt, bevor er einen
Anbieter erreicht.

## `StorageClass`

```go
type StorageClass string

const (
    // SessionOnly storage lives in memory for the lifetime of the process and
    // is never persisted.
    SessionOnly StorageClass = "session-only"
    // ProtectedDurable storage is persisted behind host protection (keyring,
    // OS-protected volume).
    ProtectedDurable StorageClass = "protected-durable"
)
```

## `ManagementAccessProfile`

```go
type ManagementAccessProfile struct {
    Kind          ManagementAccessKind `json:"version"`   // management-access/v1 marker
    Version       string               `json:"kind"`
    InstallationID string              `json:"installation_id"`
    TrustCAPEM    string               `json:"ca_pem"`
    TrustSHA256   string               `json:"sha256"`
    Recipient     string               `json:"recipient"`
    Scope         string               `json:"scope"`
    NotBefore     time.Time            `json:"not_before"`
    NotAfter      time.Time            `json:"not_after"`
    Import        ImportCapability     `json:"import"`
    Storage       StorageDescriptor    `json:"storage"`
    Export        *EncryptionDescriptor `json:"export,omitempty"`
}
```

## `EncryptionDescriptor`

```go
type EncryptionDescriptor struct {
    Algorithm string `json:"algorithm"` // aes-256-gcm | chacha20-poly1305
    Encrypted bool   `json:"encrypted"`
    Nonce     string `json:"nonce"`
}
```

## Rufbarer Produktionsweg

`contracts/managementaccess/v1` ist ein compilables Go-Paket, das den Wire-
Vertrag an die bestehende autoritative Schnittstelle bindet, statt einen
ungebrauchten Adapter zu deklarieren. `ManagementAccessProfile.ValidateKindEnforcement`
schlägt pro Typ fail-closed fehl: Ein Management-Profil braucht vorhandene,
sichere Storage; eine Enrollment muss sensibel sein; eine Zugangsdat AUSGABE
braucht einen verschlüsselten Descriptor. `ManagementAccessProfile.ValidateTrust`
berechnet den CA-Fingerprint aus dem eigentlichen Zertifikat
(`internal/hosttrust.ParseCA`) und lehnt einen gespeicherten Fingerprint ab, der
nicht übereinstimmt. `ValidityWindow` lehnt ein abgelaufenes,zukünftiges oder
verkehrtes Fenster ab. `ValidateRecipientBind` bindet den Empfänger an den
Einmal-Enrollment-Scope (`internal/targetenrollment`) und lehnt einen fremden Pin
ab, und `EnrollDevice` führt die Einmal-Enrollment über
`targetenrollment.Authority` aus.

## Beziehung zu `contracts/connection-profile/v1`

`management-access/v1` und `connection-profile/v1` überschneiden sich bei
Identitäts- und Trust-Feldern, erfüllen aber eine andere Aufgabe.
`connection-profile/v1` ist **nur Transport**: es wählt den Core-Endpunkt und
den Transport-Trust einer Verbindung. `management-access/v1` ist **nur Zugriff**:
es deklariert, was ein nicht-geheimes Management-Profil, eine sensible
Einmal-Enrollment oder eine verschlüsselte ZugangsdatAusgabe mitführt und was das
Importieren dieser Objekte nicht darf.

Beides vorhanden: der `connection-profile/v1`-Trust verankert den Transport und
der `management-access/v1`-Trust muss zur selben CA hashen (`sha256`) und am
Empfänger/Scope des Zugriffs-Records gebunden bleiben; der Transport-Binding
erweitert den Zugriffs-Record nicht. Der `management-profile`-Typ darf keine
Transport-`connection-profile/v1`-Rollenfelder mitführen (Umfang für den Zugriff,
nicht für den Transport).

## Wiederverwendung, Speicher und Disaster Recovery

Trust wird über den gemeinsamen `internal/hosttrust`-Anker und -Fingerprint
geprüft; der Zugriffs-Record wiederverwendet dasselbe CA-Material statt einer
neuen Vertrauenswurzel. Einmal-Enrollment wiederverwendet
`internal/targetenrollment` (Core-Issuer + einmaliger Store); die Knoten-Zulassung
wiederverwendet den bestehenden `internal/targetsession`-Pfad. Zugriffs-Records
führen keinen neuen Transport mit; sie wiederverwenden die nativen Transportmittel
(Git, SSH, Kubernetes, Cloud), die für das Ziel bereits existieren. Speicher-Trennung:
eine Einmal-Enrollment ist sensibel und einmalig, eine ZugangsdatAusgabe ist
verschlüsselt, und Disaster-Recovery-Speicher steht außerhalb des sensiblen
Einmal-Pfads (`disaster_recovery` beim Storage, `encrypted` beim Export).

## Vertragsinvarianten

Erzwungen von Schema + Go-Tests:

- Genau eines der drei `kind`-Werte ist vorhanden.
- `import` ist immer `{rights_granting:false, plugin_execution:false, credential_redirect:false}`.
- Ein `device-enrollment`-Record muss sensibel sein (protected-durable oder session-only, nie ein Plaintext-Durable-Ausgabe).
- Ein `credential-export`-Record muss mit einem realen Algorithmus und Nonce verschlüsselt sein.

Erzwungen vom rufbaren Go-Weg:

- Der gespeigte Trust-Fingerprint passt zum eigentlichen Zertifikat (`ValidateTrust`).
- Das Gültigkeitsfenster ist geordnet, aktuell und nicht abgelaufen (`ValidityWindow`).
- Der Empfänger ist am Einmal-Enrollment-Scope gebunden; ein fremder Pin wird abgelehnt (`ValidateRecipientBind` / `EnrollDevice`).

Nicht schema-gesteuert (Doku-Verhalten, kein Gate):

- Der `management-profile`-Typ darf keine Transport-`connection-profile/v1`-Rollenfelder `access.role` mitführen (Umfang für den Zugriff, nicht für den Transport).

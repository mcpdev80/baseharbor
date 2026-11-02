# Management-Zugriffsschnittstellen (v1)

Minimal, anbieterneutrale Schnittstellen für den portablen Management-Zugiffs-
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

## Vertragsinvarianten (erzwungen von Schema + Go-Tests)

- Genau eines der drei `kind`-Werte ist vorhanden.
- `import` ist immer `{rights_granting:false, plugin_execution:false, credential_redirect:false}`.
- Ein `device-enrollment`-Record muss sensibel sein (protected-durable oder session-only, nie ein Plaintext-Durable-Ausgabe).
- Ein `credential-export`-Record muss mit einem realen Algorithmus und Nonce verschlüsselt sein.
- `not_before`/`not_after` sind RFC3339; ein Record, dessen Fenster abgelaufen ist, wird durch die Gültigkeitsprüfung abgelehnt.
- Der `management-profile`-Typ darf keine Transport-`connection-profile/v1`-Rollenfelder mitführen (Umfang für den Zugriff, nicht für den Transport).

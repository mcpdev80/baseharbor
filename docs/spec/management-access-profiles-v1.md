# Management access interfaces (v1)

Minimal, provider-neutral interfaces for the portable management-access contract.
These are the callable production path surfaces; they declare *what* a profile,
enrollment or credential export carries and *what* importing them may not do.
They declare no provider implementation and carry no secrets.

## `ManagementAccessKind`

```go
type ManagementAccessKind string

const (
    ManagementProfile          ManagementAccessKind = "management-profile"
    DeviceEnrollment           ManagementAccessKind = "device-enrollment"
    CredentialExport           ManagementAccessKind = "credential-export"
)
```

`ManagementProfile` is the non-secret descriptor of an installation's
management access (identity / trust / recipient / scope / validity window).
`DeviceEnrollment` is the sensitive one-use registration of a node.
`CredentialExport` is the explicitly encrypted local credential payload.

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

A record is contract-valid for import only when every field is `false`. Any
record that advertises a true flag is rejected before it reaches a provider.

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

## Contract invariants (enforced by schema + Go tests)

- Exactly one of the three `kind` values is present.
- `import` is always `{rights_granting:false, plugin_execution:false, credential_redirect:false}`.
- A `device-enrollment` record must be sensitive (protected durable or session-only, never a plaintext durable export).
- A `credential-export` record must be encrypted with a real algorithm and nonce.
- `not_before`/`not_after` are RFC3339; a record whose window has elapsed is rejected by the validity check.
- The `management-profile` kind must not carry transport `connection-profile/v1` role fields (scope for access, not for transport).

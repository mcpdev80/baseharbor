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

## Callable production path

`contracts/managementaccess/v1` is a compilable Go package that binds the wire
contract to the existing authoritative interfaces rather than declaring an unused
adapter. `ManagementAccessProfile.ValidateKindEnforcement` fails closed per kind
(management-profile needs present, safe storage; device-enrollment must be
sensitive; credential-export needs an encrypted descriptor).
`ManagementAccessProfile.ValidateTrust` recomputes the CA fingerprint from the
actual certificate (`internal/hosttrust.ParseCA`) and rejects a stored fingerprint
that does not match it. `ValidityWindow` rejects an elapsed, future, or inverted
window. `ValidateRecipientBind` binds the recipient to the one-use enrollment
scope (`internal/targetenrollment`), rejecting a foreign pin, and `EnrollDevice`
performs the one-use enrollment through `targetenrollment.Authority`.

## Relationship to `contracts/connection-profile/v1`

`management-access/v1` and `connection-profile/v1` overlap on identity and trust
fields but play different roles. `connection-profile/v1` is **transport only**:
it selects the core endpoint and transport trust for a connection.
`management-access/v1` is **access only**: it declares what a non-secret
management profile, a sensitive one-use enrollment, or an encrypted credential
export carries, and what importing them may not do.

When both appear, the `connection-profile/v1` trust anchors the transport and
the `management-access/v1` trust must hash to the same CA (`sha256`) and remain
bound to the recipient/scope of the access record; the transport binding does
not widen the access record. The `management-profile` kind must not carry the
`connection-profile/v1` transport `access.role` fields (scope for access, not
for transport).

## Native reuse, storage, and disaster recovery

Trust is validated through the shared `internal/hosttrust` anchor and fingerprint;
the access record reuses the same CA material rather than introducing a new trust
root. One-use enrollment reuses `internal/targetenrollment` (Core issuer + single
use store); node admission reuses the existing `internal/targetsession` path.
Access records carry no new transport: they reuse native transport means (Git,
SSH, Kubernetes, cloud) that already exist for the target. Storage separation: a
device-enrollment is sensitive and single-use, a credential-export is encrypted,
and disaster-recovery storage is kept out of the sensitive one-use path
(`disaster_recovery` on storage, `encrypted` on export).

## Contract invariants

Enforced by schema + Go tests:

- Exactly one of the three `kind` values is present.
- `import` is always `{rights_granting:false, plugin_execution:false, credential_redirect:false}`.
- A `device-enrollment` record must be sensitive (protected durable or session-only, never a plaintext durable export).
- A `credential-export` record must be encrypted with a real algorithm and nonce.

Enforced by the callable Go path:

- The stored trust fingerprint matches the actual certificate (`ValidateTrust`).
- The validity window is ordered, current, and non-elapsed (`ValidityWindow`).
- The recipient is bound to the one-use enrollment scope; a foreign pin is rejected (`ValidateRecipientBind` / `EnrollDevice`).

Not yet schema-enforced (documented behavior, not a gate):

- The `management-profile` kind must not carry transport `connection-profile/v1` role fields (scope for access, not for transport).

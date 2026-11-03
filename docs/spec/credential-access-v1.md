# Credential and access ownership v1

## Scope

This specification defines the normative BaseHarbor credential taxonomy and ownership boundary.

It classifies credential material and identity used by BaseHarbor-managed infrastructure. It does not create a new IAM, RBAC, secret-store or provider protocol.

The taxonomy applies consistently across provider implementations, management surfaces, secure bindings, CLI, machine JSON, MCP, status, doctor and evidence.

## Credential classes

### A. Human / management identity

Class A represents infrastructure-facing human access only.

Examples include managed Identity login, provider/management GUIs, and BaseHarbor-managed infrastructure roles.

The portable infrastructure-management role profile is:

```text
admin
edit
view
audit
```

These roles MUST NOT become application business roles.

### B. Application-service credentials

Class B represents technical credentials used by an application to consume a capability.

Examples include PostgreSQL, Valkey/cache, RabbitMQ/messaging, MongoDB/document database, object storage/S3 and technical application client secrets.

Class B credentials remain scoped to the application, environment and logical resource as required by the capability contract. They MUST NOT be treated as human identity.

### C. BaseHarbor-internal machine identity

Class C represents internal machine identity and control credentials.

Examples include mTLS private keys, SPIFFE/workload identities, runtime broker tokens, OpenBao AppRoles, provider-control/provider-admin credentials and internal service-to-service credentials.

Class C credentials MUST remain isolated and MUST NOT inherit or reuse shared human/developer credentials or application-service credentials.

## Application business identity is out of scope

Application business users, groups, roles and permissions are owned by the application and its selected identity provider.

BaseHarbor MUST NOT maintain a parallel application-user database or proprietary business-RBAC model.

Applications may use normal OIDC, OAuth2, claims, groups, client roles or another selected external identity system.

## Environment policy

### Development

- Class A MAY use one Target-scoped developer-facing credential across compatible BaseHarbor-managed non-OIDC surfaces where technically safe.
- Native OIDC/OAuth2 remains preferred when a surface supports it.
- Class B MAY use simple or shared defaults only when application/environment/resource scope and required isolation remain correct.
- Class C MUST remain isolated.

### Test and production

- Class A SHOULD use central managed Identity/OIDC where supported.
- Class A MUST default to isolated per-surface credentials when OIDC is unavailable.
- Class B MUST default to isolated least-privilege credentials.
- Explicit supported sharing MAY be selected only when the provider can preserve the required authorization, ownership and isolation semantics.
- Class C MUST remain isolated.

## Ownership and sharing rules

Credential sharing is a policy decision only within the same credential class.

```text
A Human / management
    != B Application-service
    != C BaseHarbor-internal machine
```

Sharing MUST NOT collapse class boundaries.

A shared development credential MUST NOT replace provider-control, workload identity, runtime-broker, AppRole or other machine credentials.

Shared provider infrastructure MUST NOT imply shared Class B credentials between applications.

## Provider requirements

Provider implementations MUST consume this taxonomy instead of inventing provider-local credential classes.

Provider behavior MUST preserve credential class, owner, application/environment/resource scope where applicable, isolation requirements, authoritative secret-source semantics, projection/delivery semantics, and rotation/revocation semantics.

A provider MUST report an unsupported or limited mapping explicitly rather than silently weakening the requested security semantics.

## Management surface boundary

Human authentication for BaseHarbor-managed management surfaces belongs to Class A.

Provider administration credentials used internally by BaseHarbor belong to Class C.

A provider-native username/password used directly by a human as the effective management login belongs to Class A even when the provider stores it in its native credential system.

## Secure-binding boundary

`secure-binding/v1` remains the workload/capability security and lifecycle binding contract.

It carries provider-neutral references and security metadata for Class B and Class C material when that material participates in workload/capability binding.

It does not become a human-login or business-RBAC contract.

Plaintext secret values remain forbidden in portable intent, provider registry state, plan, status, doctor, evidence and normal logs.

## Machine-interface parity

CLI, machine JSON and MCP MUST expose the same credential classification and MUST NOT conflate classes.

Status, doctor and evidence MUST NOT claim stronger ownership, isolation or authentication guarantees than the implementation has verified.

## Versioning

This document defines credential/access ownership semantics version 1.

Additive evolution MAY add metadata or additional explicit classifications without weakening existing class boundaries.

A future incompatible reassignment of ownership semantics requires an explicit new contract version.

## Management surface access profiles

This section is the normative folding point for the portable management-access contract previously documented in `docs/spec/management-access-profiles-v1.md`. It classifies what a non-secret management profile, a sensitive one-use device enrollment, and an encrypted local credential export carry, and what importing them may not do. It declares no provider implementation and carries no secrets.

`ManagementAccessKind` is the portable management-access kind marker:

```go
type ManagementAccessKind string

const (
    ManagementProfile          ManagementAccessKind = "management-profile"
    DeviceEnrollment           ManagementAccessKind = "device-enrollment"
    CredentialExport           ManagementAccessKind = "credential-export"
)
```

`ManagementProfile` is the non-secret descriptor of an installation's management access (identity / trust / recipient / scope / validity window). `DeviceEnrollment` is the sensitive one-use registration of a node. `CredentialExport` is the explicitly encrypted local credential payload.

Import capability is fail-closed. A record is contract-valid for import only when every field is `false`:

```go
type ImportCapability struct {
    RightsGranting   bool `json:"rights_granting"`
    PluginExecution  bool `json:"plugin_execution"`
    CredentialRedirect bool `json:"credential_redirect"`
}
```

Any record that advertises a true flag is rejected before it reaches a provider.

Storage is separated by sensitivity:

```go
type StorageClass string

const (
    SessionOnly    StorageClass = "session-only"
    ProtectedDurable StorageClass = "protected-durable"
)
```

Session-only storage lives in memory for the lifetime of the process and is never persisted. Protected-durable storage is persisted behind host protection (keyring, OS-protected volume).

`ManagementAccessProfile` binds installation identity, trust, recipient, scope, validity window, import capability, storage, and optional export encryption:

```go
type ManagementAccessProfile struct {
    Kind          ManagementAccessKind `json:"version"`
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

Export encryption carries the algorithm, encrypted flag, and nonce:

```go
type EncryptionDescriptor struct {
    Algorithm string `json:"algorithm"` // aes-256-gcm | chacha20-poly1305
    Encrypted bool   `json:"encrypted"`
    Nonce     string `json:"nonce"`
}
```

These profiles are callable production-path surfaces. `contracts/managementaccess/v1` binds the wire contract to the existing authoritative interfaces rather than declaring an unused adapter. `ManagementAccessProfile.ValidateKindEnforcement` fails closed per kind (management-profile needs present, safe storage; device-enrollment must be sensitive; credential-export needs an encrypted descriptor). `ManagementAccessProfile.ValidateTrust` recomputes the CA fingerprint from the actual certificate (`internal/hosttrust.ParseCA`) and rejects a stored fingerprint that does not match it. `ValidityWindow` rejects an elapsed, future, or inverted window. `ValidateRecipientBind` binds the recipient to the one-use enrollment scope (`internal/targetenrollment`), rejecting a foreign pin, and `EnrollDevice` performs the one-use enrollment through `targetenrollment.Authority`.

Trust is validated through the shared `internal/hosttrust` anchor and fingerprint; the access record reuses the same CA material rather than introducing a new trust root. One-use enrollment reuses `internal/targetenrollment` (Core issuer + single use store); node admission reuses the existing `internal/targetsession` path. Access records carry no new transport: they reuse native transport means (Git, SSH, Kubernetes, cloud) that already exist for the target. Disaster-recovery storage is kept out of the sensitive one-use path (`disaster_recovery` on storage, `encrypted` on export).

Relationship to `contracts/connection-profile/v1`: `management-access/v1` and `connection-profile/v1` overlap on identity and trust fields but play different roles. `connection-profile/v1` is **transport only**: it selects the core endpoint and transport trust for a connection. `management-access/v1` is **access only**: it declares what a non-secret management profile, a sensitive one-use enrollment, or an encrypted credential export carries, and what importing them may not do. When both appear, the `connection-profile/v1` trust anchors the transport and the `management-access/v1` trust must hash to the same CA (`sha256`) and remain bound to the recipient/scope of the access record; the transport binding does not widen the access record. The `management-profile` kind must not carry the `connection-profile/v1` transport `access.role` fields (scope for access, not for transport).

Contract invariants:

- Exactly one of the three `kind` values is present.
- `import` is always `{rights_granting:false, plugin_execution:false, credential_redirect:false}`.
- A `device-enrollment` record must be sensitive (protected durable or session-only, never a plaintext durable export).
- A `credential-export` record must be encrypted with a real algorithm and nonce.
- The stored trust fingerprint matches the actual certificate (`ValidateTrust`).
- The validity window is ordered, current, and non-elapsed (`ValidityWindow`).
- The recipient is bound to the one-use enrollment scope; a foreign pin is rejected (`ValidateRecipientBind` / `EnrollDevice`).
- Not yet schema-enforced (documented behavior, not a gate): the `management-profile` kind must not carry transport `connection-profile/v1` role fields (scope for access, not for transport).

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

Export encryption carries the algorithm, encrypted flag, and nonce:

```go
type EncryptionDescriptor struct {
    Algorithm string `json:"algorithm"` // aes-256-gcm only in v1
    Encrypted bool   `json:"encrypted"`
    Nonce     string `json:"nonce"`
}
```

These profiles are a fail-closed contract, not yet wired into a production consumer. `contracts/managementaccess/v1` binds the wire contract to the existing authoritative interfaces on develop: `ManagementAccessProfile.ValidateKindEnforcement` fails closed per kind (management-profile needs present, safe storage; device-enrollment must be sensitive; credential-export needs an encrypted descriptor), and the import boundary additionally rejects an elapsed/future/inverted window (`ValidityWindow`) and a trust fingerprint that does not hash to its own certificate (`ValidateTrust` / `internal/hosttrust.ParseCA`). `ValidateRecipientBind` binds the recipient to the one-use enrollment scope (`internal/targetenrollment`), rejecting a foreign pin. `EnrollDevice` enrols the validated device-enrollment profile (pinned CA, recipient, validity window; its `installation_id` field is carried in the profile and schema-validated, but `EnrollDevice` neither reads nor binds it) against the one-use enrollment and runs a validate-then-enroll sequence: it requires `Kind=device-enrollment` on the enrollment entry (any other kind is rejected at the boundary before the single-use store is consumed), self-binds the pinned CA via `Enforcer.ResolveFingerprint` (comparing the profile's pinned CA hash to the fingerprint of the certificate it supplies — a self-consistent foreign CA passes this self-bind check), enforces the full kind contract (`ValidateKindEnforcement`) **before** it performs the enrollment effect (`enforcer.Enroll`, which consumes the single-use store), rejects an elapsed/future/inverted validity window and a foreign recipient, and only afterwards verifies the issued leaf against the authoritative trust bundle (`serviceaccess.ValidateSignedCSR`), so an expired window, a foreign recipient or an invalid profile fail closed without spending the one-use pin. The wire record serializes the kind value into the JSON key `kind` and the version marker into `version`, matching the schema exactly (not a field swap). The enrolled profile is **not** verified against an independent CA authority, nor is the enrollment bound to an independent installation identity: EnrollDevice checks (a) the self-consistency of the profile certificate against its own fingerprint, (b) the recipient of the request, and (c) the validity window. Independent CA / installation binding and the same-CA relation to `connection-profile/v1` remain deferred future work, owned by [t-20261018-r27-authority-contract](https://github.com/mcpdev80/baseharbor-plan/blob/main/tasks/t-20261018-r27-authority-contract.md) and [t-20261018-r27-state-owner](https://github.com/mcpdev80/baseharbor-plan/blob/main/tasks/t-20261018-r27-state-owner.md); no claim in this document asserts that EnrollDevice verifies them now.

Within this contract, trust validation is self-consistency: the certificate material carried by a profile is checked against the fingerprint inside the same profile, together with the request recipient and the validity window; comparison against an independent anchor (for example via `internal/hosttrust`) and installation binding are REQUIRED BY this contract but enforced only with the follow-up work named in *Deferred interfaces* below. The same-CA relationship to `contracts/connection-profile/v1` is required by this contract; its enforcement is deferred as named above. One-use enrollment reuses `internal/targetenrollment` (Core issuer + single use store). Access records carry no new transport: they reuse native transport means (Git, SSH, Kubernetes, cloud) that already exist for the target. Disaster-recovery storage is kept out of the sensitive one-use path (`disaster_recovery` on storage, `encrypted` on export).

Relationship to `contracts/connection-profile/v1`: `management-access/v1` and `connection-profile/v1` overlap on identity and trust fields but play different roles. `connection-profile/v1` is **transport only**: it selects the core endpoint and transport trust for a connection and declares its implementation as deferred. `management-access/v1` is **access only**: it declares what a non-secret management profile, a sensitive one-use enrollment, or an encrypted credential export carries, and what importing them may not do. When both appear, the `connection-profile/v1` trust anchors the transport and the `management-access/v1` trust must hash to the same CA (`sha256`) and remain bound to the recipient/scope of the access record; the transport binding does not widen the access record. The `management-profile` kind must not carry the `connection-profile/v1` transport `access.role` fields (scope for access, not for transport).

Deferred interfaces:

`management-access/v1` binds to existing, accepted surfaces on develop (the `internal/hosttrust` anchor, `serviceaccess.ValidateSignedCSR` leaf verification, and `internal/targetenrollment` one-use authority). The remaining authorities are outside this card's bounded write reservation and land with their own card:

- **Production consumer / entry point.** No production consumer currently imports `contracts/managementaccess/v1`; the callable path is exposed as an interface, not yet a wired endpoint.
- **Anchor + installation/recipient/scope binding + trusted time.** The full production binding of installation identity, recipient/scope, trusted wall-clock time, and credential ownership is enforced later, with [t-20261018-r27-authority-contract](https://github.com/mcpdev80/baseharbor-plan/blob/main/tasks/t-20261018-r27-authority-contract.md) (independent anchor + installation/scope binding) and [t-20261018-r27-state-owner](https://github.com/mcpdev80/baseharbor-plan/blob/main/tasks/t-20261018-r27-state-owner.md) (unaccepted, same files). No claim here asserts that this profile path verifies them now.
- **Disaster recovery separation in a production flow.** The DR separation declared here is not yet wired into a production recovery path.

Same-CA/DR enforcement with an external `connection-profile/v1` transport anchor and an authoritative Core scope resolver is a cross-slice binding; it is documented here but not yet wired by this card, because those authorities (connection-profile/Core scope resolver, host-trust anchor) are owned outside this write-set and are deferred to their owning slices.

Contract invariants:

- Exactly one of the three `kind` values is present.
- `import` is always `{rights_granting:false, plugin_execution:false, credential_redirect:false}`.
- A `device-enrollment` record must be sensitive (protected durable or session-only, never a plaintext durable export).
- An `export` descriptor is credential-export-only: a `management-profile` or `device-enrollment` record carrying an export descriptor is rejected, and an export is required on a `credential-export`.
- A `credential-export` record must be encrypted with the one supported AEAD cipher (`aes-256-gcm`, the only supported AEAD cipher in v1) and a canonical AEAD nonce of 12 random bytes encoded as 16 base64url characters without padding (`4fIDBAUGBwgJCgsM` for the golden fixtures; full octet count, non-canonical trailing bits and any other length fail closed).
- The stored trust fingerprint matches the actual certificate (`ValidateTrust`).
- The validity window is ordered, current, and non-elapsed (`ValidityWindow`).
- The recipient is bound to the one-use enrollment scope; a foreign pin is rejected (`ValidateRecipientBind` / `EnrollDevice`).
- Not yet schema-enforced (documented behavior, not a gate): the `management-profile` kind must not carry transport `connection-profile/v1` role fields (scope for access, not for transport).

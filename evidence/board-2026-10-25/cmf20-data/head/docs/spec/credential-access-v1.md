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

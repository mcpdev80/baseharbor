# Dynamic application secrets

BaseHarbor can store application-created credentials in the managed OpenBao secret plane while the application persists only an opaque reference.

This is intended for runtime-created credentials such as AI-provider API keys, SMTP passwords, webhook secrets or external API tokens.

## Ownership boundary

Application-owned configuration remains in the application:

```text
provider = openai-compatible
endpoint = http://agentgateway:4000/v1
model = qwen
credential_ref = baseharbor://secrets/dyn-...
```

Only the credential value is BaseHarbor-managed.

## Stable reference

Dynamic references use:

```text
baseharbor://secrets/dyn-<128-bit-random-id>
```

The reference is opaque. It does not encode tenant, application, environment, OpenBao mount or provider details. Applications must treat it as an uninterpreted string.

The same reference remains valid after value rotation. Deleting the referenced secret makes subsequent resolve operations fail closed.

## HTTP lifecycle

The protected control-plane API exposes:

```text
POST   /api/v1/apps/{app}/secret-refs
POST   /api/v1/apps/{app}/secret-refs/resolve
PUT    /api/v1/apps/{app}/secret-refs/resolve
DELETE /api/v1/apps/{app}/secret-refs/resolve
```

Create request:

```json
{"value":"secret-value"}
```

Create response:

```json
{"ref":"baseharbor://secrets/dyn-...","configured":true}
```

Resolve request:

```json
{"ref":"baseharbor://secrets/dyn-..."}
```

Resolve is the only operation that returns plaintext and its response is marked `Cache-Control: no-store`.

Rotate request:

```json
{"ref":"baseharbor://secrets/dyn-...","value":"new-secret-value"}
```

The response returns the same reference and never echoes the new value.

Delete removes the OpenBao secret metadata/history for the dynamic key. The application should then remove its stored reference.

## Named secrets remain separate

Deployment/operator-managed names such as `SMTP_PASSWORD` and `AGENT_GATEWAY_API_KEY` continue to use the existing named secret interface and `secrets.required` manifest contract.

Keys beginning with `dyn-` are reserved for dynamic references and are hidden from the normal named-secret list.

## Security

- values never belong in `baseharbor.yaml` or Git
- dynamic IDs are generated from 128 bits of cryptographic randomness
- OpenBao storage remains application/environment scoped
- the HTTP contract remains behind BaseHarbor identity, tenant ownership and RBAC checks
- create and rotate never echo values
- normal secret listings contain metadata only
- invalid/missing references fail closed

## Runtime application identity

A human OIDC token is not an acceptable long-term runtime dependency for MailFlow/ACS. The app-scoped runtime identity/broker transport is tracked separately and must be complete before the MailFlow reference acceptance closes the MVP secret gate.

Standalone applications remain free to use their own encrypted credential storage instead of this optional BaseHarbor integration.

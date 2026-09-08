# Secrets and OpenBao

BaseHarbor uses OpenBao as the bundled managed secret provider while keeping the application contract provider-neutral. Applications consume normal environment variables, mounted files, or opaque secret references; they do not need an OpenBao SDK or a BaseHarbor SDK.

## Platform bootstrap

```bash
baha up
baha openbao bootstrap --recovery-file /secure/off-host/openbao-recovery.json
baha openbao status
```

Bootstrap initializes and unseals OpenBao, enables the `baseharbor/` KV v2 mount and AppRole auth, creates the restricted BaseHarbor manager identity, verifies it, and revokes the initial root token. The recovery file is owner-only, must live outside `.baseharbor`, and is never overwritten or printed.

The persistent manager bootstrap state is owner-only under `.baseharbor/runtime/` and contains no root token or unseal key.

## Application secret scope

An application opts into managed secrets declaratively:

```yaml
services:
  postgres:
    enabled: true
  secrets:
    enabled: true
```

`baha app apply` provisions an isolated application/environment namespace:

```text
baseharbor/apps/<app>/<environment>/
```

Each application receives its own OpenBao policy and AppRole. That identity can access only the exact application/environment namespace. Cross-application and cross-environment access fails closed.

`baha app down` preserves the secret scope. `baha app destroy --yes` removes the managed application secrets, policy, AppRole and BaseHarbor-owned application state while preserving the repository manifest and application-owned data.

## Operator secret input

Secret values are never accepted as positional command-line arguments and are never printed back.

From stdin:

```bash
printf '%s' "$API_KEY" | baha app secret set API_TOKEN --stdin
```

Directly from a file:

```bash
baha app secret set TLS_KEY_FILE --file ./private-key.pem
```

Exactly one input source is required. Input is limited to 1 MiB and must be non-empty.

List configured names without values:

```bash
baha app secret list
```

Delete with an explicit confirmation:

```bash
baha app secret delete API_TOKEN --yes
```

## Validated TLS import

Certificate and private-key material can be validated together before it is stored:

```bash
baha app secret tls-set \
  --cert-file ./certificate.pem \
  --key-file ./private-key.pem \
  --chain-file ./intermediate.pem
```

`--chain-file` is optional. Certificate input may be PEM or DER X.509. BaseHarbor normalizes certificate material to PEM, verifies that the private key matches the leaf certificate, and rejects certificates that are not yet valid or have expired before writing the values to OpenBao.

The conventional secret names are:

```text
TLS_CERT_FILE
TLS_KEY_FILE
```

The source file paths are not stored in the application manifest.

## Static runtime delivery

Required secrets use two normal application-consumption forms.

### Environment value

A normal required secret is injected as the same environment variable:

```yaml
secrets:
  required:
    - name: SECRET_KEY
```

Runtime:

```text
SECRET_KEY=<resolved OpenBao value>
```

### File binding

A required secret whose logical name ends in `_FILE` uses a protected file binding instead of putting the secret payload in the process environment:

```yaml
secrets:
  required:
    - name: TLS_CERT_FILE
    - name: TLS_KEY_FILE
```

BaseHarbor materializes owner-only host files and mounts the binding directory read-only into the selected workload containers. The application receives only the stable path:

```text
TLS_CERT_FILE=/run/baseharbor/bindings/secrets/TLS_CERT_FILE
TLS_KEY_FILE=/run/baseharbor/bindings/secrets/TLS_KEY_FILE
```

This convention is generic; it is not TLS-specific. Any required secret ending in `_FILE` receives the same delivery mechanism.

The materialized host directory is `0700`, individual files are `0600`, and the workload mount is read-only. Values are never included in `baseharbor.yaml`, `baha app status`, `baha app doctor`, generated committed files, or normal logs.

## Dynamic application-created secrets

Applications such as MailFlow can create credentials at runtime while keeping normal application configuration application-owned.

Example ownership split:

```text
provider endpoint  -> application database
model               -> application database
API key             -> BaseHarbor/OpenBao
secret reference    -> application database
```

The stable opaque reference has the form:

```text
baseharbor://secrets/dyn-<opaque-id>
```

The TLS runtime API supports app-scoped create, resolve, rotate and delete operations under:

```text
/runtime/v1/apps/<app>/secret-refs
```

A workload receives a BaseHarbor-generated runtime identity through a protected token file, never through the manifest or Git. The identity is scoped to exactly one application/environment and cannot access operator APIs or another application's secrets. Runtime identity rotation/revocation does not change stored secret references.

The runtime API does not require a human OIDC login. `baha serve` can run in runtime-only mode with TLS plus application runtime identities. If OIDC issuer/audiences are additionally configured, the human/operator `/api/` surface is enabled and still requires the control-plane database, OIDC verification, tenancy, RBAC and ownership checks.

A configured runtime URL must be absolute HTTPS. Network reachability alone is never trusted.

## Security invariants

- secret values are not printed by normal CLI commands
- list/status/doctor expose metadata only
- mutation responses do not echo submitted values
- dynamic resolve responses use `Cache-Control: no-store`
- OpenBao manager/root credentials are never projected into applications
- runtime credentials are app/environment scoped
- cross-app access fails closed
- file bindings are owner-only on the host and read-only in workloads
- applications may still run without BaseHarbor by supplying their normal environment variables/files through another mechanism

## Deliberate boundaries

BaseHarbor does not own application configuration such as LLM provider, endpoint, model, mailbox settings or user preferences. Only sensitive credentials that the application chooses to delegate belong in the managed secret layer.

Still outside this MVP slice:

- dynamic PostgreSQL credentials
- moving all BaseHarbor-generated PostgreSQL/Valkey credentials into OpenBao
- OpenBao token renewal/agent integration
- TLS/PKI for the bundled OpenBao listener itself
- KMS/HSM/transit auto-unseal profiles

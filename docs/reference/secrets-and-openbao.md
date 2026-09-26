# Secrets and OpenBao

BaseHarbor uses OpenBao as the bundled managed secret provider while keeping the application contract provider-neutral. Applications consume normal environment variables, mounted files, or opaque secret references; they do not need an OpenBao SDK or a BaseHarbor SDK.

## Platform bootstrap

```bash
baha up
baha openbao bootstrap --recovery-file /secure/off-host/openbao-recovery.json
baha openbao status
```

Bootstrap initializes and unseals OpenBao, enables the `baseharbor/` KV v2 mount and AppRole auth, creates the restricted BaseHarbor manager identity, verifies it, and revokes the initial root token. The recovery file is owner-only, must live outside BaseHarbor-managed state, and is never overwritten or printed.

For normal `baha up` onboarding, BaseHarbor proposes the target-scoped default `$XDG_DATA_HOME/baseharbor-recovery/<target>/openbao-recovery.json` (or `~/.local/share/baseharbor-recovery/<target>/openbao-recovery.json`). After successful bootstrap, only the absolute path reference is persisted in Target configuration. The recovery material itself is never copied into BaseHarbor runtime/application state. On later restarts, `baha up` automatically resolves the persisted path; an explicit `--recovery-file PATH` always overrides it.

The persistent manager bootstrap state is owner-only below the effective Target's `$XDG_DATA_HOME/baseharbor/targets/<target>/runtime/` directory (or the corresponding `~/.local/share` fallback) and contains no root token or unseal key.

### Managed provider runtime hardening

The bundled OpenBao Compose service runs directly as the image's non-root `openbao` user. Its root filesystem is read-only, all Linux capabilities are dropped and `no-new-privileges` is enabled. BaseHarbor does not rely on a root init container or a temporary `CAP_CHOWN` grant.

The image-generated local configuration is written only to an ephemeral writable `/openbao/config` tmpfs. Durable OpenBao data remains on the dedicated `/openbao/file` volume. BaseHarbor enables the image-supported `SKIP_CHOWN` behavior because ownership repair by a privileged entrypoint is neither needed nor permitted by the BaseHarbor security model.

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

## Generated application secrets

BaseHarbor can generate values that are internal to the application and do not need to come from an external provider or human operator. Generation is always explicit in the manifest:

```yaml
secrets:
  required:
    - name: SECRET_KEY
      generate:
        type: random
        length: 64

    - name: ENCRYPTION_KEY
      generate:
        type: hex
        bytes: 32

    - name: OPENAI_API_KEY
```

The first two values are safe for BaseHarbor to create. `OPENAI_API_KEY` is external and therefore still requires user input.

Supported generators are intentionally small and bounded:

- `random` uses a cryptographically secure URL-safe 64-character alphabet and requires `length` between 16 and 4096.
- `hex` generates cryptographically secure random bytes and hex-encodes them; `bytes` must be between 16 and 1024.

`baha app apply` generates only explicitly declared values that are currently absent, writes them directly through the verified OpenBao application-secret path, and then runs the normal required-secret readiness gate. Generated values are never printed, written into `baseharbor.yaml`, or exported as a special BaseHarbor metadata file.

Generation is idempotent. If a generated secret already exists, BaseHarbor leaves it unchanged. If an existing value is present but unusable, BaseHarbor also does not replace it automatically; remediation remains an explicit operator action. Automatic rotation is deliberately out of scope for `apply`.

Readiness output distinguishes the cases:

```text
REQUIRED SECRET    STATUS                                      ACTION
SECRET_KEY         missing - will be generated automatically   baha app apply
OPENAI_API_KEY     missing - user input required                baha app secret set OPENAI_API_KEY
```

This follows the developer rule: provide only values BaseHarbor cannot safely know or generate.

## Operator secret input

Secret values are never accepted as positional command-line arguments and are never printed back.

Interactive terminal input is the normal human path:

```bash
baha app secret set API_TOKEN
```

The value is entered with terminal echo disabled. Automation keeps the explicit stdin path:

```bash
printf '%s' "$API_KEY" | baha app secret set API_TOKEN --stdin
```

Directly from a file:

```bash
baha app secret set TLS_KEY_FILE --file ./private-key.pem
```

When `--stdin` or `--file` is used, that explicit source is authoritative. Without either option BaseHarbor requires an interactive terminal and prompts securely. Input is limited to 1 MiB and must be non-empty.

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

## Required and optional application secrets

The portable contract distinguishes startup-gating secrets from optional application configuration:

```yaml
secrets:
  required:
    - name: API_TOKEN
  optional:
    - name: SMTP_PASSWORD
```

Missing required secrets block workload startup. Missing optional secrets do not. When an optional secret is configured in managed storage, BaseHarbor projects it through the same environment/file binding rules as a required secret.

Generated values are supported in either group. The guided init flow may ask whether a non-generated secret should be entered on first apply or configured later; that is onboarding UX, not a separate portable secret type.

## Static runtime delivery

Application secrets use two normal application-consumption forms.

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
- generated values are created with `crypto/rand` and written directly to the application OpenBao scope
- existing generated-secret values are never replaced implicitly by `apply`
- dynamic resolve responses use `Cache-Control: no-store`
- OpenBao manager/root credentials are never projected into applications
- runtime credentials are app/environment scoped
- cross-app access fails closed
- file bindings are owner-only on the host and read-only in workloads
- applications may still run without BaseHarbor by supplying their normal environment variables/files through another mechanism

## Deliberate boundaries

BaseHarbor does not own application configuration such as LLM provider, endpoint, model, mailbox settings or user preferences. Only sensitive credentials that the application chooses to delegate belong in the managed secret layer.

Still outside this MVP slice:

- automatic rotation schedules for generated application secrets
- dynamic PostgreSQL credentials
- moving all BaseHarbor-generated PostgreSQL/Valkey credentials into OpenBao
- OpenBao token renewal/agent integration
- TLS/PKI for the bundled OpenBao listener itself
- KMS/HSM/transit auto-unseal profiles

## Provider-neutral secure binding in v0.4.5

The existing OpenBao/runtime-broker implementation now maps into the shared `secure-binding/v1` model.

The application-facing contract still declares only required secret names. Internally, BaseHarbor represents the managed connection with:

- SPIFFE workload identity `spiffe://baseharbor/apps/<app>/<environment>`;
- an opaque runtime-authentication credential reference;
- an opaque runtime CA/trust reference;
- least-privilege `managed-secrets` / `secrets.read` authorization metadata;
- opaque references for required secret names;
- declared renewal, rotation and revocation support.

These are references only. OpenBao AppRole names, RoleIDs, SecretIDs, policies, KV paths, certificates, private keys and secret values remain protected provider/runtime state.

The existing OpenBao scope, broker, mTLS, restore and rotation implementation remains authoritative. v0.4.5 standardizes its semantics so later providers can reuse the same security boundary.

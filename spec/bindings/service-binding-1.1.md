# Service binding alignment

BaseHarbor uses Service Binding Specification 1.1 well-known entry names for service connection outputs whenever their semantics match.

## Well-known names

The canonical names are:

```text
type
provider
host
port
uri
username
password
certificates
private-key
```

BaseHarbor MUST NOT introduce aliases such as `hostname`, `connectionHost`, `user`, `pass` or `connectionString` when the standard name has the required meaning.

## Secret safety

Using the Service Binding name does not require BaseHarbor to expose plaintext secret material through provider metadata.

Secret-bearing values such as:

- `password`
- `private-key`
- credential-bearing `uri`

may be represented internally by protected/opaque references and resolved only at the trusted workload projection boundary.

Normal diagnostics, registry state and portable application intent never expose those values.

## BaseHarbor extensions

Service Binding does not cover all BaseHarbor lifecycle/security semantics. BaseHarbor extensions remain separate from standard connection entries.

Current extension semantics include:

- workload identity reference;
- credential/secret reference identity;
- authorization audience/scopes;
- renewal support;
- rotation support;
- revocation support;
- lifecycle ownership and verification metadata.

These extensions belong to the versioned BaseHarbor binding/security namespace. They do not rename or replace Service Binding well-known entries.

## Compatibility projections

Existing environment variables such as `DATABASE_URL`, `REDIS_URL`, `VALKEY_URL`, `S3_ENDPOINT` and standard OpenTelemetry variables may remain workload-specific compatibility projections.

They are not the canonical generic service-binding field model.

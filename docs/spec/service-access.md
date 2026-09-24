# Managed service access and PKI

BaseHarbor-managed network services use TLS or a stronger authenticated transport.

Certificate issuance and trust ownership are deliberately separate from portable application intent. BaseHarbor does not require its own CA.

## PKI sources

The service-access layer supports:

- `managed-local` — BaseHarbor resolves certificate issuance through the provider-neutral issuer boundary; the current Compose reference realization is OpenBao PKI;
- `external-pki` — an operator supplies externally issued X.509 server/client certificates and trust material;
- `byoc` — an operator supplies certificate/key/trust files without transferring CA ownership to BaseHarbor.

Provider-specific operator inputs override the generic service defaults. For example, `BASEHARBOR_PROMETHEUS_PKI_SOURCE` overrides `BASEHARBOR_SERVICE_PKI_SOURCE`.

The generic operator variables are:

```text
BASEHARBOR_SERVICE_PKI_SOURCE
BASEHARBOR_SERVICE_TLS_CERT_FILE
BASEHARBOR_SERVICE_TLS_KEY_FILE
BASEHARBOR_SERVICE_TLS_TRUST_FILE
BASEHARBOR_SERVICE_TLS_CLIENT_CERT_FILE
BASEHARBOR_SERVICE_TLS_CLIENT_KEY_FILE
BASEHARBOR_SERVICE_TLS_SERVER_NAME
BASEHARBOR_SERVICE_PKI_ISSUER_REF
```

Provider-specific forms replace `SERVICE` with the upper-case provider identifier, for example `BASEHARBOR_LOKI_TLS_CERT_FILE`.

These values are deployment/operator state, not fields in `baseharbor.yaml`.

## Environment rules

- dev: TLS is mandatory; trusted-local access may remain authentication-light where the provider surface is loopback/internal only.
- test: TLS and automatable authentication are mandatory.
- prod and custom managed environments: TLS and authentication are mandatory for management/observability/control surfaces.

A provider with native credentials/ACLs may use them. A service without suitable native authentication may use an mTLS gateway. The authentication mechanism is provider/runtime realization state.

## Developer host trust

Managed-local PKI may expose its public CA to the developer host without transferring issuer ownership to the CLI.

Rules:

- `baha up` checks whether the active managed-local CA is already trusted by the host when the repository lifecycle reaches an initialized, unsealed issuer;
- interactive `baha up` may offer host-trust installation, but only after an explicit yes/no prompt;
- `--yes` never implies host-trust consent;
- automation opts in explicitly with `baha up --trust-host-ca` or `baha trust install --yes`;
- `baha trust export --output PATH` exports only the public CA certificate/bundle; CA private keys remain inside the issuer provider;
- BaseHarbor records only trust anchors that it installed itself, keyed by CA fingerprint;
- ordinary `baha down` preserves host trust because the environment still exists;
- global `baha destroy --yes` removes only BaseHarbor-owned host trust anchors and verifies the installed certificate fingerprint before deletion;
- external-pki and BYOC trust roots remain operator-owned and are never installed, exported as BaseHarbor-owned material, or removed by BaseHarbor.

Fingerprint-specific anchor names allow old and new roots to overlap during CA rotation instead of forcing destructive in-place replacement.

## External PKI

External PKI is first-class.

BaseHarbor accepts external server certificates, private keys, trust bundles and, where mTLS is required, client certificates/keys. The application contract does not change when switching from managed-local material to an enterprise CA.

Private keys remain protected state. Normal status/evidence output records only source/ownership/verification metadata, never key material or credential-bearing URLs.

The issuer boundary owns trust retrieval, issue/renew/revoke lifecycle and readiness. The current managed-local reference issuer is OpenBao PKI. CA private keys and issuer state stay inside the issuer provider and must not be owned by the `baha` CLI host.

Future issuer integrations (for example ACME, enterprise certificate services, cert-manager or OpenShift issuers) attach behind this same boundary. An issuer product must never become portable application intent.

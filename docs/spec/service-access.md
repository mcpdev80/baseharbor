# Managed service access and PKI

BaseHarbor-managed network services use TLS or a stronger authenticated transport.

Certificate issuance and trust ownership are deliberately separate from portable application intent. BaseHarbor does not require its own CA.

## PKI sources

The service-access layer supports:

- `managed-local` — BaseHarbor resolves certificate issuance through the provider-neutral issuer boundary; the current Compose reference realization is OpenBao PKI;
- `external-pki` — either static externally issued X.509 material or a provider-neutral external issuer adapter selected by `BASEHARBOR_SERVICE_PKI_ISSUER_REF`;
- `byoc` — an operator supplies certificate/key/trust files without transferring issuance or CA ownership to BaseHarbor.

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
- BaseHarbor records only trust anchors that it installed itself, keyed by CA fingerprint together with issuer reference, backend and anchor path;
- ordinary `baha down` preserves host trust because the environment still exists;
- global `baha destroy --yes` removes only BaseHarbor-owned host trust anchors and verifies the installed certificate fingerprint before deletion;
- trust roots already installed by an operator or another tool are detected as trusted but are never claimed as BaseHarbor-owned;
- external-pki and BYOC trust roots remain operator-owned and are never installed, exported as BaseHarbor-owned material, or removed by BaseHarbor.

Fingerprint-specific anchor names allow old and new roots to overlap during CA rotation instead of forcing destructive in-place replacement.

## Certificate lifecycle and external PKI

Certificate lifecycle remains behind the provider/runtime boundary and never changes portable application intent or workload binding names.

### Managed local

Managed-local leaf certificates are short-lived and reconciled before expiry. The current renewal window is seven days.

The service-access state records only non-secret lifecycle metadata:

```text
issuer_reference
lifecycle_owner=issuer
renewal_mode=automatic-reconcile
server_serial
server_expires_at
client_serial
client_expires_at
```

A stable certificate outside the renewal window is left untouched. A near-expiry certificate is replaced through `Issuer.Renew()`. A changed issuer trust root forces replacement issuance even when the old leaf is still otherwise valid. The surrounding provider/runtime reconcile projects the replacement atomically and restarts/reconciles the affected service through its existing lifecycle path.

The previous leaf is not revoked before replacement rollout. This avoids creating an outage window; it may expire naturally or be revoked by a later provider-specific verified-rollout hook.

### External PKI

External PKI is first-class and has two deployment modes:

1. **static external material** — certificate/key/trust files remain externally owned and are validated/projected by BaseHarbor;
2. **issuer-backed external PKI** — `BASEHARBOR_SERVICE_PKI_ISSUER_REF` selects a provider-neutral issuer adapter.

Issuer-backed external PKI uses the same `Issuer` contract as managed-local PKI. The adapter must report the exact issuer reference requested by policy; a mismatched adapter fails closed. This prevents an OpenBao/default issuer from accidentally satisfying an enterprise-PKI policy.

Static `external-pki` and `byoc` material record secret-free lifecycle evidence in protected service-access state:

```text
source
lifecycle_owner
renewal_mode=replace-and-reconcile
server_fingerprint
server_expires_at
client_expires_at
health
warning
```

Expiry within 30 days is `warn`; expiry within seven days is `critical`. Replacement uses the same configured source paths: after the operator/provider replaces the certificate/key files, the next reconcile validates the new pair, updates the fingerprint/expiry evidence and rolls it out through the same workload/provider binding paths.

`byoc` is always operator-owned and cannot declare an issuer adapter. BaseHarbor never claims issuance ownership for BYOC material.

Private keys remain protected state. Normal status/evidence output records only source/ownership/fingerprint/expiry metadata, never key material or credential-bearing URLs.

The current managed-local reference issuer is OpenBao PKI. Future issuer integrations (for example enterprise certificate services, cert-manager or OpenShift issuers) attach behind the same boundary. An issuer product must never become portable application intent.

# Security and trust commands

## Host trust

```text
baha trust status
baha trust export --output PATH
baha trust install --yes
```

`trust install` is an explicit host mutation and always requires approval.

BaseHarbor exports/installs only the public managed-local CA. External PKI/BYOC trust roots remain operator-owned.

## OpenBao control-plane operations

```text
baha openbao status
baha openbao bootstrap --recovery-file PATH
baha openbao unseal --recovery-file PATH
baha openbao rotate --recovery-file PATH
```

The rotate command replaces the manager AppRole credential, control-plane database credentials and managed service PKI. Replacement paths are verified before prior material is retired.

## Operator authentication

```text
baha login
baha logout
baha whoami
```

Development can remain trusted-local where configured; test/prod operator access follows the configured authentication boundary.

## Initial operator and tenant bootstrap (v0.4.27 candidate)

On the **locally owned, ready management Core**, explicitly initialize the first operator tenant:

```bash
baha target show
baha up --control-plane-only
baha login -e dev --init-tenant
baha whoami -e dev
```

This first-owner operation is **not** performed implicitly by `baha up`. It uses interactive OIDC Authorization Code + PKCE. If no operator OIDC boundary exists for the selected Target/environment, interactive setup can configure managed/shared Keycloak or an external OIDC provider. Managed Keycloak uses a dedicated environment-specific operator realm/client, distinct from the Core Identity `baseharbor` realm.

Only after a verified, unexpired login does BaseHarbor create tenant `default`, the exact (issuer, subject) external identity and its `editor` membership in one transaction with RLS. Repeating with the same owner is idempotent; different owners, existing foreign tenants, remote Targets, invalid credentials or an unready local Core fail closed. No anonymous first-admin endpoint exists. Unconfigured dev keeps the trusted-local behavior; after explicit setup, ordinary `baha login -e dev` uses the stored operator OIDC boundary.

**Connector prerequisite:** Configure `BASEHARBOR_API_OIDC_ISSUER` and `BASEHARBOR_API_OIDC_AUDIENCES` on the protected operator API to match the actual operator realm/client. An API still using `.../realms/baseharbor` does **not** accept tokens issued by `.../realms/bh-operators-dev`; verify discovery and audience rather than bypassing OIDC or tenant checks.

After first-owner setup, enroll with `baha node add`, establish the connection with `baha node connect`, deploy with `baha app up`, and verify `baha doctor`. Docker and Podman require separate real end-to-end evidence. This functionality is being developed in [Core PR #1437](https://github.com/mcpdev80/baseharbor/pull/1437), **not yet merged, released or natively accepted**.

## Connections

Connection-oriented commands include:

```text
baha connect
baha disconnect
baha connections
```

Credentials, private keys and secret values must not appear in portable Application Intent or normal machine output.

See [Security](../explanation/security.md) and [Authentication](../explanation/authentication.md).

## Example: inspect and export local browser trust

After a successful local deployment has created the managed development CA:

```bash
baha trust status
baha trust export --output ./baseharbor-dev-ca.pem
```

The exported file is a public CA certificate, not a private key or application secret. Inspect it before deciding to trust it. To approve installation into the supported host trust store explicitly:

```bash
baha trust install --yes
```

Host installation may need platform-specific permissions. Check `baha trust status` afterward. This does not install an external provider's company CA or change its ownership.

## Example: check the current operator

```bash
baha whoami
baha login
baha whoami
baha logout
```

Use the configured authentication flow; do not paste tokens into command arguments. The reported identity helps verify authorization context before a protected operation. Login does not grant permissions beyond the effective policy.

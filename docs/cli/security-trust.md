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

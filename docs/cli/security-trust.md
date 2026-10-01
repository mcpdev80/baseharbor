# Security and trust commands

## Host trust

```text
baha trust status
baha trust export --output PATH
baha trust install --yes
```

`trust install` is an explicit host mutation and always requires approval.

BaseHarbor exports/installs only the public managed-local CA. External PKI/BYOC trust roots remain operator-owned.

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

# Application contract

The application contract describes what an application needs.

It should stay small and portable.

```yaml
version: 1

app:
  name: my-app

services:
  postgres:
    enabled: true

  redis:
    enabled: true

secrets:
  required:
    - name: APP_SECRET
```

The important rule is:

```text
application intent != provider configuration
```

The application may require SQL, cache or object storage without knowing whether an environment uses a bundled reference provider, a shared platform service or an external managed service.

Deployment context, provider selection, placement, generated credentials and runtime-specific names stay outside portable intent.

For exact fields, see [Manifest reference](../reference/manifest.md). For normative compatibility rules, see [Application contract v1](../spec/application-contract-v1.md).

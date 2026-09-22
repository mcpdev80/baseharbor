# Choose an environment

Use an environment to select deployment context without coupling the application contract to a runtime.

```bash
baha up -e dev
baha status -e dev
```

BaseHarbor uses the environment to resolve policy, provider placement and protected deployment state.

Use:

- `dev` for secure local development with convenience enabled where safe;
- `test` for production-like validation;
- `prod` for restrictive, auditable defaults.

Do not use environments to encode product choices such as PostgreSQL versus another SQL provider.

For the exact manifest behavior, see [Manifest reference](../reference/manifest.md).

# Select a deployment environment

An environment selects policy, placement and protected deployment state. It does not change SQL intent into a product choice.

## Inspect development before starting it

Inside the `orders-api` repository from [Getting started](../tutorials/getting-started.md):

```bash
baha plan -e dev
baha up -e dev
baha status -e dev
baha doctor -e dev
```

A configured runtime Target is required for deployment. Confirm the reported application, environment and Target before relying on its URLs or data.

## Compare test policy without deploying

```bash
baha plan -e test
baha policy check -e test
baha policy explain -e test
```

Use this to discover a missing provider, authorization requirement or policy restriction before a test deployment. A successful `dev` deployment does not prove that `test` or `prod` is permitted or configured.

| Environment | Typical purpose |
| --- | --- |
| `dev` | Secure local development |
| `test` | Production-like validation |
| `prod` | Restrictive, auditable operation |

Select a separate Target explicitly when needed, for example `baha --target docker-dev plan -e test`. Creating an environment is not a substitute for configuring the Target and its providers. See the [Manifest reference](../reference/manifest.md) for environment intent.

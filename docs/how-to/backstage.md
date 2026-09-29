# Use BaseHarbor from Backstage

BaseHarbor integrates with Backstage by contract. Backstage remains the portal and repository-publication layer; BaseHarbor remains the source of application intent, development planning and lifecycle evidence.

## Create a repository through a Scaffolder custom action

A Backstage custom action can invoke the supported BaseHarbor machine interface instead of rewriting application source itself.

For CLI-based actions:

```bash
baha app new catalog-api --stack go --all --emit-backstage --backstage-owner platform-team --output json
```

The JSON result is the automation contract. Do not parse human output.

For MCP-capable integrations, call:

```text
baseharbor.app.new
```

with the same semantic inputs:

```json
{
  "name": "catalog-api",
  "stack": "go",
  "capabilities": [
    "exposure.http",
    "database.sql",
    "cache.key-value"
  ],
  "emit_backstage": true,
  "backstage_owner": "platform-team"
}
```

The portal owns repository creation, credentials, publishing and registration. BaseHarbor does not receive portal credentials and does not call the Backstage Catalog API.

## Catalog ingestion

When explicitly requested, `baha app new` emits an application-owned `catalog-info.yaml`.

BaseHarbor only projects stable repository metadata:

- Backstage `Component` identity;
- explicit portal owner supplied by the caller/profile;
- explicit lifecycle, defaulting to `experimental`;
- a reference to the BaseHarbor application contract.

BaseHarbor never derives Backstage owner from BaseHarbor ownership and never derives Backstage lifecycle from the BaseHarbor environment.

Target, runtime, provider placement, credentials and generated deployment state are not emitted into catalog metadata.

After repository publication, configure normal Backstage Catalog ingestion for `catalog-info.yaml`. BaseHarbor does not continuously reconcile this file; it belongs to the application repository.

## Evidence and portal status

Runtime verification remains available through the versioned BaseHarbor evidence contract:

```text
baseharbor.evidence/v1
```

A portal plugin or backend can invoke the normal BaseHarbor evidence/machine interface and render the returned desired, enforced, observed and verified state. Do not create a Backstage-specific evidence format.

## Trust boundary

The consumer environment owns:

- Backstage authentication and authorization;
- Backstage plugins and custom actions;
- Catalog configuration;
- repository credentials and publication;
- execution policy for invoking BaseHarbor.

BaseHarbor Core does not include a Backstage SDK, Node runtime, Catalog API client, template interpreter or `${{ ... }}` evaluator.

See [ADR 0016](../decisions/0016-external-developer-portals-integration-by-contract.md).

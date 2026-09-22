# Use PostgreSQL

Declare that the application needs SQL. Do not make PostgreSQL-specific deployment topology part of portable application intent unless the capability contract requires a PostgreSQL-specific semantic.

Typical flow:

```bash
baha app inspect .
baha app init
baha plan
baha up -e dev
baha doctor
```

The application consumes the binding BaseHarbor provides, typically through a standard database URL.

BaseHarbor owns lifecycle only for resources it owns. Shared or external database providers keep their own lifecycle boundary.

For exact fields, see [Manifest reference](../reference/manifest.md).

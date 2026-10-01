# Manifest reference

The application manifest is portable Application Intent.

## Top-level rule

Only application requirements belong here.

Runtime/provider implementation details, generated credentials, private keys, machine-local trust paths and protected deployment state do not.

## Core fields

| Field | Meaning |
|---|---|
| `version` | application contract version |
| `app.id` | stable opaque `application_id` owned by the portable repository contract |
| `app.name` | human-readable application name |
| `app.environment` | declared/default environment context where supported |
| `services` | declared application capability needs |
| `secrets.required` | required logical secret names |
| `workload` | repository workload-source selection where explicitly required |
| `identity` | portable OIDC/application identity requirements |
| `telemetry` / `metrics` / `logs` | portable observability requirements |
| `runtime.permissions` | explicit application Runtime API permissions |
| `exposures` | provider-neutral HTTP exposure requirements |

## v0.4.19 service families

The manifest supports portable intent for:

- SQL;
- cache key-value;
- durable key-value;
- document database;
- messaging queue;
- messaging pub/sub;
- messaging stream;
- object storage;
- secrets;
- identity;
- provider management-surface requests where explicitly modeled.

The exact schema/types remain authoritative for field structure.

Product names such as PostgreSQL, Valkey, MongoDB or RabbitMQ do not replace the portable service kinds.

Capability-specific exact semantics are defined by the versioned service/capability specifications.

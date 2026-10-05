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
| `workload.components` | stable logical workload-component identities; source kind/path lives outside portable intent |
| `identity` | portable OIDC/application identity requirements |
| `telemetry` / `metrics` / `logs` | portable observability requirements |
| `runtime.permissions` | explicit application Runtime API permissions |
| `exposures` | provider-neutral HTTP exposure requirements |
| `ha` | one global high-availability request; false/default means no HA guarantee requested |
| `availability` | sparse component/capability HA and fixed-cardinality overrides only |
| `consumes` | logical Application/Component interface consumption; runtime addressing is excluded |

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


## v0.4.21 availability

Omitting `ha` or setting `ha: false` uses a single-server PostgreSQL realization. Explicit HA is selected by one top-level boolean:

```yaml
ha: true
```

Optional `availability` entries are sparse deviations only. They may set `ha` and/or a fixed `instances` value. Provider/runtime-specific cluster vocabulary is forbidden.

An effective HA request is negotiated independently with the selected Runtime Provider and Capability Provider. Unsupported guarantees fail before mutation; BaseHarbor never silently downgrades to a single instance.

## v0.4.21 application consumption

`consumes` references a logical producer by stable `application_id`, logical component and interface. Omitting `application_id` means the current Application.

Runtime-native addresses, Pod/container names, namespaces, nodes and replica identities are not portable consumption identity. The realized endpoint is deployment state.

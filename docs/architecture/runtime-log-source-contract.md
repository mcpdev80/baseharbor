# Runtime-neutral log-source contract

Status: v0.4.17

## Goal

BaseHarbor exposes one log-source contract independent of the selected Runtime Provider.

Portable application intent and Core verification must not depend on whether the runtime transports logs through Docker syslog, Podman journald, future CRI log files or another standards-compatible mechanism.

## Normalized source identity

A log source is identified semantically by:

- application;
- environment;
- source class;
- provider;
- service.

The normalized source classes are:

- `application`
- `application-provider`
- `platform-provider`

Transport details such as UDP ports, Docker logging-driver options, journal paths, container names and runtime-specific tags are realization details and are not portable application intent.

## Runtime adapter boundary

Runtime providers implement the small `runtime.LogSourceAdapter` contract:

```go
type LogSourceAdapter interface {
    LogCollectionMode() LogCollectionMode
    VerifyProjectServiceLogCollection(
        context.Context,
        string,
        string,
        string,
    ) error
}
```

`LogCollectionMode` currently has two first-party realizations:

- Docker: `syslog`
- Podman: `journald`

The mode is a runtime capability, not a product branch in Core.

## Docker realization

Docker attaches workload/provider stdout/stderr using the syslog logging driver and RFC5424.

BaseHarbor verifies the currently running service's effective logging configuration and expected semantic tag.

Stale or stopped containers must not satisfy verification for the current service instance.

## Podman realization

Podman uses native Quadlet plus `systemd --user`. Log collection is journald-backed.

BaseHarbor verifies that the currently running project/service instance exists and resolves to the journald collection path. Missing or stale services fail verification.

There is no `podman compose` fallback.

## Collector normalization

Alloy maps runtime-specific source metadata into the same BaseHarbor labels before Loki verification:

```text
baseharbor_application
baseharbor_environment
baseharbor_source_class
baseharbor_provider
baseharbor_service
```

Equivalent Docker and Podman sources therefore produce equivalent semantic labels even though their transports differ.

## Lifecycle rules

Source registration and collector reconciliation are deterministic and idempotent.

When a service is recreated or its runtime realization changes:

1. the selected Runtime Provider reconciles the current service instance;
2. BaseHarbor verifies the provider-owned source attachment;
3. the collector normalizes the source labels;
4. Loki verification uses the normalized semantic identity.

Old runtime objects or stale logging configuration must not be accepted as evidence for the current service.

## Standards

BaseHarbor does not define a proprietary logging protocol.

Current implementations reuse:

- container stdout/stderr;
- RFC5424 syslog for Docker;
- systemd journal semantics for Podman;
- Loki/Alloy for collection and storage.

Future Kubernetes/OpenShift providers may map CRI/platform logging into the same normalized source model without changing application intent or verification semantics.

## Security and evidence

Runtime transport details may appear in diagnostics, but normal status/evidence reports semantic source identity rather than product-specific transport configuration.

Credentials and secret values must never be encoded in log-source labels, tags or registration state.

## Compatibility

Changing Runtime Provider must not change:

- application log requirements;
- provider/service/application/environment source identity;
- verification semantics;
- Loki query semantics;
- user-visible log readiness.

Only the runtime-specific attachment/transport mechanism changes.

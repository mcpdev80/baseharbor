# ADR 0008: Application contract evolution and compatibility

## Status

Accepted for v0.4.0.

## Context

BaseHarbor must let one application contract survive the path from local development and single-host Compose to future Kubernetes and OpenShift deployments. The current public `baseharbor.yaml` schema is Manifest v1 and still contains fields whose present realization is Compose-oriented. Breaking that schema merely to make the implementation look provider-neutral would violate the pre-v1 compatibility promise and would force existing applications to be re-operationalized.

ADR 0005 requires contracts to describe capabilities rather than infrastructure products. ADR 0006 introduces the provider-neutral `PortableContract` domain view. ADR 0007 keeps runtime provider selection in protected deployment state rather than in the application contract.

## Decision

### 1. Manifest v1 remains a compatibility surface

v0.4 does not replace existing Manifest v1 files. A valid v0.2/v0.3 manifest remains valid unless it relied on behavior that was already invalid or unsafe.

The compatibility boundary is one-way:

```text
repository baseharbor.yaml (Manifest v1 compatibility)
                    |
                    v
          PortableContract
                    |
          +---------+---------+
          |                   |
          v                   v
 capability providers    runtime provider
```

Provider implementations consume normalized application intent. They must not require applications to rewrite provider-specific infrastructure objects into the common contract.

### 2. Field classification

Manifest v1 fields are classified as follows.

| Manifest concept | Classification | v0.4 treatment |
| --- | --- | --- |
| `app.name` | portable application identity | retained as stable logical identity |
| `app.environment` | deployment context | accepted for compatibility; not intrinsic application identity and not a runtime-provider selector |
| named PostgreSQL instances | portable SQL capability intent | translated to `database.sql` requirements |
| named Redis/Valkey instances | portable key-value capability intent | translated to `cache.key-value` requirements |
| `secrets.required` names/generation intent | portable secret requirement | translated without secret values |
| `services.secrets.enabled` | portable managed-secret intent | translated without selecting OpenBao/Vault |
| workload Compose path/service selection | provider-specific compatibility input | remains supported by the Compose implementation; it is not copied into provider-neutral capability identity |
| generated Compose networks/project names/ports/volumes | provider implementation detail | never part of the portable contract |
| FQDN, TLS source directory, selected TLS mode | deployment/operator state | stored outside `baseharbor.yaml` |
| runtime provider/profile | deployment/platform state | stored outside `baseharbor.yaml` |
| OpenBao paths/AppRoles/policies | capability-provider implementation detail | never part of the portable contract |

Future object storage, persistent storage, exposure, TLS intent, health, backup relevance, observability, generic inputs and availability intent are portable contract domains when they express application requirements. Their concrete provider products, topology objects and security policy remain environment/platform concerns.

### 3. Versioning rules

The top-level contract version is the compatibility gate.

- Additive optional fields may be introduced within a supported contract version when absence has an unambiguous safe meaning.
- New required semantics that cannot be represented safely by an older reader require a new contract version.
- Unknown required contract versions fail closed with a clear unsupported-version error; BaseHarbor never silently ignores required semantics.
- Provider-specific escape hatches, if ever necessary, must be optional and explicitly namespaced. They must not redefine the meaning of portable fields.
- Secret values, provider credentials and operator security policy never become committed contract fields merely to simplify a provider implementation.
- Logical resource names are stable application-facing identities. Changing the provider or topology must not silently rename them.

### 4. Compatibility adapters are explicit and removable only by release policy

`PortableContractFromManifest` is the v0.4 compatibility adapter. Provider-neutral code should depend on the portable view rather than grow new dependencies on Manifest v1 product/Compose fields.

A later contract version may gain first-class workload/endpoints, object storage, persistent storage, exposure/TLS intent, health, backup/observability, input and availability declarations. Manifest v1 translation remains supported until a separately documented release policy explicitly removes it.

### 5. Runtime and capability providers are independent axes

A deployment may combine, for example:

```text
runtime: openshift
SQL capability: external PostgreSQL
object storage: Ceph RGW
secrets: Vault
```

Kubernetes or OpenShift therefore must not become shorthand for PostgreSQL, secrets or object-storage provider selection.

### 6. Lifecycle stability is the acceptance test

The intended lifecycle is:

```text
application source + baseharbor.yaml
            |
            +--> laptop / homelab       (Compose)
            +--> test / staging          (Compose or future Kubernetes)
            +--> production              (provider selected by deployment)
            +--> Kubernetes              (future v0.7 provider)
            +--> OpenShift / enterprise  (future v0.8 specialization)
```

Moving between those stages may change deployment state, provider selection, topology and policy. It must not require rewriting the application's logical capability identities or introducing Kubernetes/OpenShift objects into the common application contract.

## Consequences

- Compose remains a first-class implementation without remaining the conceptual API.
- Existing repositories continue to work through the Manifest v1 compatibility adapter.
- New provider-neutral features should extend `PortableContract` and its compatibility translation before adding provider-specific realization.
- Kubernetes/OpenShift work can be implemented later without changing the v0.4 architectural boundary.
- Breaking contract evolution is explicit, versioned and fail-closed rather than inferred from implementation details.

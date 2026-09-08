# Application runtime identity

Applications that opt into BaseHarbor-managed secrets need a non-human identity when their backend creates or resolves dynamic secret references at runtime.

The runtime identity is infrastructure supplied by BaseHarbor. It is not part of application business configuration and therefore does not add fields to `baseharbor.yaml`.

## Workload contract

For a repository Compose workload with managed secrets enabled, BaseHarbor mounts an owner-only generated token read-only at:

```text
/run/baseharbor/runtime/token
```

and injects:

```text
BASEHARBOR_RUNTIME_TOKEN_FILE=/run/baseharbor/runtime/token
BASEHARBOR_RUNTIME_API_URL=https://<baseharbor-runtime-api>
```

`BASEHARBOR_RUNTIME_API_URL` is selected by the BaseHarbor installation/operator and must be an absolute HTTPS URL. The first workload apply fails closed if it is missing or invalid. BaseHarbor persists the validated value in its generated owner-only workload override so later lifecycle commands do not depend on the operator shell environment.

Applications read the token file with ordinary file I/O and send it as an HTTP Bearer credential to the dedicated runtime API. No BaseHarbor SDK is required.

## Runtime API

The runtime identity is accepted only on the restricted dynamic-secret surface:

```text
POST   /runtime/v1/apps/{app}/secret-refs
POST   /runtime/v1/apps/{app}/secret-refs/resolve
PUT    /runtime/v1/apps/{app}/secret-refs/resolve
DELETE /runtime/v1/apps/{app}/secret-refs/resolve
```

It does not authenticate to the normal operator API under `/api/v1/...`.

Each token is scoped to exactly one application. A token issued for one application cannot be reused for another application or environment runtime state.

## Rotation and revocation

Operators can invalidate a runtime credential without changing any secret references stored by the application:

```text
baha app runtime-identity revoke [NAME] --yes
baha app runtime-identity rotate [NAME] --yes
```

Inside a repository, `NAME` can be omitted.

Revocation immediately makes the token fail authentication. Rotation replaces the token in the existing binding file, clears revocation and leaves persisted references such as:

```text
baseharbor://secrets/dyn-0123456789abcdef0123456789abcdef
```

unchanged.

Applications that do not enable BaseHarbor-managed secrets receive no runtime identity and retain their normal standalone credential mechanisms.

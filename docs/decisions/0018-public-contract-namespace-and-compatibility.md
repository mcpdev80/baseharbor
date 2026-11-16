# ADR 0018: Public contract namespace and compatibility

Status: Accepted for pre-freeze convergence; contract freeze remains pending.

## Context

Shipped JSON Schema IDs previously named `schemas.baseharbor.dev`, and Stack
Profiles used `baseharbor.dev/v1`. Historical examples also named `baseharbor.io`.
DNS or HTTP reachability is not evidence of project ownership. This repository
contains no verified domain-control record for those names.

## Decision

Use the project-controlled GitHub repository URI namespace:

```text
https://github.com/mcpdev80/baseharbor/blob/HEAD/contracts/
```

Every shipped JSON Schema ID is that prefix followed by its exact packaged
path, including its contract-version directory. StackProfile `apiVersion` is:

```text
https://github.com/mcpdev80/baseharbor/blob/HEAD/contracts/development/v1
```

These URIs identify contracts; they are not a schema-hosting service or an
immutable download reference. Consumers resolve JSON Schemas through the
packaged offline registry. An external fetch must use a verified immutable
repository commit and the registry's path, never silently follow `HEAD`.
The schema dialect remains the standard JSON Schema 2020-12 URI.

The embedded registry rejects noncanonical/duplicate IDs, unregistered nested
IDs and unresolved or foreign references. No domain aliases are retained.
Logical protocol identifiers such as `baseharbor.machine/v1` remain versioned
protocol names; they do not assert DNS ownership.

## Compatibility status

[COMPATIBILITY.md](https://github.com/mcpdev80/baseharbor/blob/HEAD/COMPATIBILITY.md)
owns current compatibility policy. Versioned v0.4 contracts remain drafts.
This decision supersedes only pre-freeze legacy/migration-preservation promises
in ADRs 0005, 0006, 0008 and 0013; their original architectural rationale remains
historical evidence. It does not supersede the separation of portable intent,
providers, lifecycle or authorization.

## Standards and consequences

Adopt JSON Schema 2020-12, URI references and JSON Pointer. The BaseHarbor
extension is the packaged identifier-to-artifact registry and versioned
StackProfile semantics. Namespace changes are breaking pre-freeze changes:
producers, consumers and tests must advance together. A future domain move
requires verified administrative control and an explicit new decision.

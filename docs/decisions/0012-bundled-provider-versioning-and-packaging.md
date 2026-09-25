# ADR 0012: Bundled providers stay in the monorepo behind a versioned provider boundary

Status: Accepted

Date: 2026-09-23

## Context

BaseHarbor already separates portable capabilities from concrete products and defines the language-neutral `baseharbor.provider/v1` protocol. The current reference providers are still implemented inside the BaseHarbor repository because their lifecycle code evolved with the Compose-first runtime.

Creating one repository per provider before the provider contract is frozen would multiply release, CI and compatibility work while the boundary is still changing.

## Decision

First-party providers remain in the BaseHarbor monorepo until the provider contract is frozen.

They are treated as independently versioned provider implementations:

```text
BaseHarbor version
Provider implementation version
Capability specification version
Concrete product version
```

are separate version axes.

Bundled provider discovery is exposed through `internal/provider/builtin`. Core code should resolve bundled providers through that boundary instead of depending on concrete product packages for selection metadata.

Each provider integration has:

- a stable namespaced provider ID, for example `baseharbor/postgresql`;
- an implementation version;
- a provider protocol version;
- explicit versioned capability specifications;
- supported placement and lifecycle semantics.

The protected runtime provider registry persists this distribution identity separately from the logical capability binding.

Concrete provider lifecycle code may remain in existing packages while it is migrated incrementally. Code MUST NOT be moved merely to make the directory tree look complete if doing so creates a second lifecycle path or speculative abstraction.

After the provider contract is frozen, a provider may be moved to its own repository and release train without changing the application capability contract.

## Consequences

- current development remains one-repository and testable as one product;
- provider versions can evolve independently from BaseHarbor releases;
- future external providers can implement the same protocol and capability specifications;
- later repository extraction is a packaging decision rather than an application-contract migration;
- provider/product details remain visible in diagnostics but never become portable application intent.

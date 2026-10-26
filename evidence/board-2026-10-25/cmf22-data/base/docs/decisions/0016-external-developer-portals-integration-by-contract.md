# ADR 0016: External Developer Portals integrate through emitted contracts

## Status

Accepted for v0.4.18.

## Context

BaseHarbor owns portable application intent, lifecycle evidence and machine-readable application semantics. External developer portals own catalog ingestion, scaffolding UX, repository publication and portal-specific credentials.

Coupling BaseHarbor Core to one portal would duplicate responsibilities and make portal availability part of the BaseHarbor runtime architecture.

## Decision

External Developer Portals integrate with BaseHarbor through emitted, versioned contracts and optional ecosystem-native repository metadata.

Backstage is the first reference consumer. BaseHarbor may emit a static `catalog-info.yaml` projection when explicitly requested, but Core does not depend on Backstage.

The emitted catalog metadata:

- is derived only from BaseHarbor-owned application contract data;
- is deterministic and static;
- does not infer organization ownership;
- contains no repository-publishing credentials;
- contains no Backstage template expressions;
- does not require a live portal.

BaseHarbor Core does not:

- depend on the Backstage SDK or Node runtime;
- call the Backstage Catalog API;
- implement a Backstage Catalog client;
- parse or execute `template.yaml`;
- evaluate `${{ ... }}` expressions;
- implement a generic scaffolder/action engine;
- publish repositories on behalf of a portal.

The same architectural boundary applies to Port and other external developer portals.

## Consequences

Portal integration remains optional and default-off.

The canonical source of truth remains the BaseHarbor Application Contract, machine contracts and evidence model. Portal metadata is a projection and can be regenerated without changing portable application semantics.

A portal can consume BaseHarbor output without BaseHarbor needing portal credentials or a portal-specific runtime dependency.

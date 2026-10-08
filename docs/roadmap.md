# Roadmap

Detailed planning lives in GitHub Issues. This page shows product direction only.

The authoritative high-level planning issue is #155. Release-specific scope and
acceptance remain in the linked release umbrella and child issues.

## Current — v0.4.22

Machine Operator Authorization, Extension Trust and release readiness.

- transport-neutral machine-operation authorization;
- MCP enforcement of the shared authorization boundary;
- product-neutral extension trust metadata and policy boundary;
- public versioned provider/extension conformance artifacts;
- documented CLI/JSON/MCP coverage and practical automation examples;
- consistent repository init and lifecycle preflight, plus buildable Go starters;
- single-instance control-plane and PostgreSQL defaults, with explicit HA intent;
- ownership-safe target/provider cleanup and accurate resource planning;
- earlier Docker/Podman journeys, actionable failure diagnostics and selective
  evidence reuse for unchanged gate inputs, without weaker acceptance criteria.

Implementation is complete in PR #790. Full Docker/Podman acceptance and all 55
required proofs passed in prerelease run 37385682137.

## Before the v0.5 freeze

### v0.4.23

Freeze Readiness Policy, Public Namespace and Platform Contract.

- explicit compatibility/freeze policy;
- public namespace and schema-governance decisions;
- supported/tested platform classification;
- remaining runtime-neutral contract boundaries;
- machine HTTP/streaming and Target Access boundaries required so future Console
  work does not reopen frozen Core semantics.
- repository-independent secure Core bootstrap with mandatory SQL, Secrets and
  Identity, first-application continuation and machine-role defaults;
- optional Console attached to one selected Core in the same installation and
  security boundary, with same-origin HTTPS as the default;
- explicit shared/application-isolated placement and measured Core resource
  evidence;
- complete remote application and private-consumer integration qualification
  before pre-release approval.

Implementation is in progress in PR #809. The [draft release notes](releases/v0.4.23.md)
do not claim release or pre-release approval.

### v0.4.24

Human CLI and Documentation Consolidation.

- compact task-oriented human CLI;
- progressive disclosure for advanced/operator namespaces;
- task/category-first documentation information architecture;
- complete canonical command reference aligned with the final CLI.

### v0.4.25

Semantic Acceptance.

- prove CLI/JSON/MCP semantic parity;
- prove runtime/provider boundaries and ownership;
- prove provider compatibility is richer than capability-name matching;
- perform the final architecture and lifecycle acceptance before freeze.

## v0.5

Freeze, version and harden the public contracts.

No new major platform primitive belongs in the v0.5 line.

## Later

### v0.6

Post-freeze portability/guarantee hardening and preparatory work that does not
require changing frozen portable Application semantics.

The original portable-availability contract work was pulled forward into v0.4.21
so v0.6 must not introduce a second availability model.

### v0.7

Kubernetes Runtime implementation behind the frozen Runtime Provider boundary.

Namespace-only operation remains the primary authorization profile.

### v0.8

Kubernetes Complete: full lifecycle, capability/provider, recovery,
observability, update/migration, agent/DX and production-hardening parity.

OpenShift, enterprise and cloud/runtime expansion build on the frozen portable
contracts rather than changing them.

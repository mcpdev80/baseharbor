# ADR 0021: One installation authority for local and remote work

- Status: Proposed for v0.4.27 (part of task `t-20261018-r27-authority-contract`).

## Context

BaseHarbor runs local direct execution (the `baha` CLI and in-process control
surfaces) and remote clients (JSON, MCP, HTTP). Both must agree on **which
object owns authoritative state**. Without an explicit authority the system would
accumulate parallel owners: the local installation root, the remote management
connection, the Target, and the developer workstation's cached copy. The v0.4.27
release goal is one selected installation owning authoritative deployment,
provider, identity, consent-revision and mutation state for its admitted Targets,
with the management connection and the Target named as separate selections.

This ADR refines the existing `target-access-v1.md` boundary (Runtime Provider vs
Target Access Provider) around **authority**, not transport. It does not invent a
new generic state framework, a new `coreinstallation` state database, or a
second installation axis. It reuses the already authoritative Core lifecycle in
`internal/coreinstallation` and the deployment/state owners in
`internal/deployment`.

## Decision

### 1. Installation identity is authoritative; management connection and Target are distinct

Three independent selections, never conflated:

- `installation` is one per machine root, opaque UUID.
- `management-connection` is how a remote client reaches the selected Core.
- `target` is the admitted environment Core operates on.

- **Installation** is the durable authority. It is owned by
  `internal/coreinstallation` (`installation_id`, `owner=baseharbor`,
  `placement=shared`, version `baseharbor.core-installation/v1`, capabilities
  `{sql,secrets,identity}`). Its selection `{target, runtime, machineRole, ha}`
  is immutable once persisted; a conflicting selection is rejected, not replaced.
- **Management connection** is how a remote client addresses the selected Core.
  It is a `Target Access Provider` (e.g. `baseharbor-node-connector`,
  `native-api`) behind the canonical Target Access contract. It is **not** the
  installation and **not** the Target.
- **Target** is the admitted environment. It is a projection
  `{runtime_provider, access_provider, access_reference, scope}` and carries no
  authority over installation identity or consent.

A remote client **addresses** the selected installation through its management
connection; it **never** becomes the installation's authority.

### 2. One authoritative state/lock owner per managed deployment

- Local direct execution owns deployment state directly through the Core. There
  is no second local owner and no reconciliation between a "local cache" and a
  "remote truth".
- Deployment state is keyed by `targets/<target>/deployments/<deployment_id>/`
  (`internal/deployment/deployment_store.go`). One deployment record per
  `{target, deployment_id}`; multiple records claiming the same
  `{application_id, environment}` on one Target fail closed.
- Target configuration mutation takes a single owner via
  `.installation.lock` + `coreinstallation.AcquireStateFileLock`
  (`internal/deployment/config_mutation.go`). No concurrent mutation owner.
- The installation identity, the deployment owner and the mutation lock are the
  same semantic core, not three competing authorities.

### 3. Local direct execution reuses the semantic core without a login/server/SQL/Secrets/Identity bundle

Local execution of an already-selected operation uses the same Core types and
does not mandate an unconditional stack:

- **No mandatory login for trusted local `dev`.** `dev` local work must not
  require an operator OIDC boundary to perform installation-local operations
  (see `security-invariants.md`). Login, server and an OIDC operator boundary are
  requirements of the **remote** control-plane surface, not of local execution.
- **No bundled SQL/Secrets/Identity stack.** Local work consumes only the
  capabilities the selected installation actually needs. `installation.json`
  exposes readiness per capability; a capability the operation does not use is
  neither provisioned nor required. A cached capability does not establish
  live readiness.
- **No mandatory server.** The control-plane server (`baha serve`) is a remote
  boundary. Local execution is independent from the server being present or
  reachable.

### 4. Remote clients never own authoritative state and never fall back locally

- Remote clients address the selected installation through the management
  connection. They never write authoritative installation, deployment, consent or
  mutation state.
- **No local authoritative cache.** No authoritative remote state is stored on
  developer workstations, NFS mounts, home-directory synchronization,
  Baha-to-Baha federation, or any other local copy. A cached last-known status
  is a **stale observation**, not authoritative state.
- **Observed-at freshness.** A cached observation carries its `observed_at`
  timestamp. When live observation fails, the client reports the observation as
  **stale/rejected** and its freshness; it never downgrades a failed live
  check to "ready from cache". Cached state may never claim current `READY` when
  observation could not confirm it.
- **No local fallback.** A missing, foreign or expired management connection
  fails closed. There is no automatic switch to executing against a local
  installation or runtime. "A missing or foreign session never selects a local
  runtime" (already stated in `target-access-v1.md`; this ADR generalizes it to
  the authority model).

### 5. Consent/revision acceptance, observed-at freshness and independent security boundaries

- **Consent/revision acceptance.** A stale plan or consent, an incompatible
  revision, a foreign installation selection, a wrong installation, a lost
  response, a process restart, an unavailable Target or a revoked device are all
  **negative** cases that fail closed. A consent accepted once does not persist as
  a standing remote authority; each authoritative decision re-validates against
  the live installation state.
- **Observed-at freshness** is a first-class, enforced property (point 4).
- **Independent security boundaries.** Remote access surfaces, the local
  execution path, and the installation state root remain independently owned and
  verified. A green gate on one boundary does not authorize another; the
  strongest boundary is never weakened to carry a weaker one. Foreign revision
  or consent on any boundary is rejected without adopting it.

## Consequences

- Downstream owners (state-owner, connection-client, delivery-boundary,
  execution-journal, node-service, client-commands) consume the minimal callable
  selection/authority types this ADR points to: one installation, one state
  owner, remote-only authority with freshness — not a new framework.
- The canonical Target Access contract (`target-access-v1.md`, canonical wire
  records) stays the transport boundary; this ADR adds the authority layer on top
  without replacing it.
- No second installation axis, no new `coreinstallation` state database, no
  generic state framework are implied.

## Bounded production surface (see task `t-20261018-r27-authority-contract`)

- `internal/coreinstallation`: minimal, callable selection/authority types that
  express *installation identity + one state/lock owner + remote-only authority
  with freshness*; bounded, not a framework.
- Positive and negative regression tests covering:
  - management connection ≠ Target ≠ installation (selections never interchanged);
  - no local fallback for a missing/foreign/expired remote authority;
  - stale `observed_at` ⇒ rejected, never claimed `READY`;
  - foreign revision/consent ⇒ rejected.

## Compatibility

This ADR formalizes invariants already present in `target-access-v1.md`,
`security-invariants.md`, ADR 0009 (shared core) and ADR 0017 (stable identity).
It does not change portable Application Intent or the existing Core lifecycle
types; it documents the authority contract for downstream owners.

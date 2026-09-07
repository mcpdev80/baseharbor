# BaseHarbor Development Guidelines

These guidelines are mandatory for all BaseHarbor development. They define the engineering standard for code, tests, migrations, documentation, CLI behavior, runtime changes, and pull-request review.

The priorities are:

- correctness before convenience
- security and isolation by default
- explicit, understandable control flow
- clear responsibility boundaries
- small, incremental changes
- strong typing and deterministic state
- observable failures
- testable invariants
- minimal unnecessary abstraction and dependencies
- long-term human maintainability

When code, documentation, architecture, and task scope disagree, do not silently invent a new design. Stop the change, identify the inconsistency, and resolve it explicitly.

## 1. Respect the current architecture and task scope

Changes must fit the architecture documented in this repository. Do not pull unrelated capabilities, later roadmap phases, or speculative abstractions into the current task.

BaseHarbor has deliberate boundaries between, at minimum:

- CLI / operator interface
- application desired state
- control-plane state
- service provisioning and convergence
- persistence and migrations
- authorization / tenancy
- secrets and credentials
- runtime providers
- health / preflight / verification
- backup / restore
- observability

Do not merge responsibilities merely because they are convenient to call from one place.

Do not refactor unrelated modules in the same change.

## 2. Prefer the smallest correct design

Use the smallest mechanism that enforces the required invariant.

Prefer direct, readable code over clever framework-style abstractions. A small amount of duplication is acceptable when it keeps control flow and ownership obvious.

Do not introduce abstract factories, generic workflow engines, plugin systems, strategy hierarchies, or storage abstractions unless a current, demonstrated requirement needs them.

Prefer existing repository patterns before adding a new framework or architectural style.

## 3. Keep modules cohesive

A package or file should have one clearly explainable primary responsibility.

Avoid both extremes:

- monster modules that become the home for every new feature
- micro-modules that split trivial logic into many tiny files

Split code when there is a real responsibility boundary, not to satisfy an arbitrary line-count target.

For Go code, keep packages small and focused and avoid God files.

## 4. Use explicit typed domain models

Stable concepts should have explicit types.

Prefer structs, enums/constants, typed results, and dedicated configuration types over loosely structured maps or generic containers.

Types should make ownership, valid state, and invalid state visible.

Do not use global mutable business state as a shortcut.

## 5. Separate domain logic, persistence, and orchestration

Domain rules must remain independent of transport and persistence details where practical.

Persistence belongs behind a clear persistence boundary. Do not scatter SQL across orchestration code.

Services and command handlers should orchestrate:

- load authoritative state
- validate input
- run policy/domain logic
- call providers
- persist results
- coordinate retries and state transitions

They should not become permanent containers for unrelated logic.

## 6. `baha` is a product interface

The `baha` binary is BaseHarbor's primary operator interface and must be treated as a stable product surface.

Every command must have:

- useful `--help`
- clear usage text
- actionable errors
- stable exit semantics
- deterministic behavior
- no secret leakage

Command structure should remain discoverable and consistent.

Mutating commands must follow the lifecycle principle:

```text
plan -> preflight -> apply -> verify
```

Preflight and plan operations must not mutate state.

Do not report success merely because a process or container started. Verification must test the capability that matters to the operator.

## 7. Fail closed on security and ambiguity

Security-sensitive operations must fail closed.

This includes:

- tenant and application isolation
- ownership relationships
- permissions and RBAC
- credential access
- secret scopes
- certificate activation
- restore validation
- destructive lifecycle actions

Knowing a resource identifier must never be sufficient to bypass authorization or ownership checks.

Unknown identity, ambiguous ownership, missing policy, invalid state, or unverifiable security assumptions must not become implicit success.

## 8. Enforce invariants at the authoritative boundary

Do not rely only on caller convention for important invariants.

Where appropriate, enforce relationships at the persistence layer using PostgreSQL constraints, foreign keys, uniqueness, RLS, transactions, or other minimal mechanisms.

Application isolation and ownership combinations require explicit negative tests.

A cached projection, generated status view, queue, or runtime snapshot must not become a second mutable source of truth.

## 9. Secrets never belong in normal data paths

Secrets, credentials, API keys, tokens, private keys, passwords, authorization headers, and credential-bearing URLs must not appear in:

- logs
- error messages
- API responses
- audit records
- metrics labels
- test fixtures committed to the repository
- job payloads
- generated manifests intended for normal source control

Pass stable resource identifiers through jobs and runtime orchestration; resolve secrets at the trusted boundary when needed.

Secret values must be redacted before diagnostic output is persisted or rendered.

## 10. Keep audit and operational telemetry separate

Audit data records meaningful security, lifecycle, ownership, or operator decisions.

Operational logs, metrics, and traces record implementation behavior such as retries, timing, health, queue state, and provider failures.

Do not generate audit noise for every successful internal step.

Neither audit nor telemetry may contain secret material or unnecessary sensitive payloads.

## 11. Errors must stay observable

Do not silently swallow failures.

Ignored errors, broad panic-based control flow, generic catch-and-discard behavior, and hidden persistence failures are defects unless there is a documented reason.

Callers must be able to distinguish materially different states such as:

- not found
- conflict
- validation failure
- authorization denial
- provider unavailable
- timeout
- degraded state
- verification failure

Retries may hide transient noise from the user, but the final state and relevant telemetry must remain observable.

## 12. Configuration must be explicit and owned

Do not scatter magic values across packages.

Timeouts, limits, versions, retention periods, retry policies, concurrency, paths, feature settings, and security thresholds need a clear configuration owner.

Do not create one oversized global configuration object containing unrelated state.

Configuration is not a second runtime database.

## 13. Preserve compatibility incrementally

Do not rewrite a working subsystem only because a cleaner theoretical design exists.

For a feature or structural change:

1. identify the existing path
2. define the invariant and acceptance criteria
3. make the smallest structural change needed
4. preserve existing behavior where required
5. add tests for the new behavior and negative cases
6. remove legacy behavior only after the replacement is proven

Avoid cleanup-only churn in unrelated files.

## 14. Database migrations must be safe and reproducible

Every persistence change must be reviewed for both forward and rollback behavior.

Rules:

- migrations are reproducible
- a migration only reverses objects or changes it owns
- rollback must not destroy schema created by an earlier migration
- destructive changes require explicit justification and recovery planning
- referential integrity and uniqueness must reflect required ownership invariants
- version/history state must be preserved when reconstructability requires it
- repeated or concurrent execution must not produce ambiguous current state
- cross-tenant or mismatched ownership cases require negative tests

Migration success alone is not enough; application behavior against the migrated schema must be verified.

## 15. Backup is not complete without restore

A backup feature is not considered supported until restore has been exercised and verified.

Restore must validate the authoritative invariants needed before workloads resume, such as:

- expected schema version
- required secrets or key access
- application/tenant ownership integrity
- service connectivity
- restored persistent state
- runtime compatibility

During restore or uncertain recovery, mutating workloads should remain paused or fail closed until validation succeeds.

## 16. Runtime changes require preflight and post-verification

Risky infrastructure or runtime changes must separate discovery/preflight from mutation.

Before mutation, validate prerequisites and produce actionable failures.

After mutation, verify actual capability rather than configuration presence alone.

Examples:

- PostgreSQL: establish a real connection / query
- Redis/Valkey: real protocol health check
- OpenBao: health, initialization/seal/auth state as appropriate
- TLS: key match, chain, hostname, validity and trust
- backup: restorable artifact

Operations should be idempotent where practical.

## 17. Dependencies must earn their place

Before adding a dependency, verify that the Go standard library or existing stack does not solve the problem adequately.

A dependency is justified when it materially improves correctness, security, compatibility, interoperability, or maintainability.

Do not add a dependency merely to save a small amount of straightforward code.

Pin and update dependencies through the repository's established dependency-management process.

## 18. Use repository-provided build and test paths

Use the toolchain and wrappers defined by the repository.

Do not require contributors to install alternate host toolchains merely because an equivalent repository-provided workflow exists.

Keep local validation easy enough that contributors can run it before spending CI resources.

## 19. Tests follow responsibility boundaries

Tests must validate behavior and invariants rather than lock implementation details unnecessarily.

Depending on the change, cover:

- successful paths
- invalid input
- failure paths
- authorization denial
- cross-tenant / cross-application isolation
- secret non-disclosure
- idempotency
- repeated and concurrent operations
- retry / degraded behavior
- restart / resume behavior
- migration forward/rollback safety
- backup / restore
- preflight non-mutation
- post-apply verification

A security or isolation claim without a negative test is incomplete unless the test is demonstrably infeasible and the reason is documented.

Integration tests should target boundaries where real defects are likely: CLI -> application layer, API -> service -> persistence, runtime provider -> service, restore -> verification, etc.

## 20. Comments explain intent and invariants

Comments should explain why, not restate obvious code.

Document non-obvious constraints close to the code that depends on them.

New code, comments, identifiers, commit messages, and technical documentation should be written in English. User-facing text may be localized separately.

Avoid language-only cleanup commits.

## 21. Documentation is part of the implementation

Architecture, CLI contracts, manifests, operational procedures, and security behavior must be documented when a change affects them.

If implementation and canonical documentation disagree, treat that as a defect. Do not silently choose whichever is convenient.

Documentation must describe current behavior clearly and mark future behavior as future work.

Do not claim a capability is working without observed evidence.

## 22. Pull requests remain focused and evidence-driven

Review the actual diff, not only the PR description.

Every PR should:

- have a clear scope
- avoid unrelated file churn
- explain important invariants or security effects
- include appropriate tests
- include migration/restore implications where relevant
- update documentation when contracts change
- distinguish implemented behavior from future roadmap work

Review findings use these severities:

- `BLOCKER`: unsafe to merge; security, isolation, data-loss, or fundamental correctness failure
- `MAJOR`: required behavior or architecture contract is incorrect or missing
- `MINOR`: real defect with limited impact
- `INFO`: non-blocking improvement or observation

Do not treat an intentionally deferred future feature as a blocker for the current scoped change.

## 23. CI evidence must be truthful and economical

Never claim CI is green unless the run for the relevant code has actually succeeded.

Inspect existing failures before rerunning workflows. Fix deterministic defects before starting another run.

Do not rerun CI merely to see whether a deterministic failure disappears.

Keep workflows compact and avoid unnecessary parallel jobs or repeated validation that creates cost without additional evidence.

Where practical, validate locally or with repository-provided tooling before pushing a PR update.

## 24. Human readability wins

When choosing between two correct designs, prefer the one a new contributor can understand faster.

A maintainer should be able to answer without reverse-engineering the repository:

- Where does this feature live?
- What owns this state?
- What is the source of truth?
- Where is it persisted?
- What authorizes access?
- What happens when the dependency is unavailable?
- What happens after restart?
- How is the operation verified?
- How is it backed up and restored?

If these answers are difficult to find, the design is probably too indirect.

## Mandatory pre-merge checklist

Before a BaseHarbor change is considered ready, verify:

- [ ] The change stays inside its declared scope.
- [ ] Existing architecture boundaries are preserved or deliberately updated and documented.
- [ ] Code is explicit and avoids unnecessary abstractions.
- [ ] Files/packages remain cohesive.
- [ ] Stable state is represented with strong types.
- [ ] Persistence invariants are enforced at an authoritative boundary.
- [ ] Tenant/application isolation fails closed.
- [ ] Secrets cannot leak through logs, errors, APIs, telemetry, audits, jobs, or manifests.
- [ ] Failures remain observable and actionable.
- [ ] Configuration has a clear owner.
- [ ] Migrations are forward/rollback safe where applicable.
- [ ] Backup/restore implications were considered where persistent state changes.
- [ ] Mutating runtime operations have preflight and post-verification where applicable.
- [ ] New dependencies are justified.
- [ ] Tests include meaningful negative cases for claimed security/isolation guarantees.
- [ ] Documentation matches the implemented behavior.
- [ ] The actual diff contains no unrelated changes.
- [ ] CI status is reported truthfully and was not rerun unnecessarily.
- [ ] Another developer can understand and maintain the change without hidden context.

## Guiding rule

BaseHarbor is not optimized for architectural novelty.

Optimize for correctness, security, isolation, explicit control flow, reliable operations, and long-term human maintenance.

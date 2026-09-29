# Security

BaseHarbor treats security as behavior, not a profile name.

Core rules:

- fail closed on ambiguity;
- least privilege;
- explicit ownership;
- secrets never enter normal output;
- generated credentials remain protected state;
- provider and runtime boundaries remain isolated;
- mutation is preceded by plan and preflight;
- success requires verification.

Development may be convenient, but convenience must not disable isolation, ownership checks or secret safety.

Human identity, workload identity and provider-administration credentials are different concerns and must not be reused as one another.

For normative requirements, see [Security invariants](../spec/security-invariants.md).

## Shared database administration boundary

Shared infrastructure does not create shared credentials.

For the managed shared PostgreSQL provider, `baseharbor_admin` is an internal control-plane credential only. Workloads receive only their application-specific role/password and database binding. Provider-admin credentials are protected provider state and are never exposed through application bindings, environment contracts, status, doctor, evidence or normal diagnostics.

Destructive shared-provider operations are registry/state driven and fail closed. BaseHarbor must prove that a database and role belong to the exact Application + Environment + SQL instance registration before dropping them. Reconstructed names or naming heuristics alone never authorize deletion.


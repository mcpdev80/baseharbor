# Security invariants

These requirements apply across BaseHarbor.

- Security-sensitive ambiguity MUST fail closed.
- Secret values MUST NOT appear in normal logs, errors, audit records, metrics labels, machine results or committed manifests.
- Ownership MUST be checked before destructive mutation.
- Knowing a resource identifier MUST NOT be sufficient authorization.
- Workload identity, human/operator identity and provider-admin credentials MUST remain distinct.
- Long-lived unrestricted provider-admin credentials MUST NOT be exposed to application workloads.
- Environment convenience MUST NOT disable isolation, ownership or secret-safety invariants.
- Required verification MUST NOT silently downgrade to process-health-only success.

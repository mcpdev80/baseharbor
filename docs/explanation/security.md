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

# Security invariants

These requirements apply across BaseHarbor.

- Security-sensitive ambiguity MUST fail closed.
- Secret values MUST NOT appear in normal logs, errors, audit records, metrics labels, machine results or committed manifests.
- Ownership MUST be checked before destructive mutation.
- Knowing a resource identifier MUST NOT be sufficient authorization.
- Workload identity, application-user identity, human/operator identity and provider-admin credentials MUST remain distinct.
- Managed credentials MUST follow the versioned A/B/C ownership taxonomy in `credential-access-v1`; Class C machine identity MUST never inherit shared human/developer or application-service credentials.
- Trusted local `dev` MUST NOT require operator login; `test` and `prod` application operations MUST fail closed without a valid Target/Environment OIDC operator boundary.
- Shared identity-provider infrastructure MUST NOT imply shared application/environment identity scope.
- Environment policy MAY strengthen application authentication requirements but MUST NOT silently weaken them.
- BaseHarbor MUST NOT process or store end-user passwords, TOTP seeds or WebAuthn/passkey credentials as an identity provider.
- Long-lived unrestricted provider-admin credentials MUST NOT be exposed to application workloads.
- Environment convenience MUST NOT disable isolation, ownership or secret-safety invariants.
- Required verification MUST NOT silently downgrade to process-health-only success.

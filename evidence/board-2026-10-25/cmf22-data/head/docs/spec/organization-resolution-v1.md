# Organization configuration resolution v1

Status: implemented resolver and Target-selection boundary; complete field-wise
lifecycle consumer qualification remains pending #609/#808.

Portable Application Intent declares requirements. It is not a preference layer
and no preference may delete, weaken or silently reinterpret those requirements.
The distribution foundation loads and pins the active organization source;
this resolver determines effective references and independent constraints.

## Scopes

| Order | Scope | Authority |
| --- | --- | --- |
| 0 | `builtin` | Core defaults (`local` Target fallback); versioned canonical input digest |
| 1 | `organization` | Active managed configuration; common defaults, then selected environment defaults |
| 2 | `team` | Optional fixed team block in that managed source; no request-selected team switching |
| 3 | `user` | Local preferences; immutable canonical-input digest |
| 4 | `repository` | Application-local preferences; immutable canonical-input digest, separate from portable manifest requirements |
| 5 | `invocation` | Explicit permitted request selections |

Higher-order preferences win field by field. Different input ordering does not
change the result. A request may supply only user/repository/invocation scopes.
It cannot forge a managed organization/team layer or inject policy references.
Team membership/onboarding is outside this resolver; the managed source fixes
the applicable team block. It does not assert that a caller's claimed team is
an authenticated group membership.

For Target selection, configured/activated local Target preference is `user`;
an explicit operation Target is `invocation`. Both are checked against the same
managed constraints before resolving a runtime. Local use without an active
organization still follows native Target configuration, without mandatory login.

## Field semantics

| Field | Merge / absence / replacement |
| --- | --- |
| Target / stack | Nonempty selection replaces the prior preference. Empty/absent is no selection, not deletion. |
| Providers | Map keys are capabilities; each supplied provider entry replaces that key as one unit. Other keys remain. |
| Trust | Map entries replace their own named reference. Unspecified names remain. |
| Optional policy references | A supplied managed policy list replaces optional inherited references. |
| Mandatory policy references | Always retained; empty lists and `mandatory: false` cannot remove/downgrade inherited mandatory policy. |
| Org/team constraints | Independent conjunction over the final choice; never a preference override. |

Supported constrained fields are `target`, `stack`, `provider.<capability>` and
`trust.<name>`. Each has a nonempty unique `allowed` set. Org and team constraints
intersect; a conflict is a typed `policy_denied` with cause
`configuration_constraint_denied`, affected field and safe next step.
Named Target/provider/stack references are canonicalized before comparison so
using a resolved reference cannot bypass or spuriously fail a named constraint.
Managed undeclared references fail. A request-selected Target must also exist
in the deployment Target registry before runtime use.

Unknown scopes, duplicate scope layers, invalid digests, unsupported constraint
fields, unknown wire fields, inline credentials/private keys and lower-trust
policy injection fail before mutation. This is reference selection, not a
general company-policy scripting language or a second authorization model.

## Provenance and pinning

`effective.provenance[field]` exposes `kind: preference`, the winning
scope/identity/digest/source/value and ordered overridden candidates.
`effective.policy_decisions` identifies each independent constraint's scope,
identity and allow/deny outcome. Existing mandatory policy references retain
their own source and mandatory flag.

Org/team provenance binds to the active immutable distribution digest/revision.
Preference digests are SHA-256 over JSON serialization of the typed defaults,
with deterministic map-key ordering; `orgconfig.PreferenceDigest` is canonical.
An invocation without a supplied digest receives that digest from Core. A
supplied digest must match the same canonical input. It is a content identity,
not proof of privileged authority. Credentials are obtained separately through
existing protected credential references.

`organization.check` never changes active resolution; an explicit authorized
`organization.update` activates new pinned content. Offline reuse resolves the
active inputs; it never silently follows a mutable Git/OCI tag.

CLI `config organization show --preferences FILE -o json`, MCP
`organization.inspect` and HTTP `organization.inspect` accept the same lower-trust
preference-layer array and return the same typed effective model. Browser clients
display Core provenance rather than recomputing company rules locally.

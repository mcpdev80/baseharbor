# ADR 0020: Scoped Connector enrollment through the existing authority

Status: accepted for the enrollment boundary; production HTTP/session integration
and runtime qualification remain in progress for #807.

## Decision

Core signs a Connector's locally generated PKCS#10 key through the existing
protected OpenBao PKI authority. A separate `baseharbor-nodes` role signs only
client identities below `spiffe://baseharbor/platform/connectors/`. Core neither
generates nor accepts a Connector private key. The role limits certificates to
24 hours and digital-signature/client-auth usage. The existing manager policy
receives only the additional signing endpoint permission.

Core selects tenant, Target, node and Docker/Podman runtime before granting
bootstrap admission. Their stable URI identity includes all three scope
components. Tenant identifiers are canonical lowercase UUIDs; Target/node
identifiers are bounded path segments. The scoped token and nonce each contain
256 random bits. An authorization expires within ten minutes. PostgreSQL stores
only SHA-256 digests of the token and nonce, not bearer material.

The durable node registry binds a tenant's node identifier to exactly one Target
and runtime. Pending unconsumed grants cannot be duplicated. After successful
signing, Core persists the exact admitted certificate serial and expiry before
returning material; an enrolled identity requires the separate renewal path.
Certificate persistence failure attempts to revoke the newly issued leaf and
returns no success. This registry alone is not live TLS/session admission.

The persistent store consumes authorization with one conditional `UPDATE` and
commits before requesting a signature. Tenant RLS and an explicit tenant predicate
both apply. Wrong scope, expired authorization and already consumed authorization
return the same denial. Consumption retains a digest of the signed CSR. A
signing failure requires a fresh authorization: the consumed credential is not
restored, including after process restart or an ambiguous response failure.

Consumption and identity recording form two phases that commit in separate
transactions. Between them the grant is durably consumed while the node has no
admitted serial yet. The boundary is deliberate and stays fail-closed: no active
identity exists before the second phase commits, the one-use grant is never
restored, and the only recovery is a fresh initial or renewal authorization;
`Enroll` also revokes the certificate it signed when that second phase fails. The
`previous_certificate_serial` clause in the consume statement and the
serial-is-null guard plus consumed-grant predicate in the identity write prevent a
competing request from racing the serial write, so a half-finished enrollment can
never admit two identities for one node. What the two phases leave behind is
therefore a dead, permanently consumed grant, not a usable partial identity.

Core verifies the result against its authoritative trust bundle, CSR public key,
scoped URI, client-only key usage, serial and expiry. A CA returned next to a leaf
cannot replace authoritative trust. PEM parsing rejects additional requests,
private keys and leading/trailing material. CSR and manager credentials travel
through protected process input, not command arguments.

The role parameters follow the existing [OpenBao PKI API](https://openbao.org/docs/api/secret/pki/).
This extends the existing issuer boundary rather than introducing a second CA.

## Qualification boundary

The implemented authority/store are not yet a complete operator admission,
bootstrap HTTP endpoint, live node admission, renewal/revocation or outbound
session service. Shared authorization/audit wiring, duplicate identity admission,
authenticated capability negotiation, remote runtime realization and the exact-ref
consumer gates must pass before remote support is advertised. Source tests do not
qualify real Docker/Podman operations or production authority behavior.

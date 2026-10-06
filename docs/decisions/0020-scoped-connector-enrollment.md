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

The persistent store consumes authorization with one conditional `UPDATE` and
commits before requesting a signature. Tenant RLS and an explicit tenant predicate
both apply. Wrong scope, expired authorization and already consumed authorization
return the same denial. Consumption retains a digest of the signed CSR. A
signing failure requires a fresh authorization: the consumed credential is not
restored, including after process restart or an ambiguous response failure.

Core verifies the result against its authoritative trust bundle, CSR public key,
scoped URI, client-only key usage, serial and expiry. A CA returned next to a leaf
cannot replace authoritative trust. PEM parsing rejects additional requests,
private keys and leading/trailing material. CSR and manager credentials travel
through protected process input, not command arguments.

The role parameters follow the existing [OpenBao PKI API](https://openbao.org/docs/api/secret/pki/).
This extends the existing issuer boundary rather than introducing a second CA.

## Qualification boundary

The implemented authority/store are not yet a complete operator admission,
bootstrap HTTP endpoint, durable node registry, renewal/revocation or outbound
session service. Shared authorization/audit wiring, duplicate identity admission,
authenticated capability negotiation, remote runtime realization and the exact-ref
consumer gates must pass before remote support is advertised. Source tests do not
qualify real Docker/Podman operations or production authority behavior.

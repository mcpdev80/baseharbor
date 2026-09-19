# Runtime broker security properties

The Application Runtime Broker is intentionally designed around blast-radius reduction rather than one shared multi-tenant service.

A compromise of one application broker must not grant access to:

- another application's backend network;
- another application's runtime identity;
- another application's OpenBao AppRole or secret namespace;
- the BaseHarbor manager AppRole;
- the Docker/Podman socket;
- the host filesystem outside explicitly mounted runtime identity files.

The broker therefore runs per application, on two networks only: the application backend network and the internal `baseharbor-secrets` network. It has no published host port for normal runtime traffic.

Authentication is defense in depth: mTLS with an application-specific URI SAN is required at the TLS layer, then the application-scoped runtime token is verified, then OpenBao independently limits the broker through the application's AppRole policy.

## v0.4.5 secure-binding mapping

The broker implementation remains unchanged, but its stable security semantics are now represented through `secure-binding/v1`: SPIFFE workload identity, opaque runtime credential/trust references, least-privilege authorization metadata and rotation/revocation support.

Provider-specific OpenBao/AppRole details and concrete certificate/key paths are intentionally not part of that binding model.

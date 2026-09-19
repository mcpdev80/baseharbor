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


## Development documentation listener

Interactive Runtime API documentation is a separate listener from the authenticated mTLS Runtime API.

In `dev`/`development`, BaseHarbor may publish that documentation listener to an automatically allocated host-loopback address only:

```text
127.0.0.1:<allocated-port>
```

The listener serves embedded Swagger UI assets and the canonical OpenAPI document only. It does not proxy Runtime API calls, does not expose runtime bearer tokens, client private keys, OpenBao credentials or secure bindings, and does not weaken mTLS or authorization on the real Runtime API.

Test/staging and production keep this listener disabled unless an operator explicitly opts in.

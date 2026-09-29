# Troubleshooting knowledge base

BaseHarbor keeps recurring runtime and acceptance failures in a versioned repository knowledge base.

Use this directory before starting a new Docker, Podman, TLS, identity, provider, or acceptance investigation.

## Workflow

1. Search `.github/known-failures.yaml` for an error signature.
2. Open the matching incident document in `docs/troubleshooting/incidents/`.
3. Check the recorded root cause, fixes, related issues, and regression tests.
4. Verify whether the current branch already contains the documented fix.
5. Only start a new CI investigation when the failure is new or the documented fix is present and the failure still reproduces.

## Incidents

- [Podman Quadlet rootless storage context](incidents/podman-quadlet-rootless-storage-context.md)
- [Podman local candidate image resolution](incidents/podman-local-candidate-image-resolution.md)
- [Podman network alias scope](incidents/podman-compose-network-alias-scope.md)
- [Developer gateway effective port](incidents/dev-gateway-effective-port.md)

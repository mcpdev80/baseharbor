# Podman Compose network alias scope

## Symptom

A service works under Docker Compose but cannot resolve a dependency by its expected alias under Podman Quadlet.

## Root cause

Compose aliases are scoped to an individual network. A global Quadlet `NetworkAlias=` entry does not preserve that model when a container joins several networks.

## Fix

Render aliases as options on the individual network attachment:

```text
Network=<network>.network:alias=<service>:alias=<compose-alias>
```

External networks use their actual network name instead of a generated `.network` unit.

Relevant commits:

- `11c4b065`
- `3c17c856`

Regression tests live in `internal/providers/runtime/podman/quadlet_project_test.go`.

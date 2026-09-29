# Podman Quadlet rootless storage context

## Signature

```text
Quadlet network resource <name> is missing after restarting <unit>-network.service
```

The same pattern can affect volumes.

## Symptom

A generated Quadlet resource unit starts or restarts, but the follow-up Podman CLI resource check does not see the network or volume.

Docker is not affected because the Docker Compose path does not cross the rootless Podman CLI/systemd boundary.

## Root cause

Rootless Podman state is sensitive to the process environment and storage configuration. The interactive BaseHarbor process and the systemd user manager that executes generated Quadlet units must use the same relevant XDG and containers-storage environment.

## Fixes already applied

- `5568542f` propagates Podman storage environment into generated Quadlet units.
- `81d3627d` adds regression coverage for that environment propagation.

## Current status

Investigating. If the signature still appears with both fixes present, inspect whether the resource unit actually creates the resource before changing timeouts.

## Verification

```text
go test ./internal/providers/runtime/podman
targeted Podman guided gate
targeted Podman identity gate
```

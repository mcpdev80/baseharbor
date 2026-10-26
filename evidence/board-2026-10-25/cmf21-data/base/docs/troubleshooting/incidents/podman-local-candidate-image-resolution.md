# Podman local candidate image resolution

## Signatures

```text
image pull ... baseharbor-runtime:demo-candidate
```

or runtime behavior that does not match the checked-out source.

## Root cause

Docker accepts the locally built `baseharbor-runtime:demo-candidate` tag directly. Podman/Quadlet can resolve an unqualified image differently.

## Fix

Local BaseHarbor images are rendered for Podman as:

```text
Image=localhost/baseharbor-runtime:demo-candidate
Pull=never
```

Commits:

- `c2c6fbd7`
- `7589bef4`

This guarantees that a targeted Podman acceptance run uses the freshly built local candidate image.

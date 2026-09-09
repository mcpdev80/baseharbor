# Releases and versioning

BaseHarbor uses Semantic Versioning with Git tags prefixed by `v`.

Examples:

- `v0.1.0`
- `v0.1.1`
- `v0.2.0`
- `v1.0.0`

## Stability policy

BaseHarbor is currently in the `0.x` development series.

During `0.x`:

- patch releases (`0.1.0` -> `0.1.1`) are backward-compatible bug and security fixes;
- minor releases (`0.1.x` -> `0.2.0`) may contain intentionally documented breaking changes;
- every breaking change must be called out in `CHANGELOG.md` and in the GitHub Release notes;
- consumers should pin an explicit compatible range instead of tracking `main`.

After `v1.0.0`:

- breaking public CLI, manifest, persisted-state or supported integration changes require a new major version;
- minor releases add backward-compatible functionality;
- patch releases contain backward-compatible fixes.

## What is part of the compatibility contract?

The versioned BaseHarbor contract includes:

- `baha` command names, flags, exit-code behavior and machine-consumable output that is documented as stable;
- the `baseharbor.yaml` manifest schema;
- persisted BaseHarbor control-plane and application runtime state;
- backup/restore formats documented as supported;
- application-facing environment/service contracts;
- published runtime image tags and their relationship to CLI versions.

Internal Go packages are not a public API unless explicitly documented otherwise.

## Development versus releases

`main` is development state and must not be used as a production dependency.

Published channels:

- GitHub tag/release `vX.Y.Z`: immutable supported release;
- `ghcr.io/mcpdev80/baseharbor-runtime:X.Y.Z`: versioned runtime image;
- `ghcr.io/mcpdev80/baseharbor-runtime:latest`: latest stable release;
- `ghcr.io/mcpdev80/baseharbor-runtime:edge`: moving development build from `main`.

Applications should install a released `baha` binary and pin a compatible version range.

## Release preparation

Every release starts with a release-preparation pull request.

1. Ensure all required CI and real-product acceptance gates are green on the intended commit.
2. Move the relevant entries from `[Unreleased]` into a dated `## [X.Y.Z] - YYYY-MM-DD` section in `CHANGELOG.md`.
3. Review the change set for compatibility impact and select the SemVer increment.
4. Merge the release-preparation pull request to `main`.
5. Create an annotated tag `vX.Y.Z` on that exact green `main` commit.
6. Push the tag.
7. The release workflow validates the tag, confirms the changelog section exists, tests the source, builds release artifacts, creates the GitHub Release and publishes provenance attestations.
8. Verify the published artifacts and runtime image before declaring the release usable by downstream products.

Do not move or recreate published version tags. If a release is bad, publish a new patch release.

## Release artifacts

Each stable release publishes:

- `baseharbor_linux_amd64.tar.gz`
- `baseharbor_linux_arm64.tar.gz`
- `checksums.txt`
- GitHub build-provenance attestations
- versioned GHCR runtime images

The binary reports its exact version and source metadata through:

```bash
baha version
```

## Verification

Verify the checksum before installing a downloaded archive:

```bash
sha256sum -c checksums.txt --ignore-missing
```

GitHub provenance can additionally be verified with GitHub CLI:

```bash
gh attestation verify baseharbor_linux_amd64.tar.gz \
  -R mcpdev80/baseharbor
```

## Consumer guidance

A real application should never silently follow `main`.

During the `0.x` series, an application tested against `v0.1.0` should normally constrain itself to the compatible minor line, for example:

```text
>=0.1.0 <0.2.0
```

Moving to the next minor line should be an explicit tested upgrade because `0.x` minor releases may contain documented breaking changes.

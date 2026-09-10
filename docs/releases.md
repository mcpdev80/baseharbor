# Releases and versioning

BaseHarbor uses Semantic Versioning with Git tags prefixed by `v`.

Examples: `v0.2.0`, `v0.2.1`, `v0.3.0`, `v1.0.0`.

## Stability policy

BaseHarbor is currently in the `0.x` series.

During `0.x`:

- patch releases (`0.2.0` -> `0.2.1`) are backward-compatible bug and security fixes;
- minor releases (`0.2.x` -> `0.3.0`) may contain intentionally documented breaking changes;
- every breaking change must be called out in `CHANGELOG.md` and GitHub Release notes;
- consumers should pin an explicit compatible range instead of tracking `main`.

After `v1.0.0`, breaking public CLI, manifest, persisted-state or supported integration changes require a new major version.

## Compatibility contract

The versioned contract includes documented `baha` commands and flags, the `baseharbor.yaml` schema, persisted BaseHarbor state, supported backup/restore formats, application-facing environment/service contracts, and runtime image tags coupled to CLI versions.

Internal Go packages are not public API unless explicitly documented.

## Development versus releases

`main` is development state and must not be used as a production dependency.

Published channels:

- GitHub tag/release `vX.Y.Z`: immutable supported release;
- `ghcr.io/mcpdev80/baseharbor-runtime:X.Y.Z`: matching runtime image;
- `ghcr.io/mcpdev80/baseharbor-runtime:latest`: latest stable release;
- `ghcr.io/mcpdev80/baseharbor-runtime:edge`: moving development build from `main`.

## Release preparation

Every release starts with a release-preparation pull request.

1. Ensure all required CI and real-product acceptance gates are green.
2. Move relevant entries from `[Unreleased]` into a dated `## [X.Y.Z] - YYYY-MM-DD` section in `CHANGELOG.md`.
3. Review compatibility impact and select the SemVer increment.
4. Merge the release-preparation PR to `main`.
5. Create an immutable tag `vX.Y.Z` on that exact green `main` commit.
6. Push the tag.
7. The release workflow validates the tag, changelog section and source, tests the code, publishes artifacts and provenance.
8. Verify binaries, checksums, provenance and the matching runtime image before declaring the release usable.

Never move a published version tag. Fix a bad release with a new patch release.

## Release artifacts

Each release publishes:

- `baseharbor_linux_amd64.tar.gz`
- `baseharbor_linux_arm64.tar.gz`
- `checksums.txt`
- GitHub build-provenance attestations
- matching versioned GHCR runtime images

Verify binary metadata with:

```bash
baha version
```

Verify checksums with:

```bash
sha256sum -c checksums.txt --ignore-missing
```

GitHub provenance can additionally be verified with:

```bash
gh attestation verify baseharbor_linux_amd64.tar.gz -R mcpdev80/baseharbor
```

## Consumer guidance

A real application should never silently follow `main`. During the `0.x` series, an application tested against `v0.2.0` should normally constrain itself to the compatible minor line, for example `>=0.2.0 <0.3.0`, unless it intentionally validates against a newer minor release.

# Dependency updates

BaseHarbor uses Renovate as the default dependency update mechanism.

## Policy

BaseHarbor is consumed by real applications, so dependency automation must reduce maintenance work without silently making platform, security or compatibility decisions.

- Renovate scans the repository once per day.
- Supported dependency sources include Go modules, GitHub Actions, Dockerfiles and Docker Compose images.
- `config:best-practices` is used so GitHub Actions and container references can be pinned to immutable revisions/digests where supported.
- Routine patch, pin and digest updates may automerge only after the configured release-age buffer and only when all required checks are green.
- Minor and major updates always require human review.
- Multiple major-version jumps are kept separate so breaking changes are reviewed sequentially.
- Go language/toolchain changes always require human review, including the `go` directive and `golang` build images.
- Authentication and cryptography dependencies always require human review, even for patch updates.
- Container minor/major updates always remain separate and require explicit runtime review.
- Vulnerability-alert PRs bypass the normal release-age cooldown, are labeled `security`, and never automerge.
- A fourteen-day minimum release age is used for normal dependency updates before they become eligible for automerge.
- Lock-file maintenance runs weekly and may automerge after green checks because it does not intentionally change declared dependency ranges.
- Renovate uses a maximum of five concurrent regular PRs/branches and five concurrent vulnerability PRs to avoid update floods.
- If CI is red or pending, Renovate must not merge the PR.

## Update classes

### Automatic after green CI and cooldown

- patch updates for non-sensitive libraries
- pin updates
- immutable digest/SHA refreshes
- routine container patch/digest updates
- weekly lock-file maintenance

### Manual review required

- all minor updates
- all major updates
- Go language/toolchain changes
- authentication/OIDC/OAuth dependencies
- cryptography dependencies
- container runtime/base-image minor or major changes
- vulnerability/security alert PRs

A green CI result is necessary but not sufficient for these classes. The reviewer must consider compatibility, release notes, migration impact, runtime behavior and the current BaseHarbor release contract.

## Supply-chain policy

GitHub Actions should resolve to immutable commit SHAs and container images should use immutable digests where practical. Renovate owns the routine maintenance of those pins. Human-readable version information should be retained in comments or metadata so pinned revisions remain understandable during review.

Release workflows are part of the product supply chain and follow the same rule. Dependency updates that alter build, signing, provenance, release publishing or packaging behavior require explicit review regardless of CI status.

## Authentication

The self-hosted Renovate workflow requires a repository secret named `RENOVATE_TOKEN`.

Use a dedicated GitHub token rather than the workflow `GITHUB_TOKEN`, because dependency PRs created with `GITHUB_TOKEN` otherwise require manual workflow approval. The token must be able to update this public repository and, because Renovate manages GitHub Actions versions, must also be permitted to modify workflow files.

Keep the token scoped to BaseHarbor as narrowly as practical and rotate it regularly.

For GitHub vulnerability-alert integration, the repository must also have Dependency Graph and Dependabot alerts enabled, and the Renovate token/app must have read access to those alerts.

## Repository protection

`main` should require the BaseHarbor CI and acceptance checks relevant to the changed surface before merge. This remains a defense-in-depth control even though Renovate is configured with platform automerge disabled and verifies green checks itself before merging.

Human-authored feature PRs continue to follow the normal review/merge process; this policy applies only to Renovate dependency PRs.

# Dependency updates

BaseHarbor uses Renovate as the default dependency update mechanism.

## Policy

- Renovate scans the repository once per day.
- Supported dependency sources include Go modules, GitHub Actions, Dockerfiles and Docker Compose images.
- Routine patch/minor/pin/digest updates are grouped into a daily PR.
- Major updates remain separate so failures are easier to isolate.
- All Renovate PRs are eligible for automerge, including major updates.
- Renovate itself performs the merge only after required status checks are green.
- If CI is red or pending, Renovate must not merge the PR.
- Renovate does not rerun after every CI completion; this avoids unnecessary GitHub Actions usage.
- Lock-file maintenance is checked daily.

## Authentication

The self-hosted Renovate workflow requires a repository secret named `RENOVATE_TOKEN`.

Use a dedicated GitHub token rather than the workflow `GITHUB_TOKEN`, because dependency PRs created with `GITHUB_TOKEN` otherwise require manual workflow approval. The token must be able to update this public repository and, because Renovate manages GitHub Actions versions, must also be permitted to modify workflow files.

Keep the token scoped to BaseHarbor as narrowly as practical and rotate it regularly.

## Repository protection

`main` should require the BaseHarbor CI check before merge. This remains a defense-in-depth control even though Renovate is configured with platform automerge disabled and verifies green checks itself before merging.

Human-authored feature PRs continue to follow the normal review/merge process; this policy applies only to Renovate dependency PRs.

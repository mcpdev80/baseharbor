# Dependency updates

BaseHarbor uses Renovate as the default dependency update mechanism.

## Policy

- Renovate scans the repository once per day.
- Supported dependency sources include Go modules, GitHub Actions, Dockerfiles and Docker Compose images.
- Routine patch/minor/pin/digest updates are grouped into a daily PR.
- Major updates remain separate so failures are easier to isolate.
- All Renovate PRs are eligible for automerge, including major updates.
- Renovate itself performs the merge only after status checks are green.
- If CI is red or pending, Renovate must not merge the PR.
- Renovate runs again whenever the `ci` workflow completes so a green dependency PR can be merged without waiting for the next daily scan.
- Lock-file maintenance is checked daily.

## Authentication

The self-hosted Renovate workflow requires a repository secret named `RENOVATE_TOKEN`.

Use a dedicated GitHub token rather than the workflow `GITHUB_TOKEN`. GitHub requires manual approval for workflows created by PRs that originate from `GITHUB_TOKEN`, which would defeat unattended dependency updates. The token must be able to update this public repository and, because Renovate manages GitHub Actions versions, must also be permitted to modify workflow files.

For a classic PAT on this public repository, use the narrowest practical permissions that include `public_repo` and `workflow`. A narrowly scoped GitHub App token is preferable later if BaseHarbor grows into an organization-managed project.

Keep the token dedicated to Renovate and rotate it regularly.

## Repository protection

`main` should require the BaseHarbor CI check before merge. This is defense in depth: Renovate has platform automerge disabled and checks green CI itself before merging, but branch protection should still prevent accidental human or automation merges that bypass CI.

Human-authored feature PRs continue to follow the normal review/merge process; this policy applies only to Renovate dependency PRs.

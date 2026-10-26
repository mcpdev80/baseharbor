# Troubleshooting agent rules

Before investigating a Docker, Podman, TLS, identity, provider, or acceptance failure:

1. Search `.github/known-failures.yaml` for the observed error signature.
2. Read the linked incident under `docs/troubleshooting/incidents/`.
3. Verify whether the documented fix commits are present in the current branch.
4. Reuse the documented regression test before starting new GitHub CI.
5. Do not repeat a known CI experiment unless the existing evidence is stale or the relevant code changed.
6. When a new recurring failure is resolved, add or update both the registry entry and its incident document.

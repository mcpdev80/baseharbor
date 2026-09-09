# Pre-release documentation audit

This audit is the cleanup gate before the first official BaseHarbor release and before publishing the GitHub Pages site.

## Confirmed current behavior on `main`

- `baha up` supports guided first-run port selection plus `--yes`, `--postgres-port` and `--openbao-port`.
- control-plane runtime state is user-global by default and survives application checkout changes.
- `baha app init` creates a repository-owned `baseharbor.yaml` and is the preferred application workflow.
- repository app commands can resolve the nearest `baseharbor.yaml` without repeating the application name.
- PostgreSQL and Valkey support multiple named logical instances.
- `baha app env` exposes the protected application environment/binding contract without making BaseHarbor a runtime SDK dependency.
- required secrets gate workload startup.
- static secret injection/file binding and dynamic app-scoped secret references are implemented.
- the per-application runtime broker uses scoped runtime identity and mTLS.
- `baha app backup` and `baha app restore` create and restore encrypted recovery units including application metadata, managed PostgreSQL instances and the application OpenBao scope.
- `baha app status` and `baha app doctor` verify real service/application boundaries rather than only container state.
- OpenBao bootstrap, status and manual unseal are implemented.

## Problems corrected

The pre-release refresh replaced the old early-development README story, updated the CLI command tree, documented current OpenBao lifecycle and guided port handling, added backup/restore documentation and consolidated duplicate PostgreSQL entry points.

The earlier control-plane state-location blocker has been resolved: machine/user-scoped Docker resources and manager credentials now use user-global runtime state by default, with explicit override and legacy compatibility.

## Remaining release/documentation gate

Before tagging `v0.1.0`:

1. merge the release-management machinery on top of the final `main`;
2. publish and verify the bilingual GitHub Pages site from these sources;
3. run the final real-product clean-install acceptance path;
4. finalize the dated `0.1.0` changelog/release notes;
5. verify release binaries, checksums, provenance and runtime-image version coupling.

The Pages site must render repository documentation rather than maintaining a separate independent prose source.
# Pre-release documentation audit

This audit is the cleanup gate before the first official BaseHarbor release and before publishing the GitHub Pages site.

## Confirmed current behavior on `main`

- `baha up` supports guided first-run port selection plus `--yes`, `--postgres-port` and `--openbao-port`.
- `baha app init` creates a repository-owned `baseharbor.yaml` and is the preferred application workflow.
- Repository app commands can resolve the nearest `baseharbor.yaml` without repeating the application name.
- PostgreSQL and Valkey support multiple named logical instances.
- `baha app env` exposes the protected application environment/binding contract without making BaseHarbor a runtime SDK dependency.
- Required secrets gate workload startup.
- Static secret injection/file binding and dynamic app-scoped secret references are implemented.
- The per-application runtime broker uses scoped runtime identity and mTLS.
- `baha app backup` and `baha app restore` create and restore encrypted recovery units including application metadata, all managed PostgreSQL instances and the application OpenBao scope.
- `baha app status` and `baha app doctor` verify real service/application boundaries rather than only container state.
- OpenBao bootstrap, status and manual unseal are implemented.

## Documentation problems found

### `README.md`

Previously:

- described the project mainly as "Early development";
- presented `go build` as the normal installation path;
- omitted repository-first workflow, named service instances, app env bindings and backup/restore from the primary product story;
- described dynamic/runtime secret delivery as future work even though it exists.

Action: rewrite as product overview + quickstart and route details into `docs/`.

### `docs/cli.md`

Previously missing or stale:

- `baha serve`;
- `baha app init`;
- `baha app env`;
- `baha app backup` / `restore`;
- runtime identity commands;
- named PostgreSQL/Valkey instances;
- repository manifest as the preferred workflow;
- current secret delivery and broker behavior;
- current `baha up` port-selection behavior.

Action: replace command tree and lifecycle examples with the current CLI contract.

### `docs/runtime-compose.md`

Previously stated that OpenBao initialization/unseal commands were future work. They are implemented.

Action: document current guided ports, current OpenBao lifecycle and the exact pre-v0.1 state-location caveat.

### Duplicate/overlapping PostgreSQL documentation

`postgresql.md` and `postgresql-and-migrations.md` overlap. The longer `postgresql-and-migrations.md` is the canonical technical document. `postgresql.md` should remain only if it has a distinct purpose; otherwise it should become a short redirect or be removed in a follow-up cleanup.

## Important pre-v0.1 caveat: control-plane state location

The Docker control plane is machine/user scoped, but current `main` still resolves default control-plane files from `.baseharbor/runtime` relative to the current working tree. This is the architectural inconsistency found by the real MailFlow clean-install test.

PR #72 changes the default to user-global XDG/Home state while preserving explicit overrides and legacy compatibility. Until that change lands, public documentation must not claim that global state is already the released behavior.

This item is a **release blocker** for `v0.1.0` because a fresh clone of an application can otherwise create local manager state against already-existing global OpenBao volumes.

## Release/documentation gate

Before tagging `v0.1.0`:

1. merge the global control-plane state fix;
2. merge release management and Renovate hardening;
3. re-run the clean real-product acceptance path;
4. update any state-path wording in these docs to the final merged behavior;
5. ensure the README, CLI, application contract, runtime, secrets and backup/restore pages agree with executable behavior;
6. only then publish the bilingual GitHub Pages site.

The Pages site must render these Markdown sources rather than maintaining separate prose.
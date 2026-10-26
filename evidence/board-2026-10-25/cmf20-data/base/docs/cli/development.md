# Development commands

Development commands configure how applications are created and worked on locally. Development preferences remain separate from portable Application Intent.

## Stack Profiles

```text
baha stack list
baha stack show NAME
baha stack create
```

Stack Profiles describe reusable development preferences and can be supplied by built-in, user, repository or organization catalogs.

## Developer access

```text
baha dev credentials
baha dev domain
```

These commands expose Target-local developer access information without changing application capability intent.

## Workspaces

Application workspace commands manage local source mappings for multi-repository applications.

Workspace paths are developer-local state and never become portable application identity.

The normal workspace surface is:

```text
baha app workspace status
baha app workspace update --check
baha app workspace update
```

`status` is read-only. `update --check` fetches upstream state and previews safe changes without moving checked-out revisions. `update` uses native Git and only performs fast-forward-only updates of clean branches with a configured upstream.

BaseHarbor never automatically stashes, resets, rebases, merges divergent history, resolves conflicts or switches branches. Dirty, detached, ahead-only and diverged repositories are left untouched with an actionable native-Git next step.

The existing `init`, `map`, `show` and `resolve` commands remain available for deterministic workspace mapping and resolution.

## Example: map a local API checkout

Inside the generated `orders-api` repository, suppose its existing Git checkout has a configured `origin`. Capture that real repository identity and map the scaffold's component `app`:

```bash
repository_url="$(git remote get-url origin)"
baha app workspace init --source backend="$repository_url" --component app=backend
baha app workspace map backend "$PWD"
baha app workspace show
baha app workspace resolve -o json
baha app workspace status
baha app workspace update --check
```

The source identity belongs in `.baseharbor/sources.yaml`; the absolute checkout path stays developer-local. `resolve` should map component `app` to this checkout. The final command fetches upstream information and previews possible updates without moving your branch. Use `baha app workspace update` only when you intend the eligible clean branches to fast-forward.

For another component, add its actual source identity with another `--source` and its manifest component with another `--component`; then map that source to its checkout. Do not invent component names absent from the application contract.

## Example: inspect stack defaults

```bash
baha stack list
baha stack show go
```

This lets you inspect the built-in Go development profile before selecting `--stack go` in `app new`. Stack preferences are development configuration, not a SQL/cache provider selection.

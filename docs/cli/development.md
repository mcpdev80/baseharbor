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

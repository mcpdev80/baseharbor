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

See the application namespace help for the current `baha app workspace ...` command tree.

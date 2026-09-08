# Repository-first application workflow

BaseHarbor treats `baseharbor.yaml` in an application repository as the preferred application contract.

The repository contains only declarative requirements. Secret values, generated credentials, runtime environment files and service state do not belong in Git.

## Repository layout

```text
myapp/
├── baseharbor.yaml
├── backend/
├── frontend/
└── .baseharbor/
```

`baseharbor.yaml` is intended to be committed. `.baseharbor/` is generated runtime state and is automatically protected by a nested `.gitignore`.

A normal repository manifest may use standard Git-friendly permissions such as `0644`; BaseHarbor rejects it when it is writable by group or others. Generated runtime files remain owner-only.

## Create a manifest

A deterministic non-interactive form is available for scripts and CI:

```bash
baha app init mailflow --postgres --redis
```

or for multiple logical services:

```bash
baha app init mailflow \
  --postgres-instance primary \
  --postgres-instance analytics \
  --redis-instance cache \
  --redis-instance sessions
```

When `NAME` is omitted, `baha app init` uses the current directory name when it is a valid application slug.

An interactive checkbox-based capability picker is planned on top of the same generator. The generated manifest contract is identical whether it came from flags, the guided UI or manual editing.

## Work from anywhere inside the repository

BaseHarbor searches the current directory and then parent directories for the nearest `baseharbor.yaml`.

This works from the repository root:

```bash
baha app plan
baha app apply
```

and from nested directories such as `frontend/src`:

```bash
baha app status
baha app doctor
```

Runtime state is always anchored next to the discovered manifest:

```text
<repo>/.baseharbor/apps/<application>/
```

BaseHarbor does not create another `.baseharbor` tree in a nested working directory.

## Normal developer flow

```bash
git clone <application-repository>
cd <application-repository>
baha app plan
baha app apply
```

After apply, normal application tooling consumes the generated standard runtime contract:

```bash
baha app env --path
```

The application itself does not need `baha`, a BaseHarbor login or a BaseHarbor SDK at runtime. It continues to consume normal values such as `DATABASE_URL`, named database URLs, Redis/Valkey URLs, OIDC metadata and binding files.

## Secrets

The repository manifest contains secret names only:

```yaml
secrets:
  required:
    - name: OPENAI_API_KEY
    - name: SMTP_PASSWORD
```

Inside the repository, the application name is inferred:

```bash
printf '%s' "$SMTP_PASSWORD" | baha app secret set SMTP_PASSWORD --stdin
baha app secret list
baha app secret delete SMTP_PASSWORD --yes
```

Values remain outside Git and are never accepted as command-line arguments.

## Explicit-name compatibility

Repository discovery is a convenience, not a new runtime dependency. Existing explicit-name workflows remain valid:

```bash
baha app status mailflow
baha app doctor mailflow
baha app env mailflow
```

## Source-of-truth semantics

For repository-managed applications:

```text
baseharbor.yaml
      ↓
parse + validate
      ↓
desired state
      ↓
protected internal runtime state
      ↓
PostgreSQL / Valkey / secrets / future services
```

The repository manifest is authoritative. BaseHarbor may keep a protected internal copy to support its runtime services, but changing the repository manifest and applying it again converges toward the new desired state without rotating unrelated existing credentials or state.

`baha app destroy --yes` deletes BaseHarbor-managed runtime resources and `.baseharbor` application state, but preserves the repository `baseharbor.yaml`. Running `baha app apply` can therefore recreate the backend from the committed contract.

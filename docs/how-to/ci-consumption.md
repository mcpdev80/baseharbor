# CI consumption of a BaseHarbor release

BaseHarbor Core is a single `baha` binary plus a version-coupled runtime image.
A continuous integration pipeline consumes the **already produced** release — it
pins the image by immutable digest, starts the Core, drives an application
through its lifecycle with explicit inputs, and reads back structured,
fail-closed results. The pipeline never builds BaseHarbor, never rebuilds the
workload, and never touches a runner, secret, or database that BaseHarbor owns.

This page is a developer integration guide. It is not the BaseHarbor release or
acceptance gate. The examples exercise only the public `baha` command surface on
`develop` and the pinned runtime image. Keep secrets out of the pipeline and do
not wire these steps into an automatic trigger.

## What "consumption" means here

Consumption is headless control of a pinned release from a pipeline:

- Pin the exact released image by digest, so the same artifact runs in every
  stage (commit → test → acceptance). No rebuild, no `develop` edge image.
- Start Core once with an explicit target and environment, using the
  `--control-plane-only` operator path so the pipeline owns the Core lifecycle.
- Drive the application through its lifecycle with explicit, non-interactive
  inputs (`--no-input`, `--yes` where supported).
- Read back stable JSON and exit codes before using any value.

The same image and the same commands run on a developer workstation.
Consumption is only the non-interactive, machine-readable use of that surface.

## The pinned release surface

BaseHarbor publishes, for every release `X.Y.Z` (and `0.x` hotfix `X.Y.Z.H`):

- Core image `ghcr.io/mcpdev80/baseharbor-runtime:X.Y.Z` (immutable digest coupling);
- matching runtime image `ghcr.io/mcpdev80/baseharbor-runtime:X.Y.Z`;
- the `baha` binary whose version matches the release.

See [Releases and versioning](../reference/releases.md) for the full channel table and the
compatibility policy. During the `0.x` line, patch releases stay backward
compatible within the active minor line; pin an explicit range instead of
tracking `develop`.

```text
released Core image  ghcr.io/mcpdev80/baseharbor-runtime:X.Y.Z  (immutable digest)
matching runtime     ghcr.io/mcpdev80/baseharbor-runtime:X.Y.Z
CLI build            baha <X.Y.Z>  (version matches the pinned release)
```

## Global non-interactive surface

Every command shares these global options. Automation uses them, never
decorative human output.

| Flag | Meaning |
| --- | --- |
| `--no-input` | Never prompt; fail with an actionable error when a required decision is missing. |
| `--plain` | Stable, styling-free, line-oriented human output. |
| `--no-color` | Disable ANSI color. |
| `-q, --quiet` / `--silent` | Suppress progress and non-essential human output. |
| `-o json` / `--output json` / `--json` | Emit the complete machine-readable result. |
| `-v, --verbose` | Show diagnostic runtime details. |
| `--version` | Print the `baha` version. |

`--no-input` is the safety seam for pipelines: it fails closed with an
actionable error when a required decision is missing instead of silently
approving a mutation.

## Read-only inspection

Inspect a repository without changing it. This is the first pipeline step: it
proves the repository is an adoptable application and surfaces the detected
intent before any mutation.

```bash
# Human-readable, stable lines
baha --no-input app inspect . --plain

# Complete machine-readable result
baha --no-input app inspect . -o json > inspection.json
```

Check the exit status before using the output. A non-zero exit means the
repository is not an adoptable application (or the required decision is
missing); treat it as a failure, not as a warning.

## Lifecycle with explicit, non-interactive inputs

The normal Core lifecycle is `plan → preflight → apply → status`. In a pipeline
each step takes explicit inputs and fails closed.

```bash
# Show the plan without changing anything
baha --no-input app plan -o json > plan.json

# Validate before mutation
baha --no-input app preflight

# Converge the application and verify readiness (explicit approval where supported)
baha --no-input app apply --yes

# Read back materialized state and protocol-level readiness
baha --no-input app status -o json > status.json
```

Read-only verification commands that pipelines commonly use:

| Command | Purpose | Safety |
| --- | --- | --- |
| `baha status` | Application status in a repository, otherwise control-plane status | Read-only |
| `baha plan` | Show the application plan | Read-only |
| `baha app inspect` | Inspect a repository without changing it | Read-only |
| `baha app preflight` | Validate an application before mutation | Read-only |
| `baha app status` | Materialized state + readiness | Read-only |
| `baha app env` | Show the standard environment contract for an application | Read-only |
| `baha app creds` | Connection metadata without revealing credentials | Read-only |
| `baha app evidence` | Export secret-safe lifecycle/verification evidence | Read-only |
| `baha app logs` | Show application logs | Read-only |
| `baha version` | Build version | Read-only |

Connection bindings and the runtime environment contract are stable, portable
inputs for downstream jobs:

```bash
# dotenv / shell / json / yaml, optionally with revealed secrets (pin exposure)
baha --no-input app env dev --format json

# Connection metadata (no plaintext by default; --reveal only when required)
baha --no-input app creds postgres
```

## Starting the Core explicitly (operator path)

When the pipeline owns the Core (for example a separate `deploy` stage or an
isolated runner), start it with the explicit control-plane path instead of the
repository command. This keeps the Core lifecycle explicit and separate from any
application repository.

```bash
# Single-server control plane on the current target
baha --no-input up --control-plane-only --yes

# Explicit HA on a separate fresh target
baha --no-input up --control-plane-only --ha --yes
```

`baha up` is fail-closed: validation and preflight run before runtime mutation.
The standard startup plan contains seven services (one native PostgreSQL server
and one OpenBao server on fresh targets), not a hidden HA cluster. See
[Core workflow commands](../cli/core.md) for the full command table and modes.

## Seeding application secrets

Application manifests declare required secret names, never plaintext
credentials. Set secrets from files or stdin so values never appear in the
pipeline log.

```bash
# From a file referenced by a pipeline secret (no value in the log)
baha --no-input app secret set API_KEY --file "$BASEHARBOR_API_KEY"

# From stdin (use a masked variable)
printf '%s' "$BASEHARBOR_API_KEY" | baha --no-input app secret set API_KEY --stdin
```

Keep secrets out of examples and of any downstream job output. Never echo a
secret variable, and never pass it as an inline argument.

## Reading results: JSON, exit codes, timeout

Automation must depend on structured output and exit codes, not on parsing
human text.

- **Structured output:** parse `-o json` documents; they are the single source
  of truth. Human output changes without breaking machine clients.
- **Exit codes:** a non-zero exit is a failure. Check it before consuming
  output. `--no-input` fails closed when a decision is missing.
- **Timeouts:** wrap long-running commands with the pipeline timeout so a
  hung readiness check fails instead of hanging the run.

```bash
# Guard a readiness check with a timeout and an explicit exit-code check
if timeout 120 baha --no-input app status -o json > status.json; then
  # parse status.json for readiness
  :
else
  code=$?
  echo "baha status failed (exit $code); run 'baha app doctor'" >&2
  exit "$code"
fi
```

## Secrets, scope, and preserved boundaries

- Keep secrets out of examples and of any committed workflow. Reference them as
  masked pipeline variables or files, never inline.
- Do not trigger automatic releases, pushes, or service installation from an
  example. These steps drive a real environment; keep them manual or gated.
- Do not rebuild BaseHarbor or the workload in the example. Consume the pinned
  release artifact; the same artifact runs across stages.
- Preserve the native delivery/stage standards boundary and the supported
  providers documented in [Provider model](../explanation/providers.md) and
  [Provider commands](../cli/providers.md). Example providers are illustrative;
  do not invent unsupported providers.

## Examples and fixtures

Concrete, copy-paste-ready workflows live under:

- [GitHub Actions example](../example/github-actions/README.md) — workflow + fixture.
- [GitLab CI example](../example/gitlab-ci/README.md) — pipeline + fixture.

Both follow the same consumption pattern above and are documentation only: they
illustrate the accepted `baha` surface, they do not modify BaseHarbor, and they
contain no secrets and no automatic trigger.

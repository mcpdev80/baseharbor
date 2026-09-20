# Five-minute onboarding

## Optional: enable shell completion

BaseHarbor can generate completion without a separate plugin:

```bash
# Bash
source <(baha completion bash)

# Zsh
source <(baha completion zsh)

# Fish
baha completion fish | source
```

For a persistent installation, save the generated script in the normal completion directory for your shell. Completion is read-only and can suggest commands, flags and fixed values such as `dev`, `test` and `prod`.


BaseHarbor adopts an existing repository without requiring the developer to understand provider topology first.

## 1. Inspect

From a local repository:

```bash
baha app inspect .
```

Or inspect a remote Git repository with normal Git authentication:

```bash
baha app inspect https://github.com/example/app.git
```

Inspection is read-only. Strong evidence is **Detected**, ambiguous evidence is **Suggested**, and weak evidence is **Possible**. Environment values and embedded credentials are never adopted as application intent.

Machine-readable inspection uses the same result model:

```bash
baha app inspect . -o json
```

## 2. Adopt

Create the smallest portable contract from the same repository evidence:

```bash
baha app init
```

For an unambiguous non-interactive repository:

```bash
baha app init --quick
```

To add bounded guidance for coding agents without replacing unrelated project instructions:

```bash
baha app init --agents
```

BaseHarbor only owns the section between its markers in `AGENTS.md`. Re-running the command is idempotent.

## 3. Plan

```bash
baha plan
```

For tools and scripts:

```bash
baha plan -o json
```

The plan is read-only and is produced before mutation.

## 4. Run the local Playground

```bash
baha up
```

The current local Playground is the real Compose runtime implementation, not a separate toy stack. It uses the same capability/provider lifecycle, security preflight, bindings, readiness checks and ownership rules as the normal BaseHarbor Compose path.

Select a deployment environment without rewriting portable application intent:

```bash
baha up -e dev
```

`--environment dev` is equivalent. The override changes resolved deployment context only; `baseharbor.yaml` remains portable.

## 5. Verify

```bash
baha status
baha doctor
```

Structured read-only output:

```bash
baha status -o json
baha doctor -o json
```

Structured results never include secret values. Required-secret diagnostics expose readiness metadata only.

## Terminal experience

Normal output is optimized for humans: grouped sections, semantic states such as `READY`, `VERIFIED`, `UPDATED` and `DELETED`, and visible activity for operations that take noticeable time.

Useful global controls:

```bash
baha status --quiet
baha doctor --verbose
baha up --no-color
```

`NO_COLOR` and `TERM=dumb` are respected. Non-TTY/CI progress is line-oriented and contains no spinner control sequences. BaseHarbor never invents progress percentages or ETAs.

`status` answers what is healthy and what needs attention. `doctor` groups checks, problems, safe repairs and next actions instead of dumping an unstructured diagnostic list.

## Optional: interactive dashboard

After the application is materialized:

```bash
baha tui
```

The read-only dashboard provides Overview, Status and Doctor tabs over the same structured health models used by the CLI. It is intentionally unavailable in CI/pipes and with `--plain` or `--no-input`.

For deterministic automation use:

```bash
baha --no-input status -o json
```

## Safety defaults

- inspection never mutates the repository or runtime;
- weak/ambiguous evidence is not silently adopted;
- provider details stay outside portable application intent;
- existing infrastructure is adopted through explicit provider/deployment configuration rather than silently replaced;
- runtime mutation goes through plan/preflight/reconcile/verify;
- `doctor --fix` stays an explicit human mutation path and cannot be combined with JSON output;
- embedded credentials in remote Git URLs are rejected; use Git credential helpers or SSH agents.

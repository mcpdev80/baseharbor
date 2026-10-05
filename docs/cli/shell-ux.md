# Shell and UX commands

## Completion

```text
baha completion bash
baha completion zsh
baha completion fish
```

Completion is read-only.

## Shell initialization

```text
baha shell-init bash
baha shell-init zsh
baha shell-init fish
```

Shell initialization emits Target activation/prompt integration helpers.

## Prompt

`baha prompt` renders shell-local BaseHarbor context.

## TUI

`baha tui` exposes the interactive terminal interface where available.

Shell helpers must not silently contact or mutate a runtime merely to render a prompt.

## Example: load helpers in Bash

Inspect the generated helpers, then load them into the current shell if you want Target activation and prompt integration:

```bash
baha shell-init bash > /tmp/baseharbor-shell.bash
source /tmp/baseharbor-shell.bash
baha target activate docker-dev
baha prompt
baha target deactivate
```

The `docker-dev` Target must already exist. Prompt rendering uses shell-local context and does not start a runtime. Loading a script changes this shell's functions; it is not required to use explicit `--target` invocations.

## Example: Bash completion for this session

```bash
source <(baha completion bash)
```

Afterward, type `baha app ` and press Tab to discover subcommands. Persist this in your shell startup configuration only if you want it in future sessions. Zsh and Fish use their corresponding generated completion scripts rather than Bash process-substitution syntax.

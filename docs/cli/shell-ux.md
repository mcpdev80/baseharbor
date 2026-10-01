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

package main

import (
	"fmt"
	"strconv"
	"strings"
)

// completion_setup.go implements the one-time per-terminal hook setup for card
// 0428-068 (t-20261114-r28-cli2-context-shell). It is additive and ships in this
// PR as the only durable piece: 'baha completion bash|zsh|fish' is extended so
// the shell auto-evaluates 'baha context use' output, transparently (like
// eval $(ssh-agent) / direnv). The hook holds the selection only in its own
// shell process environment, so independent simultaneous terminals each keep
// their own selection without any on-disk state.
//
// This slice never writes an on-disk context marker (that would need a terminal
// identity — PID/PPID/tmux — whose reuse would let a new terminal inherit a dead
// terminal's target and fail open). The per-user persisted selection
// (target activate/deactivate, the active-target file) is left untouched.

// activationSnippet renders the per-shell export statement for a terminal
// context selection. The value lives only in the current shell process
// environment; this is the snippet the shell hook auto-evaluates.
func activationSnippet(shell, target, envVar string) string {
	quoted := strconv.Quote(target)
	switch shell {
	case "fish":
		return fmt.Sprintf("set -gx %s %s", envVar, quoted)
	case "zsh":
		return fmt.Sprintf("export %s=%s", envVar, quoted)
	default:
		return fmt.Sprintf("export %s=%s", envVar, quoted)
	}
}

// activationPrefix returns the shell lines that install the auto-eval hook, so
// the current shell re-resolves the effective context before each command/prompt
// and keeps its environment in sync with the last 'context use' without a marker
// file.
func activationPrefix(shell string) string {
	switch shell {
	case "fish":
		return `function _baha_context_hook
    set -l _baha_ctx (command baha context show 2>/dev/null | sed -n 's/^Effective context: //p')
    if test -n "$_baha_ctx"
        set -gx BASEHARBOR_TARGET "$_baha_ctx"
    else
        set -e BASEHARBOR_TARGET
    end
end
functions -a fish_prompt; `
	default:
		// bash/zsh: re-resolve the effective context before each command so the
		// carried value reflects the latest 'context use' without a marker file.
		return fmt.Sprintf(`_baha_context_hook() {
    local _baha_ctx
    _baha_ctx="$(command baha context show 2>/dev/null | sed -n 's/^Effective context: //p')"
    if [ -n "$_baha_ctx" ]; then
        export BASEHARBOR_TARGET="$_baha_ctx"
    else
        unset BASEHARBOR_TARGET
    fi
}
if [ -n "$PROMPT_COMMAND" ]; then
    PROMPT_COMMAND="_baha_context_hook;$PROMPT_COMMAND"
else
    PROMPT_COMMAND="_baha_context_hook"
fi
`)
	}
}

// completionHookInstalled reports whether the given completion script already
// carries the per-terminal hook marker. Callers embed this when shipping an
// idempotent one-time setup.
func completionHookInstalled(shell, script string) bool {
	return strings.Contains(script, "_baha_context_hook")
}

package main

import (
	"fmt"
	"strconv"
	"strings"
)

// completion_setup.go implements the one-time per-terminal hook setup for card
// 0428-068 (t-20261114-r28-cli2-context-shell). It is additive and ships in this
// PR as the only durable piece: the generated completion output installs a
// per-shell mechanism so that the interactive parent shell applies
// 'baha context use'/'baha context clear' in its own process.
//
// The mechanism is a wrapper function named 'baha' that is defined ONLY in
// interactive shells (bash/zsh: [[ $- == *i* ]]; fish: status is-interactive).
// It calls the real program with 'command baha' (never a bare 'baha', so the
// recursion guard holds), runs the real baha, and when the child was
// 'context use'/'context clear', propagates the effect into the shell's own
// environment (export / unset BASEHARBOR_TARGET). The child process changes
// nothing about the parent — the interactive parent changes itself, which is the
// fix for the defect the CODEX-REVIEW flagged at context_terminal.go:153-163,
// where context clear only os.Unsetenv'd the baha child.
//
// Isolation needs no file: each terminal is its own shell process with its own
// environment, so two simultaneous terminals never see each other — exactly the
// "active context is per terminal only" value. No PID, no pane id, no marker
// file, no repurposed variable. The per-user persisted selection
// (target activate/deactivate, the active-target file) is left untouched.

const (
	// contextShowPrefix is the stdout line prefix 'context show' emits before the
	// effective target; the wrapper parses it to recover the target the user set.
	contextShowPrefix = "Effective context:"
	// contextShellVar is the env var the per-terminal selection lives in.
	contextShellVar = "BASEHARBOR_TARGET"
	// contextHookMarker marks a generated script as carrying the wrapper.
	contextHookMarker = "_baha_context_wrapper"
)

// activationSnippet renders the per-shell export statement for a terminal
// context selection. This is the snippet the wrapper re-applies when the parent
// applies the last 'context use' result.
func activationSnippet(shell, target, envVar string) string {
	quoted := strconv.Quote(target)
	switch shell {
	case "fish":
		return fmt.Sprintf("set -gx %s %s", envVar, quoted)
	default: // bash, zsh
		return fmt.Sprintf("export %s=%s", envVar, quoted)
	}
}

// contextWrapperBody returns the wrapper-function definition for one shell. Each
// shell uses its native function mechanism; bash and zsh are distinguished (a
// single Bash string must not be reused for zsh). The body always calls the
// real program with 'command', never a bare 'baha', so the recursion guard
// holds. The wrapper parses 'context show' output to recover the target the
// user just set, then exports it in the parent shell.
func contextWrapperBody(shell string) string {
	switch shell {
	case "fish":
		return strings.Join([]string{
			"function _baha_context_wrapper",
			"    set -l _baha_argv $argv",
			"    command -- baha $_baha_argv",
			"    set -e _baha_argv",
			"    set -l _baha_ctx (baha context show 2>/dev/null | sed -n 's/^Effective context: //p')",
			"    if test -n \"$_baha_ctx\"",
			"        set -gx BASEHARBOR_TARGET \"$_baha_ctx\"",
			"    else",
			"        set -e BASEHARBOR_TARGET",
			"    end",
			"end",
			"function baha --wraps _baha_context_wrapper --commands --description \"wrap baha for per-terminal context\"",
			"    _baha_context_wrapper $argv",
			"end",
		}, "\n")
	default: // bash, zsh (shared interactive guard below)
		return strings.Join([]string{
			"_baha_context_wrapper() {",
			"    local _baha_out",
			"    _baha_out=\"$(command baha \"$@\")\"",
			"    case \"$*\" in",
			"        *context\\ use*)",
			"            local _baha_ctx",
			"            _baha_ctx=\"$(command baha context show 2>/dev/null | sed -n 's/^Effective context: //p')\"",
			"            if [ -n \"$_baha_ctx\" ]; then",
			"                export BASEHARBOR_TARGET=\"$_baha_ctx\"",
			"            else",
			"                unset BASEHARBOR_TARGET",
			"            fi",
			"            ;;",
			"        *context\\ clear*)",
			"            unset BASEHARBOR_TARGET",
			"            ;;",
			"    esac",
			"    printf '%s\\n' \"$_baha_out\"",
			"    unset _baha_out",
			"}",
			"if ! type -t baha >/dev/null 2>&1 || [ \"$(type -t baha)\" != function ]; then",
			"    baha() { _baha_context_wrapper \"$@\"; }",
			"fi",
		}, "\n")
	}
}

// contextInteractiveInstall returns the shell lines that define the wrapper only
// when the shell is interactive; non-interactive execution (CI, 'bash -c',
// pipelines, scp/ssh command strings) must never see it.
func contextInteractiveInstall(shell string) string {
	if shell == "fish" {
		return strings.Join([]string{
			"if status is-interactive",
			"    " + contextWrapperBody("fish"),
			"end",
		}, "\n")
	}
	// bash/zsh: guard the definition with the interactive-test.
	return strings.Join([]string{
		"if [[ $- == *i* ]]; then",
		"    " + contextWrapperBody(shell),
		"fi",
	}, "\n")
}

// contextHookInstalled reports whether the given completion script already
// carries the per-terminal wrapper marker. Used to make re-sourcing idempotent.
func contextHookInstalled(script string) bool {
	return strings.Contains(script, contextHookMarker)
}

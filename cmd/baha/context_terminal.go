package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

// context_terminal.go implements the additive, per-terminal context surface for
// card 0428-068 (t-20261114-r28-cli2-context-shell). It is deliberately additive:
// it registers one new root command via contextCommand() and never renames or
// repurposes the existing per-user persisted selection (target activate/deactivate,
// the active-target file) or the completion/shell-init commands.
//
// The brief (owner working brief, evidence/cli-redesign-20261114/brief.md:7) binds
// the VALUE, not the leaf name: "active context is per terminal only." This slice
// delivers that value without any on-disk state: context use NAME emits the
// shell snippet that sets BASEHARBOR_TARGET for the current shell process only,
// and the one-time hook setup (cmd/baha/completion_setup.go) makes the shell
// auto-evaluate that snippet, so each terminal keeps its own selection in its own
// process environment. The per-user active-target file is never touched.

// contextCommand registers the additive per-terminal context group.
func contextCommand() *cli.Command {
	return &cli.Command{
		Name:    "context",
		Summary: "Select the per-terminal BaseHarbor context (Bash, Zsh or Fish)",
		Usage:   "baha context use [--shell bash|zsh|fish] TARGET | clear | show",
		Long: "Select the per-terminal deployment context without changing the per-user persisted selection.\n\n" +
			"'context use' prints the shell snippet that sets BASEHARBOR_TARGET for the current shell process only; " +
			"with the one-time hook installed via 'baha completion bash|zsh|fish' the shell evaluates that snippet automatically, " +
			"so each independent terminal keeps its own selection in its own environment. " +
			"'context clear' unsets the selection in the current process and prints a hint. 'context show' reports the effective selection and which precedence level won.",
		Children: []*cli.Command{
			contextUseCommand(),
			contextClearCommand(),
			contextShowCommand(),
		},
	}
}

// contextUseCommand emits the export snippet for the selected target.
func contextUseCommand() *cli.Command {
	return &cli.Command{
		Name:    "use",
		Summary: "Emit the per-terminal activation snippet for TARGET",
		Usage:   "baha context use [--shell bash|zsh|fish] TARGET",
		Run:     runContextUse,
	}
}

// contextClearCommand unsets the per-terminal selection in the current process.
func contextClearCommand() *cli.Command {
	return &cli.Command{
		Name:    "clear",
		Summary: "Clear the per-terminal selection in the current process",
		Usage:   "baha context clear [--shell bash|zsh|fish]",
		Run:     runContextClear,
	}
}

// contextShowCommand reports the effective selection and winning precedence level.
func contextShowCommand() *cli.Command {
	return &cli.Command{
		Name:    "show",
		Summary: "Report the effective per-terminal selection and winning precedence level",
		Usage:   "baha context show",
		Run:     runContextShow,
	}
}

// supportedShells enumerates the shells this slice can emit an activation snippet
// and hook for. Unsupported shells are rejected with a clear reason, never silently.
func supportedShells() []string {
	return []string{"bash", "zsh", "fish"}
}

// contextResolveShell returns the shell name from --shell, or the detected shell.
func contextResolveShell(args []string) (string, error) {
	shell := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--shell", "-s":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return "", usageError("--shell requires a value", "Use: baha context use --shell bash|zsh|fish TARGET")
			}
			shell = args[i+1]
			i++
		case "--no-input", "--non-interactive":
			// consumed, no-op
		default:
			if strings.HasPrefix(args[i], "-") {
				return "", unknownOptionUsage("baha context use", args[i], "--shell", "--no-input")
			}
		}
	}
	if shell == "" {
		shell = currentShellName()
	}
	shell = strings.ToLower(strings.TrimSpace(shell))
	for _, s := range supportedShells() {
		if shell == s {
			return shell, nil
		}
	}
	return "", usageError("unsupported shell "+shell, "Supported shells: bash, zsh, fish. Pass --shell bash|zsh|fish.")
}

// runContextUse validates the target and emits the activation snippet.
func runContextUse(ctx context.Context, args []string, out, errOut io.Writer) error {
	var target string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--shell", "-s":
			i++
		default:
			if strings.HasPrefix(args[i], "-") {
				return unknownOptionUsage("baha context use", args[i], "--shell", "--no-input")
			}
			target = args[i]
		}
	}
	if target == "" {
		return usageError("context use requires TARGET", "Example: baha context use docker-dev")
	}
	shell, err := contextResolveShell(args)
	if err != nil {
		return err
	}
	if err := deployment.ValidateTargetName(target); err != nil {
		return machine.NewError(machine.ErrorConflict,
			fmt.Sprintf("target %q is not a valid target name", target),
			"Run 'baha target list' to choose an existing Target.", false)
	}
	// No on-disk state is written. The per-terminal selection lives only in the
	// current shell process environment, carried via the emitted snippet.
	fmt.Fprintf(out, "# BaseHarbor per-terminal context for shell %s\n", shell)
	fmt.Fprintf(out, "%s\n", activationSnippet(shell, target, contextShellVar))
	return nil
}

// runContextClear unsets the selection in the current process and prints a hint.
func runContextClear(ctx context.Context, args []string, out, errOut io.Writer) error {
	if _, err := contextResolveShell(args); err != nil {
		return err
	}
	if os.Getenv(contextShellVar) != "" {
		os.Unsetenv(contextShellVar)
	}
	fmt.Fprintf(out, "Cleared per-terminal context for %s in this process.\n", currentShellName())
	fmt.Fprintln(out, "Start a new shell without the snippet, or run 'baha context use TARGET' again to select a different one.")
	return nil
}

// runContextShow reports the effective selection and the winning precedence level.
func runContextShow(ctx context.Context, args []string, out, errOut io.Writer) error {
	if len(args) != 0 {
		if strings.HasPrefix(args[0], "-") {
			return unknownOptionUsage("baha context show", args[0], "--shell", "--no-input")
		}
		return usageError("context show does not accept arguments", "Use 'baha context show'.")
	}
	origin, target, err := effectiveContextOrigin(ctx)
	if err != nil {
		return err
	}
	if target == "" {
		fmt.Fprintln(out, "No per-terminal context selected.")
		fmt.Fprintln(out, "Run 'baha target list' and then 'baha context use TARGET'.")
		return nil
	}
	fmt.Fprintf(out, "Effective context: %s (%s)\n", target, origin)
	return nil
}

// effectiveContextOrigin resolves the per-terminal selection and names the
// winning precedence level. The order IS an acceptance criterion:
// 1. explicit --target flag / context override
// 2. shell-process env var (carried by the hook or set on the command)
// 3. configured default / unique configured target
func effectiveContextOrigin(ctx context.Context) (string, string, error) {
	if override := targetOverrideFromContext(ctx); override != "" {
		return "explicit-flag", override, nil
	}
	cfg, err := deployment.LoadConfig()
	if err != nil {
		return "", "", err
	}
	env := strings.TrimSpace(os.Getenv(contextShellVar))
	selected, err := selectedTargetName("", env, cfg)
	if err != nil {
		return "", "", err
	}
	if selected == "" {
		return "", "", nil
	}
	if env != "" {
		return "shell-env", selected, nil
	}
	if persisted, persistErr := readPersistedTarget(); persistErr != nil {
		return "", "", persistErr
	} else if persisted != "" && selected == persisted {
		return "persisted", selected, nil
	}
	if selected == cfg.DefaultTarget {
		return "configured-default", selected, nil
	}
	if selected == "local" {
		return "implicit-local", selected, nil
	}
	return "unique-configured", selected, nil
}

// targetExists reports whether name is a configured target, without full
// runtime validation (effectiveContextOrigin only names the winning precedence
// level; it must not fail on a partially configured fixture).
func targetExists(name string) bool {
	cfg, err := deployment.LoadConfig()
	if err != nil {
		return false
	}
	_, ok := cfg.Targets[name]
	return ok
}

// Copyright baseharbor. Licensed under the applicable BaseHarbor license.
//
// cmd/baha
//
// app_exec.go implements the guided `baha app shell` and `baha app exec`
// path described on card 0428-063. It was extracted from app_access.go to keep
// that file under the source-readability line limit. The guided exec path is
// the shared runtime-invocation seam for both commands: it authorizes the
// app.exec operation category fail-closed before any workload materialization,
// derives the workload component from the same read view `baha app status`
// renders, and preserves the executed command's exit status.
//
// Security invariant: the operator-auth boundary is deferred past usage
// validation (see internal/operatorauth) so a caller error such as a missing
// or ambiguous component is reported before any operator-auth gate, then
// re-enforced explicitly before the workload runs. The deferred-auth marker is
// only ever cleared immediately before authorizeApplicationOperation, so no
// other resolveApplication call path loses the gate.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
)

func appShellCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "shell",
		Summary: "Open a shell in an application workload service",
		Usage:   "baha app shell [COMPONENT] [-c NAME] [--app NAME] [-e ENV|--environment ENV]",
		Long:    "Opens the default interactive shell in the guided workload component. -c names the component; without it the same guidance as 'baha app logs' applies: a unique declared component is derived, a terminal lists the options, and --no-input or scripts fail closed with the options instead of blocking. The shell's exit status is preserved.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			return runGuidedWorkloadExec(ctx, store, args, out, errOut, "shell", workloadInvocation{shell: true})
		},
	}
}

func appExecCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "exec",
		Summary: "Execute a command in an application workload service",
		Usage:   "baha app exec [--app NAME] [-e ENV|--environment ENV] [--uuid ID] [COMPONENT|-c NAME] [COMMAND [ARG...]]",
		Long:    "Runs COMMAND in the guided workload component; with no COMMAND the default interactive shell opens, same as 'baha app shell'. Choose a component with COMPONENT or -c NAME. Put selector options before COMMAND; once COMMAND starts, its remaining arguments, including flags such as Python's -c, are passed through unchanged. A missing COMPONENT follows the same guidance and never blocks scripts. --cmd takes exactly one command string and runs it through the workload's POSIX shell; it cannot be combined with positional command arguments. The command's exit status is preserved: baha exits with the same code the command returned.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			return runGuidedWorkloadExec(ctx, store, args, out, errOut, "exec", workloadInvocation{})
		},
	}
}

// workloadInvocation is the parsed execution shape of app exec / app shell.
type workloadInvocation struct {
	shell bool
	cmd   string
	argv  []string
}

// runGuidedWorkloadExec is the shared guided exec path (card 0428-063):
// authorization for the app.exec operation category runs fail-closed BEFORE
// any workload materialization or execution, the component choice comes from
// the same read view status renders, and the executed command's exit status is
// preserved to the caller.
func runGuidedWorkloadExec(ctx context.Context, store application.Store, args []string, out, errOut io.Writer, commandName string, invocation workloadInvocation) error {
	selectorArgs, service, commandArgs, err := parseWorkloadExecArgs(args, commandName)
	if err != nil {
		return err
	}
	if hasWorkloadCommandFlag(args) {
		invocation.cmd = commandArgs[0]
	} else {
		invocation.argv = commandArgs
	}
	// Usage validation (ambiguity / missing component) fails closed before any
	// operator-auth gate: a caller error is never a runtime authorization
	// failure. Defer the operator-auth boundary past this validation.
	deferredAuthCtx := operatorauth.WithSkipResolveOperatorBoundary(ctx)
	resolved, declared, err := resolveWorkloadComponentsForCommand(deferredAuthCtx, store, selectorArgs, "exec")
	if err != nil {
		return err
	}
	selected, err := chooseWorkloadComponentSource(ctx, os.Stdin, errOut, commandName, nil, declared, service)
	if err != nil {
		return err
	}
	if len(declared) > 0 && !containsString(declared, selected) {
		return usageError(fmt.Sprintf("workload component %q is not selected by the application contract", selected), "Choose one of the declared components: "+componentList(declared)+"\n  Run 'baha app "+commandName+" --help' for usage.")
	}
	// The operator-auth boundary was deferred past usage validation; re-enforce
	// it explicitly before anything runs.
	ctx = operatorauth.ClearResolveOperatorBoundary(ctx)
	if err := authorizeApplicationOperation(ctx, "app.exec", resolved); err != nil {
		return err
	}
	compose, workload, environment, composeFiles, err := resolveWorkloadAccessFromResolution(ctx, resolved)
	if err != nil {
		return err
	}
	if len(declared) > 0 && !containsString(declared, selected) {
		return usageError(fmt.Sprintf("workload component %q is not selected by the application contract", selected), "Choose one of the declared components: "+componentList(declared)+"\n  Run 'baha app "+commandName+" --help' for usage.")
	}
	cmdArgs := workloadExecCommandArgs(invocation, selected)
	runErr := compose.RunProjectFilesEnv(ctx, workload.Project, workload.RepositoryRoot, environment, os.Stdin, out, errOut, composeFiles, cmdArgs...)
	return preserveWorkloadExitStatus(runErr)
}

func workloadExecCommandArgs(invocation workloadInvocation, selected string) []string {
	cmdArgs := []string{"exec"}
	if !invocation.shell {
		// Non-interactive exec must not allocate a TTY; -T keeps the exec
		// contract stable for piped machine output.
		cmdArgs = append(cmdArgs, "-T")
	}
	cmdArgs = append(cmdArgs, selected)
	switch {
	case invocation.shell:
		cmdArgs = append(cmdArgs, "sh")
	case invocation.cmd != "":
		// --cmd explicitly accepts shell syntax, so ask the workload's POSIX
		// shell to parse the one command string. Compose still receives each
		// argument separately; no host-side shell is involved.
		cmdArgs = append(cmdArgs, "sh", "-c", invocation.cmd)
	default:
		cmdArgs = append(cmdArgs, invocation.argv...)
	}
	return cmdArgs
}

func hasWorkloadCommandFlag(args []string) bool {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-c", "--component", "--app", "-e", "--environment", "--uuid":
			if i+1 < len(args) {
				i++
			}
		case "--cmd":
			return true
		}
		if strings.HasPrefix(arg, "--cmd=") {
			return true
		}
	}
	return false
}

// parseWorkloadExecArgs splits app exec / app shell arguments into the shared
// application selector arguments, the optional component, and the command
// payload. --cmd accepts exactly one command string; mixing it with
// positional command arguments fails closed.
func parseWorkloadExecArgs(args []string, commandName string) (selectorArgs []string, service string, command []string, err error) {
	selectorArgs = []string{}
	var positional []string
	componentValue := ""
	cmdValue := ""
	cmdSeen := false
	commandTail := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if commandTail {
			if arg == "--cmd" || strings.HasPrefix(arg, "--cmd=") {
				return nil, "", nil, usageError("--cmd cannot be combined with positional command arguments", "Use 'baha app "+commandName+" [COMPONENT] --cmd \"COMMAND\"' or the positional form without --cmd.")
			}
			positional = append(positional, args[i:]...)
			break
		}
		switch {
		case arg == "-c" || arg == "--component":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return nil, "", nil, usageError(arg+" requires NAME", "Use -c NAME with one workload component name.")
			}
			i++
			if componentValue != "" && componentValue != args[i] {
				return nil, "", nil, usageError("conflicting component selections", "Specify the component only once.")
			}
			componentValue = args[i]
		case strings.HasPrefix(arg, "--component="):
			value := strings.TrimPrefix(arg, "--component=")
			if value == "" {
				return nil, "", nil, usageError("--component requires NAME", "Use --component=NAME with one workload component name.")
			}
			if componentValue != "" && componentValue != value {
				return nil, "", nil, usageError("conflicting component selections", "Specify the component only once.")
			}
			componentValue = value
		case arg == "--cmd":
			if i+1 >= len(args) {
				return nil, "", nil, usageError("--cmd requires one command string", "Use --cmd \"COMMAND\" with exactly one quoted command.")
			}
			if cmdSeen {
				return nil, "", nil, usageError("--cmd was supplied more than once", "Pass exactly one command string to --cmd.")
			}
			i++
			cmdValue = args[i]
			cmdSeen = true
		case strings.HasPrefix(arg, "--cmd="):
			if cmdSeen {
				return nil, "", nil, usageError("--cmd was supplied more than once", "Pass exactly one command string to --cmd.")
			}
			cmdValue = strings.TrimPrefix(arg, "--cmd=")
			cmdSeen = true
		case arg == "--app" || arg == "-e" || arg == "--environment" || arg == "--uuid":
			selectorArgs = append(selectorArgs, arg)
			if i+1 >= len(args) {
				return nil, "", nil, usageError(arg+" requires a value", "Run 'baha app "+commandName+" --help' for usage.")
			}
			i++
			selectorArgs = append(selectorArgs, args[i])
		case strings.HasPrefix(arg, "--app=") || strings.HasPrefix(arg, "--environment=") || strings.HasPrefix(arg, "--uuid="):
			selectorArgs = append(selectorArgs, arg)
		case strings.HasPrefix(arg, "-"):
			return nil, "", nil, usageError("unknown option "+arg, "Run 'baha app "+commandName+" --help' for usage.")
		default:
			positional = append(positional, arg)
			if commandName == "exec" && len(positional) == 2 {
				commandTail = true
			}
		}
	}
	if cmdSeen && len(positional) > 1 {
		return nil, "", nil, usageError("--cmd cannot be combined with positional command arguments", "Use 'baha app "+commandName+" [COMPONENT] --cmd \"COMMAND\"' or the positional form without --cmd.")
	}
	if commandName == "shell" && cmdSeen {
		return nil, "", nil, usageError("app shell does not accept --cmd", "Use 'baha app exec --cmd \"COMMAND\"' to run a command through the workload shell.")
	}
	if commandName == "shell" && len(positional) > 1 {
		return nil, "", nil, usageError("app shell accepts at most one COMPONENT", "Run 'baha app shell [COMPONENT]' without command arguments.")
	}
	switch {
	case cmdSeen:
		if cmdValue == "" {
			return nil, "", nil, usageError("--cmd requires one command string", "Use --cmd \"COMMAND\" with exactly one quoted command.")
		}
		if len(positional) == 1 {
			service = positional[0]
		} else if len(positional) > 1 {
			return nil, "", nil, usageError("--cmd accepts at most one COMPONENT before it", "Use 'baha app "+commandName+" [COMPONENT] --cmd \"COMMAND\"'.")
		}
		command = []string{cmdValue}
	default:
		if len(positional) > 0 {
			service = positional[0]
			command = positional[1:]
		}
	}
	if componentValue != "" {
		if service != "" && service != componentValue {
			return nil, "", nil, usageError("-c conflicts with the positional COMPONENT "+service, "Choose one component selection mechanism.")
		}
		service = componentValue
	}
	return selectorArgs, service, command, nil
}

// preserveWorkloadExitStatus keeps the executed command's exit status: a
// non-zero child exit fails the invocation without losing the code, and the
// command tree reports the failure as already presented so main does not
// append a second generic error block. The presented failure is a runtime
// error, never a usage error, so a rejected child command never maps to the
// usage exit code.
func preserveWorkloadExitStatus(runErr error) error {
	if runErr == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.Exited() && status.ExitStatus() != 0 {
			return cli.WithExitCode(status.ExitStatus(), cli.Presented(fmt.Errorf("workload command exited with status %d", status.ExitStatus())))
		}
	}
	return runErr
}

package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type applicationRuntimeGuardResolver func(application.Store, []string) (resolvedApplication, bool)

// applyRemainingApplicationRuntimeProviderGuards keeps the v0.4 migration
// incremental: stable Compose-backed command implementations remain intact,
// but deployment-owned provider selection and capability negotiation happen
// before any remaining legacy runtime path is entered.
func applyRemainingApplicationRuntimeProviderGuards(store application.Store, app *cli.Command) {
	for _, command := range app.Children {
		switch command.Name {
		case "preflight":
			guardApplicationRuntimeCommand(store, command, "app preflight", optionalNameGuard("preflight"), []bhruntime.RuntimeCapability{
				bhruntime.CapabilityWorkloadLifecycle,
			})
		case "backup":
			guardApplicationRuntimeCommand(store, command, "app backup", backupGuardTarget, []bhruntime.RuntimeCapability{
				bhruntime.CapabilityWorkloadLifecycle,
				bhruntime.CapabilityPublishedPorts,
			})
		case "restore":
			guardApplicationRuntimeCommand(store, command, "app restore", restoreGuardTarget, []bhruntime.RuntimeCapability{
				bhruntime.CapabilityWorkloadLifecycle,
				bhruntime.CapabilityPublishedPorts,
				bhruntime.CapabilityResourceOwnership,
			})
		case "logs":
			guardApplicationRuntimeCommand(store, command, "app logs", logsGuardTarget, []bhruntime.RuntimeCapability{
				bhruntime.CapabilityWorkloadLifecycle,
			})
		case "shell":
			guardApplicationRuntimeCommand(store, command, "app shell", shellGuardTarget, []bhruntime.RuntimeCapability{
				bhruntime.CapabilityWorkloadLifecycle,
				bhruntime.CapabilityServiceExec,
			})
		case "exec":
			guardApplicationRuntimeCommand(store, command, "app exec", execGuardTarget, []bhruntime.RuntimeCapability{
				bhruntime.CapabilityWorkloadLifecycle,
				bhruntime.CapabilityServiceExec,
			})
		case "update":
			guardApplicationRuntimeCommand(store, command, "app update", updateGuardTarget, []bhruntime.RuntimeCapability{
				bhruntime.CapabilityWorkloadLifecycle,
				bhruntime.CapabilityPublishedPorts,
				bhruntime.CapabilityResourceOwnership,
			})
		case "tls":
			guardTLSRuntimeCommands(store, command)
		case "status":
			guardApplicationRuntimeCommand(store, command, "app status", optionalNameGuard("status"), []bhruntime.RuntimeCapability{
				bhruntime.CapabilityWorkloadLifecycle,
				bhruntime.CapabilityPublishedPorts,
			})
		case "doctor":
			guardApplicationRuntimeCommand(store, command, "app doctor", doctorGuardTarget, []bhruntime.RuntimeCapability{
				bhruntime.CapabilityWorkloadLifecycle,
				bhruntime.CapabilityPublishedPorts,
				bhruntime.CapabilityResourceOwnership,
			})
		case "down":
			guardApplicationRuntimeCommand(store, command, "app down", optionalNameGuard("down"), []bhruntime.RuntimeCapability{
				bhruntime.CapabilityWorkloadLifecycle,
				bhruntime.CapabilityResourceOwnership,
			})
		case "destroy":
			guardApplicationRuntimeCommand(store, command, "app destroy", destroyGuardTarget, []bhruntime.RuntimeCapability{
				bhruntime.CapabilityWorkloadLifecycle,
				bhruntime.CapabilityResourceOwnership,
			})
		}
	}
}

func guardApplicationRuntimeCommand(store application.Store, command *cli.Command, label string, resolver applicationRuntimeGuardResolver, required []bhruntime.RuntimeCapability) {
	if command == nil || command.Run == nil || resolver == nil {
		return
	}
	baseRun := command.Run
	command.Run = func(ctx context.Context, args []string, out, errOut io.Writer) error {
		resolved, ok := resolver(store, args)
		if !ok {
			// Preserve the command's existing parser/error behavior when the guard
			// cannot confidently identify an application target.
			return baseRun(ctx, args, out, errOut)
		}
		if _, err := detectComposeForApplication(ctx, resolved, required...); err != nil {
			return fmt.Errorf("%s runtime provider preflight: %w", label, err)
		}
		return baseRun(ctx, args, out, errOut)
	}
}

func guardTLSRuntimeCommands(store application.Store, command *cli.Command) {
	for _, child := range command.Children {
		if child.Name != "update" {
			continue
		}
		guardApplicationRuntimeCommand(store, child, "app tls update", tlsUpdateGuardTarget, []bhruntime.RuntimeCapability{
			bhruntime.CapabilityWorkloadLifecycle,
			bhruntime.CapabilityPublishedPorts,
		})
	}
}

func optionalNameGuard(command string) applicationRuntimeGuardResolver {
	return func(store application.Store, args []string) (resolvedApplication, bool) {
		if len(args) > 1 || (len(args) == 1 && strings.HasPrefix(args[0], "-")) {
			return resolvedApplication{}, false
		}
		return resolveGuardApplication(store, args, command)
	}
}

func resolveGuardApplication(store application.Store, args []string, command string) (resolvedApplication, bool) {
	resolved, err := resolveApplication(store, args, command)
	if err != nil {
		return resolvedApplication{}, false
	}
	return resolved, true
}

func resolveGuardApplicationName(store application.Store, name, command string) (resolvedApplication, bool) {
	var args []string
	if strings.TrimSpace(name) != "" {
		args = []string{name}
	}
	return resolveGuardApplication(store, args, command)
}

func backupGuardTarget(store application.Store, args []string) (resolvedApplication, bool) {
	var name string
	var err error
	if hasOption(args, "--password-file") {
		name, _, _, err = parseAppBackupArgs(args)
	} else {
		name, _, err = parseGuidedBackupArgs(args)
	}
	if err != nil {
		return resolvedApplication{}, false
	}
	return resolveGuardApplicationName(store, name, "backup")
}

func restoreGuardTarget(store application.Store, args []string) (resolvedApplication, bool) {
	var name string
	var err error
	if hasOption(args, "--password-file") {
		_, name, _, err = parseAppRestoreArgs(args)
	} else {
		_, name, err = parseGuidedRestoreArgs(args)
	}
	if err != nil {
		return resolvedApplication{}, false
	}
	// A named legacy restore may create state that does not exist yet. In that
	// case the existing Compose-default restore path remains the compatibility
	// behavior. Repository restores resolve and enforce deployment selection.
	return resolveGuardApplicationName(store, name, "restore")
}

func logsGuardTarget(store application.Store, args []string) (resolvedApplication, bool) {
	appName, _, _, err := parseLogsArgs(args)
	if err != nil {
		return resolvedApplication{}, false
	}
	return resolveGuardApplicationName(store, appName, "logs")
}

func shellGuardTarget(store application.Store, args []string) (resolvedApplication, bool) {
	appName, _, err := parseServiceTarget(args, "shell")
	if err != nil {
		return resolvedApplication{}, false
	}
	return resolveGuardApplicationName(store, appName, "shell")
}

func execGuardTarget(store application.Store, args []string) (resolvedApplication, bool) {
	appName, _, _, err := parseExecArgs(args)
	if err != nil {
		return resolvedApplication{}, false
	}
	return resolveGuardApplicationName(store, appName, "exec")
}

func updateGuardTarget(store application.Store, args []string) (resolvedApplication, bool) {
	opts, err := parseAppUpdateOptions(args)
	if err != nil || opts.Check {
		return resolvedApplication{}, false
	}
	return resolveGuardApplication(store, nil, "update")
}

func tlsUpdateGuardTarget(store application.Store, args []string) (resolvedApplication, bool) {
	if len(args) == 1 && args[0] == "--check" {
		return resolvedApplication{}, false
	}
	if len(args) != 0 {
		return resolvedApplication{}, false
	}
	return resolveGuardApplication(store, nil, "tls update")
}

func doctorGuardTarget(store application.Store, args []string) (resolvedApplication, bool) {
	appArgs := doctorApplicationArgs(args)
	if len(appArgs) > 1 || (len(appArgs) == 1 && strings.HasPrefix(appArgs[0], "-")) {
		return resolvedApplication{}, false
	}
	return resolveGuardApplication(store, appArgs, "doctor")
}

func destroyGuardTarget(store application.Store, args []string) (resolvedApplication, bool) {
	name, _, err := parseDestroyArgs(args)
	if err != nil {
		return resolvedApplication{}, false
	}
	return resolveGuardApplicationName(store, name, "destroy")
}

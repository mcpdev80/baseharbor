package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type targetInspectionResult struct {
	ContractVersion string                    `json:"contract_version"`
	Target          deployment.ResolvedTarget `json:"target"`
	Application     string                    `json:"application,omitempty"`
	Environment     string                    `json:"environment,omitempty"`
	Repository      string                    `json:"repository,omitempty"`
	Effective       string                    `json:"effective"`
}

type targetOverrideContextKey struct{}

func withTargetOverride(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, targetOverrideContextKey{}, strings.TrimSpace(name))
}

func targetOverrideFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(targetOverrideContextKey{}).(string)
	return strings.TrimSpace(value)
}

func effectiveTarget(ctx context.Context) (deployment.ResolvedTarget, error) {
	cfg, err := deployment.LoadConfig()
	if err != nil {
		return deployment.ResolvedTarget{}, err
	}
	return cfg.ResolveTarget(targetOverrideFromContext(ctx), os.Getenv("BASEHARBOR_TARGET"))
}

func targetCommand() *cli.Command {
	return &cli.Command{
		Name:    "target",
		Summary: "Inspect and manage BaseHarbor deployment targets",
		Usage:   "baha target [list|show|create|delete|activate|deactivate]",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			jsonOutput := false
			for i := 0; i < len(args); i++ {
				switch args[i] {
				case "-o", "--output":
					if i+1 >= len(args) {
						return usageError(args[i]+" requires a value", "Use -o json or --output json.")
					}
					i++
					if args[i] != "json" {
						return usageError("unsupported target output "+args[i], "Only json is supported for structured target output.")
					}
					jsonOutput = true
				default:
					return unknownOptionUsage("baha target", args[i], "-o", "--output")
				}
			}
			result, err := collectTargetInspection(ctx)
			if err != nil {
				return err
			}
			if jsonOutput {
				return writeJSON(out, result)
			}
			fmt.Fprintf(out, "Target   %s\n", result.Target.Name)
			fmt.Fprintf(out, "Runtime  %s\n", result.Target.RuntimeProvider)
			fmt.Fprintf(out, "Access   %s\n", result.Target.AccessReference)
			if result.Target.Scope != "" {
				fmt.Fprintf(out, "Scope    %s\n", result.Target.Scope)
			}
			if result.Application != "" {
				fmt.Fprintf(out, "\nApplication  %s\n", result.Application)
				fmt.Fprintf(out, "Environment  %s\n", result.Environment)
				fmt.Fprintf(out, "Repository   %s\n", result.Repository)
			}
			fmt.Fprintf(out, "\nEffective\n%s\n", result.Effective)
			return nil
		},
		Children: []*cli.Command{
			{
				Name:    "list",
				Summary: "List configured targets",
				Usage:   "baha target list",
				Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
					if len(args) != 0 {
						return usageError("baha target list does not accept arguments", "Run 'baha target list --help' for usage.")
					}
					cfg, err := deployment.LoadConfig()
					if err != nil {
						return err
					}
					activated := strings.TrimSpace(os.Getenv("BASEHARBOR_TARGET"))
					effective, err := cfg.ResolveTarget("", activated)
					if err != nil {
						return err
					}
					names := cfg.TargetNames()
					if _, configured := cfg.Targets["local"]; !configured {
						names = append(names, "local")
						sort.Strings(names)
					}
					fmt.Fprintf(out, "%-20s %-12s %-20s %-16s %s\n", "TARGET", "RUNTIME", "ACCESS", "SCOPE", "SELECTOR")
					for _, name := range names {
						var (
							provider string
							access   string
							scope    string
							marks    []string
						)
						if target, configured := cfg.Targets[name]; configured {
							provider = target.Runtime.Provider
							access = target.Access.Reference
							scope = target.Scope
						} else {
							provider = "compose"
							access = "local"
							scope = "default"
							marks = append(marks, "implicit")
						}
						if name == cfg.DefaultTarget {
							marks = append(marks, "default")
						}
						if name == activated {
							marks = append(marks, "active")
						}
						if name == effective.Name {
							marks = append(marks, "effective")
						}
						fmt.Fprintf(out, "%-20s %-12s %-20s %-16s %s\n", name, provider, access, scope, strings.Join(marks, ","))
					}
					return nil
				},
			},
			{
				Name:    "show",
				Summary: "Show one target",
				Usage:   "baha target show [NAME]",
				Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
					if len(args) > 1 {
						return usageError("baha target show accepts at most one NAME", "Run 'baha target show NAME'.")
					}
					cfg, err := deployment.LoadConfig()
					if err != nil {
						return err
					}
					name := ""
					if len(args) == 1 {
						name = args[0]
					}
					target, err := cfg.ResolveTarget(name, os.Getenv("BASEHARBOR_TARGET"))
					if err != nil {
						return err
					}
					fmt.Fprintf(out, "Target   %s\nRuntime  %s\nAccess   %s\n", target.Name, target.RuntimeProvider, target.AccessReference)
					if target.Scope != "" {
						fmt.Fprintf(out, "Scope    %s\n", target.Scope)
					}
					root, err := deployment.TargetStateRoot(target.Name)
					if err != nil {
						return err
					}
					fmt.Fprintf(out, "State    %s\n", root)
					return nil
				},
			},
			{
				Name:    "create",
				Summary: "Create a deployment target",
				Usage:   "baha target create NAME --provider PROVIDER --access ACCESS --reference REFERENCE [--scope SCOPE] [--default]",
				Run:     createTarget,
			},
			{
				Name:    "delete",
				Summary: "Delete an unused target",
				Usage:   "baha target delete NAME",
				Run:     deleteTarget,
			},
			{
				Name:    "activate",
				Summary: "Print shell code that activates a target in the current shell",
				Usage:   "baha target activate NAME",
				Long:    "Activation is shell-local. Evaluate the emitted assignment in the current shell; BaseHarbor never mutates a parent process environment.",
				Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
					if len(args) != 1 {
						return usageError("baha target activate requires NAME", "Example: eval \"$(baha target activate docker-dev)\"")
					}
					cfg, err := deployment.LoadConfig()
					if err != nil {
						return err
					}
					if _, ok := cfg.Targets[args[0]]; !ok {
						return fmt.Errorf("target %q is not configured", args[0])
					}
					fmt.Fprint(out, shellActivationCode(currentShellName(), args[0]))
					return nil
				},
			},
			{
				Name:    "deactivate",
				Summary: "Print shell code that clears the active target",
				Usage:   "baha target deactivate",
				Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
					if len(args) != 0 {
						return usageError("baha target deactivate does not accept arguments", "Example: eval \"$(baha target deactivate)\"")
					}
					fmt.Fprint(out, shellDeactivationCode(currentShellName()))
					return nil
				},
			},
		},
	}
}

func collectTargetInspection(ctx context.Context) (targetInspectionResult, error) {
	target, err := effectiveTarget(ctx)
	if err != nil {
		return targetInspectionResult{}, err
	}
	result := targetInspectionResult{
		ContractVersion: machine.ContractVersion,
		Target:          target,
		Effective:       target.Name,
	}
	if cwd, cwdErr := os.Getwd(); cwdErr == nil {
		if selection, selectionErr := application.ResolveRepositoryEnvironment(cwd, applicationEnvironmentOverride); selectionErr == nil {
			result.Application = selection.Manifest.Name
			result.Environment = selection.Manifest.Environment
			result.Repository = selection.RepositoryRoot
			result.Effective = target.Name + " / " + result.Application + " / " + result.Environment
		}
	}
	return result, nil
}

func createTarget(ctx context.Context, args []string, out, errOut io.Writer) error {
	if len(args) == 0 {
		return usageError("baha target create requires NAME", "Example: baha target create docker-dev --provider docker --access local-docker --reference local")
	}
	name := args[0]
	if err := deployment.ValidateTargetName(name); err != nil {
		return err
	}
	var provider, accessName, reference, scope string
	makeDefault := false
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--provider", "--access", "--reference", "--scope":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return usageError(args[i]+" requires a value", "Run 'baha target create --help' for usage.")
			}
			key, value := args[i], strings.TrimSpace(args[i+1])
			i++
			switch key {
			case "--provider":
				provider = value
			case "--access":
				accessName = value
			case "--reference":
				reference = value
			case "--scope":
				scope = value
			}
		case "--default":
			makeDefault = true
		default:
			return unknownOptionUsage("baha target create", args[i], "--provider", "--access", "--reference", "--scope", "--default")
		}
	}
	if provider == "" || accessName == "" || reference == "" {
		return usageError("target create requires --provider, --access and --reference", "Example: baha target create docker-dev --provider docker --access local-docker --reference local")
	}
	cfg, err := deployment.LoadConfig()
	if err != nil {
		return err
	}
	if _, exists := cfg.Targets[name]; exists {
		return fmt.Errorf("target %q already exists", name)
	}
	if existing, exists := cfg.Access[accessName]; exists {
		if existing.Provider != provider || existing.Reference != reference {
			return fmt.Errorf("access %q already exists with different provider/reference", accessName)
		}
	} else {
		cfg.Access[accessName] = deployment.AccessDefinition{Provider: provider, Reference: reference}
	}
	cfg.Targets[name] = deployment.TargetDefinition{
		Runtime: deployment.RuntimeDefinition{Provider: provider},
		Access:  deployment.TargetAccess{Reference: accessName},
		Scope:   scope,
	}
	if makeDefault {
		cfg.DefaultTarget = name
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	fmt.Fprintf(out, "Target %s created (%s, access %s", name, provider, accessName)
	if scope != "" {
		fmt.Fprintf(out, ", scope %s", scope)
	}
	fmt.Fprintln(out, ")")
	return nil
}

func deleteTarget(ctx context.Context, args []string, out, errOut io.Writer) error {
	if len(args) != 1 {
		return usageError("baha target delete requires NAME", "Example: baha target delete docker-dev")
	}
	name := args[0]
	cfg, err := deployment.LoadConfig()
	if err != nil {
		return err
	}
	if _, ok := cfg.Targets[name]; !ok {
		return fmt.Errorf("target %q is not configured", name)
	}
	deployments, err := deployment.ListDeployments(name)
	if err != nil {
		return err
	}
	if len(deployments) != 0 {
		return fmt.Errorf("target %q still owns %d deployment(s); destroy them before deleting the target", name, len(deployments))
	}
	root, err := deployment.TargetStateRoot(name)
	if err != nil {
		return err
	}
	if entries, err := os.ReadDir(root); err == nil && len(entries) != 0 {
		return fmt.Errorf("target %q still owns runtime state under %s; destroy or detach owned state before deleting the target", name, root)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	delete(cfg.Targets, name)
	if cfg.DefaultTarget == name {
		cfg.DefaultTarget = ""
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	if err := os.Remove(root); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove empty target state directory: %w", err)
	}
	fmt.Fprintf(out, "Target %s deleted\n", name)
	return nil
}

func targetRuntimeStateRoot(target deployment.ResolvedTarget) (string, error) {
	root, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "runtime"), nil
}

func targetDataRoot(target deployment.ResolvedTarget) (string, error) {
	return deployment.TargetStateRoot(target.Name)
}

func targetRuntimeProjectName(target deployment.ResolvedTarget) string {
	return bhruntime.SharedProjectName(target.Name)
}

func targetRuntimeFiles(ctx context.Context) (deployment.ResolvedTarget, bhruntime.Files, error) {
	target, err := effectiveTarget(ctx)
	if err != nil {
		return deployment.ResolvedTarget{}, bhruntime.Files{}, err
	}
	root, err := targetRuntimeStateRoot(target)
	if err != nil {
		return deployment.ResolvedTarget{}, bhruntime.Files{}, err
	}
	files, err := bhruntime.ExistingFilesForProject(root, targetRuntimeProjectName(target))
	if err != nil {
		return target, bhruntime.Files{}, err
	}
	return target, files, nil
}

func existingTargetRuntimeFiles(ctx context.Context) (bhruntime.Files, error) {
	_, files, err := targetRuntimeFiles(ctx)
	return files, err
}

func ensureTargetRuntimeFiles(ctx context.Context, ports bhruntime.Ports) (deployment.ResolvedTarget, bhruntime.Files, error) {
	target, err := effectiveTarget(ctx)
	if err != nil {
		return deployment.ResolvedTarget{}, bhruntime.Files{}, err
	}
	root, err := targetRuntimeStateRoot(target)
	if err != nil {
		return deployment.ResolvedTarget{}, bhruntime.Files{}, err
	}
	files, err := bhruntime.EnsureFilesForProject(root, targetRuntimeProjectName(target), ports)
	if err != nil {
		return target, bhruntime.Files{}, err
	}
	return target, files, nil
}

func detectComposeForTarget(ctx context.Context, target deployment.ResolvedTarget) (bhruntime.Compose, error) {
	provider, err := bhruntime.DetectProviderForKind(ctx, bhruntime.ProviderKind(target.RuntimeProvider))
	if err != nil {
		return bhruntime.Compose{}, err
	}
	compose, ok := provider.(bhruntime.Compose)
	if !ok {
		return bhruntime.Compose{}, fmt.Errorf("target %q runtime provider %q is not compatible with the local container lifecycle", target.Name, target.RuntimeProvider)
	}
	return compose, nil
}

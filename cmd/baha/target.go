package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

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
			if len(args) != 0 {
				return usageError("baha target does not accept positional arguments", "Use 'baha target show NAME' or 'baha target --help'.")
			}
			target, err := effectiveTarget(ctx)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "Target   %s\n", target.Name)
			fmt.Fprintf(out, "Runtime  %s\n", target.RuntimeProvider)
			fmt.Fprintf(out, "Access   %s\n", target.AccessReference)
			if target.Scope != "" {
				fmt.Fprintf(out, "Scope    %s\n", target.Scope)
			}
			if cwd, err := os.Getwd(); err == nil {
				if selection, err := application.ResolveRepositoryEnvironment(cwd, applicationEnvironmentOverride); err == nil {
					fmt.Fprintf(out, "\nApplication  %s\n", selection.Manifest.Name)
					fmt.Fprintf(out, "Environment  %s\n", selection.Manifest.Environment)
					fmt.Fprintf(out, "Repository   %s\n", selection.RepositoryRoot)
					fmt.Fprintf(out, "\nEffective\n%s / %s / %s\n", target.Name, selection.Manifest.Name, selection.Manifest.Environment)
				}
			}
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
					if len(cfg.Targets) == 0 {
						fmt.Fprintln(out, "No targets configured.")
						return nil
					}
					fmt.Fprintf(out, "%-20s %-12s %-20s %-16s %s\n", "TARGET", "RUNTIME", "ACCESS", "SCOPE", "SELECTOR")
					for _, name := range cfg.TargetNames() {
						target := cfg.Targets[name]
						var marks []string
						if name == cfg.DefaultTarget {
							marks = append(marks, "default")
						}
						if name == activated {
							marks = append(marks, "active")
						}
						fmt.Fprintf(out, "%-20s %-12s %-20s %-16s %s\n", name, target.Runtime.Provider, target.Access.Reference, target.Scope, strings.Join(marks, ","))
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
						return usageError("baha target activate requires NAME", "Example: eval "$(baha target activate docker-dev)"")
					}
					cfg, err := deployment.LoadConfig()
					if err != nil {
						return err
					}
					if _, ok := cfg.Targets[args[0]]; !ok {
						return fmt.Errorf("target %q is not configured", args[0])
					}
					fmt.Fprintf(out, "export BASEHARBOR_TARGET=%q\n", args[0])
					return nil
				},
			},
			{
				Name:    "deactivate",
				Summary: "Print shell code that clears the active target",
				Usage:   "baha target deactivate",
				Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
					if len(args) != 0 {
						return usageError("baha target deactivate does not accept arguments", "Example: eval "$(baha target deactivate)"")
					}
					fmt.Fprintln(out, "unset BASEHARBOR_TARGET")
					return nil
				},
			},
		},
	}
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
	if makeDefault || cfg.DefaultTarget == "" {
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
	return "baseharbor-" + strings.ReplaceAll(target.Name, ".", "-")
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

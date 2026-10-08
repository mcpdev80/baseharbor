package main

import (
	"context"
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
	"github.com/mcpdev80/baseharbor/internal/orgconfig"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	runtimeresolver "github.com/mcpdev80/baseharbor/internal/runtime/resolver"
	"github.com/mcpdev80/baseharbor/internal/targetaccess"
)

type targetListItem struct {
	Name           string   `json:"name"`
	Runtime        string   `json:"runtime"`
	Access         string   `json:"access"`
	AccessProvider string   `json:"access_provider"`
	Scope          string   `json:"scope"`
	Selectors      []string `json:"selectors,omitempty"`
}

type targetShowResult struct {
	ContractVersion string                    `json:"contract_version"`
	Target          deployment.ResolvedTarget `json:"target"`
	StateRoot       string                    `json:"state_root"`
}

type targetInspectionResult struct {
	ContractVersion    string                             `json:"contract_version"`
	Target             deployment.ResolvedTarget          `json:"target"`
	Application        string                             `json:"application,omitempty"`
	Environment        string                             `json:"environment,omitempty"`
	Repository         string                             `json:"repository,omitempty"`
	Effective          string                             `json:"effective"`
	SelectionOrigin    string                             `json:"selection_origin"`
	OperatorAuth       map[string]operatorAuthObservation `json:"operator_auth,omitempty"`
	AccessCapabilities *targetaccess.Descriptor           `json:"access_capabilities,omitempty"`
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
	explicit := targetOverrideFromContext(ctx)
	activated := strings.TrimSpace(os.Getenv("BASEHARBOR_TARGET"))
	selected, selectionErr := selectedTargetName(explicit, activated, cfg)
	if selectionErr != nil {
		return deployment.ResolvedTarget{}, selectionErr
	}
	state, configured, err := orgconfig.LoadActiveOptional()
	if err != nil || !configured {
		if err == nil {
			return cfg.ResolveTarget(selected, "")
		}
		return deployment.ResolvedTarget{}, err
	}
	var preferences []orgconfig.PreferenceLayer
	userTarget := selected
	if userTarget != "" {
		defaults := orgconfig.EnvironmentDefaults{Target: userTarget}
		preferences = append(preferences, orgconfig.PreferenceLayer{Scope: orgconfig.ScopeUser,
			Identity: "target-selection", Digest: orgconfig.PreferenceDigest(defaults), Defaults: defaults})
	}
	if explicit != "" {
		preferences = append(preferences, orgconfig.PreferenceLayer{Scope: orgconfig.ScopeInvocation,
			Identity: "target-selection", Defaults: orgconfig.EnvironmentDefaults{Target: explicit}})
	}
	effective, err := orgconfig.ResolveEffective(state, organizationEnvironment(ctx), preferences...)
	if err != nil {
		return deployment.ResolvedTarget{}, err
	}
	if effective.Target != nil {
		return cfg.ResolveTarget(effective.Target.Value, "")
	}
	return cfg.ResolveTarget(selected, "")
}

func organizationDefaultTarget() (string, error) {
	state, ok, err := orgconfig.LoadActiveOptional()
	if err != nil || !ok {
		return "", err
	}
	effective, err := orgconfig.ResolveEffective(state, organizationEnvironment(context.Background()))
	if err != nil {
		return "", fmt.Errorf("resolve organization target default: %w", err)
	}
	if effective.Target == nil {
		return "", nil
	}
	return strings.TrimSpace(effective.Target.Value), nil
}

type organizationEnvironmentKey struct{}

func withOrganizationEnvironment(ctx context.Context, environment string) context.Context {
	return context.WithValue(ctx, organizationEnvironmentKey{}, strings.ToLower(strings.TrimSpace(environment)))
}

func organizationEnvironment(ctx context.Context) string {
	if value, ok := ctx.Value(organizationEnvironmentKey{}).(string); ok && value != "" {
		return value
	}

	environment := "dev"
	if cwd, cwdErr := os.Getwd(); cwdErr == nil {
		if selection, selectionErr := application.ResolveRepositoryEnvironment(cwd, applicationEnvironmentOverride); selectionErr == nil {
			if value := strings.TrimSpace(selection.Manifest.Environment); value != "" {
				environment = value
			}
		}
	}
	return environment
}

func targetCommand() *cli.Command {
	return &cli.Command{
		Name:    "target",
		Summary: "Inspect and manage BaseHarbor deployment targets",
		Usage:   "baha target [list|show|create|delete|activate|deactivate] [-o json|--output json|--json]",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			filtered, format, err := parseReadOutputArgs(args, "target")
			if err != nil {
				return err
			}
			if len(filtered) != 0 {
				return unknownOptionUsage("baha target", filtered[0], "-o", "--output", "--json")
			}
			result, err := collectTargetInspection(ctx)
			if err != nil {
				return err
			}
			if format == outputJSON {
				return writeJSON(out, result)
			}
			fmt.Fprintf(out, "Target   %s\n", result.Target.Name)
			fmt.Fprintf(out, "Runtime  %s\n", result.Target.RuntimeProvider)
			fmt.Fprintf(out, "Selected %s\n", result.SelectionOrigin)
			fmt.Fprintf(out, "Access   %s (%s)\n", result.Target.AccessReference, result.Target.AccessProvider)
			if result.Target.Scope != "" {
				fmt.Fprintf(out, "Scope    %s\n", result.Target.Scope)
			}
			if len(result.OperatorAuth) > 0 {
				fmt.Fprintln(out, "\nOperator authentication")
				environments := make([]string, 0, len(result.OperatorAuth))
				for environment := range result.OperatorAuth {
					environments = append(environments, environment)
				}
				sort.Strings(environments)
				for _, environment := range environments {
					auth := result.OperatorAuth[environment]
					fmt.Fprintf(out, "  %-12s %-16s %-14s %s\n", environment, auth.Status, auth.Session, auth.Provider)
				}
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
				Usage:   "baha target list [-o json|--output json|--json]",
				Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
					filtered, format, err := parseReadOutputArgs(args, "target list")
					if err != nil {
						return err
					}
					if len(filtered) != 0 {
						return usageError("baha target list does not accept positional arguments", "Use --json or -o json for structured output.")
					}
					cfg, err := deployment.LoadConfig()
					if err != nil {
						return err
					}
					activated := strings.TrimSpace(os.Getenv("BASEHARBOR_TARGET"))
					selection, selectionErr := selectedTargetName(targetOverrideFromContext(ctx), activated, cfg)
					// Listing must remain usable when multiple Targets require a choice.
					// All other resolver failures (corrupt selection/config) fail closed.
					if selectionErr != nil {
						if typed := machine.Classify(selectionErr); typed.Code != machine.ErrorConflict {
							return selectionErr
						}
					}
					effectiveName := ""
					if selectionErr == nil {
						effective, resolveErr := cfg.ResolveTarget(selection, "")
						if resolveErr != nil {
							return resolveErr
						}
						effectiveName = effective.Name
					}
					names := cfg.TargetNames()
					if _, configured := cfg.Targets["local"]; !configured {
						names = append(names, "local")
						sort.Strings(names)
					}
					items := make([]targetListItem, 0, len(names))
					for _, name := range names {
						var (
							provider       string
							access         string
							accessProvider string
							scope          string
							marks          []string
						)
						if target, configured := cfg.Targets[name]; configured {
							provider = target.Runtime.Provider
							access = target.Access.Reference
							if definition, ok := cfg.Access[access]; ok {
								accessProvider = definition.Provider
							}
							scope = target.Scope
						} else {
							provider = "docker"
							access = "local"
							accessProvider = "local"
							scope = "default"
							marks = append(marks, "implicit")
						}
						if name == cfg.DefaultTarget {
							marks = append(marks, "default")
						}
						if persisted, _ := readPersistedTarget(); name == activated || (activated == "" && name == persisted) {
							marks = append(marks, "active")
						}
						if name == effectiveName {
							marks = append(marks, "effective")
						}
						items = append(items, targetListItem{Name: name, Runtime: provider, Access: access, AccessProvider: accessProvider, Scope: scope, Selectors: marks})
					}
					return writeTargetList(out, format, items)
				},
			},
			{
				Name:    "show",
				Summary: "Show one target",
				Usage:   "baha target show [NAME] [-o json|--output json|--json]",
				Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
					filtered, format, err := parseReadOutputArgs(args, "target show")
					if err != nil {
						return err
					}
					if len(filtered) > 1 {
						return usageError("baha target show accepts at most one NAME", "Run 'baha target show NAME [--json]'.")
					}
					cfg, err := deployment.LoadConfig()
					if err != nil {
						return err
					}
					name := ""
					if len(filtered) == 1 {
						name = filtered[0]
					}
					explicit := name
					if explicit == "" { explicit = targetOverrideFromContext(ctx) }
					selection, err := selectedTargetName(explicit, strings.TrimSpace(os.Getenv("BASEHARBOR_TARGET")), cfg)
					if err != nil {
						return err
					}
					target, err := cfg.ResolveTarget(selection, "")
					if err != nil {
						return err
					}
					root, err := deployment.TargetStateRoot(target.Name)
					if err != nil {
						return err
					}
					if format == outputJSON {
						return writeJSON(out, targetShowResult{ContractVersion: machine.ContractVersion, Target: target, StateRoot: root})
					}
					fmt.Fprintf(out, "Target   %s\nRuntime  %s\nAccess   %s (%s)\n", target.Name, target.RuntimeProvider, target.AccessReference, target.AccessProvider)
					if target.Scope != "" {
						fmt.Fprintf(out, "Scope    %s\n", target.Scope)
					}
					fmt.Fprintf(out, "State    %s\n", root)
					return nil
				},
			},
			{
				Name:    "create",
				Summary: "Create a deployment target",
				Usage:   "baha target create NAME --runtime-provider PROVIDER --access ACCESS --access-provider PROVIDER --reference REFERENCE [--scope SCOPE] [--default]",
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
				Summary: "Persist the active deployment target for this user",
				Usage:   "baha target activate NAME",
				Long:    "Persists per-user target selection across CLI processes. --target and BASEHARBOR_TARGET override the persisted selection.",
				Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
					if len(args) == 1 && strings.HasPrefix(args[0], "-") {
						return unknownOptionUsage("baha target activate", args[0])
					}
					if len(args) == 0 {
						return guidedTargetActivation(ctx, out, errOut)
					}
					if len(args) != 1 {
						return usageError("baha target activate requires NAME", "Example: baha target activate docker-dev")
					}
					cfg, err := deployment.LoadConfig()
					if err != nil {
						return err
					}
					if _, ok := cfg.Targets[args[0]]; !ok && args[0] != "local" {
						return machine.NewError(machine.ErrorNotFound, fmt.Sprintf("target %q is not configured", args[0]), "Run baha target list to choose an existing deployment Target.", false)
					}
					if err := writePersistedTarget(args[0]); err != nil {
						return err
					}
					fmt.Fprintf(out, "Active target: %s\n", args[0])
					return nil
				},
			},
			{
				Name:    "deactivate",
				Summary: "Clear persisted active deployment target",
				Usage:   "baha target deactivate",
				Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
					if len(args) != 0 && strings.HasPrefix(args[0], "-") {
						return unknownOptionUsage("baha target deactivate", args[0])
					}
					if len(args) != 0 {
						return usageError("baha target deactivate does not accept arguments", "Example: baha target deactivate")
					}
					if err := clearPersistedTarget(); err != nil {
						return err
					}
					fmt.Fprintln(out, "Active target cleared")
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
		SelectionOrigin: targetSelectionOrigin(ctx),
	}
	if kind, parseErr := targetaccess.ParseProviderKind(target.AccessProvider); parseErr == nil {
		if descriptor, builtIn := targetaccess.BuiltInDescriptor(kind); builtIn {
			result.AccessCapabilities = &descriptor
		}
	}
	cfg, cfgErr := deployment.LoadConfig()
	if cfgErr == nil {
		if definition, ok := cfg.Targets[target.Name]; ok && len(definition.OperatorAuth) > 0 {
			result.OperatorAuth = make(map[string]operatorAuthObservation, len(definition.OperatorAuth))
			for environment := range definition.OperatorAuth {
				result.OperatorAuth[environment] = collectOperatorAuthObservation(ctx, target.Name, environment)
			}
		}
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
		return runGuidedNewLocalTarget(ctx, out, errOut)
	}
	filtered, format, err := parseReadOutputArgs(args, "target create")
	if err != nil {
		return err
	}
	args = filtered
	if len(args) == 0 {
		return usageError("baha target create requires NAME", "Example: baha target create docker-dev --provider docker --access local-docker --reference local")
	}
	name := args[0]
	if err := deployment.ValidateTargetName(name); err != nil {
		return err
	}
	var runtimeProvider, accessProvider, accessName, reference, scope string
	makeDefault := false
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--provider", "--runtime-provider", "--access-provider", "--access", "--reference", "--scope":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return usageError(args[i]+" requires a value", "Run 'baha target create --help' for usage.")
			}
			key, value := args[i], strings.TrimSpace(args[i+1])
			i++
			switch key {
			case "--provider", "--runtime-provider":
				runtimeProvider = value
			case "--access-provider":
				accessProvider = value
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
			return unknownOptionUsage("baha target create", args[i], "--provider", "--runtime-provider", "--access-provider", "--access", "--reference", "--scope", "--default")
		}
	}
	if runtimeProvider == "" || accessName == "" || reference == "" {
		return usageError("target create requires --runtime-provider (or legacy --provider), --access and --reference", "Example: baha target create docker-dev --runtime-provider docker --access local-docker --access-provider local --reference local")
	}
	if accessProvider == "" {
		if reference == "local" {
			accessProvider = "local"
		} else {
			return usageError("non-local target access requires --access-provider", "Example: baha target create docker-remote --runtime-provider docker --access node-a --access-provider baseharbor-node-connector --reference node-a")
		}
	}
	result, err := createTargetDefinition(ctx, machineTargetCreateInput{Name: name, RuntimeProvider: runtimeProvider, AccessProvider: accessProvider, Access: accessName, Reference: reference, Scope: scope, Default: makeDefault})
	if err != nil {
		return err
	}
	if format == outputJSON {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "Target %s created (runtime %s, access %s via %s", name, runtimeProvider, accessName, accessProvider)
	if scope != "" {
		fmt.Fprintf(out, ", scope %s", scope)
	}
	fmt.Fprintln(out, ")")
	return nil
}

func deleteTarget(ctx context.Context, args []string, out, errOut io.Writer) error {
	filtered, format, err := parseReadOutputArgs(args, "target delete")
	if err != nil {
		return err
	}
	args = filtered
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			return usageError("unknown target delete option "+arg, "Run 'baha target delete --help'; target deletion does not accept --yes.")
		}
	}
	if len(args) != 1 {
		return usageError("baha target delete requires NAME", "Example: baha target delete docker-dev")
	}
	name := args[0]
	result, err := deleteTargetDefinition(ctx, name)
	if err != nil {
		return err
	}
	if format == outputJSON {
		return writeJSON(out, result)
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

func ensureTargetRuntimeFiles(ctx context.Context, ports bhruntime.Ports, ha bool) (deployment.ResolvedTarget, bhruntime.Files, error) {
	target, err := effectiveTarget(ctx)
	if err != nil {
		return deployment.ResolvedTarget{}, bhruntime.Files{}, err
	}
	root, err := targetRuntimeStateRoot(target)
	if err != nil {
		return deployment.ResolvedTarget{}, bhruntime.Files{}, err
	}
	files, err := bhruntime.EnsureFilesForProjectAndResources(root, targetRuntimeProjectName(target), bhruntime.SharedResourceProjectName(target.Name), ports, ha)
	if err != nil {
		return target, bhruntime.Files{}, err
	}
	return target, files, nil
}

func detectRuntimeForTarget(ctx context.Context, target deployment.ResolvedTarget) (bhruntime.RuntimeProvider, error) {
	if access := strings.TrimSpace(target.AccessProvider); access != "" && access != "local" {
		return nil, machine.NewError(machine.ErrorCapabilityMissing,
			"Selected remote Target has no authenticated live runtime binding.",
			"Establish the selected Target Access session; BaseHarbor never executes a remote selection locally.", true)
	}
	provider, err := runtimeresolver.RuntimeProvider(ctx, bhruntime.ProviderKind(target.RuntimeProvider))
	if err != nil {
		return nil, err
	}
	return provider, nil
}

func writeTargetList(out io.Writer, format cliOutputFormat, items []targetListItem) error {
	if format == outputJSON {
		return writeJSON(out, map[string]any{"contract_version": machine.ContractVersion, "targets": items})
	}
	fmt.Fprintf(out, "%-20s %-12s %-20s %-20s %-16s %s\n", "TARGET", "RUNTIME", "ACCESS", "ACCESS PROVIDER", "SCOPE", "SELECTOR")
	for _, item := range items {
		fmt.Fprintf(out, "%-20s %-12s %-20s %-20s %-16s %s\n", item.Name, item.Runtime, item.Access, item.AccessProvider, item.Scope, strings.Join(item.Selectors, ","))
	}
	return nil
}

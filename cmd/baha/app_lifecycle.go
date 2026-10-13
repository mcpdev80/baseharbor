package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	"github.com/mcpdev80/baseharbor/internal/preflight"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func appDownCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "down",
		Summary: "Stop an application runtime while preserving persistent data",
		Usage:   "baha app down [NAME]",
		Long:    "Stops a repository application workload and its per-application Application Runtime Broker first, then removes BaseHarbor-managed backend containers and transient network while preserving persistent data volumes, runtime credentials, application-owned Compose volumes and managed OpenBao scope. Without NAME it resolves the nearest repository baseharbor.yaml.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			filtered, format, err := parseReadOutputArgs(args, "app down")
			if err != nil {
				return err
			}
			for _, arg := range filtered {
				if strings.HasPrefix(arg, "-") {
					return &cli.UsageError{Message: "unknown option " + arg, Hint: "Use baha app down [NAME]; down preserves persistent data and requires acknowledgement of its current consequence plan. --yes does not provide either acknowledgement.", NoSuggestions: arg == "--yes" || arg == "-y"}
				}
			}
			args = filtered
			resolved, err := resolveApplication(ctx, store, args, "down")
			if err != nil {
				return err
			}
			humanOut, humanErr := out, errOut
			if format == outputJSON {
				humanOut, humanErr = io.Discard, io.Discard
			}
			result, err := stopApplicationRuntimeWithResult(ctx, resolved, humanOut, humanErr)
			if err != nil {
				return err
			}
			if format == outputJSON {
				return writeJSON(out, result)
			}
			return writeJSON(out, result)
		},
	}
}

type applicationStopResult struct {
	Application          string                                 `json:"application"`
	Environment          string                                 `json:"environment"`
	State                string                                 `json:"state"`
	DataPreserved        bool                                   `json:"data_preserved"`
	ProviderChangeResult *application.ProviderImpactScopeResult `json:"provider_change_result,omitempty"`
}

func executeApplicationStop(ctx context.Context, resolved resolvedApplication, out, errOut io.Writer) error {
	m := resolved.Manifest
	runtimeProject := application.RuntimeComposeProjectNameForStore(resolved.Store, m)
	resourceProject := application.RuntimeProjectNameForStore(resolved.Store, m)
	term := cli.NewTerminal(ctx, out, errOut)
	term.Header(m.Name, m.Environment)
	term.Info("target", resolved.Target.Name)
	files, err := application.ExistingRuntimeFiles(resolved.Store, m)
	if err != nil {
		return err
	}
	var compose bhruntime.RuntimeProvider
	var before []bhruntime.ProjectResource
	checks := []preflight.Check{
		{Name: "manifest", Run: func(context.Context) error { return m.Validate() }},
		{Name: "supported desired services", Run: func(context.Context) error { return application.CheckSupportedRuntimeServices(m) }},
		{Name: "manifest permissions", Run: func(context.Context) error {
			return checkManifestPermissions(resolved.ManifestPath, resolved.FromRepository)
		}},
		applicationWorkloadContractCheck(resolved),
		{Name: "runtime permissions", Run: func(context.Context) error { return application.CheckRuntimePermissions(files) }},
		{Name: "managed runtime definition", Run: func(context.Context) error { return application.CheckManagedRuntimeDefinition(files, m) }},
		{Name: "runtime orchestration", Run: func(ctx context.Context) error {
			var err error
			compose, err = detectRuntimeForApplication(ctx, resolved, bhruntime.CapabilityWorkloadLifecycle, bhruntime.CapabilityResourceOwnership)
			return err
		}},
		{Name: "runtime configuration", Run: func(ctx context.Context) error {
			if !application.HasApplicationScopedRuntimeServices(m) {
				return nil
			}
			return compose.ConfigProject(ctx, runtimeProject, files.Compose, files.Env)
		}},
		{Name: "runtime ownership", Run: func(ctx context.Context) error {
			var err error
			before, err = compose.InspectProjectResources(ctx, runtimeProject, application.ExpectedRuntimeResourcesForIdentity(m, runtimeProject, resourceProject))
			return err
		}},
	}
	results, ok := preflight.RunWithTimeout(ctx, checks, 30*time.Second)
	renderPreflightUX(term, results)
	if !ok {
		return errors.New("application down preflight failed")
	}

	if err := suspendConnectivityForManifest(ctx, compose, resolved); err != nil {
		return fmt.Errorf("suspend cross-application connectivity: %w", err)
	}
	runBestEffortDevelopmentRouteSuspension(term, func() error {
		return removeApplicationDevelopmentRoutesBeforeDown(ctx, compose, resolved, m)
	})
	if len(m.Exposures) > 0 {
		if err := stopManagedExposure(ctx, compose, m, files); err != nil {
			return err
		}
		term.Result("STOPPED", "managed-exposure", "application exposure provider stopped")
	}
	registry, err := (capability.RegistryStore{Path: filepath.Join(resolved.TargetStateRoot, "provider-registry.json")}).Load()
	if err != nil {
		return err
	}
	for _, instance := range registry.Instances {
		if standaloneProviderKind(instance.Provider.Kind) && instance.Scope == capability.ScopeApplication && instance.Ownership == capability.OwnershipBaseHarbor && instance.OwnerApplicationID == m.ApplicationID && instance.OwnerEnvironment == m.Environment {
			if err := retireStandaloneApplicationProvider(ctx, resolved, compose, instance, registry, out); err != nil {
				return fmt.Errorf("stop application-scoped provider: %w", err)
			}
			continue
		}
		if instance.Provider.Kind != capability.ProviderKeycloak || instance.Scope != capability.ScopeApplication || instance.Ownership != capability.OwnershipBaseHarbor || instance.OwnerApplicationID != m.ApplicationID || instance.OwnerEnvironment != m.Environment {
			continue
		}
		result, err := identityprovider.RetireKeycloakRuntime(ctx, compose, m, providerInstancePlacement(instance), resolved.TargetStateRoot, resolved.Target.Name)
		for _, item := range result.Retained {
			fmt.Fprintf(out, "Retained %s %s: %s\n", item.Resource.Kind, item.Resource.Name, item.Reason)
		}
		if err != nil {
			return fmt.Errorf("stop application-scoped Identity runtime: %w", err)
		}
	}
	if removed, err := destroyRepositoryWorkloadRuntime(ctx, compose, resolved, files); err != nil {
		return err
	} else if removed {
		term.Result("STOPPED", "workload", "repository workload containers and networks removed; application-owned volumes preserved")
	}
	if application.RequiresRuntimeBroker(m) {
		if err := stopRuntimeBroker(ctx, compose, m, files); err != nil {
			return err
		}
		term.Result("STOPPED", "runtime-broker", "application runtime broker stopped")
	}

	project := runtimeProject
	if application.HasApplicationScopedRuntimeServices(m) {
		if err := compose.DownProject(ctx, project, files.Compose, files.Env); err != nil {
			return err
		}
	}
	after, err := compose.InspectProjectResources(ctx, runtimeProject, application.ExpectedRuntimeResourcesForIdentity(m, runtimeProject, resourceProject))
	if err != nil {
		return fmt.Errorf("verify application down: %w", err)
	}
	if application.ResourceExists(after, "container") || application.ResourceExists(after, "network") {
		return errors.New("verify application down: container or network still exists")
	}
	for _, volume := range application.ExpectedPersistentRuntimeResourcesForProject(m, resourceProject) {
		if application.ResourceNamedExists(before, volume) && !application.ResourceNamedExists(after, volume) {
			return fmt.Errorf("verify application down: persistent volume %s was not preserved", volume.Name)
		}
	}
	if err := recordObservedDeployment(resolved, "stopped", false); err != nil {
		return fmt.Errorf("record stopped application state: %w", err)
	}
	term.Section("Application")
	term.Result("STOPPED", "application", "persistent data preserved")
	return nil
}

func runBestEffortDevelopmentRouteSuspension(term *cli.Terminal, suspend func() error) {
	if suspend == nil {
		return
	}
	if err := suspend(); err != nil {
		term.Warn(
			"development-routes",
			"canonical development-route reconciliation deferred; application shutdown will continue: "+err.Error(),
		)
	}
}

func removeApplicationDevelopmentRoutesBeforeDown(ctx context.Context, compose bhruntime.RuntimeProvider, resolved resolvedApplication, m application.Manifest) error {
	owner := "app/" + m.Name + "/" + m.Environment
	if err := removeApplicationGatewayOwners(ctx, compose, resolved, owner); err != nil {
		return fmt.Errorf("suspend canonical development routes before application down: %w", err)
	}
	return nil
}

func appDestroyCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "destroy",
		Summary: "Retire owned application runtime while retaining data and recovery state",
		Usage:   "baha app destroy [NAME] [--yes] [--full-reset]",
		Long:    "Shows the current provider consequence plan before retiring owned workload/provider runtime and application routes. Default destruction retains persistent data, credentials, recovery material and the canonical deployment owner for reactivation with apply. Provider changes require two explicit acknowledgements of the current impact plan; --yes alone does not supply those acknowledgements. --full-reset selects the separate destructive cleanup path, including owned application state, while preserving authored source and repository Compose data.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			filtered, format, err := parseReadOutputArgs(args, "app destroy")
			if err != nil {
				return err
			}
			if format != outputJSON {
				return executeApplicationDestroyLifecycle(ctx, store, filtered, out, errOut)
			}
			execution, err := newApplicationDestroyExecution(ctx, store, filtered, io.Discard, io.Discard)
			if err != nil {
				return err
			}
			if !execution.confirmed && !execution.fullReset && !isRemoteApplication(execution.resolved) {
				_, plan, err := buildApplicationRuntimeRetirementPlan(ctx, execution.resolved)
				if err != nil {
					return err
				}
				return writeJSON(out, plan)
			}
			if err := executeApplicationDestroyLifecycle(ctx, store, filtered, io.Discard, io.Discard); err != nil {
				return err
			}
			if !execution.confirmed {
				return nil
			}
			result, err := applicationDestroyResult(execution.resolved, execution.fullReset)
			if err != nil {
				return err
			}
			return writeJSON(out, result)
		},
	}
}

func executeApplicationDestroyLifecycle(ctx context.Context, store application.Store, args []string, out, errOut io.Writer) error {
	execution, err := newApplicationDestroyExecution(ctx, store, args, out, errOut)
	if err != nil {
		return err
	}
	if isRemoteApplication(execution.resolved) {
		unlock, err := lockApplicationTargetLifecycle(execution.resolved)
		if err != nil {
			return err
		}
		defer unlock()
		runtime, intent, err := restoreRemoteApplication(ctx, execution.resolved)
		if err != nil {
			return err
		}
		if !execution.confirmed {
			fmt.Fprintln(out, "No changes were made. Re-run with --yes to delete the owned remote Application runtime.")
			return nil
		}
		plan, err := authorizeApplicationDestruction(ctx, execution.resolved, execution.fullReset, out)
		if err != nil {
			return err
		}
		if !execution.fullReset {
			if plan.Providers.Impact == nil {
				return runtimeMutationOwnerError()
			}
			if _, err := recordPendingApplicationRuntimeRetirement(execution.resolved, *plan.Providers.Impact); err != nil {
				return err
			}
		}
		if err := runtime.DestroyApplication(ctx, intent, execution.fullReset); err != nil {
			return err
		}
		// Keep the snapshot available through state cleanup; removing the
		// deployment record removes it together with the verified binding.
		execution.manifest = intent
		if !execution.fullReset {
			if err := application.ReleaseApplicationProviderReferencesAt(execution.resolved.TargetStateRoot, intent); err != nil {
				return err
			}
			return recordVerifiedApplicationRuntimeRetirement(execution.resolved, *plan.Providers.Impact, nil, plan.Materials)
		}
		if err := execution.removeApplicationState(); err != nil {
			return err
		}
		return recordApplicationAudit(ctx, execution.resolved, "destroy", "success", "ownership verified", "retained remote Application publication destroyed with owned cleanup verified")
	}
	unlock, err := lockApplicationTargetLifecycle(execution.resolved)
	if err != nil {
		return err
	}
	defer unlock()
	if !execution.fullReset {
		return execution.retireRuntimePreservingData(ctx, out)
	}
	if err := execution.runPreflight(ctx); err != nil {
		return err
	}
	if err := execution.renderDeletePlan(); err != nil {
		return err
	}
	if !execution.confirmed {
		fmt.Fprintln(out, "No changes were made. Re-run with --yes to permanently delete BaseHarbor-managed resources.")
		return nil
	}
	if _, err := authorizeApplicationDestruction(ctx, execution.resolved, true, out); err != nil {
		return err
	}
	if err := execution.cleanupDevelopmentCanonicalRoutes(ctx); err != nil {
		return err
	}
	if err := execution.destroyRuntimeResources(ctx); err != nil {
		return err
	}
	if err := execution.cleanupProviderState(ctx); err != nil {
		return err
	}
	if err := execution.removeApplicationState(); err != nil {
		return err
	}
	return recordApplicationAudit(ctx, execution.resolved, "destroy", "success", "ownership verified", "BaseHarbor-owned application runtime resources and state removed")
}

func parseDestroyArgs(args []string) (string, bool, bool, error) {
	var name string
	confirmed := false
	fullReset := false
	for _, arg := range args {
		switch {
		case arg == "--yes":
			confirmed = true
		case arg == "--full-reset":
			fullReset = true
		case strings.HasPrefix(arg, "-"):
			return "", false, false, usageError("unknown option "+arg, "Run 'baha app destroy --help' for available options.")
		default:
			if name != "" {
				return "", false, false, usageError("baha app destroy accepts at most one NAME", "Inside an application repository omit NAME.")
			}
			name = arg
		}
	}
	return name, confirmed, fullReset, nil
}

func openBaoDestroyScopeRequired(state openbao.State) (bool, error) {
	if !state.Initialized {
		return false, nil
	}
	if state.Sealed {
		return false, openbao.ErrSealed
	}
	return true, nil
}

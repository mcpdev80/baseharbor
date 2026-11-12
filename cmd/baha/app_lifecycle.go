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
	command := appStopCommand(store)
	command.Name = "down"
	command.Usage = "baha app down [NAME] [--yes]"
	return command
}

type applicationStopModeKey struct{}

func isApplicationStopMode(ctx context.Context) bool {
	stop, _ := ctx.Value(applicationStopModeKey{}).(bool)
	return stop
}

func appStopCommand(store application.Store) *cli.Command {
	command := appDestroyCommand(store)
	run := command.Run
	command.Name = "stop"
	command.Summary = "Stop application runtimes while preserving data for reactivation"
	command.Usage = "baha app stop [NAME] [--yes]"
	command.Long = "Stops the owned workload and provider runtime, releases its active provider references, and retains persistent data, credentials, recovery material and the deployment owner so 'baha app apply' can reactivate it. Pass --yes to execute; the current provider consequence plan still requires its own acknowledgement. Use 'baha app destroy' to permanently delete app-owned data."
	command.Run = func(ctx context.Context, args []string, out, errOut io.Writer) error {
		ctx = context.WithValue(ctx, applicationStopModeKey{}, true)
		return run(ctx, args, out, errOut)
	}
	return command
}

type applicationStopResult struct {
	Application           string                                 `json:"application"`
	Environment           string                                 `json:"environment"`
	Target                string                                 `json:"target"`
	State                 string                                 `json:"state"`
	Verified              bool                                   `json:"verified"`
	DataPreserved         bool                                   `json:"data_preserved"`
	ProviderChangeResult  *application.ProviderImpactScopeResult `json:"provider_change_result,omitempty"`
	ProviderChangeReceipt *application.ProviderChangeResult      `json:"provider_change_receipt,omitempty"`
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
		Summary: "Permanently remove an application and its exclusively owned data",
		// --uuid DEPLOYMENT-ID is accepted (0427-175): it is the only
		// unambiguous identity on a registry poisoned by two records claiming
		// one application_id, and the conflict message advertises exactly this
		// shape as the repair.
		Usage: "baha app destroy [NAME] [--uuid DEPLOYMENT-ID] [--yes]",
		Long:  "Permanently removes the application, its BaseHarbor-owned runtime resources, application data, credentials, recovery state and deployment registration. Exclusively app-owned Compose data volumes are removed too. Shared, foreign or ambiguously owned resources are preserved and reported. Use 'baha app stop' to stop runtime services while preserving data. Provider changes and permanent data deletion require explicit approval of the current plan.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			target, err := effectiveTarget(ctx)
			if err != nil {
				return err
			}
			if target.AccessProvider != "" && target.AccessProvider != "local" {
				return remoteApplicationDestroyCLI(ctx, target, args, out, errOut)
			}
			filtered, format, err := parseReadOutputArgs(args, "app destroy")
			if err != nil {
				return err
			}
			if format != outputJSON {
				if err := executeApplicationDestroyLifecycle(ctx, store, filtered, out, errOut); err != nil {
					return err
				}
				if containsExactArgument(filtered, "--yes") {
					refreshWorkstationHosts(ctx, target, out)
				}
				return nil
			}
			execution, err := newApplicationDestroyExecution(ctx, store, filtered, io.Discard, io.Discard)
			if err != nil {
				return err
			}
			if !execution.confirmed && !isRemoteApplication(execution.resolved) {
				if !execution.fullReset {
					_, plan, err := buildApplicationRuntimeRetirementPlan(ctx, execution.resolved)
					if err != nil {
						return err
					}
					return writeJSON(out, plan)
				}
				plan, err := buildApplicationDestructionPlan(ctx, execution.resolved, true)
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
			refreshWorkstationHosts(ctx, target, errOut)
			result, err := applicationDestroyResult(execution.resolved, execution.fullReset)
			if err != nil {
				return err
			}
			if isApplicationStopMode(ctx) {
				result.Operation = "stop"
				result.State = "stopped"
				result.Detail = "Owned runtime retired; persistent data and recovery owner retained for reactivation."
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
		unlock, err := lockApplicationTargetLifecycle(execution.resolved, true)
		if err != nil {
			return err
		}
		defer unlock()
		runtime, intent, err := restoreRemoteApplication(ctx, execution.resolved)
		if err != nil {
			return err
		}
		if !execution.confirmed {
			if isApplicationStopMode(ctx) {
				fmt.Fprintln(out, "No changes were made. Re-run with --yes to stop the owned remote Application runtime while retaining its data.")
			} else {
				fmt.Fprintln(out, "No changes were made. Re-run with --yes to delete the owned remote Application runtime and data.")
			}
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
	// destroy takes priority over concurrent up/apply so it cannot be starved
	// by a continuous stream of writers on the same target.
	unlock, err := lockApplicationTargetLifecycle(execution.resolved, true)
	if err != nil {
		return err
	}
	defer unlock()
	if !execution.fullReset {
		return execution.retireRuntimePreservingData(ctx, out)
	}
	if neverDelivered, err := execution.verifyOwnershipWhenNoDeploymentRecord(ctx); err != nil {
		return err
	} else if neverDelivered {
		if !execution.confirmed {
			fmt.Fprintln(out, "No changes were made. Re-run with --yes to remove this never-deployed application and its local BaseHarbor state.")
			return nil
		}
		if err := authorizeApplicationOperation(ctx, "destroy", execution.resolved); err != nil {
			return err
		}
		if err := execution.assertSelectionResolvedIdentity(); err != nil {
			return err
		}
		if err := execution.removeApplicationState(); err != nil {
			return err
		}
		return recordApplicationAudit(ctx, execution.resolved, "destroy", "success", "never delivered; local state removed", "never-deployed application and its BaseHarbor registration were permanently removed")
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
	// Selector-vs-resolved identity guard. The repair deletes a directory, so
	// the path it took must be the path the operator selected. Fail closed on
	// drift HERE, in the resolve-and-authorize phase, BEFORE the runtime/route/
	// provider cleanup that follows (cleanupDevelopmentCanonicalRoutes,
	// destroyRuntimeResources, cleanupProviderState) and before the
	// DeploymentRecord removal in removeApplicationState — so a selection that
	// drifted onto another twin never removes a record the operator did not
	// address (0427-175). A check after the remove cannot prevent the loss.
	if err := execution.assertSelectionResolvedIdentity(); err != nil {
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
	fullReset := true
	// The --uuid FLAG is consumed by extractApplicationEnvironment, but its bare
	// VALUE stays in the list it returns. Neither token may enter the positional
	// scan: the value would be taken for the application NAME (the wrong target
	// on a poisoned registry, 0427-175) and the flag would trip the unknown-
	// option branch. Drop the flag here and the value with it (the index skips
	// one). The selector itself is captured by extractApplicationUUID.
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--yes":
			confirmed = true
		case arg == "--full-reset":
			fullReset = true
		case arg == "--uuid":
			i++
		case strings.HasPrefix(arg, "--uuid="):
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

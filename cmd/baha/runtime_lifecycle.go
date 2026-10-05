package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/connectivityrelay"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/devgateway"
	"github.com/mcpdev80/baseharbor/internal/hosttrust"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	metricsprovider "github.com/mcpdev80/baseharbor/internal/metrics"
	"github.com/mcpdev80/baseharbor/internal/objectstorage"
	"github.com/mcpdev80/baseharbor/internal/runtimeexecutor"
	"github.com/mcpdev80/baseharbor/internal/telemetry"
	tracesprovider "github.com/mcpdev80/baseharbor/internal/traces"
)

func runtimeDestroy(parent context.Context, args []string, out io.Writer) error {
	confirmed := false
	for _, arg := range args {
		switch arg {
		case "--yes":
			confirmed = true
		default:
			return usageError("unknown argument "+arg, "Usage: baha destroy [--yes]")
		}
	}

	return destroyControlPlane(parent, confirmed, out)
}
func destroyControlPlane(parent context.Context, confirmed bool, out io.Writer) error {
	if err := authorizeCurrentMCPContext(parent, "control-plane.destroy", "", "", ""); err != nil {
		return err
	}

	target, err := effectiveTarget(parent)
	if err != nil {
		return err
	}
	files, err := existingTargetRuntimeFiles(parent)
	if err != nil {
		return fmt.Errorf("runtime is not initialized: %w", err)
	}
	dataDir, err := targetDataRoot(target)
	if err != nil {
		return err
	}
	if err := application.CheckControlPlaneDestroySafeAt(dataDir); err != nil {
		return fmt.Errorf("target destroy preflight: %w", err)
	}
	sharedBackends, err := application.SharedBackendDestroyPlan(dataDir, target.Name)
	if err != nil {
		return fmt.Errorf("target shared-backend destroy preflight: %w", err)
	}
	inactive, err := inactiveTargetDeployments(parent, target.Name)
	if err != nil {
		return fmt.Errorf("inspect retained deployment observations: %w", err)
	}
	runtimeDir, err := targetRuntimeStateRoot(target)
	if err != nil {
		return err
	}
	inventoryCtx, inventoryCancel := context.WithTimeout(parent, 30*time.Second)
	plan, err := collectTargetDestroyInventory(inventoryCtx, target, true)
	inventoryCancel()
	if err != nil {
		return fmt.Errorf("target destroy ownership inventory: %w", err)
	}
	preserved, err := preservedTargetRecovery(parent, target)
	if err != nil {
		return fmt.Errorf("inventory external recovery file: %w", err)
	}
	recoveryPreserved := []fullDestroyResult{}
	if preserved.Status != "" {
		recoveryPreserved = append(recoveryPreserved, preserved)
	}
	recordDestroyPlan(parent, []targetDestroyInventory{plan}, recoveryPreserved)
	fmt.Fprintln(out, "BaseHarbor target destroy plan")
	fmt.Fprintf(out, "  control plane: project %s (containers, network and BaseHarbor-owned volumes)\n", files.Project)
	if _, err := objectstorage.ExistingProviderFilesAt(dataDir, target.Name); err == nil {
		fmt.Fprintln(out, "  object storage: shared SeaweedFS provider (container, network and BaseHarbor-owned volume)")
	}
	if _, err := telemetry.ExistingProviderFilesAt(dataDir, target.Name); err == nil {
		fmt.Fprintln(out, "  telemetry: shared OpenTelemetry Collector provider (container and network)")
	}
	if instances, err := metricsprovider.ExistingSharedProviderInstancesAt(dataDir, target.Name); err == nil && len(instances) > 0 {
		fmt.Fprintf(out, "  metrics: %d shared Prometheus provider instance(s) across default/sharing boundaries\n", len(instances))
	}
	for _, shared := range sharedBackends {
		fmt.Fprintf(out, "  shared data: project %s module %s (owned SQL/cache containers, network and volumes)\n", shared.Project, shared.Dir)
	}
	fmt.Fprintf(out, "  runtime state: %s\n", runtimeDir)
	fmt.Fprintf(out, "  registry:      %s\n", filepath.Join(dataDir, "provider-registry.json"))
	if records, trustErr := hosttrust.StateRecords(dataDir); trustErr != nil {
		return fmt.Errorf("inspect BaseHarbor-owned host trust: %w", trustErr)
	} else if len(records) > 0 {
		fmt.Fprintf(out, "  host trust:    %d BaseHarbor-owned CA anchor(s)\n", len(records))
	}
	fmt.Fprintln(out, "  application registrations/inputs: preserved; inactive observations reconciled")
	fmt.Fprintln(out, "  application-owned repository data/volumes: preserved")
	plan.render(out)
	if preserved.Status != "" {
		fmt.Fprintf(out, "  PRESERVED %s %s\n", preserved.Resource, preserved.Detail)
	}
	if !confirmed {
		fmt.Fprintln(out, "No changes were made. Re-run with --yes to permanently remove the selected BaseHarbor target control plane.")
		return nil
	}

	ctx, cancel := context.WithTimeout(parent, time.Minute)
	defer cancel()
	compose, err := detectRuntimeForTarget(ctx, target)
	if err != nil {
		return err
	}
	if removed, err := hosttrust.RemoveOwned(ctx, dataDir); err != nil {
		return fmt.Errorf("remove BaseHarbor-owned host trust before destroy: %w", err)
	} else if removed > 0 {
		fmt.Fprintf(out, "[OK] host trust         removed %d BaseHarbor-owned CA anchor(s)\n", removed)
	}
	if relays, err := connectivityrelay.ExistingInstancesAt(dataDir, target.Name); err != nil {
		return fmt.Errorf("inspect connectivity relay state: %w", err)
	} else {
		for _, relay := range relays {
			if err := compose.DestroyProject(ctx, relay.Project, relay.Compose, relay.Env); err != nil {
				return fmt.Errorf("destroy connectivity relay project %s: %w", relay.Project, err)
			}
		}
	}
	if err := devgateway.DestroyTarget(ctx, compose, target.Name); err != nil {
		return fmt.Errorf("destroy target development gateway: %w", err)
	}
	if err := identityprovider.DestroyAllSharedKeycloakAt(ctx, compose, dataDir, target.Name); err != nil {
		return fmt.Errorf("destroy unreferenced shared identity provider: %w", err)
	}
	if err := runtimeexecutor.DestroySharedAt(ctx, compose, dataDir, target.Name); err != nil {
		return fmt.Errorf("destroy shared runtime provider executor: %w", err)
	}
	if err := application.DestroyAllSharedBackendsAt(ctx, compose, dataDir, target.Name); err != nil {
		return fmt.Errorf("destroy target shared SQL/cache providers: %w", err)
	}
	if err := objectstorage.DestroySharedProviderAt(ctx, compose, dataDir, target.Name); err != nil {
		return fmt.Errorf("destroy shared object-storage provider: %w", err)
	}
	if err := telemetry.DestroySharedProviderAt(ctx, compose, dataDir, target.Name); err != nil {
		return fmt.Errorf("destroy shared telemetry provider: %w", err)
	}
	if err := tracesprovider.DestroyAllSharedProvidersAt(ctx, compose, dataDir, target.Name); err != nil {
		return fmt.Errorf("destroy shared traces providers: %w", err)
	}
	if err := metricsprovider.DestroyAllSharedProvidersAt(ctx, compose, dataDir, target.Name); err != nil {
		return fmt.Errorf("destroy shared metrics providers: %w", err)
	}
	if err := compose.DestroyProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		return fmt.Errorf("destroy BaseHarbor control-plane Compose project: %w", err)
	}
	resourceResults := []fullDestroyResult{}
	plan.cleanup(ctx, &resourceResults)
	if preserved.Status != "" {
		resourceResults = append(resourceResults, preserved)
	}
	recordDestroyResults(parent, resourceResults, false)
	renderFullDestroyReport(out, resourceResults)
	if countFullDestroyBlockers(resourceResults) > 0 {
		return errors.New("target resource cleanup incomplete; runtime and registry state preserved")
	}
	for _, shared := range sharedBackends {
		remaining, err := compose.ListOwnedProjectResources(ctx, shared.Project)
		if err != nil {
			return fmt.Errorf("verify target shared-provider teardown before removing runtime state: %w", err)
		}
		if len(remaining) != 0 {
			return fmt.Errorf("target shared-provider teardown incomplete: project %s retains %d owned resource(s); runtime and registry state preserved", shared.Project, len(remaining))
		}
	}
	if err := os.RemoveAll(runtimeDir); err != nil {
		return fmt.Errorf("remove BaseHarbor runtime state: %w", err)
	}
	for _, name := range []string{
		"provider-registry.json",
		"provider-registry.json.lock",
		"connectivity.json",
		"connectivity.json.lock",
	} {
		path := filepath.Join(dataDir, name)
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove BaseHarbor platform state %s: %w", name, err)
		}
	}
	if err := os.RemoveAll(filepath.Join(dataDir, "connectivity")); err != nil {
		return fmt.Errorf("remove BaseHarbor connectivity runtime state: %w", err)
	}
	if len(inactive) > 0 {
		containers, err := compose.ListRuntimeContainers(ctx)
		if err != nil {
			return fmt.Errorf("verify application runtime absence after target teardown: %w", err)
		}
		if err := markInactiveTargetDeployments(inactive, containers); err != nil {
			return err
		}
	}
	recordDestroyResults(parent, nil, true)
	fmt.Fprintln(out, "BaseHarbor target control plane and unreferenced shared providers were permanently destroyed.")
	return nil
}

func runtimeStatus(parent context.Context, out io.Writer) error {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	result, err := inspectControlPlane(ctx)
	if err != nil {
		return err
	}
	target, err := effectiveTarget(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "BaseHarbor · %s\n\nTarget\n  EFFECTIVE  %s\n  Runtime    %s\n  Access     %s (%s)\n", target.Name, target.Name, target.RuntimeProvider, target.AccessReference, target.AccessProvider)
	if target.Scope != "" {
		fmt.Fprintf(out, "  Scope      %s\n", target.Scope)
	}
	fmt.Fprintln(out, "\nControl Plane")
	switch result.State {
	case "not_deployed":
		fmt.Fprintln(out, "  NOT DEPLOYED")
		return nil
	case "stopped":
		fmt.Fprintln(out, "  STOPPED")
	default:
		if len(result.Checks) == 0 {
			fmt.Fprintf(out, "  RUNNING    %d service(s)\n", len(result.Running))
		}
		for _, check := range result.Checks {
			state := "READY"
			if !check.Ready {
				state = "FAILED"
			}
			fmt.Fprintf(out, "  %-9s %s\n", state, check.Name)
		}
		if !result.Ready {
			return errors.New("runtime is running but not ready")
		}
		state := "SATISFIED"
		if !result.AvailabilitySatisfied {
			state = "UNSATISFIED"
		}
		fmt.Fprintf(out, "  %-9s availability · %s\n", state, result.AvailabilityDetail)
	}
	records, warnings, listErr := deployment.ListDeploymentsForDisplay(target.Name)
	fmt.Fprintln(out, "\nApplications")
	if listErr != nil {
		fmt.Fprintf(out, "  UNKNOWN    %v\n", listErr)
	} else {
		fmt.Fprintf(out, "  %d registered deployment(s)\n", len(records))
		for _, warning := range warnings {
			fmt.Fprintf(out, "  WARN       %v\n", warning)
		}
	}
	return nil
}

func initConfig(out io.Writer) error {
	fmt.Fprintln(out, "baha init no longer creates a global baseharbor.yaml.")
	fmt.Fprintln(out, "Application intent belongs in the repository manifest: run 'baha app init'.")
	fmt.Fprintln(out, "Deployment runtime/target selection belongs in Target configuration: run 'baha target create --help'.")
	return nil
}

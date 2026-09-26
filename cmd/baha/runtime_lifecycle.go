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
	"github.com/mcpdev80/baseharbor/internal/config"
	"github.com/mcpdev80/baseharbor/internal/connectivityrelay"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/health"
	"github.com/mcpdev80/baseharbor/internal/hosttrust"
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
	runtimeDir, err := targetRuntimeStateRoot(target)
	if err != nil {
		return err
	}
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
	fmt.Fprintf(out, "  runtime state: %s\n", runtimeDir)
	fmt.Fprintf(out, "  registry:      %s\n", filepath.Join(dataDir, "provider-registry.json"))
	if records, trustErr := hosttrust.StateRecords(dataDir); trustErr != nil {
		return fmt.Errorf("inspect BaseHarbor-owned host trust: %w", trustErr)
	} else if len(records) > 0 {
		fmt.Fprintf(out, "  host trust:    %d BaseHarbor-owned CA anchor(s)\n", len(records))
	}
	fmt.Fprintln(out, "  application-owned repository data/volumes: preserved")
	if !confirmed {
		fmt.Fprintln(out, "No changes were made. Re-run with --yes to permanently remove the selected BaseHarbor target control plane.")
		return nil
	}

	ctx, cancel := context.WithTimeout(parent, time.Minute)
	defer cancel()
	compose, err := detectComposeForTarget(ctx, target)
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
	if err := runtimeexecutor.DestroySharedAt(ctx, compose, dataDir, target.Name); err != nil {
		return fmt.Errorf("destroy shared runtime provider executor: %w", err)
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
	fmt.Fprintln(out, "BaseHarbor target control plane was permanently destroyed.")
	return nil
}

func runtimeStatus(parent context.Context, out io.Writer) error {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()

	target, err := effectiveTarget(ctx)
	if err != nil {
		return err
	}
	compose, err := detectComposeForTarget(ctx, target)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "BaseHarbor · %s\n\n", target.Name)
	fmt.Fprintln(out, "Target")
	fmt.Fprintf(out, "  EFFECTIVE  %s\n", target.Name)
	fmt.Fprintf(out, "  Runtime    %s (%s)\n", target.RuntimeProvider, compose.Engine())
	fmt.Fprintf(out, "  Access     %s\n", target.AccessReference)
	if target.Scope != "" {
		fmt.Fprintf(out, "  Scope      %s\n", target.Scope)
	}

	files, err := existingTargetRuntimeFiles(ctx)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintln(out, "\nControl Plane")
			fmt.Fprintln(out, "  NOT DEPLOYED")
			return nil
		}
		return fmt.Errorf("runtime is not initialized: %w", err)
	}
	running, err := compose.RunningServicesProject(ctx, files.Project, files.Compose, files.Env)
	if err != nil {
		return err
	}

	fmt.Fprintln(out, "\nControl Plane")
	if len(running) == 0 {
		fmt.Fprintln(out, "  STOPPED")
	} else {
		checks := health.RuntimeChecksForFiles(files)
		if len(checks) == 0 {
			fmt.Fprintf(out, "  RUNNING    %d service(s)\n", len(running))
		} else {
			ready := true
			for _, check := range checks {
				state := "READY"
				if !check.OK {
					state = "FAILED"
					ready = false
				}
				fmt.Fprintf(out, "  %-9s %s\n", state, check.Name)
			}
			if !ready {
				return errors.New("runtime is running but not ready")
			}
		}
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
	const path = config.DefaultFile
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	cfg := config.Default()
	if err := os.WriteFile(path, []byte(cfg.YAML()), 0o600); err != nil {
		return err
	}
	fmt.Fprintf(out, "created %s\n", path)
	fmt.Fprintln(out, "next: run 'baha doctor'")
	return nil
}

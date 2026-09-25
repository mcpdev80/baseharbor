package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/mcpdev80/baseharbor/internal/application"
	metricsprovider "github.com/mcpdev80/baseharbor/internal/metrics"
	"github.com/mcpdev80/baseharbor/internal/objectstorage"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/runtimeexecutor"
	"github.com/mcpdev80/baseharbor/internal/telemetry"
	"io"
	"os"
	"time"
)

func runtimeDown(parent context.Context, out io.Writer) error {
	ctx, cancel := context.WithTimeout(parent, time.Minute)
	defer cancel()

	target, err := effectiveTarget(ctx)
	if err != nil {
		return err
	}
	compose, err := detectComposeForTarget(ctx, target)
	if err != nil {
		return err
	}
	files, err := existingTargetRuntimeFiles(ctx)
	if err != nil {
		return fmt.Errorf("runtime is not initialized: %w", err)
	}
	if err := suspendSharedPlatformRuntime(ctx, compose, out); err != nil {
		return fmt.Errorf("suspend shared platform runtime: %w", err)
	}
	if err := compose.DownProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		return err
	}
	fmt.Fprintln(out, "BaseHarbor control-plane runtime stopped")
	return nil
}

func suspendSharedPlatformRuntime(ctx context.Context, compose bhruntime.Compose, out io.Writer) error {
	target, err := effectiveTarget(ctx)
	if err != nil {
		return err
	}
	dataDir, err := targetDataRoot(target)
	if err != nil {
		return err
	}
	rules, err := application.LoadConnectivityRulesAt(dataDir)
	if err != nil {
		return err
	}
	if len(rules) > 0 {
		containers, err := compose.ListComposeContainers(ctx)
		if err != nil {
			return err
		}
		for _, rule := range rules {
			if err := suspendConnectivityRuleAt(ctx, compose, dataDir, target.Name, rule, containers); err != nil {
				return err
			}
		}
		fmt.Fprintf(out, "[OK] connectivity       suspended %d platform connection(s); policy preserved\n", len(rules))
	}

	if files, err := runtimeexecutor.ExistingFilesAt(dataDir, target.Name); err == nil {
		if err := compose.StopProject(ctx, files.Project, files.Compose, files.Env); err != nil {
			return fmt.Errorf("stop shared runtime provider executor: %w", err)
		}
		fmt.Fprintln(out, "[OK] runtime-executor   shared provider executor stopped")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if instances, err := metricsprovider.ExistingSharedProviderInstancesAt(dataDir, target.Name); err != nil {
		return err
	} else {
		for _, instance := range instances {
			if err := compose.StopProject(ctx, instance.Placement.Project, instance.Files.Compose, instance.Files.Env); err != nil {
				return fmt.Errorf("stop shared Prometheus project %s: %w", instance.Placement.Project, err)
			}
		}
		if len(instances) > 0 {
			fmt.Fprintf(out, "[OK] metrics            %d shared Prometheus provider(s) stopped\n", len(instances))
		}
	}
	if files, err := telemetry.ExistingProviderFilesAt(dataDir, target.Name); err == nil {
		if err := compose.StopProject(ctx, files.Project, files.Compose, files.Env); err != nil {
			return fmt.Errorf("stop shared telemetry provider: %w", err)
		}
		fmt.Fprintln(out, "[OK] telemetry          shared OpenTelemetry Collector stopped")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if files, err := objectstorage.ExistingProviderFilesAt(dataDir, target.Name); err == nil {
		if err := compose.StopProject(ctx, files.Project, files.Compose, files.Env); err != nil {
			return fmt.Errorf("stop shared object-storage provider: %w", err)
		}
		fmt.Fprintln(out, "[OK] object-storage     shared SeaweedFS provider stopped")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func resumeSharedPlatformRuntime(ctx context.Context, compose bhruntime.Compose, out io.Writer) error {
	target, err := effectiveTarget(ctx)
	if err != nil {
		return err
	}
	dataDir, err := targetDataRoot(target)
	if err != nil {
		return err
	}
	if files, err := objectstorage.ExistingProviderFilesAt(dataDir, target.Name); err == nil {
		if err := compose.ConfigProject(ctx, files.Project, files.Compose, files.Env); err != nil {
			return fmt.Errorf("validate shared object-storage provider: %w", err)
		}
		if err := compose.UpProject(ctx, files.Project, files.Compose, files.Env); err != nil {
			return fmt.Errorf("start shared object-storage provider: %w", err)
		}
		fmt.Fprintln(out, "[OK] object-storage     shared SeaweedFS provider resumed")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if files, err := telemetry.ExistingProviderFilesAt(dataDir, target.Name); err == nil {
		if err := compose.ConfigProject(ctx, files.Project, files.Compose, files.Env); err != nil {
			return fmt.Errorf("validate shared telemetry provider: %w", err)
		}
		if err := compose.UpProject(ctx, files.Project, files.Compose, files.Env); err != nil {
			return fmt.Errorf("start shared telemetry provider: %w", err)
		}
		fmt.Fprintln(out, "[OK] telemetry          shared OpenTelemetry Collector resumed")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if instances, err := metricsprovider.ExistingSharedProviderInstancesAt(dataDir, target.Name); err != nil {
		return err
	} else {
		for _, instance := range instances {
			if err := compose.ConfigProject(ctx, instance.Placement.Project, instance.Files.Compose, instance.Files.Env); err != nil {
				return fmt.Errorf("validate shared Prometheus project %s: %w", instance.Placement.Project, err)
			}
			if err := compose.UpProject(ctx, instance.Placement.Project, instance.Files.Compose, instance.Files.Env); err != nil {
				return fmt.Errorf("start shared Prometheus project %s: %w", instance.Placement.Project, err)
			}
		}
		if len(instances) > 0 {
			fmt.Fprintf(out, "[OK] metrics            %d shared Prometheus provider(s) resumed\n", len(instances))
		}
	}

	if files, err := runtimeexecutor.ExistingFilesAt(dataDir, target.Name); err == nil {
		if err := compose.ConfigProject(ctx, files.Project, files.Compose, files.Env); err != nil {
			return fmt.Errorf("validate shared runtime provider executor: %w", err)
		}
		if err := compose.UpProject(ctx, files.Project, files.Compose, files.Env); err != nil {
			return fmt.Errorf("start shared runtime provider executor: %w", err)
		}
		fmt.Fprintln(out, "[OK] runtime-executor   shared provider executor resumed")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if err := reconcileAllConnectivity(ctx, out, compose); err != nil {
		return err
	}
	return nil
}

func reconcileAllConnectivity(ctx context.Context, out io.Writer, compose bhruntime.Compose) error {
	target, err := effectiveTarget(ctx)
	if err != nil {
		return err
	}
	dataDir, err := targetDataRoot(target)
	if err != nil {
		return err
	}
	rules, err := application.LoadConnectivityRulesAt(dataDir)
	if err != nil {
		return err
	}
	if len(rules) == 0 {
		return nil
	}
	containers, err := compose.ListComposeContainers(ctx)
	if err != nil {
		return err
	}
	for _, rule := range rules {
		sourceContainers := containersForResolvedEndpoint(rule.Source, containers, target.Name)
		targetContainers := containersForResolvedEndpoint(rule.Target, containers, target.Name)
		if len(sourceContainers) == 0 || len(targetContainers) == 0 {
			continue
		}
		targetNetwork, err := resolveConnectivityTargetNetwork(ctx, compose, rule.Target, containers, target.Name)
		if err != nil {
			return err
		}
		if err := convergeConnectivityRuleAt(ctx, compose, dataDir, target.Name, rule, sourceContainers, targetNetwork); err != nil {
			return err
		}
		fmt.Fprintf(out, "[OK] connectivity       %s -> %s\n", formatConnectivityEndpoint(rule.Source), formatConnectivityEndpoint(rule.Target))
	}
	return nil
}
